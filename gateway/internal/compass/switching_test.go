package compass

import (
	"context"
	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
	"github.com/basetenlabs/baseten-switch/gateway/internal/telemetry"
	"path/filepath"
	"testing"
	"time"
)

func TestSwitchWindowBoundaries(t *testing.T) {
	for _, x := range []struct {
		n   int64
		yes bool
	}{{0, true}, {999999999, true}, {1000000000, false}, {10000000000, false}, {10000000001, true}, {-1, false}} {
		if SwitchWindow(x.n) != x.yes {
			t.Fatalf("window %d", x.n)
		}
	}
}
func TestSwitchCostGatePreservesValuableCache(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	s := SessionUsage{Last: telemetry.UsageV1{InputTokens: n(0), OutputTokens: n(1000), CacheReadInputTokens: n(1000000), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
	quote := func(input, cache, output float64) pricing.Quote {
		return pricing.Quote{Priced: true, Price: pricing.Price{Prompt: input, CacheRead: cache, Completion: output}}
	}
	current, target, router := quote(10, .1, 1), quote(2, .1, 1), quote(.1, 0, .1)
	if SwitchWorthwhile(s, 1000000, current, target, router) {
		t.Fatal("discarded valuable warm cache")
	}
	s.Last.CacheReadInputTokens = n(0)
	s.Last.InputTokens = n(1000000)
	if !SwitchWorthwhile(s, 1000000, current, target, router) {
		t.Fatal("rejected clear savings without cache")
	}
	s.Unknown = true
	if SwitchWorthwhile(s, 1000000, current, target, router) {
		t.Fatal("accepted unknown accounting")
	}
	s.Unknown = false
	if SwitchWorthwhile(s, 1000000, current, pricing.Quote{}, router) {
		t.Fatal("accepted missing prices")
	}
}
func TestExistingRulesDefaultOnAndDisabledClassifierNotCalled(t *testing.T) {
	if !(Rules{}).Select(false) || !(Rules{}).Select(true) {
		t.Fatal("legacy defaults changed")
	}
	off := false
	c := Defaults()
	c.Rules.SessionStart = &off
	path := filepath.Join(t.TempDir(), "compass.json")
	r := &Router{}
	d, err := r.Pick(context.Background(), path, "main", c, false, func(context.Context) (string, string, error) {
		t.Fatal("classifier called while disabled")
		return "", "", nil
	})
	if err != nil || d.Model != c.Default {
		t.Fatalf("%+v %v", d, err)
	}
}
func TestSwitchReviewSkipsMiddleSpendAndOverlappingRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	r := &Router{}
	c := Defaults()
	c.Rules.SessionSwitching = true
	d := Decision{Model: c.Mid, CreatedAt: time.Now()}
	for _, cost := range []int64{2_000_000_000, 500_000_000} {
		if err := writeAccounting(path, "k", SessionUsage{SpentNano: cost, Completed: 1}); err != nil {
			t.Fatal(err)
		}
		release := r.Begin(path, "k")
		release2 := r.Begin(path, "k")
		got := r.Review(context.Background(), path, "k", c, d, 1000, "turn", pricing.New().Capture(), func(context.Context) (string, string, error) { t.Fatal("unexpected routing call"); return "", "", nil })
		release2()
		release()
		if got.Model != d.Model {
			t.Fatal("changed model")
		}
	}
}

func TestSwitchReviewPersistsBeneficialDecision(t *testing.T) {
	p := pricing.New()
	err := p.ReplaceModelsDev([]byte(`{"anthropic":{"id":"anthropic","models":{"claude-test":{"id":"claude-test","name":"Test","cost":{"input":1,"output":1}}}},"baseten":{"id":"baseten","models":{"test/model":{"id":"test/model","name":"Test","cost":{"input":1,"output":1}}}},"openai":{"id":"openai","models":{"gpt-6-luna":{"id":"gpt-6-luna","name":"Luna","limit":{"context":2000000,"output":100000},"cost":{"input":0.1,"output":0.5,"cache_read":0.01}}}}}`), time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	c := Defaults()
	c.Router = "gpt-6-luna"
	c.Low = "gpt-6-luna"
	c.Rules.SessionSwitching = true
	path := filepath.Join(t.TempDir(), "compass.json")
	r := &Router{}
	n := func(v int64) *int64 { return &v }
	s := SessionUsage{SpentNano: 500000000, Completed: 1, Last: telemetry.UsageV1{InputTokens: n(1000000), OutputTokens: n(1000), CacheReadInputTokens: n(0), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
	if err := writeAccounting(path, "main", s); err != nil {
		t.Fatal(err)
	}
	original := Decision{Model: "gpt-6-astra", Tier: "high", CreatedAt: time.Now()}
	if err := saveDecision(path, "main", original); err != nil {
		t.Fatal(err)
	}
	release := r.Begin(path, "main")
	defer release()
	got := r.Review(context.Background(), path, "main", c, original, 1000000, "turn", p.Capture(), func(context.Context) (string, string, error) { return "low", "Routine continuation", nil })
	if got.Model != c.Low {
		t.Fatalf("did not switch: %+v", got)
	}
	persisted, err := LoadDecision(path, "main")
	if err != nil || persisted.Model != c.Low {
		t.Fatal("switch was not persisted")
	}
	got = r.Review(context.Background(), path, "main", c, got, 1000000, "turn", p.Capture(), func(context.Context) (string, string, error) { t.Fatal("duplicate review"); return "high", "", nil })
	if got.Model != c.Low {
		t.Fatal("pin changed")
	}
}
func TestOldSessionAccountingCannotBeMistakenForCheapNewSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	r := &Router{}
	if err := saveDecision(path, "old", Decision{Model: "some/model", CreatedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cost := int64(100)
	r.Record(path, "old", telemetry.EventV1{StartedAt: time.Now(), UsageComplete: true, TerminationReason: telemetry.TerminationCompleted, ActualCost: telemetry.CostSnapshotV1{Priced: true, NanoUSD: &cost}})
	s, err := readAccounting(path, "old")
	if err != nil || !s.Unknown {
		t.Fatal("old spend history treated as known")
	}
}
