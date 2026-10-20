package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unity-gateway/gateway/internal/desktop"
	"unity-gateway/gateway/internal/unity"
)

func TestDesktopManualModelAliases(t *testing.T) {
	cfg := testConfig(t, "https://inference.gateway.invalid", "https://api.anthropic.com")
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	if err := unity.Save(cfg.ConfigPath, unity.Connection{Workspace: "https://workspace.cloud.databricks.com", Profile: "demo"}); err != nil {
		t.Fatal(err)
	}
	rc := resolvedAnthropicDatabricks(t)
	rc.Name = desktop.Client
	g, _, _ := newGateway(t, cfg, rc)
	cl := g.clients[desktop.Client]
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	for _, model := range []string{"system.ai.gpt-6-1-sol", "system.ai.glm-5-3", "system.ai.kimi-k3"} {
		body := []byte(`{"model":"` + desktop.WireModel(model) + `","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"test"}],"tools":[{"name":"calculate","input_schema":{"type":"object"}}],"thinking":{"type":"enabled","budget_tokens":100}}`)
		ats, err := g.resolveAttempts(cl, r, body, "messages")
		if err != nil {
			t.Fatal(err)
		}
		at := ats[0]
		if len(ats) != 1 || at.compassDecision != nil || at.res.UpstreamModel != model || at.telRequestedModel() != model || at.modelForCost != model || at.translate {
			t.Fatalf("wrong identity/routing: %+v", at)
		}
		var original, forwarded map[string]any
		_ = json.Unmarshal(body, &original)
		_ = json.Unmarshal(at.res.NewBody, &forwarded)
		original["model"] = model
		if !reflect.DeepEqual(original, forwarded) {
			t.Fatal("changed request beyond model alias")
		}
	}
	bad := []byte(`{"model":"claude-unity-model-invalid"}`)
	if _, err := g.resolveAttempts(cl, r, bad, "messages"); err == nil {
		t.Fatal("silently fell back for invalid alias")
	}
	valid := []byte(`{"model":"` + desktop.WireModel("system.ai.gpt-6-1-sol") + `"}`)
	if _, err := g.resolveAttempts(cl, r, valid, "responses"); err == nil {
		t.Fatal("accepted alias on wrong API")
	}
	other := *cl
	other.cfg.Name = "claude-code"
	if _, err := g.resolveAttempts(&other, r, valid, "messages"); err == nil {
		t.Fatal("accepted alias on other client")
	}
}

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
