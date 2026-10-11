package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
)

func TestCompassRoutesOncePerThreadAndSeparatelyForSubagent(t *testing.T) {
	var calls atomic.Int32
	classifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("ChatGPT-Account-Id") != "" || r.Header.Get("Authorization") == "Bearer native-token" {
			t.Error("native credential leaked to classifier")
		}
		raw, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(raw, &request)
		if request["model"] != "zai-org/GLM-5.3-Fast" {
			t.Error("wrong classifier model")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tier\":\"high\",\"reason\":\"Difficult reasoning\"}"}}]}`)
	}))
	defer classifier.Close()
	cfg := testConfig(t, classifier.URL, "http://127.0.0.1:1")
	cfg.OpenAIURL = DefaultOpenAIURL
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	g, _, _ := newGateway(t, cfg, resolvedOpenAIBaseten(t, "codex", "baseten"))
	for i, thread := range []string{"parent", "parent", "child", "child"} {
		body := []byte(`{"model":"baseten-switch-compass","input":[{"role":"user","content":"Diagnose a distributed consistency bug"}],"stream":true}`)
		if i%2 == 1 {
			body = []byte(strings.ReplaceAll(string(body), compass.LegacyModel, compass.Model))
		}
		r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
		r.Header.Set("Thread-Id", thread)
		r.Header.Set("Session-Id", "shared-parent")
		r.Header.Set("Authorization", "Bearer native-token")
		r.Header.Set("ChatGPT-Account-Id", "account")
		if thread == "child" {
			r.Header.Set("X-OpenAI-Subagent", "spawn")
		}
		attempts, err := g.resolveAttempts(g.clients["codex"], r, body, "responses")
		if err != nil {
			t.Fatal(err)
		}
		if len(attempts) != 1 || attempts[0].route != "openai" || attempts[0].modelForCost != compass.Defaults().High || attempts[0].origRequestedModel != compass.Model {
			t.Fatalf("wrong route: %+v", attempts)
		}
		if attempts[0].compassDecision == nil || attempts[0].subagent != (thread == "child") {
			t.Fatal("missing Compass decision or subagent identity")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("classified %d times; want once per thread", calls.Load())
	}
}
func TestCompassContextExcludesInstructionsAndBinary(t *testing.T) {
	raw := []byte(`{"instructions":"private developer configuration","input":[{"role":"system","content":"system secret"},{"role":"user","content":[{"type":"input_text","text":"Fix the parser"},{"type":"input_image","image_url":"data:image/png;base64,secret"}]}]}`)
	got := compassContext(raw)
	if !strings.Contains(got, "Fix the parser") || strings.Contains(got, "secret") || strings.Contains(got, "configuration") {
		t.Fatalf("unsafe context: %s", got)
	}
}

func TestCompassNativeReasoningUsesProviderDefault(t *testing.T) {
	body := []byte(`{"model":"gpt-6-astra","reasoning":{"effort":"none"},"input":[]}`)
	got := compassTargetReasoning(body, "gpt-6-astra")
	if strings.Contains(string(got), "reasoning") {
		t.Fatalf("unsupported native effort retained: %s", got)
	}
	if string(compassTargetReasoning(body, "zai-org/GLM-5.3")) != string(body) {
		t.Fatal("Baseten reasoning changed")
	}
	explicit := []byte(`{"reasoning":{"effort":"high"}}`)
	if string(compassTargetReasoning(explicit, "gpt-6-astra")) != string(explicit) {
		t.Fatal("explicit effort changed")
	}
}
