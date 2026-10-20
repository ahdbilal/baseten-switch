package pricing

import (
	"testing"
	"time"
)

func TestWorkspacePricesAreNativeAndReplacePriorWorkspace(t *testing.T) {
	p := NewWorkspace()
	if p.Quote(ProviderDatabricks, "zai-org/GLM-5.3").Priced {
		t.Fatal("vendor rate leaked")
	}
	if e := p.ReplaceWorkspaceModels([]string{"system.ai.glm-5-3", "system.ai.gpt-6-astra", "system.ai.custom"}, "workspace-a", time.Now()); e != nil {
		t.Fatal(e)
	}
	q := p.Quote(ProviderDatabricks, "system.ai.glm-5-3")
	if !q.Priced || q.Price.Prompt != 1.4 || q.Price.CacheRead != .26 {
		t.Fatal(q)
	}
	if p.Quote(ProviderDatabricks, "system.ai.custom").Priced {
		t.Fatal("unknown custom price guessed")
	}
	if ApplyContextRates(p.Quote(ProviderDatabricks, "system.ai.gpt-6-astra"), 128001, 0, 0, 0).Priced {
		t.Fatal("unknown long context threshold treated as known")
	}
	if e := p.ReplaceWorkspaceModels([]string{"system.ai.custom"}, "workspace-b", time.Now()); e != nil {
		t.Fatal(e)
	}
	if p.Quote(ProviderDatabricks, "system.ai.glm-5-3").Priced {
		t.Fatal("old workspace rate retained")
	}
}
