package compass

// Policies are complete snapshots. Missing harness policies inherit the legacy
// shared values, so upgrading does not change any existing routing decisions.
type Policy struct {
	Enabled *bool  `json:"enabled,omitempty"`
	Router  string `json:"router"`
	Default string `json:"default"`
	High    string `json:"high"`
	MidHigh string `json:"mid_high"`
	Mid     string `json:"mid"`
	Low     string `json:"low"`
	Rules   Rules  `json:"rules"`
}

func Harness(client string) string {
	switch client {
	case "codex", "codex-desktop":
		return "codex"
	case "claude-code", "claude-desktop":
		return "claude"
	case "pi":
		return "pi"
	default:
		return ""
	}
}

func (c Config) Policy() Policy {
	return Policy{c.Enabled, c.Router, c.Default, c.High, c.MidHigh, c.Mid, c.Low, c.Rules}
}
func (p Policy) Config() Config {
	return Config{Enabled: p.Enabled, Router: p.Router, Default: p.Default, High: p.High, MidHigh: p.MidHigh, Mid: p.Mid, Low: p.Low, Rules: p.Rules}
}
func (c Config) HarnessPolicy(harness string) Policy {
	if p, ok := c.Harnesses[harness]; ok {
		return p
	}
	p := c.Policy()
	p.Enabled = nil // master power is independent of the harness switch
	return p
}
func (c Config) ClientPolicy(client string) Policy {
	if p, ok := c.Overrides[client]; ok {
		return p
	}
	return c.HarnessPolicy(Harness(client))
}
func (c Config) ForClient(client string) Config {
	p := c.ClientPolicy(client)
	resolved := p.Config()
	enabled := c.IsEnabled() && resolved.IsEnabled()
	resolved.Enabled = &enabled
	resolved.ClaudeModel, resolved.ManualModels, resolved.Comparison = c.ClaudeModel, c.ManualModels, c.Comparison
	return resolved
}
