package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
)

var portkeyCatalogBaseURL = pricing.PortkeyURL
var portkeyProviders = []string{pricing.ProviderOpenAI, pricing.ProviderAnthropic}

type portkeyCache struct {
	Version    int       `json:"version"`
	CapturedAt time.Time `json:"captured_at"`
	Body       []byte    `json:"body"`
}

func portkeyCachePath(configPath, provider string) string {
	return filepath.Join(providerCatalogCacheDir(configPath), provider+"-portkey.json")
}

// Public bulk requests contain no prompts, account IDs, or provider credentials.
// Failure preserves the last validated snapshot and its original capture time.
func (g *Gateway) refreshPortkeyCatalogs(parent context.Context) {
	if portkeyCatalogBaseURL == "" {
		return
	}
	for _, provider := range portkeyProviders {
		if parent.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(parent, pricing.ModelsDevFetchTimeout)
		result, err := pricing.FetchModelsDev(ctx, &http.Client{}, portkeyCatalogBaseURL+"/"+provider+".json", "", pricing.ModelsDevMaxResponseBytes)
		cancel()
		at := time.Now().UTC()
		if err == nil {
			err = g.pricing.ReplacePortkey(provider, result.Body, at)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[gateway] Portkey %s pricing refresh failed: %v; retaining prior prices\n", provider, err)
			continue
		}
		raw, _ := json.Marshal(portkeyCache{Version: 1, CapturedAt: at, Body: result.Body})
		path := portkeyCachePath(g.activeConfigPath(), provider)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err == nil {
			err = writePrivateAtomic(path, raw)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[gateway] Portkey pricing cache: %v\n", err)
		}
	}
}
func loadPortkeyCaches(p *pricing.Pricing, configPath string) {
	for _, provider := range portkeyProviders {
		raw, err := readProviderCatalogCache(portkeyCachePath(configPath, provider))
		if os.IsNotExist(err) {
			continue
		}
		if err == nil {
			var cache portkeyCache
			err = json.Unmarshal(raw, &cache)
			if err == nil && (cache.Version != 1 || cache.CapturedAt.IsZero() || cache.CapturedAt.After(time.Now().Add(time.Minute))) {
				err = fmt.Errorf("invalid Portkey cache metadata")
			}
			if err == nil {
				err = p.RestorePortkey(provider, cache.Body, cache.CapturedAt)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[gateway] ignored Portkey %s cache: %v\n", provider, err)
		}
	}
}
