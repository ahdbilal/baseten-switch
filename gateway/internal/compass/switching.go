package compass

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
	"github.com/basetenlabs/baseten-switch/gateway/internal/safefile"
	"github.com/basetenlabs/baseten-switch/gateway/internal/telemetry"
)

type SessionUsage struct {
	SpentNano         int64             `json:"spent_nano"`
	Unknown           bool              `json:"unknown"`
	Last              telemetry.UsageV1 `json:"last"`
	Completed         int               `json:"completed"`
	Switches          int               `json:"switches"`
	LastReview        string            `json:"last_review"`
	LastReviewCost    int64             `json:"last_review_cost"`
	LastSwitchRequest int               `json:"last_switch_request"`
}

func accountingPath(path, key string) string {
	return filepath.Join(filepath.Dir(path), "compass-usage", key+".json")
}
func readAccounting(path, key string) (SessionUsage, error) {
	var s SessionUsage
	raw, err := os.ReadFile(accountingPath(path, key))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	return s, err
}
func writeAccounting(path, key string, s SessionUsage) error {
	file := accountingPath(path, key)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	snap, err := safefile.Read(file)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = snap.Replace(raw, 0600)
	return err
}

// Begin spans the entire upstream response, so overlapping requests cannot switch models.
func (r *Router) Begin(path, key string) func() {
	r.accountingMu.Lock()
	if r.active == nil {
		r.active = map[string]int{}
	}
	r.active[path+key]++
	r.accountingMu.Unlock()
	return func() {
		r.accountingMu.Lock()
		r.active[path+key]--
		if r.active[path+key] == 0 {
			delete(r.active, path+key)
		}
		r.accountingMu.Unlock()
	}
}
func (r *Router) Record(path, key string, event telemetry.EventV1) {
	if key == "" {
		return
	}
	r.accountingMu.Lock()
	defer r.accountingMu.Unlock()
	s, err := readAccounting(path, key)
	if err != nil && !os.IsNotExist(err) {
		return
	}
	if os.IsNotExist(err) {
		// Old pins lack a complete spend history; do not treat them as new/free.
		if pin, pinErr := LoadDecision(path, key); pinErr != nil || pin.CreatedAt.Before(event.StartedAt) {
			s.Unknown = true
		}
	}
	if event.EffectiveSpeed != nil && *event.EffectiveSpeed != "" && *event.EffectiveSpeed != "standard" {
		s.Unknown = true
	}
	if !event.UsageComplete || !event.ActualCost.Priced || event.ActualCost.NanoUSD == nil || event.TerminationReason != telemetry.TerminationCompleted {
		s.Unknown = true
	} else {
		cost := *event.ActualCost.NanoUSD
		if cost < 0 || s.SpentNano > int64(^uint64(0)>>1)-cost {
			s.Unknown = true
		} else {
			s.SpentNano += cost
		}
		s.Last = event.Usage
	}
	s.Completed++
	if err := writeAccounting(path, key, s); err != nil {
		if r.accountingFailed == nil {
			r.accountingFailed = map[string]bool{}
		}
		r.accountingFailed[path+key] = true
	}
}
func SwitchWindow(cost int64) bool {
	return cost >= 0 && (cost < 1_000_000_000 || cost > 10_000_000_000)
}

// SwitchWorthwhile conservatively assumes the target is cold for every forecast
// request, while the current model retains its last observed cached prefix.
// Counts are mutually exclusive input buckets. No cache state transfers across models.
func SwitchWorthwhile(s SessionUsage, promptBound int64, current, target, router pricing.Quote) bool {
	u := s.Last
	if s.Unknown || promptBound <= 0 || u.InputTokens == nil || u.OutputTokens == nil || u.CacheReadInputTokens == nil || u.CacheWrite5mInputTokens == nil || u.CacheWrite1hInputTokens == nil {
		return false
	}
	if *u.InputTokens < 0 || *u.OutputTokens < 0 || *u.CacheReadInputTokens < 0 || *u.CacheWrite5mInputTokens < 0 || *u.CacheWrite1hInputTokens < 0 {
		return false
	}
	cached := int64(0)
	for _, value := range []int64{*u.CacheReadInputTokens, *u.CacheWrite5mInputTokens, *u.CacheWrite1hInputTokens} {
		if value > promptBound-cached {
			cached = promptBound
			break
		}
		cached += value
	}
	output := *u.OutputTokens
	if !current.HasRatesForUsage(promptBound-cached, output, cached, 0, 0) || !target.HasRatesForUsage(promptBound, output, 0, 0, 0) || !router.HasRatesForUsage(66000, 1024, 0, 0, 0) {
		return false
	}
	current = pricing.ApplyContextRates(current, promptBound-cached, cached, 0, 0)
	target = pricing.ApplyContextRates(target, promptBound, 0, 0, 0)
	// If cache writes are separately priced, use at least that rate for cold input.
	coldRate := target.Price.Prompt
	if target.Price.CacheWrite5m > coldRate {
		coldRate = target.Price.CacheWrite5m
	}
	if target.Price.CacheWrite1h > coldRate {
		coldRate = target.Price.CacheWrite1h
	}
	stay := (float64(promptBound-cached)*current.Price.Prompt + float64(cached)*current.Price.CacheRead + float64(output)*current.Price.Completion) / 1e6
	cold := (float64(promptBound)*coldRate + float64(output)*target.Price.Completion) / 1e6
	routing := (66000*router.Price.Prompt + 1024*router.Price.Completion) / 1e6
	savings := 5*(stay-cold) - routing
	return savings >= 0.10 && savings >= 5*stay*0.20
}
func quoteFor(p *pricing.Snapshot, model string) pricing.Quote {
	provider := "openai"
	if strings.Contains(model, "/") {
		provider = "baseten"
	}
	return p.QuoteProfile(provider, model, pricing.ProfileStandard)
}

func (r *Router) Review(ctx context.Context, path, key string, c Config, d Decision, bound int64, turn string, p *pricing.Snapshot, classify func(context.Context) (string, string, error)) Decision {
	if !c.Rules.SessionSwitching || d.Subagent || key == "" || turn == "" || p == nil {
		return d
	}
	r.accountingMu.Lock()
	s, err := readAccounting(path, key)
	if err != nil || r.accountingFailed[path+key] || s.Unknown || s.Completed == 0 || !SwitchWindow(s.SpentNano) || s.Switches >= 2 || s.LastReview == turn || r.active[path+key] != 1 || (s.Switches > 0 && s.Completed-s.LastSwitchRequest < 5) || (s.SpentNano > 10_000_000_000 && s.LastReviewCost > 10_000_000_000 && s.SpentNano-s.LastReviewCost < 1_000_000_000) {
		r.accountingMu.Unlock()
		return d
	}
	// Unknown router or usage pricing must not generate repeated paid routing calls.
	if !quoteFor(p, c.Router).HasRatesForUsage(66000, 1024, 0, 0, 0) {
		r.accountingMu.Unlock()
		return d
	}
	s.LastReview = turn
	s.LastReviewCost = s.SpentNano
	if writeAccounting(path, key, s) != nil {
		r.accountingMu.Unlock()
		return d
	}
	r.accountingMu.Unlock()
	tier, reason, err := classify(ctx)
	target := c.Target(tier)
	if err != nil || target == "" || target == d.Model {
		return d
	}
	provider := "openai"
	if strings.Contains(target, "/") {
		provider = "baseten"
	}
	record, known := p.Model(provider, target)
	if !known || record.ContextTokens <= bound+16384 {
		return d
	}

	r.accountingMu.Lock()
	defer r.accountingMu.Unlock()
	latest, err := readAccounting(path, key)
	if err != nil || r.active[path+key] != 1 || latest.Completed != s.Completed {
		return d
	}
	if !SwitchWorthwhile(s, bound, quoteFor(p, d.Model), quoteFor(p, target), quoteFor(p, c.Router)) {
		d.ReviewReason = "Kept model: cache rebuild or uncertain costs outweigh expected savings"
		d.CreatedAt = time.Now().UTC()
		_ = saveDecision(path, key, d)
		return d
	}
	original := d
	previous := d.Model
	d.ReviewReason = ""
	d.Model = target
	d.Tier = tier
	d.Reason = fmt.Sprintf("Switched from %s after cache/cost check: %s", previous, reason)
	d.CreatedAt = time.Now().UTC()
	s.Switches++
	s.LastSwitchRequest = s.Completed
	if writeAccounting(path, key, s) != nil {
		return original
	}
	if saveDecision(path, key, d) != nil {
		return original
	}
	return d
}
