package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"unity-gateway/gateway/internal/compass"
)

func TestSharedConfigurationUsesPiCatalogWithoutCodex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", dir)
	raw := `{"providers":{"databricks":{"models":[{"id":"system.ai.pi-only"}]},"other":{"models":[{"id":"other/model"}]}}}`
	if err := os.WriteFile(filepath.Join(dir, piModelsFilename), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	a := &codexAdapter{configPath: filepath.Join(t.TempDir(), "gateway.yaml"), overlayPath: filepath.Join(t.TempDir(), "missing.toml")}
	known := a.compassKnownModels()
	if !known["system.ai.pi-only"] || known["other/model"] {
		t.Fatal(known)
	}
}

func TestSharedConfigurationIsAtomicAndPreservesOtherPreferences(t *testing.T) {
	cfg := compass.Defaults()
	off := false
	cfg.Enabled = &off
	cfg.ManualModels = map[string]string{"codex": "system.ai.manual"}
	cfg.ClaudeModel = compass.Model
	cfg.Comparison = cfg.High
	draft := map[string]any{"router": cfg.Router, "default": cfg.Default, "high": "system.ai.new", "mid_high": cfg.MidHigh, "mid": cfg.Mid, "low": cfg.Low, "rules": map[string]bool{"session_start": false, "subagents": true, "session_switching": true}}
	raw, _ := json.Marshal(draft)
	updated, err := compassSharedConfiguration(cfg, string(raw), func(m string) bool { return m == "system.ai.new" })
	if err != nil || updated.High != "system.ai.new" || updated.IsEnabled() || updated.Rules.Select(false) || !updated.Rules.SessionSwitching {
		t.Fatal(updated, err)
	}
	if !reflect.DeepEqual(cfg.ManualModels, updated.ManualModels) || cfg.ClaudeModel != updated.ClaudeModel || cfg.Comparison != updated.Comparison {
		t.Fatal("overwrote client or analytics settings")
	}
	for _, invalid := range []string{`null`, `{}`, string(raw) + ` {}`, `{"enabled":true}`, string(bytes.Replace(raw, []byte("system.ai.new"), []byte("Auto(Compass)"), 1))} {
		got, err := compassSharedConfiguration(cfg, invalid, func(string) bool { return true })
		if err == nil || !reflect.DeepEqual(cfg, got) {
			t.Fatalf("partially applied invalid draft %s: %+v %v", invalid, got, err)
		}
	}
	got, err := compassSharedConfiguration(cfg, string(raw), func(string) bool { return false })
	if err == nil || !reflect.DeepEqual(cfg, got) {
		t.Fatal("unknown model accepted")
	}
}

func TestCompassPowerCommandRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var output bytes.Buffer
	a := &codexAdapter{configPath: filepath.Join(t.TempDir(), "gateway.yaml"), out: &output}
	for _, value := range []string{"off", "on"} {
		output.Reset()
		if code := a.compass([]string{"power", value}); code != 0 {
			t.Fatal(code, output.String())
		}
		var snapshot struct {
			Config compass.Config `json:"config"`
		}
		if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil {
			t.Fatal(err, output.String())
		}
		if snapshot.Config.IsEnabled() != (value == "on") {
			t.Fatal("wrong status after power change")
		}
	}
	output.Reset()
	if a.compass([]string{"power", "invalid"}) != 2 {
		t.Fatal("invalid power accepted")
	}
	cfg, err := compass.Load(compass.Path(a.configPath))
	if err != nil || !cfg.IsEnabled() {
		t.Fatal("invalid command changed settings", err)
	}
}
