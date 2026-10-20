package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T) (Store, State) {
	t.Helper()
	root := t.TempDir()
	s := Store{Dir: filepath.Join(root, "Claude-3p"), StatePath: filepath.Join(root, "gateway", "desktop.json")}
	st := State{Models: []Model{{"claude-unity-compass", "Auto(Compass)"}, {"system.ai.claude-sonnet-4-6", "Sonnet"}, {"system.ai.claude-opus-4-6", "Opus"}}, Default: "claude-unity-compass", Hidden: map[string]bool{}}
	return s, st
}
func TestConnectRefreshDisconnectPreservesUserSettings(t *testing.T) {
	s, st := fixture(t)
	if err := writeJSON(s.metaPath(), map[string]any{"appliedId": "prior", "entries": []map[string]any{{"id": "prior", "name": "My gateway"}}, "extra": true}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(s.configPath(), map[string]any{"deploymentMode": "1p", "mcpServers": map[string]any{"local": map[string]any{"command": "my-server"}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(st); err != nil {
		t.Fatal(err)
	}
	if !s.Active() {
		t.Fatal("not active")
	}
	st, _ = s.Load()
	st.Default = "system.ai.claude-opus-4-6"
	st.Hidden["system.ai.claude-sonnet-4-6"] = true
	if err := s.Apply(st); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.profilePath())
	var profile struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"inferenceModels"`
	}
	_ = json.Unmarshal(raw, &profile)
	if len(profile.Models) != 2 || profile.Models[0].Name != st.Default {
		t.Fatalf("wrong visible/default models: %s", raw)
	}
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if s.Active() {
		t.Fatal("still active")
	}
	_, meta, _ := readObject(s.metaPath())
	_, cfg, _ := readObject(s.configPath())
	if string(meta["appliedId"]) != `"prior"` || string(meta["extra"]) != "true" || string(cfg["deploymentMode"]) != `"1p"` || !strings.Contains(string(cfg["mcpServers"]), "my-server") {
		t.Fatalf("settings were lost: %v %v", meta, cfg)
	}
}
func TestPreserveOutsideEdits(t *testing.T) {
	s, st := fixture(t)
	if err := s.Apply(st); err != nil {
		t.Fatal(err)
	}
	_ = writeJSON(s.profilePath(), map[string]any{"user": "modified"})
	if s.Active() {
		t.Fatal("reported externally modified profile as connected")
	}
	if err := s.Apply(st); err == nil {
		t.Fatal("overwrote user-edited profile")
	}
	_, meta, _ := readObject(s.metaPath())
	set(meta, "appliedId", "different")
	_ = writeJSON(s.metaPath(), meta)
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	_, meta, _ = readObject(s.metaPath())
	if string(meta["appliedId"]) != `"different"` {
		t.Fatal("overwrote newer profile selection")
	}
}
func TestRejectInvalidCatalogAndHiddenDefault(t *testing.T) {
	_, st := fixture(t)
	st.Models = append(st.Models, Model{"not-a-workspace-model", "Invalid"})
	if _, err := st.Profile(); err == nil {
		t.Fatal("accepted invalid workspace model")
	}
	_, st = fixture(t)
	st.Hidden[st.Default] = true
	if _, err := st.Profile(); err == nil {
		t.Fatal("accepted hidden default")
	}
}

func TestNonClaudePickerAliasesPreserveIdentityAndSettings(t *testing.T) {
	s, st := fixture(t)
	for _, m := range []Model{{"system.ai.gpt-6-1-sol", "GPT-6.1 Sol"}, {"system.ai.glm-5-3", "GLM 5.3"}, {"system.ai.kimi-k3", "Kimi K3"}} {
		st.Models = append(st.Models, m)
	}
	st.Default = "system.ai.gpt-6-1-sol"
	st.Hidden["system.ai.kimi-k3"] = true
	if err := s.Apply(st); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Load()
	if err != nil || saved.Default != st.Default || !saved.Has("system.ai.glm-5-3") || !saved.Hidden["system.ai.kimi-k3"] {
		t.Fatalf("lost actual IDs/settings: %+v, %v", saved, err)
	}
	profile, err := saved.Profile()
	if err != nil {
		t.Fatal(err)
	}
	models := profile["inferenceModels"].([]map[string]any)
	if models[0]["name"] != WireModel(st.Default) || models[0]["labelOverride"] != "GPT-6.1 Sol" {
		t.Fatal("wrong default/label", models[0])
	}
	for _, m := range models {
		wire := m["name"].(string)
		id, err := ResolveModel(wire)
		if err != nil || !saved.Has(id) || saved.Hidden[id] || !strings.Contains(wire, "claude") {
			t.Fatalf("invalid picker identity %s: %s, %v", wire, id, err)
		}
	}
	if len(models) != len(st.Models)-1 {
		t.Fatal("hidden model in picker")
	}
	// Refresh remains idempotent and disconnect still restores the old profile.
	if err := s.Apply(saved); err != nil {
		t.Fatal(err)
	}
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectMalformedDesktopAliases(t *testing.T) {
	for _, id := range []string{ModelAliasPrefix, ModelAliasPrefix + "xyz", ModelAliasPrefix + "2e2e2f", ModelAliasPrefix + "73797374656d2e61692e636c617564652d6861696b75"} {
		if _, err := ResolveModel(id); err == nil {
			t.Fatalf("accepted malformed/noncanonical alias %q", id)
		}
	}
}
func TestRejectMalformedOrOrganizationProfile(t *testing.T) {
	s, st := fixture(t)
	_ = writeJSON(s.metaPath(), map[string]any{"hybridPointer": map[string]string{"bootstrapUrl": "https://example.com"}})
	if err := s.Apply(st); err == nil {
		t.Fatal("replaced organization profile")
	}
}
