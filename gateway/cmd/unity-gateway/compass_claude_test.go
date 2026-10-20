package main

import (
	"path/filepath"
	"testing"

	"unity-gateway/gateway/internal/compass"
)

func TestClaudeCompassPreferencesRoundTrip(t *testing.T) {
	a := &codexAdapter{configPath: filepath.Join(t.TempDir(), "gateway.yaml")}
	cfg := compass.Defaults()
	cfg.ManualModels = map[string]string{"claude-code": "sonnet"}
	if err := a.selectCompassClient("claude-code", true, &cfg); err != nil {
		t.Fatal(err)
	}
	if !compass.IsModel(cfg.ClaudeModel) {
		t.Fatal("Compass not selected")
	}
	if err := compass.Save(compass.Path(a.configPath), cfg); err != nil {
		t.Fatal(err)
	}
	if !a.compassClients()["claude-code"].Enabled {
		t.Fatal("selection missing from status")
	}
	if err := a.setCompassManual("claude-code", "opus", &cfg); err != nil {
		t.Fatal(err)
	}
	if !compass.IsModel(cfg.ClaudeModel) {
		t.Fatal("manual default disabled Compass")
	}
	if err := a.selectCompassClient("claude-code", false, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClaudeModel != "opus" {
		t.Fatal("manual default not restored")
	}
	if err := compass.Save(compass.Path(a.configPath), cfg); err != nil {
		t.Fatal(err)
	}
	got, err := compass.Load(compass.Path(a.configPath))
	if err != nil || got.ClaudeModel != "opus" {
		t.Fatalf("roundtrip failed: %v", err)
	}
}
