package compass

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestPolicyInheritanceAndIsolation(t *testing.T) {
	cfg := Defaults()
	cfg.ManualModels = map[string]string{"codex": "system.ai.manual"}
	for _, client := range []string{"codex", "codex-desktop", "claude-code", "claude-desktop", "pi"} {
		got := cfg.ForClient(client)
		if got.High != cfg.High || !got.IsEnabled() {
			t.Fatal("legacy defaults changed", client)
		}
	}
	codex := cfg.Policy()
	codex.High = "system.ai.codex-high"
	codex.Rules.EncourageSubagents = true
	cfg.Harnesses = map[string]Policy{"codex": codex}
	override := cfg.ClientPolicy("codex-desktop")
	override.High = "system.ai.desktop-high"
	cfg.Overrides = map[string]Policy{"codex-desktop": override}
	codex.High = "system.ai.codex-new"
	cfg.Harnesses["codex"] = codex
	if cfg.ForClient("codex").High != codex.High || cfg.ForClient("codex-desktop").High != override.High || cfg.ForClient("claude-code").High != cfg.High || cfg.ForClient("pi").Rules.EncourageSubagents {
		t.Fatal("scope leaked")
	}
	path := filepath.Join(t.TempDir(), "compass.json")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || !reflect.DeepEqual(cfg, got) {
		t.Fatal("roundtrip", err)
	}
	delete(cfg.Overrides, "codex-desktop")
	if cfg.ForClient("codex-desktop").High != codex.High {
		t.Fatal("override reset failed")
	}
	off := false
	codex.Enabled = &off
	cfg.Harnesses["codex"] = codex
	if cfg.ForClient("codex").IsEnabled() || cfg.ForClient("codex-desktop").IsEnabled() || !cfg.ForClient("pi").IsEnabled() {
		t.Fatal("harness power leaked")
	}
	on := true
	override.Enabled = &on
	cfg.Overrides["codex-desktop"] = override
	cfg.Enabled = &off
	for _, client := range []string{"codex", "codex-desktop", "claude-code", "claude-desktop", "pi"} {
		if cfg.ForClient(client).IsEnabled() {
			t.Fatal("master off bypassed", client)
		}
	}
}
