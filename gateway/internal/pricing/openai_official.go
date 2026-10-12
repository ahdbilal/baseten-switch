package pricing

import "time"

// Reviewed against https://developers.openai.com/api/docs/pricing and the exact
// model pages on 2026-10-11. Only fills missing quotes; never advertises access.
const OpenAIOfficialSource = "openai_official_pricing_2026-10-11"
const OpenAIOfficialURL = "https://developers.openai.com/api/docs/pricing"

func officialOpenAIQuote(provider, model string, profile ExecutionProfile) Quote {
	if provider != ProviderOpenAI {
		return Quote{ExecutionProfile: profile}
	}
	var price Price
	switch model {
	case "gpt-6-astra":
		price = Price{Prompt: 10, Completion: 50, CacheRead: 1, CacheWrite5m: 12.5}
	case "gpt-6.1-sol":
		price = Price{Prompt: 2, Completion: 10, CacheRead: 0.1, CacheWrite5m: 2.5}
	case "gpt-6-luna":
		price = Price{Prompt: 0.1, Completion: 0.5, CacheRead: 0.01, CacheWrite5m: 0.125}
	default:
		return Quote{ExecutionProfile: profile}
	}
	switch profile {
	case ProfileStandard, "":
	case ProfileFast:
		price.Prompt *= 2
		price.Completion *= 2
		price.CacheRead *= 2
		price.CacheWrite5m *= 2
	default:
		return Quote{ExecutionProfile: profile}
	}
	return Quote{Price: price, Priced: true, RatePresenceKnown: true, RatePresence: RatePresence{Input: true, Output: true, CacheRead: true, CacheWrite5m: true}, Source: OpenAIOfficialSource, Revision: OpenAIOfficialSource, CapturedAt: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), ExecutionProfile: profile}
}

// ApplyContextRates applies captured Portkey context tiers or the reviewed
// OpenAI full-request surcharge. Ordinary catalog quotes remain unchanged.
func ApplyContextRates(q Quote, input, cacheRead, cacheWrite5m, cacheWrite1h int64) Quote {
	if q.Source == PortkeySource && q.contextTiers != nil {
		total := float64(input) + float64(cacheRead) + float64(cacheWrite5m) + float64(cacheWrite1h)
		for _, tier := range *q.contextTiers {
			if tier.after == 0 || total > float64(tier.after) {
				q.Price, q.RatePresence = tier.price, tier.presence
				q.RateProvenance = rateProvenanceForPresence(tier.presence, q.RateProvenance.Input)
			}
		}
		return q
	}
	if q.Source == OpenAIOfficialSource && float64(input)+float64(cacheRead)+float64(cacheWrite5m)+float64(cacheWrite1h) > 272000 {
		q.Price.Prompt *= 2
		q.Price.CacheRead *= 2
		q.Price.CacheWrite5m *= 2
		q.Price.Completion *= 1.5
	}
	return q
}
