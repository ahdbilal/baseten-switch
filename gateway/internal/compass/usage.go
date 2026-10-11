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

func SummarizeUsage(events []telemetry.EventV1, since time.Time, baseline string, quote pricing.Quote) UsageSummary {
	s := UsageSummary{Baseline: baseline, Since: since, Models: []ModelUsage{}}
	rows := map[string]*ModelUsage{}
	for _, e := range events {
		if e.Client != "codex" || !IsModel(e.RequestedModel) || e.CompletedAt.Before(since) {
			continue
		}
		s.Requests++
		row := rows[e.ServedModel]
		if row == nil {
			row = &ModelUsage{Model: e.ServedModel}
			rows[e.ServedModel] = row
		}
		row.Requests++
		if !e.UsageComplete || !e.ActualCost.Priced || e.ActualCost.NanoUSD == nil {
			continue
		}
		actual := float64(*e.ActualCost.NanoUSD) / 1e9
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
