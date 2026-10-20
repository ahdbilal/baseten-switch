package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPiSettingsMigrationPreservesPreferencesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"defaultProvider":"baseten","defaultModel":"baseten-switch-compass","enabledModels":["baseten/*","anthropic/claude-opus"],"theme":"dark","extensions":["custom"]}`)
	path := filepath.Join(dir, "settings.json")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := PiSettings(dir, []string{"Auto(Compass)", "system.ai.glm-5-3"}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	var d map[string]any
	json.Unmarshal(b, &d)
	if d["defaultProvider"] != "databricks" || d["defaultModel"] != "Auto(Compass)" || d["theme"] != "dark" {
		t.Fatal(string(b))
	}
	filters := d["enabledModels"].([]any)
	if filters[0] != "databricks/*" || filters[1] != "anthropic/claude-opus" {
		t.Fatal(filters)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "settings.before-unity-*.json"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	orig, _ := os.ReadFile(backups[0])
	if string(orig) != string(raw) {
		t.Fatal("backup differs")
	}
	if e := PiSettings(dir, []string{"Auto(Compass)"}); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(b) {
		t.Fatal("not idempotent")
	}
}
func TestPiSettingsLeavesUnrelatedProviderUnchanged(t *testing.T) {
	raw := []byte(`{"defaultProvider":"anthropic","defaultModel":"claude-opus","theme":"dark"}`)
	b, e := piSettings(raw, []string{"Auto(Compass)"})
	if e != nil || string(b) != string(raw) {
		t.Fatal(string(b), e)
	}
}
