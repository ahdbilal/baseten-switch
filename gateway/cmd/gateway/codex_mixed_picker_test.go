package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodexMixedPickerRoutesToSelectedProvider(t *testing.T) {
	cfg := testConfig(t, "http://127.0.0.1:1", "http://127.0.0.1:2")
	cfg.OpenAIURL = DefaultOpenAIURL
	g, _, _ := newGateway(t, cfg, resolvedOpenAIBaseten(t, "codex", "baseten"))
	for _, tc := range []struct{ model, marker, route, url string }{
		{"gpt-test", "mixed-v1", "openai", "https://chatgpt.com/backend-api/codex/responses"},
		{"example/model", "mixed-v1", "baseten", "http://127.0.0.1:1/v1/responses"},
		{CodexCompatibilityModel, "mixed-v1", "baseten", "http://127.0.0.1:1/v1/responses"},
		{"gpt-test", "", "baseten", "http://127.0.0.1:1/v1/responses"},
	} {
		t.Run(tc.model+tc.marker, func(t *testing.T) {
			body := []byte(`{"model":"` + tc.model + `","input":[],"stream":true}`)
			r := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(string(body)))
			r.Header.Set("X-Baseten-Switch-Codex-Picker", tc.marker)
			r.Header.Set("Authorization", "Bearer native-test-token")
			r.Header.Set("ChatGPT-Account-Id", "test-account")
			attempts, err := g.resolveAttemptsLadder(g.clients["codex"], r, body, "responses")
			if err != nil {
				t.Fatal(err)
			}
			if len(attempts) != 1 || attempts[0].route != tc.route || attempts[0].url != tc.url {
				t.Fatalf("attempts: %+v", attempts)
			}
			at := attempts[0]
			if at.headers.Get("X-Baseten-Switch-Codex-Picker") != "" {
				t.Fatal("private routing header leaked")
			}
			if tc.route == "openai" {
				if string(at.res.NewBody) != string(body) || at.headers.Get("Authorization") != "Bearer native-test-token" {
					t.Fatal("native request changed")
				}
			} else if at.headers.Get("Authorization") == "Bearer native-test-token" || at.headers.Get("ChatGPT-Account-Id") != "" {
				t.Fatal("native credentials leaked to Baseten")
			}
		})
	}
}
