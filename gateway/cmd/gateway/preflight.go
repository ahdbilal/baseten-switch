package gateway

// Preflight checks run at startup and on SIGHUP reload. They are warn-only:
// they log actionable lines and never stop the gateway.

import (
	"errors"
	"fmt"
	"io"
	"os"

	"unity-gateway/gateway/internal/auth"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/unity"
)

// preflightConfigPath mirrors Gateway.activeConfigPath for a bare Config.
func preflightConfigPath(cfg *Config) string {
	if cfg.ConfigPath != "" {
		return cfg.ConfigPath
	}
	return config.DefaultPath()
}

// applyGlobalAuth loads gateway.yaml and applies global.auth to the
// process environment exactly like the admin config PUT path
// (applyConfigEnv), then refreshes the key fields on cfg from the
// environment. This keeps startup, SIGHUP, and admin updates consistent.
func applyGlobalAuth(cfg *Config) {
	f, err := config.Load(preflightConfigPath(cfg))
	if err != nil {
		return
	}
	applyConfigEnv(f)
	if v := os.Getenv("DATABRICKS_API_KEY"); v != "" {
		cfg.DatabricksKey = v
	}
}

// UnresolvedPlaceholders returns the ${VAR} placeholder names referenced
// by the config that are not set in the process environment. Exported
// for `unity-gateway doctor`, which runs the same detection out of process
// (and additionally consults the gateway's env file, which the gateway
// itself loads into its environment before this check runs).
func UnresolvedPlaceholders(f *config.File) []string {
	var out []string
	for _, name := range f.CollectPlaceholders() {
		if os.Getenv(name) == "" {
			out = append(out, name)
		}
	}
	return out
}

// warnUnresolvedPlaceholders logs one actionable line per unresolved
// ${VAR}, naming the variable and where to set it.
func warnUnresolvedPlaceholders(f *config.File, out io.Writer) {
	for _, name := range UnresolvedPlaceholders(f) {
		fmt.Fprintf(out,
			"[gateway] warning: gateway.yaml references ${%s} but %s is not set; it expands to empty. Fix: add %s=... to %s or export it in the gateway's environment\n",
			name, name, name, config.EnvFilePath())
	}
}

// databricksRoutedClients returns a display entry per enabled client whose
// route or fallback_route is databricks. Passthrough routes (anthropic,
// openai) are excluded: they use harness credential passthrough and need
// no gateway-side key.
func databricksRoutedClients(resolved []resolvedClientConfig) []string {
	var out []string
	for _, rc := range resolved {
		switch {
		case rc.Route == "databricks":
			out = append(out, rc.Name+" (route: databricks)")
		case rc.FallbackRoute == "databricks":
			out = append(out, rc.Name+" (fallback_route: databricks)")
		}
	}
	return out
}

// hasDatabricksCredential reports whether any Databricks credential is
// available: an enabled, non-empty environment fallback key; an OAuth
// credential loadable from the databricks CLI store; or an api_key-type CLI
// profile whose key is readable.
func hasDatabricksCredential(profile, apiKey string, apiKeyFallback bool) bool {
	if apiKeyFallback && apiKey != "" {
		return true
	}
	tok, _, err := auth.Load(profile)
	if err != nil {
		var ak *auth.APIKeyProfileError
		if errors.As(err, &ak) {
			return ak.Key != ""
		}
		return false
	}
	return tok != nil
}

// warnMissingDatabricksCreds prints a prominent banner naming the affected
// clients and the fix. Warn-only.
func warnMissingDatabricksCreds(names []string, apiKeyIgnored bool, out io.Writer) {
	if len(names) == 0 {
		return
	}
	rule := "[gateway] =============================================================="
	fmt.Fprintln(out, rule)
	fmt.Fprintln(out, "[gateway] WARNING: no Databricks credential found that the router can use.")
	fmt.Fprintln(out, "[gateway] These clients")
	fmt.Fprintln(out, "[gateway] cannot use Databricks until a credential is configured;")
	fmt.Fprintln(out, "[gateway] a configured native fallback may serve their requests instead:")
	for _, n := range names {
		fmt.Fprintf(out, "[gateway]   - %s\n", n)
	}
	if apiKeyIgnored {
		fmt.Fprintln(out, "[gateway] DATABRICKS_API_KEY is set but ignored because")
		fmt.Fprintln(out, "[gateway] UNITY_GATEWAY_API_KEY_FALLBACK is not enabled.")
	}
	fmt.Fprintln(out, "[gateway] Fix: run 'databricks auth login', or set DATABRICKS_API_KEY and")
	fmt.Fprintf(out, "[gateway] UNITY_GATEWAY_API_KEY_FALLBACK=1 in %s.\n", config.EnvFilePath())
	fmt.Fprintln(out, rule)
}

// runPreflight runs the warn-only checks: unresolved ${VAR} placeholders
// against the process environment, and databricks-routed clients without a
// usable credential. Never fatal.
func runPreflight(cfg *Config, resolved []resolvedClientConfig, out io.Writer) {
	if c, err := unity.Load(preflightConfigPath(cfg)); err != nil {
		fmt.Fprintln(out, "[gateway] Unity Gateway connection is invalid:", err)
		return
	} else if c != nil {
		return
	}

	if f, err := config.Load(preflightConfigPath(cfg)); err == nil {
		warnUnresolvedPlaceholders(f, out)
	}
	names := databricksRoutedClients(resolved)
	if len(names) == 0 {
		return
	}
	savedKey, savedErr := auth.LoadSavedAPIKey(cfg.ConfigPath)
	if savedErr != nil {
		fmt.Fprintln(out, "[gateway] WARNING: saved API key is unavailable; remove or replace it in Switch settings")
		return
	}
	if savedKey != "" || hasDatabricksCredential(cfg.OAuthProfile, cfg.DatabricksKey, cfg.APIKeyFallback) {
		return
	}
	warnMissingDatabricksCreds(names, cfg.DatabricksKey != "" && !cfg.APIKeyFallback, out)
}
