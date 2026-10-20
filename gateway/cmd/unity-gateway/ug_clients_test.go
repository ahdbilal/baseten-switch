package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"unity-gateway/gateway/internal/codexdesktop"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/desktop"
	"unity-gateway/gateway/internal/piconfig"
	"unity-gateway/gateway/internal/ugruntime"
	"unity-gateway/gateway/internal/unity"
)

var ugTestConnection = unity.Connection{Workspace: "https://example.cloud.databricks.com", Profile: "TEST"}

func useUGRuntime(t *testing.T) {
	t.Helper()
	root := os.Getenv("UNITY_GATEWAY_TEST_UG_RUNTIME")
	if root == "" {
		t.Skip("set UNITY_GATEWAY_TEST_UG_RUNTIME for official builder integration tests")
	}
	t.Setenv("UNITY_GATEWAY_UG_RUNTIME", root)
	t.Setenv("HOME", t.TempDir())
}

func TestUGMissingRuntimePreservesClients(t *testing.T) {
	t.Setenv("UNITY_GATEWAY_UG_RUNTIME", t.TempDir())
	a, _ := testCodexAdapter(t)
	if err := unity.Save(a.configPath, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	writeOverlayFile(t, a, codexTestStaleOverlay)
	if a.on() == 0 {
		t.Fatal("accepted missing ug")
	}
	if string(fileBytes(t, a.overlayPath)) != codexTestStaleOverlay {
		t.Fatal("Codex changed before render succeeded")
	}
	c, _ := testAdapter(t)
	c.configPath = filepath.Join(t.TempDir(), "gateway.yaml")
	if err := unity.Save(c.configPath, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	writeSettingsFile(t, c, `{"theme":"dark"}`)
	before := fileBytes(t, c.settingsPath)
	if c.on() == 0 || !bytes.Equal(before, fileBytes(t, c.settingsPath)) {
		t.Fatal("Claude native settings changed on render failure")
	}
	if _, err := os.Stat(c.ugSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("overlay written on render failure")
	}
}

func TestUGCodexOverlayRoundTrip(t *testing.T) {
	useUGRuntime(t)
	a, out := testCodexAdapter(t)
	if err := unity.Save(a.configPath, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	writeOverlayFile(t, a, codexTestStaleOverlay)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	raw := fileBytes(t, a.overlayPath)
	for _, mixed := range []bool{true, false, true} {
		raw = codexMixedProviderOverlay(codexPickerOverlay(raw, "/tmp/catalog.json"), mixed)
		var d map[string]any
		if err := toml.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
		p := d["model_providers"].(map[string]any)["databricks"].(map[string]any)
		if p["auth"] == nil || p["http_headers"].(map[string]any)["User-Agent"] == nil {
			t.Fatal("picker removed ug authentication or attribution")
		}
	}
	if a.off() != 0 || string(fileBytes(t, a.overlayPath)) != codexTestStaleOverlay {
		t.Fatal("Codex original overlay was not restored")
	}
}

func TestUGClaudeIsolatedLaunchAndRestore(t *testing.T) {
	useUGRuntime(t)
	a, out := testAdapter(t)
	a.configPath = filepath.Join(t.TempDir(), "gateway.yaml")
	if err := unity.Save(a.configPath, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	native := `{"env":{"ANTHROPIC_BASE_URL":"https://native.example"},"theme":"dark"}`
	writeSettingsFile(t, a, native)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	if string(fileBytes(t, a.settingsPath)) != native {
		t.Fatal("native connection changed")
	}
	args, isolated, err := a.ugLaunchArgs([]string{"--model", "claude-unity-compass", "hello"})
	if err != nil || !isolated || len(args) != 5 || args[0] != "--settings" || args[1] != a.ugSettingsPath() {
		t.Fatal(args, isolated, err)
	}
	if _, _, err := a.ugLaunchArgs([]string{"--settings=other.json"}); err == nil {
		t.Fatal("conflicting settings accepted")
	}
	if a.off() != 0 || string(fileBytes(t, a.settingsPath)) != native {
		t.Fatal("native setup not retained")
	}
	if _, isolated, err := a.ugLaunchArgs(nil); err != nil || isolated {
		t.Fatal("disabled integration still injects settings")
	}
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	if err := os.WriteFile(a.ugSettingsPath(), []byte(`{"user_edit":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if a.off() == 0 || string(fileBytes(t, a.ugSettingsPath())) != `{"user_edit":true}` {
		t.Fatal("foreign edit removed")
	}
}

func TestUGDesktopProfilesRoundTrip(t *testing.T) {
	useUGRuntime(t)
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	if err := unity.Save(path, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNITY_GATEWAY_CODEX_HOME", t.TempDir())
	s := codexDesktopStore(path)
	original := []byte("# original\nmodel = \"gpt-native\"\n[features]\nmulti_agent = true\n")
	if err := os.WriteFile(s.ConfigPath(), original, 0600); err != nil {
		t.Fatal(err)
	}
	st := codexdesktop.State{Models: []codexdesktop.Model{{ID: compass.Model, Name: compass.Model}}, Default: compass.Model}
	if err := applyCodexDesktop(s, st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fileBytes(t, s.ConfigPath())), ".auth]") {
		t.Fatal("ug auth missing")
	}
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, fileBytes(t, s.ConfigPath())) {
		t.Fatal("native Codex config not restored exactly")
	}
	t.Setenv("UNITY_GATEWAY_CLAUDE_DESKTOP_DIR", t.TempDir())
	d, err := desktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ds := desktop.State{Models: []desktop.Model{{ID: compass.DesktopModel, Name: compass.Model}, {ID: "system.ai.glm-5-3", Name: "GLM"}}, Default: compass.DesktopModel}
	if err := d.Apply(ds); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Load()
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(saved.WrittenProfile, &p); err != nil {
		t.Fatal(err)
	}
	if p["inferenceCredentialKind"] != "helper-script" || p["inferenceModels"].([]any)[0].(map[string]any)["name"] != compass.DesktopModel {
		t.Fatal(p)
	}
	if err := d.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if d.Active() {
		t.Fatal("Claude Desktop remained active")
	}
}

func TestUGPiProviderUsesOfficialAuth(t *testing.T) {
	useUGRuntime(t)
	r, err := ugTestConnection.RenderClient(context.Background(), ugruntime.Request{Client: "pi", BaseURL: "http://127.0.0.1:45274", Models: []ugruntime.Model{{ID: compass.Model, Name: compass.Model}}, Model: compass.Model})
	if err != nil {
		t.Fatal(err)
	}
	b, err := marshalPiProvider(piProviderSpec{UGProvider: r.Config, ID: piProviderID, Name: "Databricks Unity Gateway", Models: []piProviderModel{{ID: compass.Model, Name: compass.Model, ContextWindow: 128000, MaxTokens: 16384, Input: []string{"text"}, CapabilityKnown: true}}})
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p["api"] != "openai-responses" || !strings.HasPrefix(p["apiKey"].(string), "!") || p["authHeader"] != true {
		t.Fatal(p)
	}
	if p["models"].([]any)[0].(map[string]any)["contextWindow"] != float64(128000) {
		t.Fatal("app model metadata missing")
	}
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, piModelsFilename)
	original := []byte(`{"providers":{"native":{"baseUrl":"https://native.example"}}}`)
	if err := os.WriteFile(modelsPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	s := &piFileStore{backupRoot: t.TempDir()}
	spec := testPiInstallRequest(dir)
	spec.Provider.UGProvider = r.Config
	second := spec.Provider.Models[0]
	second.ID = "example/second"
	spec.Provider.Models = append(spec.Provider.Models, second)
	if _, err := s.Install(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	for _, visible := range []bool{false, true} {
		if err := s.SetVisibility(dir, "example/model", visible); err != nil {
			t.Fatal(err)
		}
		raw, _, err := piconfig.Provider(fileBytes(t, modelsPath), piProviderID)
		if err != nil {
			t.Fatal(err)
		}
		var saved map[string]any
		if err := json.Unmarshal(raw, &saved); err != nil {
			t.Fatal(err)
		}
		if saved["authHeader"] != true || saved["apiKey"] != r.Config["apiKey"] {
			t.Fatal("visibility changed official ug authentication")
		}
	}
	if _, err := s.Uninstall(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, fileBytes(t, modelsPath)) {
		t.Fatal("Pi native provider not restored")
	}
}

func TestUGClaudeMigrationRestoresLegacyNativeValues(t *testing.T) {
	useUGRuntime(t)
	a, out := testAdapter(t)
	a.configPath = filepath.Join(t.TempDir(), "gateway.yaml")
	writeSettingsFile(t, a, `{"env":{"ANTHROPIC_BASE_URL":"https://native.example","ENABLE_TOOL_SEARCH":"auto:25"},"theme":"dark"}`)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	if err := unity.Save(a.configPath, ugTestConnection); err != nil {
		t.Fatal(err)
	}
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	if v, _ := envValue(t, a.settingsPath, claudeManagedEnvKey); v != "https://native.example" {
		t.Fatal("legacy base URL not restored")
	}
	if v, _ := envValue(t, a.settingsPath, claudeToolSearchEnvKey); v != "auto:25" {
		t.Fatal("legacy tool-search value not restored")
	}
	if s, err := a.ugSettings(); err != nil || !s.Exists {
		t.Fatal("new isolated connection missing", err)
	}
}
