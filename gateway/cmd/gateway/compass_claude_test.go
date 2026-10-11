package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
)

func TestClaudeCompassSessionAndChildIdentity(t *testing.T) {
	for _, identity := range []string{`{"session_id":"parent"}`, "user_account_session_parent"} {
		body, _ := json.Marshal(map[string]any{"metadata": map[string]string{"user_id": identity}})
		r := httptest.NewRequest("POST", "/v1/messages", nil)
		parent := compassThreadKey("claude-code", claudeCompassRequest(r, body))
		if parent == "" {
			t.Fatal("missing parent identity")
		}
		r.Header.Set(subagentAgentIDHeader, "child")
		child := compassThreadKey("claude-code", claudeCompassRequest(r, body))
		if child == "" || child == parent {
			t.Fatal("child shares parent pin")
		}
	}
	r := httptest.NewRequest("POST", "/v1/messages", nil)
	if compassThreadKey("claude-code", claudeCompassRequest(r, []byte(`{"metadata":{"user_id":"account-only"}}`))) != "" {
		t.Fatal("account used as session")
	}
}

func TestClaudeCompassBoundariesAndSystemBlocks(t *testing.T) {
	body := []byte(`{"model":"Auto(Compass)","system":[{"type":"text","text":"private instructions","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":[{"type":"text","text":"fix parser"}]}]}`)
	if ok, _ := claudeCompassReviewBoundary(body); !ok {
		t.Fatal("text boundary rejected")
	}
	for _, kind := range []string{"tool_result", "tool_use", "thinking", "redacted_thinking", "image", "document"} {
		if ok, _ := claudeCompassReviewBoundary([]byte(strings.Replace(string(body), `"text":"fix parser"`, `"text":"fix parser","type":"`+kind+`"`, 1))); ok {
			t.Fatalf("accepted %s", kind)
		}
	}
	context := compassContext(claudeCompassContextBody(body))
	if !strings.Contains(context, "fix parser") || strings.Contains(context, "private") {
		t.Fatal("unsafe classifier context")
	}
	delegated := claudeCompassDelegation(body)
	if !strings.Contains(string(delegated), `"cache_control"`) || string(claudeCompassDelegation(delegated)) != string(delegated) {
		t.Fatal("system/cache blocks changed or prompt duplicated")
	}
}

func TestClaudeCompassRoutesAndIsolatesCredentials(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Api-Key") != "" {
			t.Error("Claude key leaked to classifier")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tier\":\"high\",\"reason\":\"complex task\"}"}}]}`)
	}))
	defer server.Close()
	cfg := testConfig(t, server.URL, server.URL)
	cfg.OpenAIURL = server.URL
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	g, _, _ := newGateway(t, cfg, resolvedAnthropicBaseten(t))
	t.Setenv("OPENAI_API_KEY", "openai-test-key")
	body := []byte(`{"model":"Auto(Compass)","max_tokens":100,"messages":[{"role":"user","content":"fix parser"}],"metadata":{"user_id":"user_session_parent"}}`)
	for _, agent := range []string{"", "", "child", "child"} {
		r := httptest.NewRequest("POST", "/v1/messages", nil)
		r.Header.Set("Authorization", "Bearer claude-secret")
		r.Header.Set("X-Api-Key", "claude-secret")
		r.Header.Set(subagentAgentIDHeader, agent)
		r = claudeCompassRequest(r, body)
		ats, err := g.resolveAttempts(g.clients["claude-code"], r, body, "messages")
		if err != nil {
			t.Fatal(err)
		}
		at := ats[0]
		if len(ats) != 1 || at.route != "openai" || !at.translate || at.headers.Get("Authorization") != "Bearer openai-test-key" || at.headers.Get("X-Api-Key") != "" || at.origRequestedModel != compass.Model || at.compassDecision.Subagent != (agent != "") {
			t.Fatal("wrong route, identity, or credentials")
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("classified %d times", calls.Load())
	}
	t.Setenv("OPENAI_API_KEY", "")
	r := claudeCompassRequest(httptest.NewRequest("POST", "/v1/messages", nil), body)
	if _, err := g.resolveAttempts(g.clients["claude-code"], r, body, "messages"); err == nil {
		t.Fatal("accepted missing OpenAI credential")
	}
}

func TestBasetenResponsesIncludeCompatibility(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "http://127.0.0.1:1")
	rc := resolvedOpenAIBaseten(t, "codex", "baseten")
	rc.DefaultModel = "deepseek-ai/DeepSeek-V4.1-Flash-Fast"
	g, _, _ := newGateway(t, cfg, rc)
	body := []byte(`{"model":"test","include":["reasoning.encrypted_content","message.output_text.logprobs"],"input":[{"type":"reasoning","encrypted_content":"preserve-history"},{"role":"user","content":"hello"}],"tools":[{"type":"function","name":"test"}]}`)
	if string(stripBasetenResponseIncludes(body, "zai-org/GLM-5.3")) != string(body) {
		t.Fatal("modified supported model")
	}
	r := httptest.NewRequest("POST", "/v1/responses", nil)
	for _, route := range []string{"baseten", "openai"} {
		at, err := g.buildAttempt(g.clients["codex"], r, body, route, "responses")
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err = json.Unmarshal(at.res.NewBody, &got); err != nil {
			t.Fatal(err)
		}
		if (len(got["include"]) > 0) != (route == "openai") {
			t.Fatalf("wrong include handling for %s", route)
		}
		if !strings.Contains(string(got["input"]), "preserve-history") || len(got["tools"]) == 0 {
			t.Fatal("lost conversation or tools")
		}
	}
}
