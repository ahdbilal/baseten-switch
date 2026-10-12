package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

const PortkeySource = "portkey_pricing"
const PortkeyURL = "https://configs.portkey.ai/pricing"

type portkeyRate struct {
	Price *float64 `json:"price"`
}
type portkeyRates struct {
	Input      portkeyRate            `json:"request_token"`
	Output     portkeyRate            `json:"response_token"`
	Read       portkeyRate            `json:"cache_read_input_token"`
	Write      portkeyRate            `json:"cache_write_input_token"`
	Additional map[string]portkeyRate `json:"additional_units"`
}
type portkeyConfig struct {
	Rates    portkeyRates `json:"pay_as_you_go"`
	Currency string       `json:"currency"`
}
type portkeyMode struct {
	Config portkeyConfig `json:"pricing_config"`
	Tiers  map[string]struct {
		Config portkeyConfig `json:"pricing_config"`
	} `json:"context_tiers"`
}
type portkeyModel struct {
	Config portkeyConfig `json:"pricing_config"`
	Custom struct {
		Thresholds map[string]string `json:"context_tier_map"`
		Regions    map[string]struct {
			Modes map[string]portkeyMode `json:"execution_modes"`
		} `json:"regions"`
	} `json:"custom_pricing"`
}
type portkeyTier struct {
	after    int64
	price    Price
	presence RatePresence
}
type portkeyCatalog map[string]map[ExecutionProfile]Quote

// ReplacePortkey publishes exact, provider-scoped fallback prices. It supplies
// neither availability nor aliases; the special "default" record is never used.
func (p *Pricing) ReplacePortkey(provider string, body []byte, at time.Time) error {
	return p.replacePortkey(provider, body, at, LoadedFromLive)
}

func (p *Pricing) RestorePortkey(provider string, body []byte, at time.Time) error {
	return p.replacePortkey(provider, body, at, LoadedFromRuntimeCache)
}

func (p *Pricing) replacePortkey(provider string, body []byte, at time.Time, loadedFrom LoadedFrom) error {
	if provider != ProviderOpenAI && provider != ProviderAnthropic {
		return fmt.Errorf("unsupported Portkey provider %q", provider)
	}
	if at.IsZero() {
		return fmt.Errorf("Portkey capture time required")
	}
	var records map[string]portkeyModel
	if err := json.Unmarshal(body, &records); err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	revision := "sha256:" + hex.EncodeToString(hash[:])
	catalog := portkeyCatalog{}
	for model, record := range records {
		if model == "" || model == "default" || (record.Config.Currency != "" && record.Config.Currency != "USD") {
			continue
		}
		profiles := map[ExecutionProfile]Quote{}
		region, hasDefault := record.Custom.Regions["default"]
		if len(record.Custom.Regions) > 0 && (!hasDefault || len(region.Modes) == 0) {
			continue
		}
		modes := region.Modes
		for _, profile := range []ExecutionProfile{ProfileStandard, ProfileFast} {
			mode, exists := modes[string(profile)]
			if !exists {
				if profile != ProfileStandard || len(modes) > 0 || len(record.Custom.Thresholds) > 0 {
					continue
				}
				mode.Config = record.Config
			}
			var tiers []portkeyTier
			valid := true
			if len(mode.Tiers) > 0 {
				if len(record.Custom.Thresholds) == 0 {
					continue
				}
				for threshold, name := range record.Custom.Thresholds {
					n, err := strconv.ParseInt(threshold, 10, 64)
					tier, ok := mode.Tiers[name]
					if err != nil || n < 0 || !ok {
						valid = false
						break
					}
					price, presence, ok := portkeyPrice(tier.Config)
					if !ok {
						valid = false
						break
					}
					tiers = append(tiers, portkeyTier{n, price, presence})
				}
				sort.Slice(tiers, func(i, j int) bool { return tiers[i].after < tiers[j].after })
				if len(tiers) == 0 || tiers[0].after != 0 {
					valid = false
				}
			} else {
				price, presence, ok := portkeyPrice(mode.Config)
				valid = ok
				tiers = []portkeyTier{{0, price, presence}}
			}
			if !valid {
				continue
			}
			profiles[profile] = Quote{Price: tiers[0].price, RatePresence: tiers[0].presence, RatePresenceKnown: true, RateProvenance: rateProvenanceForPresence(tiers[0].presence, Provenance{Source: PortkeySource, LoadedFrom: loadedFrom, Revision: revision, CapturedAt: at.UTC()}), Priced: true, Source: PortkeySource, Revision: revision, CapturedAt: at.UTC(), ExecutionProfile: profile, contextTiers: &tiers}
		}
		if len(profiles) > 0 {
			catalog[model] = profiles
		}
	}
	if len(catalog) == 0 {
		return fmt.Errorf("Portkey %s has no supported token prices", provider)
	}
	p.publishMu.Lock()
	defer p.publishMu.Unlock()
	current := p.current.Load()
	next := cloneSnapshotWithLayers(current, cloneProviderLayers(current.providerLayers))
	next.portkey = make(map[string]portkeyCatalog, len(current.portkey)+1)
	for k, v := range current.portkey {
		next.portkey[k] = v
	}
	next.portkey[provider] = catalog
	p.publishLocked(next)
	return nil
}

func portkeyPrice(c portkeyConfig) (Price, RatePresence, bool) {
	if c.Currency != "" && c.Currency != "USD" {
		return Price{}, RatePresence{}, false
	}
	rates := []*float64{c.Rates.Input.Price, c.Rates.Output.Price, c.Rates.Read.Price, c.Rates.Write.Price, c.Rates.Additional["cache_write_1h"].Price}
	if rates[0] == nil || rates[1] == nil {
		return Price{}, RatePresence{}, false
	}
	var values [5]float64
	for i, v := range rates {
		if v == nil {
			continue
		}
		// Portkey documents cents/token; our quote uses USD/million tokens.
		n := *v * 10000
		if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return Price{}, RatePresence{}, false
		}
		if _, err := nanoUSDPerToken(n); err != nil {
			return Price{}, RatePresence{}, false
		}
		values[i] = n
	}
	return Price{Prompt: values[0], Completion: values[1], CacheRead: values[2], CacheWrite5m: values[3], CacheWrite1h: values[4]}, RatePresence{Input: true, Output: true, CacheRead: rates[2] != nil, CacheWrite5m: rates[3] != nil, CacheWrite1h: rates[4] != nil}, true
}

func (s *Snapshot) fallbackQuote(provider, model string, profile ExecutionProfile) Quote {
	if q, ok := s.portkey[provider][model][profile]; ok {
		return q
	}
	return officialOpenAIQuote(provider, model, profile)
}
