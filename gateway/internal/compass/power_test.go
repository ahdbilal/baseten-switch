package compass

import (
	"context"
	"path/filepath"
	"testing"

	"unity-gateway/gateway/internal/pricing"
)

func TestPowerPreservesPinsAndRulesAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "compass.json")
	cfg := Defaults()
	cfg.Rules.SessionSwitching = true
	cfg.Rules.EncourageSubagents = true
	if !cfg.IsEnabled() {
		t.Fatal("legacy configuration must remain enabled")
	}
	var router Router
	calls := 0
	classify := func(context.Context) (string, string, error) { calls++; return "high", "Complex task", nil }
	old, err := router.Pick(ctx, path, "existing", cfg, false, classify)
	if err != nil || old.Model != cfg.High {
		t.Fatal(old, err)
	}
	off := false
	cfg.Enabled = &off
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.IsEnabled() || !cfg.Rules.SessionSwitching || !cfg.Rules.EncourageSubagents {
		t.Fatal(cfg, err)
	}
	var restarted Router
	pinned, err := restarted.Pick(ctx, path, "existing", cfg, false, classify)
	if err != nil || pinned.Model != old.Model {
		t.Fatal("disabling changed existing model", pinned, err)
	}
	for _, client := range []string{"codex", "codex-desktop", "pi", "claude-code", "claude-desktop"} {
		for _, subagent := range []bool{false, true} {
			key := SessionKey(client, "account", "main")
			if subagent {
				key += "-child"
			}
			d, err := restarted.Pick(ctx, path, key, cfg, subagent, classify, client)
			if err != nil || d.Model != cfg.Default || d.Tier != "default" || d.EncourageSubagents {
				t.Fatal(d, err)
			}
		}
	}
	// A live-session review must also stop, even with switching enabled.
	reviewed := restarted.Review(ctx, path, "existing", cfg, old, 100, "next", pricing.New().Capture(), classify)
	if reviewed.Model != old.Model || calls != 1 {
		t.Fatalf("routing continued while off: %+v, %d calls", reviewed, calls)
	}
	on := true
	cfg.Enabled = &on
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := restarted.Pick(ctx, path, "new-enabled", cfg, false, classify)
	if err != nil || calls != 2 || d.Model != cfg.High || !d.EncourageSubagents {
		t.Fatal(d, calls, err)
	}
	if len(Recent(path)) != 12 {
		t.Fatal("power toggle removed history")
	}
}
