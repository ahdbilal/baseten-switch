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

func TestCompassPolicyCommandsAndOverrideReset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	a := &codexAdapter{configPath: filepath.Join(t.TempDir(), "gateway.yaml"), out: &out}
	cfg := compass.Defaults()
	p := cfg.Policy()
	off := false
	p.Enabled = &off
	p.Rules.EncourageSubagents = true
	raw, _ := json.Marshal(p)
	run := func(args ...string) {
		t.Helper()
		out.Reset()
		if a.compass(args) != 0 {
			t.Fatal(out.String())
		}
	}
	run("policy", "harness", "codex", string(raw))
	run("override", "codex-desktop", "on")
	on := true
	p.Enabled = &on
	p.Rules.Verbose = true
	raw, _ = json.Marshal(p)
	run("policy", "harness", "codex", string(raw))
	var snapshot struct {
		Resolved map[string]compass.Config `json:"resolved"`
	}
	if err := json.Unmarshal(out.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Resolved["codex"].IsEnabled() || snapshot.Resolved["codex-desktop"].IsEnabled() || snapshot.Resolved["pi"].Rules.EncourageSubagents {
		t.Fatal("wrong resolved status")
	}
	run("override", "codex-desktop", "off")
	saved, err := compass.Load(compass.Path(a.configPath))
	if err != nil {
		t.Fatal(err)
	}
	if !saved.ForClient("codex-desktop").Rules.Verbose || len(saved.Overrides) != 0 {
		t.Fatal("inheritance not restored")
	}
	before, _ := os.ReadFile(compass.Path(a.configPath))
	for _, args := range [][]string{{"policy", "harness", "unknown", string(raw)}, {"policy", "client", "pi", `{}`}, {"policy", "harness", "codex", `{"enabled":true}`}, {"policy", "client", "pi", string(bytes.Replace(raw, []byte(p.High), []byte("system.ai.unlisted"), 1))}} {
		out.Reset()
		if a.compass(args) == 0 {
			t.Fatal("invalid edit accepted", args)
		}
		after, _ := os.ReadFile(compass.Path(a.configPath))
		if !bytes.Equal(before, after) {
			t.Fatal("invalid edit changed disk")
		}
	}
}

func TestCompassDefaultDropdownSelectsAutoAndManual(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var out bytes.Buffer
	a := &codexAdapter{configPath: filepath.Join(t.TempDir(), "gateway.yaml"), out: &out}
	for _, model := range []string{"opus", compass.Model, "sonnet"} {
		out.Reset()
		if a.compass([]string{"select", "claude-code", model}) != 0 {
			t.Fatal(out.String())
		}
		cfg, err := compass.Load(compass.Path(a.configPath))
		if err != nil || cfg.ClaudeModel != model {
			t.Fatal("default not applied", cfg, err)
		}
		if model == compass.Model && cfg.ManualModels["claude-code"] != "opus" {
			t.Fatal("lost manual default")
		}
	}
}

func TestPolicyEditDoesNotMutateOriginalMaps(t *testing.T) {
	cfg := compass.Defaults()
	cfg.Overrides = map[string]compass.Policy{"pi": cfg.Policy()}
	original := cfg.Overrides["pi"]
	edited, err := compassPolicyEdit(cfg, []string{"override", "pi", "off"}, func(string) bool { return true })
	if err != nil || len(edited.Overrides) != 0 || !reflect.DeepEqual(cfg.Overrides["pi"], original) {
		t.Fatal("mutated original", err)
	}
}

func TestManualCandidatePrecedesFallbackInAnyCatalogOrder(t *testing.T) {
	for _, ids := range [][]string{{compass.Model, "manual/model", "fallback/model"}, {"fallback/model", "manual/model", compass.Model}} {
		var models []piModelJSON
		for _, id := range ids {
			models = append(models, piModelJSON{ID: id})
		}
		if got := compassManualCandidate(models, "fallback/model", "manual/model"); got != "manual/model" {
			t.Fatal(got)
		}
		if got := compassManualCandidate(models, "fallback/model", "missing/model"); got != "fallback/model" {
			t.Fatal(got)
		}
	}
}
