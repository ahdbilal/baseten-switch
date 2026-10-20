// Package migration is the only runtime component that recognizes the previous
// product identity. Originals are retained; credentials and price caches are not
// imported. Migration is explicit, atomic, and refuses to overwrite new state.
package migration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/unity"
)

func Run(home string) (string, error) {
	old := filepath.Join(home, ".config", "baseten-switch")
	dest := filepath.Join(home, ".config", "unity-gateway")
	if _, e := os.Stat(filepath.Join(dest, "gateway.yaml")); e == nil {
		return "Unity Gateway configuration already exists; nothing overwritten.", nil
	} else if !os.IsNotExist(e) {
		return "", e
	}
	raw, e := os.ReadFile(filepath.Join(old, "gateway.yaml"))
	if e != nil {
		return "", fmt.Errorf("previous configuration unavailable: %w", e)
	}
	connection, e := unity.Load(filepath.Join(old, "gateway.yaml"))
	if e != nil || connection == nil {
		return "", fmt.Errorf("connect the previous app to a Databricks workspace before migrating")
	}
	// Convert only application-owned configuration, never arbitrary credentials.
	replace := strings.NewReplacer("baseten-switch", "unity-gateway", "baseten", "databricks", "BASETEN_SWITCH", "UNITY_GATEWAY", "BASETEN_API_KEY", "DATABRICKS_API_KEY", "zai-org/GLM-5.3-Fast", "system.ai.glm-5-3-flash", "zai-org/GLM-5.3", "system.ai.glm-5-3", "zai-org/GLM-5.2", "system.ai.glm-5-2", "moonshotai/Kimi-K3", "system.ai.kimi-k3", "moonshotai/Kimi-K2.7-Code", "system.ai.kimi-k2-7-code", "gpt-6-astra", "system.ai.gpt-6-astra", "gpt-6.1-sol", "system.ai.gpt-6-1-sol")
	// Preserve existing fully-qualified IDs (replacement is otherwise not idempotent).
	text := replace.Replace(string(raw))
	text = strings.ReplaceAll(text, "system.ai.system.ai.", "system.ai.")
	var f config.File
	if e = config.UnmarshalStrict([]byte(text), &f); e != nil {
		return "", e
	}
	if e = config.ValidateRoutingPolicy(&f); e != nil {
		return "", e
	}
	if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return "", e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(dest), ".unity-migration-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(tmp)
	if e = os.WriteFile(filepath.Join(tmp, "gateway.yaml"), []byte(text), 0600); e != nil {
		return "", e
	}
	if e = unity.Save(filepath.Join(tmp, "gateway.yaml"), *connection); e != nil {
		return "", e
	}
	cp := filepath.Join(old, "compass.json")
	cfg := compass.Defaults()
	if b, err := os.ReadFile(cp); err == nil {
		b = []byte(strings.ReplaceAll(replace.Replace(string(b)), "system.ai.system.ai.", "system.ai."))
		if e = json.Unmarshal(b, &cfg); e != nil {
			return "", e
		}
	}
	// Restore Astra/Sol preferences that the first, restricted-catalog adapter
	// had substituted with its fallback. The saved original establishes intent.
	if b, err := os.ReadFile(cp + ".before-unity"); err == nil {
		var original compass.Config
		if json.Unmarshal(b, &original) == nil {
			if cfg.High == cfg.Default && original.High == "gpt-6-astra" {
				cfg.High = "system.ai.gpt-6-astra"
				if cfg.Comparison == cfg.Default {
					cfg.Comparison = cfg.High
				}
			}
			if cfg.MidHigh == cfg.Default && original.MidHigh == "gpt-6.1-sol" {
				cfg.MidHigh = "system.ai.gpt-6-1-sol"
			}
		}
	}
	if e = compass.Save(filepath.Join(tmp, "compass.json"), cfg); e != nil {
		return "", e
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		if e = os.Rename(dest, dest+".pre-migration-"+time.Now().UTC().Format("20060102T150405.000000000")); e != nil {
			return "", e
		}
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}
	if e = os.Rename(tmp, dest); e != nil {
		return "", e
	}
	return "Migrated workspace and routing preferences to " + dest + ". Previous files are retained. Run unity-gateway codex on and unity-gateway pi gateway to refresh client configuration.", nil
}
