package unity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectionValidationAndPersistence(t *testing.T) {
	for _, host := range []string{"http://example.com", "https://user:secret@example.com", "https://example.com/path", "https://example.com?x=1", "https://example.com#x"} {
		if (Connection{host, "demo"}).Validate() == nil {
			t.Fatalf("accepted %s", host)
		}
	}
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	c := Connection{"https://workspace.cloud.databricks.com", "demo"}
	if e := Save(path, c); e != nil {
		t.Fatal(e)
	}
	got, e := Load(path)
	if e != nil || *got != c {
		t.Fatal(got, e)
	}
	info, _ := os.Stat(Path(path))
	if info.Mode().Perm() != 0600 {
		t.Fatal("connection file must be private")
	}
	if c.Endpoint("responses") != "https://workspace.cloud.databricks.com/ai-gateway/codex/v1/responses" {
		t.Fatal("wrong Responses endpoint")
	}
}
func TestTransportStripsForeignCredentialsAndRejectsOtherOrigin(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer workspace-token" {
			t.Error("wrong workspace credential")
		}
		for _, h := range []string{"X-Api-Key", "ChatGPT-Account-Id", "Cookie", "Proxy-Authorization"} {
			if r.Header.Get(h) != "" {
				t.Errorf("leaked %s", h)
			}
		}
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != "request" {
			t.Error("body changed")
		}
		w.WriteHeader(200)
	}))
	defer s.Close()
	tr := Transport{Connection: Connection{s.URL, "demo"}, Base: s.Client().Transport, Token: func(context.Context) (string, error) { return "workspace-token", nil }}
	r, _ := http.NewRequest("POST", s.URL+"/ai-gateway/codex/v1/responses", strings.NewReader("request"))
	for _, h := range []string{"Authorization", "X-Api-Key", "ChatGPT-Account-Id", "Cookie", "Proxy-Authorization"} {
		r.Header.Set(h, "foreign-secret")
	}
	resp, e := tr.RoundTrip(r)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	r, _ = http.NewRequest("POST", "https://another.example/responses", nil)
	if _, e = tr.RoundTrip(r); e == nil {
		t.Fatal("forwarded to different origin")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
