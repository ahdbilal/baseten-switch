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
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
	"github.com/basetenlabs/baseten-switch/gateway/internal/config"
)

func TestCompassAvailabilityFallback(t *testing.T) {
	for _, tc := range []struct {
		name              string
		primary, fallback int
		enabled, opaque   bool
	}{
		{"429", 429, 200, true, false}, {"503", 503, 200, true, false},
		{"disabled", 429, 200, false, false}, {"bad_request", 400, 200, true, false},
		{"fallback_failed", 429, 503, true, false}, {"opaque_history", 429, 200, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var primaryHits, fallbackHits atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/chat/completions" {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"tier\":\"low\",\"reason\":\"routine\"}"}}]}`)
					return
				}
				if body["model"] == "deepseek-ai/DeepSeek-V4.1-Flash-Fast" {
					primaryHits.Add(1)
					w.WriteHeader(tc.primary)
					fmt.Fprint(w, `{"error":"Rate limit exceeded"}`)
					return
				}
				fallbackHits.Add(1)
				if body["model"] != "zai-org/GLM-5.3" {
					t.Errorf("unexpected fallback model %v", body["model"])
				}
				if len(body["tools"].([]any)) != 1 {
					t.Error("tools lost")
				}
				w.WriteHeader(tc.fallback)
				fmt.Fprint(w, `{"id":"resp_test","object":"response","status":"completed","model":"zai-org/GLM-5.3","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"4"}]}],"usage":{"input_tokens":10,"output_tokens":1,"total_tokens":11}}`)
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, upstream.URL)
			cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
			c := compass.Defaults()
			c.Low = "deepseek-ai/DeepSeek-V4.1-Flash-Fast"
			if err := compass.Save(compass.Path(cfg.ConfigPath), c); err != nil {
				t.Fatal(err)
			}
			rc := resolvedOpenAIBaseten(t, "codex", "baseten")
			rc.HasFallbackPolicy = true
			rc.FallbackPolicy = config.ResolvedFallbackPolicy{OnBaseten429: tc.enabled, OnBaseten5xx: tc.enabled}
			g, admin, _ := newGateway(t, cfg, rc)
			defer admin.Close()
			stop := start(t, g)
			defer stop()
			body := `{"model":"Auto(Compass)","input":"2+2","tools":[{"type":"function","name":"spawn_agent","parameters":{"type":"object"}}]}`
			if tc.opaque {
				body = strings.Replace(body, `"input":"2+2"`, `"input":[{"type":"reasoning","encrypted_content":"opaque"},{"role":"user","content":"2+2"}]`, 1)
			}
			req, _ := http.NewRequest("POST", "http://"+g.ClientAddr("codex").String()+"/v1/responses", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Thread-Id", "parent")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			recovered := tc.enabled && !tc.opaque && (tc.primary == 429 || tc.primary == 503)
			want := tc.primary
			if recovered {
				want = tc.fallback
			}
			if resp.StatusCode != want {
				t.Fatalf("status %d want %d", resp.StatusCode, want)
			}
			if (fallbackHits.Load() > 0) != recovered {
				t.Fatalf("fallback hits %d", fallbackHits.Load())
			}
			if g.fallbackActive("codex") {
				t.Error("Compass recovery changed listener-wide native cooldown")
			}
			waitForRows(t, cfg.TelemetryDir, 1, 2*time.Second)
			key := compass.SessionKey("codex", "", "parent")
			d, err := compass.LoadDecision(compass.Path(cfg.ConfigPath), key)
			if err != nil {
				t.Fatal(err)
			}
			if recovered && tc.fallback == 200 {
				if d.Model != c.Default || resp.Header.Get("X-Baseten-Compass-Model") != c.Default {
					t.Fatalf("fallback not reflected in pin/header: %s %s", d.Model, resp.Header.Get("X-Baseten-Compass-Model"))
				}
				req, _ = http.NewRequest("POST", "http://"+g.ClientAddr("codex").String()+"/v1/responses", strings.NewReader(body))
				req.Header.Set("Thread-Id", "parent")
				resp, err = http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if primaryHits.Load() != 1 {
					t.Error("retried failed model on next turn")
				}
			} else if d.Model != c.Low {
				t.Error("pinned unsuccessful fallback")
			}
		})
	}
}
