package pricing

import (
	"context"
	"math"
	"net/http"
	"os"
	"testing"
	"time"
)

const portkeyFixture = `{
  "default": {
    "pricing_config": {
      "pay_as_you_go": {
        "request_token": {
          "price": 0
        },
        "response_token": {
          "price": 0
        }
      }
    }
  },
  "test-model": {
    "pricing_config": {
      "pay_as_you_go": {
        "request_token": {
          "price": 0.0002
        },
        "response_token": {
          "price": 0.001
        },
        "cache_read_input_token": {
          "price": 1e-05
        },
        "cache_write_input_token": {
          "price": 0.00025
        },
        "additional_units": {
          "cache_write_1h": {
            "price": 0.0004
          }
        }
      }
    }
  },
  "free-model": {
    "pricing_config": {
      "pay_as_you_go": {
        "request_token": {
          "price": 0
        },
        "response_token": {
          "price": 0
        }
      }
    }
  },
  "bad-model": {
    "pricing_config": {
      "pay_as_you_go": {
        "request_token": {
          "price": -1
        },
        "response_token": {
          "price": 1
        }
      }
    }
  },
  "euro-model": {
    "pricing_config": {
      "currency": "EUR",
      "pay_as_you_go": {
        "request_token": {
          "price": 1
        },
        "response_token": {
          "price": 1
        }
      }
    }
  },
  "tier-model": {
    "custom_pricing": {
      "context_tier_map": {
        "0": "short",
        "100": "long"
      },
      "regions": {
        "default": {
          "execution_modes": {
            "standard": {
              "context_tiers": {
                "short": {
                  "pricing_config": {
                    "pay_as_you_go": {
                      "request_token": {
                        "price": 0.0002
                      },
                      "response_token": {
                        "price": 0.001
                      }
                    }
                  }
                },
                "long": {
                  "pricing_config": {
                    "pay_as_you_go": {
                      "request_token": {
                        "price": 0.0004
                      },
                      "response_token": {
                        "price": 0.0015
                      }
                    }
                  }
                }
              }
            },
            "fast": {
              "pricing_config": {
                "pay_as_you_go": {
                  "request_token": {
                    "price": 0.0006
                  },
                  "response_token": {
                    "price": 0.003
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

func TestPortkeyUnitsPresenceContextAndIsolation(t *testing.T) {
	p := New()
	before := p.Capture()
	at := time.Now().UTC()
	if err := p.ReplacePortkey(ProviderOpenAI, []byte(portkeyFixture), at); err != nil {
		t.Fatal(err)
	}
	q := p.Quote(ProviderOpenAI, "test-model")
	if !q.Priced || q.Source != PortkeySource || q.Price.Prompt != 2 || q.Price.Completion != 10 || q.Price.CacheRead != .1 || q.Price.CacheWrite5m != 2.5 || q.Price.CacheWrite1h != 4 {
		t.Fatalf("conversion: %+v", q)
	}
	if !q.HasRatesForUsage(1, 1, 1, 1, 1) {
		t.Fatal("lost cache rates")
	}
	if before.Quote(ProviderOpenAI, "test-model").Priced || p.Quote(ProviderAnthropic, "test-model").Priced {
		t.Fatal("snapshot/provider leak")
	}
	for _, model := range []string{"unknown", "default", "bad-model", "euro-model", "test-model-2026"} {
		if p.Quote(ProviderOpenAI, model).Priced {
			t.Fatalf("invented price: %s", model)
		}
	}
	free := p.Quote(ProviderOpenAI, "free-model")
	if !free.Priced || free.HasRatesForUsage(1, 1, 1, 0, 0) {
		t.Fatal("zero/missing confusion")
	}
	tier := p.Quote(ProviderOpenAI, "tier-model")
	if ApplyContextRates(tier, 100, 0, 0, 0).Price.Prompt != 2 || ApplyContextRates(tier, 100, 1, 0, 0).Price.Prompt != 4 {
		t.Fatal("context boundary")
	}
	long := ApplyContextRates(tier, 101, 0, 0, 0)
	if ApplyContextRates(long, 101, 0, 0, 0).Price.Prompt != 4 {
		t.Fatal("context rates compound")
	}
	if math.Abs(p.QuoteProfile(ProviderOpenAI, "tier-model", ProfileFast).Price.Prompt-6) > 1e-9 || p.QuoteProfile(ProviderOpenAI, "test-model", ProfileFast).Priced {
		t.Fatal("invented fast rate")
	}
	if _, exists := p.Capture().Model(ProviderOpenAI, "test-model"); exists {
		t.Fatal("price advertised model availability")
	}
	if err := p.ReplacePortkey(ProviderOpenAI, []byte(`{}`), at); err == nil || !p.Quote(ProviderOpenAI, "test-model").Priced {
		t.Fatal("invalid feed replaced good prices")
	}
	if err := p.ReplaceModelsDev([]byte(`{"openai":{"id":"openai","models":{"test-model":{"id":"test-model","cost":{"input":9,"output":10}}}},"anthropic":{"id":"anthropic","models":{"a":{"id":"a","cost":{"input":1,"output":2}}}},"databricks":{"id":"databricks","models":{"b":{"id":"b","cost":{"input":1,"output":2}}}}}`), at, ""); err != nil {
		t.Fatal(err)
	}
	if p.Quote(ProviderOpenAI, "test-model").Price.Prompt != 9 {
		t.Fatal("Portkey replaced primary catalog")
	}
	if !p.Quote(ProviderOpenAI, "free-model").Priced {
		t.Fatal("catalog publication lost Portkey fallback")
	}
}

// Opt-in contract test against the credential-free public feed.
func TestPortkeyLiveFeeds(t *testing.T) {
	if os.Getenv("UNITY_GATEWAY_TEST_PORTKEY_LIVE") != "1" {
		t.Skip("live API test is opt-in")
	}
	p := New()
	for _, provider := range []string{ProviderOpenAI, ProviderAnthropic} {
		ctx, cancel := context.WithTimeout(context.Background(), ModelsDevFetchTimeout)
		result, err := FetchModelsDev(ctx, &http.Client{}, PortkeyURL+"/"+provider+".json", "", ModelsDevMaxResponseBytes)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if err = p.ReplacePortkey(provider, result.Body, time.Now()); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d exact model prices", provider, len(p.Capture().portkey[provider]))
	}
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol", "gpt-5.6-terra"} {
		q := p.Quote(ProviderOpenAI, model)
		if q.Source != PortkeySource {
			t.Fatalf("missing live quote for %s", model)
		}
		t.Logf("%s USD/M: input=%g output=%g cache=%g", model, q.Price.Prompt, q.Price.Completion, q.Price.CacheRead)
	}
}
