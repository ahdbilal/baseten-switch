package compass

import (
	"testing"
	"time"
	"unity-gateway/gateway/internal/pricing"
	"unity-gateway/gateway/internal/telemetry"
)

func TestUsageComparisonExcludesUnpricedAndNonCompass(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	e := telemetry.EventV1{Client: "codex", RequestedModel: LegacyModel, ServedModel: "cheap/model", CompletedAt: time.Unix(200, 0), UsageComplete: true,
		Usage:      telemetry.UsageV1{InputTokens: n(1000000), OutputTokens: n(0), CacheReadInputTokens: n(0), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)},
		ActualCost: telemetry.CostSnapshotV1{Priced: true, NanoUSD: n(1000000000)}}
	q := pricing.Quote{Priced: true, Price: pricing.Price{Prompt: 3}}
	unpriced := e
	unpriced.RequestedModel = Model
	unpriced.ActualCost.Priced = false
	explicit := e
	explicit.RequestedModel = "gpt-example"
	old := e
	old.CompletedAt = time.Unix(50, 0)
	got := SummarizeUsage([]telemetry.EventV1{e, unpriced, explicit, old}, time.Unix(100, 0), "high", q)
	if got.Requests != 2 || got.Priced != 1 || got.Compared != 1 || got.ActualUSD != 1 || got.SavedUSD != 2 || len(got.Models) != 1 {
		t.Fatalf("summary %+v", got)
	}
	got = SummarizeUsage([]telemetry.EventV1{e}, time.Unix(100, 0), "unknown", pricing.Quote{})
	if got.Compared != 0 || got.ActualUSD != 1 || got.SavedUSD != 0 {
		t.Fatalf("unpriced baseline %+v", got)
	}
	q.Price.Prompt = 0.5
	got = SummarizeUsage([]telemetry.EventV1{e}, time.Unix(100, 0), "cheaper", q)
	if got.SavedUSD != -0.5 {
		t.Fatalf("negative savings hidden: %+v", got)
	}
}

func TestUsageRepricesHistoricalOpenAIAndIncludesPi(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	e := telemetry.EventV1{Client: "pi", RequestedModel: Model, ServedModel: "gpt-6-astra", EffectiveProvider: "openai", CompletedAt: time.Now(), UsageComplete: true,
		Usage: telemetry.UsageV1{InputTokens: n(1000), OutputTokens: n(100), CacheReadInputTokens: n(100), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
	p := pricing.New()
	got := SummarizeUsage([]telemetry.EventV1{e}, time.Now().Add(-time.Hour), "gpt-6-astra", p.Quote("openai", "gpt-6-astra"), p.Capture())
	if got.Requests != 1 || got.Repriced != 1 || got.Compared != 1 || got.SavedUSD != 0 || got.ActualUSD != 0.0151 {
		t.Fatalf("historical Pi pricing %+v", got)
	}
	if e.ActualCost.Priced {
		t.Fatal("mutated historical event")
	}
}
