package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/desktop"
)

func TestDesktopGatewayListener(t *testing.T) {
	raw := []byte("global:\n  routing_enabled: true\nclients:\n  - name: codex\n    enabled: true\n    bind_addr: 127.0.0.1:45272\n    protocol_shape: openai\n    default_model: system.ai.glm-5-3\n")
	data, err := desktopGatewayConfig(raw, "system.ai.claude-sonnet-4-6")
	if err != nil {
		t.Fatal(err)
	}
	var f config.File
	if err = config.UnmarshalStrict(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Clients) != 2 || f.Clients[1].Name != "claude-desktop" || f.Clients[1].ProtocolShape != "anthropic" {
		t.Fatal(string(data))
	}
	again, err := desktopGatewayConfig(data, "system.ai.claude-sonnet-4-6")
	if err != nil || string(again) != string(data) {
		t.Fatal("not idempotent", err)
	}
	conflict := strings.ReplaceAll(string(raw), ":45272", ":45275")
	if _, err = desktopGatewayConfig([]byte(conflict), "system.ai.claude-sonnet-4-6"); err == nil {
		t.Fatal("allowed port collision")
	}
}

func TestDesktopSelectionCommand(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gateway.yaml")
	t.Setenv("UNITY_GATEWAY_CONFIG_PATH", path)
	t.Setenv("UNITY_GATEWAY_CLAUDE_DESKTOP_DIR", filepath.Join(root, "Claude"))
	store, err := desktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	st := desktop.State{Models: []desktop.Model{{ID: "claude-unity-compass", Name: "Auto(Compass)"}, {ID: "system.ai.gpt-6-1-sol", Name: "GPT-6.1 Sol"}}, Default: "claude-unity-compass"}
	if err = store.Apply(st); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(store.StatePath)
	for _, args := range [][]string{{"selection"}, {"selection", "system.ai.gpt-6-1-sol"}, {"selection", "system.ai.gpt-6-1-sol", "missing"}} {
		if code := cmdDesktop(args); code == 0 {
			t.Fatal("accepted invalid selection", args)
		}
	}
	after, _ := os.ReadFile(store.StatePath)
	if string(before) != string(after) {
		t.Fatal("invalid selection changed state")
	}
	if code := cmdDesktop([]string{"selection", "system.ai.gpt-6-1-sol", "system.ai.gpt-6-1-sol"}); code != 0 {
		t.Fatal(code)
	}
	saved, err := store.Load()
	if err != nil || saved.Default != "system.ai.gpt-6-1-sol" || saved.Hidden[saved.Default] || !saved.Hidden["claude-unity-compass"] || !store.Active() {
		t.Fatalf("bulk selection failed: %+v, %v", saved, err)
	}
}
