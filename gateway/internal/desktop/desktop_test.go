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
	st.Models = append(st.Models, Model{"system.ai.gpt-6-sol", "GPT"})
	if _, err := st.Profile(); err == nil {
		t.Fatal("accepted direct non-Claude model")
	}
	_, st = fixture(t)
	st.Hidden[st.Default] = true
	if _, err := st.Profile(); err == nil {
		t.Fatal("accepted hidden default")
	}
}
func TestRejectMalformedOrOrganizationProfile(t *testing.T) {
	s, st := fixture(t)
	_ = writeJSON(s.metaPath(), map[string]any{"hybridPointer": map[string]string{"bootstrapUrl": "https://example.com"}})
	if err := s.Apply(st); err == nil {
		t.Fatal("replaced organization profile")
	}
}
