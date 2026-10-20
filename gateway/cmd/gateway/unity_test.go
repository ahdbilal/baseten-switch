package gateway

import (
	"unity-gateway/gateway/internal/unity"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnityNativeRequestsPreserveToolsAndReasoning(t *testing.T) {
	cfg := testConfig(t, "https://inference.gateway.invalid", "https://api.anthropic.com")
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	if e := unity.Save(cfg.ConfigPath, unity.Connection{Workspace: "https://workspace.cloud.databricks.com", Profile: "demo"}); e != nil {
		t.Fatal(e)
	}
	g, _, _ := newGateway(t, cfg, resolvedOpenAIDatabricks(t, "codex", "databricks"))
	body := []byte(`{"model":"system.ai.glm-5-3","input":"test","include":["reasoning.encrypted_content"],"tools":[{"type":"namespace","name":"agents","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}]}`)
	for _, kind := range []string{"responses", "messages", "chat"} {
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer native-secret")
		req.Header.Set("X-Api-Key", "other-secret")
		at, e := g.unityAttempt(g.clients["codex"], req, body, kind, "")
		if e != nil {
			t.Fatal(e)
		}
		c := g.unityConnection()
		if at.url != c.Endpoint(kind) {
			t.Fatal(at.url)
		}
		if string(at.res.NewBody) != string(body) {
			t.Fatal("request tools/reasoning mutated")
		}
		if at.headers.Get("Authorization") != "" || at.headers.Get("X-Api-Key") != "" {
			t.Fatal("foreign credential retained")
		}
		if at.translate {
			t.Fatal("native Anthropic payload translated")
		}
	}
	req := httptest.NewRequest("POST", "/v1/responses", nil)
	if _, e := g.unityAttempt(g.clients["codex"], req, []byte(`{"model":"zai-org/GLM-5.3"}`), "responses", ""); e == nil {
		t.Fatal("legacy vendor model silently accepted")
	}
	if e := os.WriteFile(unity.Path(cfg.ConfigPath), []byte(`{broken`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := g.unityAttempt(g.clients["codex"], req, body, "responses", ""); e == nil {
		t.Fatal("invalid connection did not fail closed")
	}
}
