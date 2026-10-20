package migration

import (
	"os"
	"path/filepath"
	"testing"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/unity"
)

func TestMigrationPreservesOriginalAndDoesNotImportCredentials(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".config", "baseten-switch")
	os.MkdirAll(old, 0700)
	raw := []byte("global:\n  routing_enabled: true\nclients:\n  - name: codex\n    enabled: true\n    bind_addr: 127.0.0.1:45272\n    protocol_shape: openai\n    default_model: zai-org/GLM-5.3\n")
	os.WriteFile(filepath.Join(old, "gateway.yaml"), raw, 0600)
	os.WriteFile(filepath.Join(old, "gateway.yaml.api-key"), []byte("never-copy"), 0600)
	if e := unity.Save(filepath.Join(old, "gateway.yaml"), unity.Connection{Workspace: "https://workspace.example", Profile: "fixture"}); e != nil {
		t.Fatal(e)
	}
	if _, e := Run(home); e != nil {
		t.Fatal(e)
	}
	original, _ := os.ReadFile(filepath.Join(old, "gateway.yaml"))
	if string(original) != string(raw) {
		t.Fatal("original modified")
	}
	dest := filepath.Join(home, ".config", "unity-gateway")
	if _, e := os.Stat(filepath.Join(dest, "gateway.yaml.api-key")); !os.IsNotExist(e) {
		t.Fatal("credential imported")
	}
	c, e := compass.Load(filepath.Join(dest, "compass.json"))
	if e != nil || c.High != "system.ai.gpt-6-astra" {
		t.Fatal(c, e)
	}
	os.WriteFile(filepath.Join(dest, "keep"), []byte("user edit"), 0600)
	if _, e := Run(home); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(dest, "keep")); e != nil {
		t.Fatal("existing state overwritten")
	}
}
