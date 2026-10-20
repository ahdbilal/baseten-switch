package ugruntime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestUGMissingRuntime(t *testing.T) {
	t.Setenv("UNITY_GATEWAY_UG_RUNTIME", t.TempDir())
	_, err := Render(context.Background(), Request{Client: "check"})
	if err == nil || !strings.Contains(err.Error(), "ug install") {
		t.Fatal(err)
	}
}

func TestUGPinnedBuilders(t *testing.T) {
	root := os.Getenv("UNITY_GATEWAY_TEST_UG_RUNTIME")
	if root == "" {
		t.Skip("set UNITY_GATEWAY_TEST_UG_RUNTIME to exercise the official pinned builders")
	}
	t.Setenv("UNITY_GATEWAY_UG_RUNTIME", root)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SMART_ROUTER_ENABLED", "1")
	for _, client := range []string{"check", "codex", "codex-desktop", "claude-code", "claude-desktop", "pi"} {
		t.Run(client, func(t *testing.T) {
			r, err := Render(context.Background(), Request{Client: client, Workspace: "https://example.cloud.databricks.com", Profile: "profile with spaces", BaseURL: "http://127.0.0.1:45276/v1", Provider: "test_gateway", Model: "Auto(Compass)", Models: []Model{{"claude-unity-compass", "Auto(Compass)"}, {"system.ai.glm-5-3", "GLM 5.3"}}})
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(r.Config)
			if client != "check" && (!strings.Contains(string(b), filepath.Join(root, "bin", "ug")) || !strings.Contains(string(b), "auth-token") || !strings.Contains(string(b), "127.0.0.1:45276")) {
				t.Fatalf("missing official auth/local routing: %s", b)
			}
			if strings.HasPrefix(client, "codex") {
				var parsed map[string]any
				if err := toml.Unmarshal([]byte(r.ProviderTOML), &parsed); err != nil {
					t.Fatal(err)
				}
				p := parsed["model_providers"].(map[string]any)["test_gateway"].(map[string]any)
				if p["requires_openai_auth"] != false || p["supports_websockets"] != false || p["auth"] == nil {
					t.Fatal(p)
				}
				if p["http_headers"].(map[string]any)["X-Unity-Gateway-Codex-Picker"] != "mixed-v1" {
					t.Fatal(p)
				}
			}
			if client == "claude-desktop" && (r.Config["inferenceCredentialKind"] != "helper-script" || r.Config["modelDiscoveryEnabled"] != false) {
				t.Fatal(r.Config)
			}
			if client == "pi" && (r.Config["authHeader"] != true || !strings.HasPrefix(r.Config["apiKey"].(string), "!")) {
				t.Fatal(r.Config)
			}
		})
	}
	for _, dir := range []string{".codex", ".claude", ".pi", "Library/Application Support/Claude-3p"} {
		if _, err := os.Stat(filepath.Join(home, dir)); !os.IsNotExist(err) {
			t.Fatalf("builder wrote harness settings: %s (%v)", dir, err)
		}
	}
	cmd := exec.Command(filepath.Join(root, "bin", "python"), "-I", "-c", bridge, "wrong-revision")
	cmd.Stdin = strings.NewReader(`{"client":"check"}`)
	if err := cmd.Run(); err == nil {
		t.Fatal("accepted an untested revision")
	}
	_, err := Render(context.Background(), Request{Client: "codex", Workspace: "https://example.cloud.databricks.com", Profile: "TEST", BaseURL: "https://another-host.example"})
	if err == nil {
		t.Fatal("accepted a non-loopback app overlay")
	}
}
