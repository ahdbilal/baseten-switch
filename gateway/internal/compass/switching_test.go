package compass

import (
	"context"
	"path/filepath"
	"testing"
	"time"
	"unity-gateway/gateway/internal/pricing"
	"unity-gateway/gateway/internal/telemetry"
)

func TestTokenReviewBoundaries(t *testing.T) {
	for _, tc := range []struct {
		tokens, reviewed int64
		gap, due         bool
	}{
		{0, 0, false, true}, {24999, 0, false, true}, {25000, 0, false, false},
		{249999, 0, false, false}, {250000, 0, false, true},
		{499999, 250000, false, false}, {500000, 250000, false, true},
		{1000, 0, true, false}, {250000, 0, true, true}, {-1, 0, false, false},
		{1, 2, false, false},
	} {
		s := SessionUsage{TokenAccountingVersion: 1, ObservedTokens: tc.tokens, LastReviewTokens: tc.reviewed, TokenHistoryIncomplete: tc.gap}
		if TokenReviewDue(s) != tc.due {
			t.Fatalf("unexpected eligibility: %+v", tc)
		}
	}
	if TokenReviewDue(SessionUsage{}) {
		t.Fatal("legacy ledger considered new")
	}
}
func TestSwitchCostGatePreservesValuableCache(t *testing.T) {
	n := func(v int64) *int64 { return &v }
	s := SessionUsage{LastUsageComplete: true, Last: telemetry.UsageV1{InputTokens: n(0), OutputTokens: n(1000), CacheReadInputTokens: n(1000000), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
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
	expensiveWrite := target
	expensiveWrite.Price.CacheWrite1h = 100
	if SwitchWorthwhile(s, 1000000, current, expensiveWrite, router) {
		t.Fatal("ignored long-retention cache-write cost")
	}
	s.LastUsageComplete = false
	if SwitchWorthwhile(s, 1000000, current, target, router) {
		t.Fatal("accepted unknown accounting")
	}
	s.LastUsageComplete = true
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
func TestSwitchReviewSkipsOverlappingRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	r := &Router{}
	c := Defaults()
	c.Rules.SessionSwitching = true
	d := Decision{Model: c.Mid, CreatedAt: time.Now()}
	for _, cost := range []int64{2_000_000_000, 500_000_000} {
		if err := writeAccounting(path, "k", SessionUsage{TokenAccountingVersion: 1, ObservedTokens: 1000, LastUsageComplete: true, SpentNano: cost, Completed: 1}); err != nil {
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
	err := p.ReplaceModelsDev([]byte(`{"anthropic":{"id":"anthropic","models":{"claude-test":{"id":"claude-test","name":"Test","cost":{"input":1,"output":1}}}},"databricks":{"id":"databricks","models":{"test/model":{"id":"test/model","name":"Test","cost":{"input":1,"output":1}}}},"openai":{"id":"openai","models":{"gpt-6-luna":{"id":"gpt-6-luna","name":"Luna","limit":{"context":2000000,"output":100000},"cost":{"input":0.1,"output":0.5,"cache_read":0.01}}}}}`), time.Now(), "")
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
	s := SessionUsage{TokenAccountingVersion: 1, ObservedTokens: 1001000, LastUsageComplete: true, SpentNano: 500000000, Completed: 1, Last: telemetry.UsageV1{InputTokens: n(1000000), OutputTokens: n(1000), CacheReadInputTokens: n(0), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
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

func TestHoldExplainsWithoutChangingSelectionAndRejectsStaleWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	original := Decision{Model: "test/current", CreatedAt: time.Now().UTC(), Reason: "original selection"}
	if err := saveDecision(path, "main", original); err != nil {
		t.Fatal(err)
	}
	r := &Router{}
	held := r.Hold(path, "main", original, "session cost or token usage is incomplete")
	if held.Model != original.Model || !held.CreatedAt.Equal(original.CreatedAt) || held.ReviewReason == "" || held.Reason != original.Reason {
		t.Fatalf("changed selection: %+v", held)
	}
	newer := original
	newer.Model = "test/newer"
	newer.CreatedAt = original.CreatedAt.Add(time.Second)
	if err := saveDecision(path, "main", newer); err != nil {
		t.Fatal(err)
	}
	r.Hold(path, "main", original, "stale reason")
	got, err := LoadDecision(path, "main")
	if err != nil || got.Model != newer.Model || got.ReviewReason != "" {
		t.Fatalf("stale hold replaced newer selection: %+v %v", got, err)
	}
}

func TestTokenAccountingRecoversAfterGapWithoutInventingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	start := time.Now()
	if err := saveDecision(path, "main", Decision{Model: "test/model", CreatedAt: start}); err != nil {
		t.Fatal(err)
	}
	r := &Router{}
	r.Record(path, "main", telemetry.EventV1{StartedAt: start, TerminationReason: telemetry.TerminationClientCancelled})
	n := func(v int64) *int64 { return &v }
	event := telemetry.EventV1{StartedAt: start, TerminationReason: telemetry.TerminationCompleted, UsageComplete: true,
		Usage: telemetry.UsageV1{InputTokens: n(1000), OutputTokens: n(10), CacheReadInputTokens: n(0), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
	r.Record(path, "main", event) // No price, but complete tokens must be retained.
	s, err := readAccounting(path, "main")
	if err != nil || !s.Unknown || !s.TokenHistoryIncomplete || !s.LastUsageComplete || s.ObservedTokens != 1010 || s.Completed != 1 || TokenReviewDue(s) {
		t.Fatalf("gap state: %+v %v", s, err)
	}
	event.Usage.InputTokens = n(250000)
	r.Record(path, "main", event)
	s, err = readAccounting(path, "main")
	if err != nil || !TokenReviewDue(s) || !s.LastUsageComplete || s.ObservedTokens != 251020 {
		t.Fatalf("did not recover: %+v %v", s, err)
	}
}

func TestLegacyTokenLedgerMigrationAndOverflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compass.json")
	if err := writeAccounting(path, "main", SessionUsage{Completed: 4, SpentNano: 123}); err != nil {
		t.Fatal(err)
	}
	r := &Router{}
	n := func(v int64) *int64 { return &v }
	e := telemetry.EventV1{TerminationReason: telemetry.TerminationCompleted, UsageComplete: true, Usage: telemetry.UsageV1{InputTokens: n(100), OutputTokens: n(1), CacheReadInputTokens: n(200), CacheWrite5mInputTokens: n(30), CacheWrite1hInputTokens: n(40)}}
	r.Record(path, "main", e)
	s, _ := readAccounting(path, "main")
	if !s.TokenHistoryIncomplete || s.ObservedTokens != 371 || TokenReviewDue(s) {
		t.Fatalf("legacy state: %+v", s)
	}
	e.Usage.InputTokens = n(9223372036854775807)
	r.Record(path, "main", e)
	s, _ = readAccounting(path, "main")
	if s.LastUsageComplete || s.ObservedTokens != 371 {
		t.Fatalf("overflow: %+v", s)
	}
}

func TestWorkspaceSwitchQuotesAndLongContextProtection(t *testing.T) {
	p := pricing.NewWorkspace()
	if e := p.ReplaceWorkspaceModels([]string{"system.ai.gpt-6-astra", "system.ai.kimi-k3", "system.ai.glm-5-3-flash"}, "https://workspace.example", time.Now()); e != nil {
		t.Fatal(e)
	}
	n := func(v int64) *int64 { return &v }
	s := SessionUsage{LastUsageComplete: true, Last: telemetry.UsageV1{InputTokens: n(1000), OutputTokens: n(1500), CacheReadInputTokens: n(0), CacheWrite5mInputTokens: n(0), CacheWrite1hInputTokens: n(0)}}
	current, target, router := quoteFor(p.Capture(), "system.ai.gpt-6-astra"), quoteFor(p.Capture(), "system.ai.kimi-k3"), quoteFor(p.Capture(), "system.ai.glm-5-3-flash")
	if !SwitchWorthwhile(s, 5000, current, target, router) {
		t.Fatal("native workspace prices did not permit clear savings")
	}
	if SwitchWorthwhile(s, 128001, current, target, router) {
		t.Fatal("switched with unknown long-context rate")
	}
}
