package main

import (
	"strings"
	"testing"
	"unity-gateway/gateway/internal/config"
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
