package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"unity-gateway/gateway/internal/pricing"
)

func TestPortkeyRefreshCacheAndFailure(t *testing.T) {
	failing := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
			t.Error("credentials sent to public pricing endpoint")
		}
		if failing {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"exact-model":{"pricing_config":{"pay_as_you_go":{"request_token":{"price":0.0002},"response_token":{"price":0.001}}}}}`)
	}))
	defer server.Close()
	old := portkeyCatalogBaseURL
	portkeyCatalogBaseURL = server.URL
	defer func() { portkeyCatalogBaseURL = old }()
	cfg := testConfig(t, server.URL, server.URL)
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	g, admin, _ := newGateway(t, cfg, resolvedOpenAIDatabricks(t, "codex", "databricks"))
	defer admin.Close()
	g.refreshPortkeyCatalogs(context.Background())
	q := g.pricing.Quote(pricing.ProviderOpenAI, "exact-model")
	if !q.Priced || q.Price.Prompt != 2 {
		t.Fatalf("refresh: %+v", q)
	}
	restored := pricing.New()
	loadPortkeyCaches(restored, cfg.ConfigPath)
	cached := restored.Quote(pricing.ProviderOpenAI, "exact-model")
	if !cached.Priced || (!cached.CapturedAt.Equal(q.CapturedAt) || cached.Revision != q.Revision) {
		t.Fatal("cache lost original provenance")
	}
	failing = true
	g.refreshPortkeyCatalogs(context.Background())
	if !g.pricing.Quote(pricing.ProviderOpenAI, "exact-model").CapturedAt.Equal(q.CapturedAt) {
		t.Fatal("failed refresh changed capture time")
	}
	if time.Since(q.CapturedAt) > time.Minute {
		t.Fatal("wrong capture date")
	}
}
