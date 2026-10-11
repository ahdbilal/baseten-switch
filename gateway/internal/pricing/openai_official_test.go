package pricing

import "testing"

func TestOfficialOpenAIQuotesAndContextBoundary(t *testing.T) {
	p := New()
	cases := []struct {
		model          string
		in, out, cache float64
	}{{"gpt-6-astra", 10, 50, 1}, {"gpt-6.1-sol", 2, 10, .1}, {"gpt-6-luna", .1, .5, .01}}
	for _, tt := range cases {
		q := p.Quote(ProviderOpenAI, tt.model)
		if !q.Priced || q.Price.Prompt != tt.in || q.Price.Completion != tt.out || q.Price.CacheRead != tt.cache || q.Source != OpenAIOfficialSource {
			t.Fatalf("quote %+v", q)
		}
		short := ApplyContextRates(q, 200000, 72000, 0, 0)
		long := ApplyContextRates(q, 200000, 72001, 0, 0)
		if short.Price.Prompt != tt.in || long.Price.Prompt != 2*tt.in || long.Price.Completion != 1.5*tt.out {
			t.Fatal("wrong context boundary")
		}
		fast := p.QuoteProfile(ProviderOpenAI, tt.model, ProfileFast)
		if fast.Price.Prompt != 2*tt.in {
			t.Fatal("fast multiplier missing")
		}
	}
	if p.Quote(ProviderBaseten, "gpt-6-astra").Priced || p.Quote(ProviderOpenAI, "gpt-6-astra-fake").Priced {
		t.Fatal("overbroad price matching")
	}
}
