package pricing

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// WorkspaceListSource is an estimate at the published $0.07/DBU reference
// price, standard serving and global processing. It is not an invoice quote.
// Sources reviewed 2026-10-20:
// https://www.databricks.com/product/pricing/foundation-model-serving
// https://www.databricks.com/product/pricing/proprietary-foundation-model-serving
const WorkspaceListSource = "databricks_list_estimate_2026-10-20"

// ReplaceWorkspaceModels publishes a workspace inventory and reference prices
// atomically. No vendor-name aliases, external-provider prices, or persisted
// prices from another workspace enter this catalog.
func (p *Pricing) ReplaceWorkspaceModels(ids []string, workspace string, at time.Time) error {
	if len(ids) == 0 || workspace == "" {
		return fmt.Errorf("workspace inventory is empty")
	}
	candidate := databricksCandidate{prices: map[string]Price{}, ratePresence: map[string]RatePresence{}, models: append([]string(nil), ids...), source: WorkspaceListSource, revision: workspace + ":" + at.UTC().Format(time.RFC3339Nano), fetchedAt: at}
	for _, id := range ids {
		var price Price
		switch id {
		case "system.ai.glm-5-3", "system.ai.glm-5-2":
			price = Price{Prompt: 1.4, Completion: 4.4, CacheRead: .26}
		case "system.ai.glm-5-3-flash":
			price = Price{Prompt: .15, Completion: .5, CacheRead: .03}
		case "system.ai.kimi-k3":
			price = Price{Prompt: 3, Completion: 15, CacheRead: .3}
		case "system.ai.gpt-6-astra":
			price = Price{Prompt: 10, Completion: 50, CacheRead: 1, CacheWrite5m: 12.5}
		case "system.ai.gpt-6-1-sol":
			price = Price{Prompt: 2, Completion: 10, CacheRead: .1, CacheWrite5m: 2.5}
		default:
			continue
		}
		candidate.prices[id] = price
		candidate.ratePresence[id] = RatePresence{Input: true, Output: true, CacheRead: true, CacheWrite5m: price.CacheWrite5m > 0}
	}
	provenance := Provenance{Source: WorkspaceListSource, LoadedFrom: LoadedFromLive, Revision: candidate.revision, CapturedAt: at}
	catalog := databricksProviderCatalog(candidate, provenance, "Databricks reference list-price estimate; $0.07/DBU, standard/global", true)
	for id, r := range catalog.models {
		r.ContextTokens = 128000
		r.MaxOutputTokens = 16384
		catalog.models[id] = r
	}
	p.publishMu.Lock()
	defer p.publishMu.Unlock()
	next := cloneSnapshotWithLayers(p.current.Load(), cloneProviderLayers(p.current.Load().providerLayers))
	for k := range next.providerLayers {
		if k.provider == ProviderDatabricks {
			delete(next.providerLayers, k)
		}
	}
	next.providerLayers[providerLayerKey{provider: ProviderDatabricks, loadedFrom: LoadedFromLive, source: WorkspaceListSource}] = catalog
	next.providerCatalogs = activeProviderCatalogsForSnapshot(next)
	p.publishLocked(next)
	return nil
}

func workspaceContextQuote(q Quote, input, read, write5, write1 int64) Quote {
	// The published short/long table does not specify every model's threshold.
	// Never extend the short-context estimate beyond a conservative 128k bound.
	if strings.HasPrefix(q.Source, WorkspaceListSource) && q.Price.CacheWrite5m > 0 && float64(input)+float64(read)+float64(write5)+float64(write1) > 128000 {
		q.Priced = false
	}
	if math.IsNaN(q.Price.Prompt) {
		q.Priced = false
	}
	return q
}
