package analytics

import (
	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
	"github.com/basetenlabs/baseten-switch/gateway/internal/telemetry"
)

// Current rates provide a labeled estimate for historical rows, never a rewrite
// of captured costs or an invented usage total for an interrupted request.
func estimateCurrentCost(e telemetry.EventV1, catalog *pricing.Snapshot) (float64, bool) {
	if catalog == nil || !e.UsageComplete {
		return 0, false
	}
	u := e.Usage
	if u.InputTokens == nil || u.OutputTokens == nil || u.CacheReadInputTokens == nil || u.CacheWrite5mInputTokens == nil || u.CacheWrite1hInputTokens == nil {
		return 0, false
	}
	profile := pricing.ProfileStandard
	if e.EffectiveSpeed != nil && *e.EffectiveSpeed != "" && *e.EffectiveSpeed != "standard" {
		if *e.EffectiveSpeed != "fast" {
			return 0, false
		}
		profile = pricing.ProfileFast
	}
	q := catalog.QuoteProfile(e.EffectiveProvider, e.ServedModel, profile)
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
	return float64(cost) / 1e9, err == nil
}
