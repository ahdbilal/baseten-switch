package compass

import (
	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
	"github.com/basetenlabs/baseten-switch/gateway/internal/telemetry"
	"sort"
	"time"
)

// UsageSummary compares fully priced pairs at API rates before classifier overhead.
// Missing prices never imply zero cost. This is not a billing savings estimate.
type UsageSummary struct {
	IncompleteUsage   int          `json:"incomplete_usage"`
	MissingPrices     int          `json:"missing_prices"`
	Repriced          int          `json:"repriced"`
	PriceSource       string       `json:"price_source"`
	Baseline          string       `json:"baseline"`
	Since             time.Time    `json:"since"`
	Requests          int          `json:"requests"`
	Priced            int          `json:"priced"`
	Compared          int          `json:"compared"`
	ActualUSD         float64      `json:"actual_usd"`
	BaselineUSD       float64      `json:"baseline_usd"`
	ComparedActualUSD float64      `json:"compared_actual_usd"`
	SavedUSD          float64      `json:"saved_usd"`
	Partial           bool         `json:"partial"`
	Models            []ModelUsage `json:"models"`
}
type ModelUsage struct {
	Model     string  `json:"model"`
	Requests  int     `json:"requests"`
	Priced    int     `json:"priced"`
	ActualUSD float64 `json:"actual_usd"`
}

func SummarizeUsage(events []telemetry.EventV1, since time.Time, baseline string, quote pricing.Quote, catalogs ...*pricing.Snapshot) UsageSummary {
	s := UsageSummary{PriceSource: quote.Source, Baseline: baseline, Since: since, Models: []ModelUsage{}}
	rows := map[string]*ModelUsage{}
	for _, e := range events {
		if (e.Client != "codex" && e.Client != "pi" && e.Client != "claude-code") || !IsModel(e.RequestedModel) || e.CompletedAt.Before(since) {
			continue
		}
		s.Requests++
		row := rows[e.ServedModel]
		if row == nil {
			row = &ModelUsage{Model: e.ServedModel}
			rows[e.ServedModel] = row
		}
		row.Requests++
		if !e.UsageComplete {
			s.IncompleteUsage++
			continue
		}
		var actual float64
		if e.ActualCost.Priced && e.ActualCost.NanoUSD != nil {
			actual = float64(*e.ActualCost.NanoUSD) / 1e9
		} else {
			if len(catalogs) == 0 || catalogs[0] == nil {
				continue
			}
			profile := pricing.ProfileStandard
			if e.EffectiveSpeed != nil && *e.EffectiveSpeed != "" && *e.EffectiveSpeed != "standard" {
				if *e.EffectiveSpeed != "fast" {
					continue
				}
				profile = pricing.ProfileFast
			}
			q := catalogs[0].QuoteProfile(e.EffectiveProvider, e.ServedModel, profile)
			var ok bool
			actual, ok = usageCost(e.Usage, q)
			if !ok {
				s.MissingPrices++
				continue
			}
			s.Repriced++
		}
		s.Priced++
		row.Priced++
		s.ActualUSD += actual
		row.ActualUSD += actual
		hypothetical, ok := usageCost(e.Usage, quote)
		if !ok {
			continue
		}
		s.Compared++
		s.BaselineUSD += hypothetical
		s.ComparedActualUSD += actual
	}
	s.SavedUSD = s.BaselineUSD - s.ComparedActualUSD
	for _, row := range rows {
		s.Models = append(s.Models, *row)
	}
	sort.Slice(s.Models, func(i, j int) bool {
		if s.Models[i].Requests != s.Models[j].Requests {
			return s.Models[i].Requests > s.Models[j].Requests
		}
		return s.Models[i].Model < s.Models[j].Model
	})
	return s
}
func usageCost(u telemetry.UsageV1, q pricing.Quote) (float64, bool) {
	if u.InputTokens == nil || u.OutputTokens == nil || u.CacheReadInputTokens == nil || u.CacheWrite5mInputTokens == nil || u.CacheWrite1hInputTokens == nil {
		return 0, false
	}
	in, out, read, w5, w1 := *u.InputTokens, *u.OutputTokens, *u.CacheReadInputTokens, *u.CacheWrite5mInputTokens, *u.CacheWrite1hInputTokens
	q = pricing.ApplyContextRates(q, in, read, w5, w1)
	if !q.HasRatesForUsage(in, out, read, w5, w1) {
		return 0, false
	}
	rates, err := q.NanoUSDRates()
	if err != nil {
		return 0, false
	}
	cost, err := rates.CostNanoUSD(in, out, read, w5, w1)
	if err != nil {
		return 0, false
	}
	return float64(cost) / 1e9, true
}
