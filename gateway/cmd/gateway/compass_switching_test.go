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
)

// Exercise real request routing and telemetry accounting, without paid model
// calls, synthetic ledger edits, or changing the user's gateway configuration.
func TestCompassMainSessionSwitchHTTP(t *testing.T) {
	for _, tc := range []struct {
		name          string
		input, cached int
		wantSwitch    bool
		wantReason    string
	}{
		{"early", 1000, 0, true, ""},
		{"late", 600000, 0, true, ""},
		{"middle_window", 100000, 0, false, "between $1 and $10"},
		{"warm_cache", 100000, 100000, false, "cache rebuild"},
		{"missing_usage", 0, 0, false, "incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var classifications atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/chat/completions" {
					tier := "high"
					if classifications.Add(1) > 1 {
						tier = "low"
					}
					fmt.Fprintf(w, `{"choices":[{"message":{"content":"{\"tier\":\"%s\",\"reason\":\"test assessment\"}"}}]}`, tier)
					return
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				usage := fmt.Sprintf(`,"usage":{"input_tokens":%d,"input_tokens_details":{"cached_tokens":%d},"output_tokens":1}`, tc.input, tc.cached)
				if tc.name == "missing_usage" {
					usage = ""
				}
				fmt.Fprintf(w, `{"id":"resp_test","object":"response","status":"completed","model":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}]%s}`, body["model"], usage)
			}))
			defer upstream.Close()
			cfg := testConfig(t, upstream.URL, upstream.URL)
			cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
			c := compass.Defaults()
			c.High = "test/expensive"
			c.Low = "test/cheap"
			c.Default = c.High
			c.Router = "test/router"
			c.Rules.SessionSwitching = true
			if err := compass.Save(compass.Path(cfg.ConfigPath), c); err != nil {
				t.Fatal(err)
			}
			g, admin, _ := newGateway(t, cfg, resolvedOpenAIBaseten(t, "codex", "baseten"))
			defer admin.Close()
			catalog := `{"anthropic":{"id":"anthropic","models":{"a":{"id":"a","cost":{"input":1,"output":1}}}},"openai":{"id":"openai","models":{"o":{"id":"o","cost":{"input":1,"output":1}}}},"baseten":{"id":"baseten","models":{"test/expensive":{"id":"test/expensive","cost":{"input":20,"output":20,"cache_read":0.01},"limit":{"context":2000000}},"test/cheap":{"id":"test/cheap","cost":{"input":0.1,"output":0.1,"cache_read":0.01},"limit":{"context":2000000}},"test/router":{"id":"test/router","cost":{"input":0.1,"output":0.1},"limit":{"context":2000000}}}}}`
			if err := g.pricing.ReplaceModelsDev([]byte(catalog), time.Now(), ""); err != nil {
				t.Fatal(err)
			}

			if err := g.pricing.ReplaceBasetenCatalog([]byte(`{"data":[{"id":"test/expensive","pricing":{"prompt":0.000020,"completion":0.000020,"input_cache_read":0.00000001}},{"id":"test/cheap","pricing":{"prompt":0.0000001,"completion":0.0000001,"input_cache_read":0.00000001}},{"id":"test/router","pricing":{"prompt":0.0000001,"completion":0.0000001}}]}`), "baseten_v1_models", time.Now(), ""); err != nil {
				t.Fatal(err)
			}
			stop := start(t, g)
			defer stop()
			for turn := 0; turn < 3; turn++ {
				body, _ := json.Marshal(map[string]any{"model": compass.Model, "input": strings.Repeat("x", 100000) + fmt.Sprint(turn)})
				req, _ := http.NewRequest("POST", "http://"+g.ClientAddr("codex").String()+"/v1/responses", strings.NewReader(string(body)))
				req.Header.Set("Thread-Id", "main")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 {
					t.Fatalf("status %d: %s", resp.StatusCode, raw)
				}
				want := c.High
				if turn > 0 && tc.wantSwitch {
					want = c.Low
				}
				if got := resp.Header.Get("X-Baseten-Compass-Model"); got != want {
					t.Fatalf("turn %d routed to %s, want %s", turn, got, want)
				}
				waitForRows(t, cfg.TelemetryDir, turn+1, 2*time.Second)
			}
			pin, err := compass.LoadDecision(compass.Path(cfg.ConfigPath), compass.SessionKey("codex", "", "main"))
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantSwitch {
				if pin.Model != c.Low || !strings.Contains(pin.Reason, "Switched from") || classifications.Load() != 2 {
					t.Fatalf("switch not pinned: %+v, classifications %d", pin, classifications.Load())
				}
			} else if !strings.Contains(pin.ReviewReason, tc.wantReason) {
				t.Fatalf("missing hold reason: %+v", pin)
			}
		})
	}
}
