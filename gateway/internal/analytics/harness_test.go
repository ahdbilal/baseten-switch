package analytics

import (
	"encoding/json"
	"testing"
	"unity-gateway/gateway/internal/telemetry"
)

func TestHarnessBreakdownMatchesModelTotals(t *testing.T) {
	events := []telemetry.EventV1{}
	for i, client := range []string{"codex", "pi", "claude-code", "claude-desktop", "codex", ""} {
		model := "system.ai.glm-5-3"
		if i == 4 {
			model = "system.ai.gpt-6-1-sol"
		}
		event := analyticsEvent(int64(100+i), "databricks", "Auto(Compass)", model)
		event.Client = client
		if i != 5 {
			setUsage(&event, 100, 20, 50, 10)
			setActualCost(&event, 1_000_000_000)
			setLatency(&event, int64(100+i*100), 2000)
		}
		events = append(events, event)
	}
	outside := events[0]
	outside.CompletedAt = outside.CompletedAt.AddDate(0, 0, 1)
	events = append(events, outside)
	result := Build(events, Window{Since: 100, Until: 200}, 200, Snapshot{Complete: true}, true, nil)
	if len(result.Harnesses) != 5 || result.Harnesses[0].Client != "codex" {
		t.Fatalf("wrong harnesses: %+v", result.Harnesses)
	}
	requests, tokens, cost := 0, int64(0), float64(0)
	for _, h := range result.Harnesses {
		requests += h.Cost.Requests
		tokens += h.Cost.Tokens
		if h.Cost.ActualCostUSD != nil {
			cost += *h.Cost.ActualCostUSD
		}
		childRequests, childTokens := 0, int64(0)
		for _, m := range h.Models {
			childRequests += m.Requests
			childTokens += m.Tokens
			if m.ModelID == "Auto(Compass)" {
				t.Fatal("grouped by requested alias instead of served model")
			}
		}
		if childRequests != h.Cost.Requests || childTokens != h.Cost.Tokens {
			t.Fatal("model and harness totals differ")
		}
		if h.Client == "unknown" && (h.Cost.ActualCostUSD != nil || h.Cost.TokenBreakdown.CompleteRequests != 0 || h.Cost.UnpricedRows != 1) {
			t.Fatal("unknown usage represented as free/complete")
		}
	}
	codex := result.Harnesses[0]
	if len(codex.Models) != 2 || codex.Cost.Requests != 2 || codex.Performance.MedianTTFTMs != 300 || codex.Performance.TTFTSamples != 2 {
		t.Fatalf("wrong Codex split/performance: %+v", codex)
	}
	if requests != result.Coverage.RequestRows || tokens != 900 || cost != result.Cost.Summary.ActualDatabricksCostUSD || cost != 5 {
		t.Fatalf("totals: %d %d %f", requests, tokens, cost)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Response
	if err = json.Unmarshal(raw, &decoded); err != nil || len(decoded.Harnesses) != 5 {
		t.Fatal("harness API roundtrip failed", err)
	}
}

func TestEmptyHarnessBreakdownIsArray(t *testing.T) {
	result := Build(nil, Window{}, 0, Snapshot{}, true, nil)
	if result.Harnesses == nil || len(result.Harnesses) != 0 {
		t.Fatal("expected empty harness array")
	}
}
