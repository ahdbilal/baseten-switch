package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unity-gateway/gateway/internal/codexdesktop"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/config"
)

func TestCodexDesktopListenerIsolation(t *testing.T) {
	raw, e := os.ReadFile("../../../config/gateway.example.yaml")
	if e != nil {
		t.Fatal(e)
	}
	before := string(raw)
	b, e := codexDesktopGatewayConfig(raw, true, "system.ai.glm-5-3")
	if e != nil {
		t.Fatal(e)
	}
	var f config.File
	if e = config.UnmarshalStrict(b, &f); e != nil {
		t.Fatal(e)
	}
	found := 0
	for _, c := range f.Clients {
		if c.Name == codexdesktop.Client {
			found++
			if c.BindAddr != "127.0.0.1:45276" || c.ProtocolShape != "openai" || !c.Enabled {
				t.Fatal(c)
			}
		}
	}
	if found != 1 {
		t.Fatal(found)
	}
	again, e := codexDesktopGatewayConfig(b, true, "system.ai.glm-5-3")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Count(string(again), "name: codex-desktop") != 1 {
		t.Fatal("duplicated listener")
	}
	off, e := codexDesktopGatewayConfig(again, false, "")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(off), "codex-desktop") {
		t.Fatal("listener retained")
	}
	var original, restored config.File
	config.UnmarshalStrict([]byte(before), &original)
	config.UnmarshalStrict(off, &restored)
	if len(original.Clients) != len(restored.Clients) {
		t.Fatal("other clients changed")
	}
	for i := range original.Clients {
		if original.Clients[i].Name != restored.Clients[i].Name || original.Clients[i].BindAddr != restored.Clients[i].BindAddr {
			t.Fatal("other listener changed")
		}
	}
}
func TestCodexDesktopSelectionCompassAndDisable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gateway.yaml")
	t.Setenv("UNITY_GATEWAY_CONFIG_PATH", path)
	t.Setenv("UNITY_GATEWAY_CODEX_HOME", filepath.Join(dir, "codex"))
	t.Setenv("UNITY_GATEWAY_GATEWAY_PIDFILE", filepath.Join(dir, "no-router.pid"))
	raw, e := os.ReadFile("../../../config/gateway.example.yaml")
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(path, raw, 0600)
	store := codexDesktopStore(path)
	st := codexdesktop.State{Models: []codexdesktop.Model{{ID: compass.Model, Name: "Auto(Compass)"}, {ID: "system.ai.glm-5-3", Name: "GLM"}}, Default: compass.Model, Hidden: map[string]bool{}}
	if e = applyCodexDesktop(store, st); e != nil {
		t.Fatal(e)
	}
	if code := cmdCodexDesktop([]string{"selection", "missing", "missing"}); code == 0 {
		t.Fatal("invalid model accepted")
	}
	a := &codexAdapter{configPath: path}
	cfg := compass.Defaults()
	cfg.ManualModels = map[string]string{"codex-desktop": "system.ai.glm-5-3"}
	if e = compass.Save(compass.Path(path), cfg); e != nil {
		t.Fatal(e)
	}
	if e = a.selectCompassClient("codex-desktop", false, &cfg); e != nil {
		t.Fatal(e)
	}
	st, _ = store.Load()
	if st.Default != "system.ai.glm-5-3" {
		t.Fatal(st.Default)
	}
	if e = a.selectCompassClient("codex-desktop", true, &cfg); e != nil {
		t.Fatal(e)
	}
	st, _ = store.Load()
	if st.Default != compass.Model {
		t.Fatal(st.Default)
	}
	if code := cmdCodexDesktop([]string{"off"}); code != 0 {
		t.Fatal(code)
	}
	status, e := readHarnessStatus("codex-desktop")
	if e != nil || status.Configured || status.Enabled {
		t.Fatal(status, e)
	}
}
