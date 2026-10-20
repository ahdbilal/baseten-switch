package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"unity-gateway/gateway/internal/compass"
)

func TestCompassMasterSwitchAcrossClientProtocols(t *testing.T) {
	for _, name := range []string{"codex", "codex-desktop", "pi", "claude-code", "claude-desktop"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tier\":\"high\",\"reason\":\"Complex task\"}"}}]}`)
			}))
			defer server.Close()
			cfg := testConfig(t, server.URL, server.URL)
			cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
			routing := legacyCompassFixture()
			routing.High = "test/high-model"
			routing.Rules.EncourageSubagents = true
			routing.Rules.SessionSwitching = true
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			client := resolvedOpenAIDatabricks(t, name, "databricks")
			kind := "responses"
			body := `{"model":"Auto(Compass)","input":[{"role":"user","content":"Design a distributed scheduler"}]}`
			if strings.HasPrefix(name, "claude") {
				client = resolvedAnthropicDatabricks(t)
				client.Name = name
				kind = "messages"
				body = `{"model":"Auto(Compass)","max_tokens":100,"messages":[{"role":"user","content":"Design a distributed scheduler"}]}`
			}
			g, _, _ := newGateway(t, cfg, client)
			request := func(thread, model string) []upstreamAttempt {
				raw := []byte(strings.Replace(body, compass.Model, model, 1))
				r := httptest.NewRequest("POST", "/v1/"+kind, nil)
				r.Header.Set("Thread-Id", thread)
				r.Header.Set("Authorization", "Bearer test")
				r.Header.Set("X-Unity-Gateway-Codex-Picker", "mixed-v1")
				ats, err := g.resolveAttempts(g.clients[name], r, raw, kind)
				if err != nil || len(ats) == 0 {
					t.Fatalf("request failed: %v", err)
				}
				return ats
			}
			ats := request("existing", compass.Model)
			if ats[0].modelForCost != routing.High || calls.Load() != 1 {
				t.Fatal("enabled route failed")
			}
			off := false
			routing.Enabled = &off
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			ats = request("existing", compass.Model)
			if ats[0].modelForCost != routing.High || strings.Contains(string(ats[0].res.NewBody), compassDelegationPrompt) {
				t.Fatal("existing session changed or delegation continued")
			}
			ats = request("new-disabled", compass.Model)
			if ats[0].modelForCost != routing.Default || ats[0].compassDecision.Tier != "default" || calls.Load() != 1 {
				t.Fatal("off did not use fallback")
			}
			ats = request("manual", routing.Low)
			if ats[0].compassDecision != nil || calls.Load() != 1 {
				t.Fatal("manual choice invoked Compass")
			}
			on := true
			routing.Enabled = &on
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			ats = request("new-enabled", compass.Model)
			if ats[0].modelForCost != routing.High || calls.Load() != 2 {
				t.Fatal("re-enable failed")
			}
		})
	}
}

func TestCompassScopedPoliciesAcrossClientProtocols(t *testing.T) {
	for _, name := range []string{"codex", "codex-desktop", "pi", "claude-code", "claude-desktop"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tier\":\"high\",\"reason\":\"Complex task\"}"}}]}`)
			}))
			defer server.Close()
			cfg := testConfig(t, server.URL, server.URL)
			cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
			routing := legacyCompassFixture()
			routing.High = "test/high-model"
			routing.Rules.EncourageSubagents = true
			routing.Rules.SessionSwitching = true
			shared := routing.Policy()
			shared.High = "test/harness-high"
			override := shared
			override.High = "test/client-high"
			routing.Harnesses = map[string]compass.Policy{compass.Harness(name): shared}
			routing.Overrides = map[string]compass.Policy{name: override}
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			client := resolvedOpenAIDatabricks(t, name, "databricks")
			kind := "responses"
			body := `{"model":"Auto(Compass)","input":[{"role":"user","content":"Design a distributed scheduler"}]}`
			if strings.HasPrefix(name, "claude") {
				client = resolvedAnthropicDatabricks(t)
				client.Name = name
				kind = "messages"
				body = `{"model":"Auto(Compass)","max_tokens":100,"messages":[{"role":"user","content":"Design a distributed scheduler"}]}`
			}
			g, _, _ := newGateway(t, cfg, client)
			request := func(thread, model string) []upstreamAttempt {
				raw := []byte(strings.Replace(body, compass.Model, model, 1))
				r := httptest.NewRequest("POST", "/v1/"+kind, nil)
				r.Header.Set("Thread-Id", thread)
				r.Header.Set("Authorization", "Bearer test")
				r.Header.Set("X-Unity-Gateway-Codex-Picker", "mixed-v1")
				ats, err := g.resolveAttempts(g.clients[name], r, raw, kind)
				if err != nil || len(ats) == 0 {
					t.Fatalf("request failed: %v", err)
				}
				return ats
			}
			ats := request("existing", compass.Model)
			if ats[0].modelForCost != override.High || calls.Load() != 1 {
				t.Fatal("enabled route failed")
			}
			off := false
			routing.Enabled = &off
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			ats = request("existing", compass.Model)
			if ats[0].modelForCost != override.High || strings.Contains(string(ats[0].res.NewBody), compassDelegationPrompt) {
				t.Fatal("existing session changed or delegation continued")
			}
			ats = request("new-disabled", compass.Model)
			if ats[0].modelForCost != routing.Default || ats[0].compassDecision.Tier != "default" || calls.Load() != 1 {
				t.Fatal("off did not use fallback")
			}
			ats = request("manual", routing.Low)
			if ats[0].compassDecision != nil || calls.Load() != 1 {
				t.Fatal("manual choice invoked Compass")
			}
			delete(routing.Overrides, name)
			on := true
			routing.Enabled = &on
			if err := compass.Save(compass.Path(cfg.ConfigPath), routing); err != nil {
				t.Fatal(err)
			}
			ats = request("new-enabled", compass.Model)
			if ats[0].modelForCost != shared.High || calls.Load() != 2 {
				t.Fatal("re-enable failed")
			}
		})
	}
}
