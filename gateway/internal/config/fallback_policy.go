package config

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// DefaultClaudeNativeFallbackModel is written explicitly for new and
	// migrated Claude Code configurations. It is not used as a hidden runtime
	// default, so a later binary cannot silently change installed intent.
	DefaultClaudeNativeFallbackModel = "claude-opus-5"

	// ManagedCodexCompatibilityModel is a Databricks routing sentinel, never a
	// protocol-native model. It is repeated here so config validation does not
	// depend on the gateway command package.
	ManagedCodexCompatibilityModel = "unity-gateway-compat-v1"
)

// FallbackPolicy is the optional raw global status policy. Pointer booleans
// preserve omission, which resolves to true independently for each trigger.
type FallbackPolicy struct {
	OnDatabricks429 *bool `yaml:"on_databricks_429,omitempty" json:"on_databricks_429,omitempty"`
	OnDatabricks5xx *bool `yaml:"on_databricks_5xx,omitempty" json:"on_databricks_5xx,omitempty"`
}

// ResolvedFallbackPolicy is the immutable effective status policy carried by
// the router and projected through the administration status endpoint.
type ResolvedFallbackPolicy struct {
	OnDatabricks429 bool `json:"on_databricks_429"`
	OnDatabricks5xx bool `json:"on_databricks_5xx"`
}

// ResolveFallbackPolicy applies the default-on contract independently to an
// absent block and absent child fields.
func ResolveFallbackPolicy(raw *FallbackPolicy) ResolvedFallbackPolicy {
	resolved := ResolvedFallbackPolicy{OnDatabricks429: true, OnDatabricks5xx: true}
	if raw == nil {
		return resolved
	}
	if raw.OnDatabricks429 != nil {
		resolved.OnDatabricks429 = *raw.OnDatabricks429
	}
	if raw.OnDatabricks5xx != nil {
		resolved.OnDatabricks5xx = *raw.OnDatabricks5xx
	}
	return resolved
}

var anthropicNativeModelID = regexp.MustCompile(`^claude-([0-9]|opus|sonnet|haiku|instant|fable|mythos)[A-Za-z0-9._-]*$`)

// IsProtocolNativeModel reports whether a model identifier is safe to send to
// the protocol-native provider. Anthropic has a stable namespace. OpenAI's
// namespace is intentionally open, so validation excludes known Databricks and
// managed identities instead of maintaining a stale closed catalog.
func IsProtocolNativeModel(protocolShape, model string) bool {
	if model == "" || model != strings.TrimSpace(model) || strings.Contains(model, "/") {
		return false
	}
	if model == ManagedCodexCompatibilityModel || inGatewayAliasNamespace(model) {
		return false
	}
	if protocolShape == "anthropic" {
		return anthropicNativeModelID.MatchString(model)
	}
	return protocolShape == "openai"
}

func inGatewayAliasNamespace(model string) bool {
	return strings.HasPrefix(model, "claude-databricks-") ||
		strings.HasPrefix(model, "anthropic-databricks-")
}

// ValidateNativeFallbackModel validates the optional client catch-all target.
func ValidateNativeFallbackModel(client Client) error {
	model := client.NativeFallbackModel
	if model == "" {
		return nil
	}
	if client.FallbackRoute == "" {
		return fmt.Errorf("routing policy: client %q native_fallback_model requires fallback_route", client.Name)
	}
	if client.ProtocolShape != "anthropic" {
		return fmt.Errorf("routing policy: client %q native_fallback_model is supported only for anthropic-shape clients", client.Name)
	}
	if model != strings.TrimSpace(model) {
		return fmt.Errorf("routing policy: client %q native_fallback_model %q must not contain surrounding whitespace", client.Name, model)
	}
	if _, alias := client.ModelAliases[model]; alias {
		return fmt.Errorf("routing policy: client %q native_fallback_model %q is a configured Databricks alias, not a native model", client.Name, model)
	}
	if !IsProtocolNativeModel(client.ProtocolShape, model) {
		return fmt.Errorf("routing policy: client %q native_fallback_model %q must be a full %s-native model ID", client.Name, model, client.ProtocolShape)
	}
	return nil
}
