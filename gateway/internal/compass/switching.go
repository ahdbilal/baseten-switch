package compass

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unity-gateway/gateway/internal/pricing"
	"unity-gateway/gateway/internal/safefile"
	"unity-gateway/gateway/internal/telemetry"
)

const (
	earlyReviewTokens int64 = 25_000
	lateReviewTokens  int64 = 250_000
)

type SessionUsage struct {
	Observations           uint64            `json:"observations"`
	TokenAccountingVersion int               `json:"token_accounting_version"`
	ObservedTokens         int64             `json:"observed_tokens"`
	TokenHistoryIncomplete bool              `json:"token_history_incomplete"`
	LastReviewTokens       int64             `json:"last_review_tokens"`
	LastUsageComplete      bool              `json:"last_usage_complete"`
	SpentNano              int64             `json:"spent_nano"`
	Unknown                bool              `json:"unknown"`
	Last                   telemetry.UsageV1 `json:"last"`
	Completed              int               `json:"completed"`
	Switches               int               `json:"switches"`
	LastReview             string            `json:"last_review"`
	LastReviewCost         int64             `json:"last_review_cost"`
	LastSwitchRequest      int               `json:"last_switch_request"`
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
	if s.TokenAccountingVersion == 0 {
		// Legacy ledgers do not contain lifetime token counts. Start a lower bound,
		// never pretend an existing session is inside the early review window.
		s.TokenHistoryIncomplete = !os.IsNotExist(err)
		s.TokenAccountingVersion = 1
	}
	if os.IsNotExist(err) {
		// Old pins lack a complete spend history; do not treat them as new/free.
		if pin, pinErr := LoadDecision(path, key); pinErr != nil || pin.CreatedAt.Before(event.StartedAt) {
			s.Unknown = true
			s.TokenHistoryIncomplete = true
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
		// Dollar accounting does not determine token-review eligibility.
	}
	// Count only complete provider observations; missing requests leave a lower
	// bound. Later reviews can recover after enough confirmed tokens accumulate.
	tokens, complete := observedTokens(event.Usage)
	complete = complete && event.UsageComplete && event.TerminationReason == telemetry.TerminationCompleted
	s.LastUsageComplete = complete && (event.EffectiveSpeed == nil || *event.EffectiveSpeed == "" || *event.EffectiveSpeed == "standard")
	if complete && s.ObservedTokens >= 0 && tokens <= math.MaxInt64-s.ObservedTokens {
		s.ObservedTokens += tokens
		s.Last = event.Usage
	} else {
		s.TokenHistoryIncomplete = true
		s.LastUsageComplete = false
	}
	s.Observations++
	if event.TerminationReason == telemetry.TerminationCompleted {
		s.Completed++
	}
	if err := writeAccounting(path, key, s); err != nil {
		if r.accountingFailed == nil {
			r.accountingFailed = map[string]bool{}
		}
		r.accountingFailed[path+key] = true
	}
}

// observedTokens counts disjoint input buckets plus output. Cached input is
// still processed input, so it counts toward review cadence exactly once.
func observedTokens(u telemetry.UsageV1) (int64, bool) {
	total := int64(0)
	for _, n := range []*int64{u.InputTokens, u.OutputTokens, u.CacheReadInputTokens, u.CacheWrite5mInputTokens, u.CacheWrite1hInputTokens} {
		if n == nil || *n < 0 || *n > math.MaxInt64-total {
			return 0, false
		}
		total += *n
	}
	return total, true
}

func TokenReviewDue(s SessionUsage) bool {
	if s.TokenAccountingVersion != 1 || s.ObservedTokens < 0 || s.LastReviewTokens < 0 || s.LastReviewTokens > s.ObservedTokens {
		return false
	}
	return (!s.TokenHistoryIncomplete && s.ObservedTokens < earlyReviewTokens) || s.ObservedTokens-s.LastReviewTokens >= lateReviewTokens
}

// SwitchWorthwhile conservatively assumes the target is cold for every forecast
// request, while the current model retains its last observed cached prefix.
// Counts are mutually exclusive input buckets. No cache state transfers across models.
func SwitchWorthwhile(s SessionUsage, promptBound int64, current, target, router pricing.Quote) bool {
	u := s.Last
	if !s.LastUsageComplete || promptBound <= 0 || u.InputTokens == nil || u.OutputTokens == nil || u.CacheReadInputTokens == nil || u.CacheWrite5mInputTokens == nil || u.CacheWrite1hInputTokens == nil {
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
	if !current.Priced || !target.Priced {
		return false
	}
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
	if strings.Contains(model, "/") || len(strings.Split(model, ".")) >= 3 {
		provider = "databricks"
	}
	return p.QuoteProfile(provider, model, pricing.ProfileStandard)
}

// Hold records an explanation without changing the model or its selection time.
func (r *Router) Hold(path, key string, d Decision, reason string) Decision {
	r.accountingMu.Lock()
	defer r.accountingMu.Unlock()
	return r.holdLocked(path, key, d, reason)
}
func (r *Router) holdLocked(path, key string, d Decision, reason string) Decision {
	if key == "" {
		return d
	}
	current, err := LoadDecision(path, key)
	if err != nil || current.Model != d.Model || !current.CreatedAt.Equal(d.CreatedAt) {
		return d
	}
	d.ReviewReason = "Kept model: " + reason
	if current.ReviewReason != d.ReviewReason {
		_ = saveDecision(path, key, d)
	}
	return d
}

func (r *Router) Review(ctx context.Context, path, key string, c Config, d Decision, bound int64, turn string, p *pricing.Snapshot, classify func(context.Context) (string, string, error)) Decision {
	if !c.IsEnabled() || !c.Rules.SessionSwitching || d.Subagent || key == "" || turn == "" || p == nil {
		return d
	}
	r.accountingMu.Lock()
	s, err := readAccounting(path, key)
	if os.IsNotExist(err) {
		r.accountingMu.Unlock()
		return d
	} // First request has not completed yet.
	hold := ""
	switch {
	case err != nil || r.accountingFailed[path+key]:
		hold = "session accounting unavailable"
	case !s.LastUsageComplete:
		hold = "awaiting complete recent token and cache usage"
	case s.Completed == 0:
		hold = "awaiting a completed request"
	case !TokenReviewDue(s):
		hold = "outside the early 25,000-token window; waiting for 250,000 observed tokens since the last review"
	case s.Switches >= 2:
		hold = "two-switch session limit reached"
	case r.active[path+key] != 1:
		hold = "another request is still active"
	case s.Switches > 0 && s.Completed-s.LastSwitchRequest < 5:
		hold = "waiting five completed requests after the last switch"
	}
	if hold != "" {
		d = r.holdLocked(path, key, d, hold)
		r.accountingMu.Unlock()
		return d
	}
	if s.LastReview == turn {
		r.accountingMu.Unlock()
		return d
	}
	if !quoteFor(p, c.Router).HasRatesForUsage(66000, 1024, 0, 0, 0) {
		d = r.holdLocked(path, key, d, "routing-model price is unavailable")
		r.accountingMu.Unlock()
		return d
	}
	s.LastReview = turn
	s.LastReviewCost = s.SpentNano
	s.LastReviewTokens = s.ObservedTokens
	if writeAccounting(path, key, s) != nil {
		d = r.holdLocked(path, key, d, "could not save session accounting")
		r.accountingMu.Unlock()
		return d
	}
	r.accountingMu.Unlock()
	tier, reason, err := classify(ctx)
	target := c.Target(tier)
	if err != nil || target == "" {
		return r.Hold(path, key, d, "routing assessment failed or target credentials are unavailable")
	}
	if target == d.Model {
		return r.Hold(path, key, d, "routing assessment selected the current model")
	}
	provider := "openai"
	if strings.Contains(target, "/") || len(strings.Split(target, ".")) >= 3 {
		provider = "databricks"
	}
	record, known := p.Model(provider, target)
	if !known || record.ContextTokens <= bound+16384 {
		return r.Hold(path, key, d, "target context capacity is unknown or too small")
	}
	r.accountingMu.Lock()
	defer r.accountingMu.Unlock()
	latest, err := readAccounting(path, key)
	current, pinErr := LoadDecision(path, key)
	if err != nil || pinErr != nil || r.active[path+key] != 1 || latest.Observations != s.Observations || latest.Completed != s.Completed || current.Model != d.Model || !current.CreatedAt.Equal(d.CreatedAt) {
		return r.holdLocked(path, key, d, "session changed during routing assessment")
	}
	if !SwitchWorthwhile(s, bound, quoteFor(p, d.Model), quoteFor(p, target), quoteFor(p, c.Router)) {
		return r.holdLocked(path, key, d, "cache rebuild and routing cost outweigh savings, or required prices are missing")
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
		return r.holdLocked(path, key, original, "could not save session accounting")
	}
	if saveDecision(path, key, d) != nil {
		return original
	}
	return d
}
