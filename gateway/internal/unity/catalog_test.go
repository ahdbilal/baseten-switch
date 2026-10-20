package unity

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWorkspaceCatalogPaginationAndCompatibility(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing workspace auth")
		}
		if r.URL.Query().Get("page_token") == "next" {
			fmt.Fprint(w, `{"model_services":[{"name":"model-services/system.ai.claude","supported_api_types":["anthropic/v1/messages"]}]}`)
			return
		}
		fmt.Fprint(w, `{"model_services":[{"name":"model-services/system.ai.gpt","supported_api_types":["codex/v1/responses","anthropic/v1/messages","mlflow/v1/chat/completions"]}],"next_page_token":"next"}`)
	}))
	defer s.Close()
	old := http.DefaultTransport
	http.DefaultTransport = s.Client().Transport
	defer func() { http.DefaultTransport = old }()
	c := Connection{s.URL, "fixture"}
	tokens.Lock()
	if tokens.values == nil {
		tokens.values = map[Connection]struct {
			token string
			until time.Time
		}{}
	}
	tokens.values[c] = struct {
		token string
		until time.Time
	}{"fixture", time.Now().Add(time.Minute)}
	tokens.Unlock()
	codex, e := c.ModelsFor(context.Background(), "responses")
	if e != nil || len(codex) != 1 || codex[0].ID != "system.ai.gpt" {
		t.Fatal(codex, e)
	}
	claude, e := c.ModelsFor(context.Background(), "messages")
	if e != nil || len(claude) != 2 {
		t.Fatal(claude, e)
	}
	if calls != 2 {
		t.Fatal("pagination/cache", calls)
	}
	if _, e = c.ModelsFor(context.Background(), "unknown"); e == nil {
		t.Fatal("unknown API accepted")
	}
}
