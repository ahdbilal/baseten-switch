package migration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unity-gateway/gateway/internal/safefile"
)

// PiSettings moves legacy defaults and model filters only after the replacement
// provider is installed. Unrelated providers and preferences are preserved.
func PiSettings(agentDir string, models []string) error {
	snap, err := safefile.Read(filepath.Join(agentDir, "settings.json"))
	if err != nil || !snap.Exists {
		return err
	}
	updated, err := piSettings(snap.Data, models)
	if err != nil || bytes.Equal(updated, snap.Data) {
		return err
	}
	backup, err := os.CreateTemp(agentDir, "settings.before-unity-*.json")
	if err != nil {
		return err
	}
	_, err = backup.Write(snap.Data)
	closeErr := backup.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	_, err = snap.Replace(updated, 0600)
	return err
}

func piSettings(raw []byte, models []string) ([]byte, error) {
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(raw, &settings); err != nil || settings == nil {
		return nil, fmt.Errorf("settings.json must be a JSON object")
	}
	available := map[string]bool{}
	for _, id := range models {
		available[id] = true
	}
	translate := func(id string) string {
		id = strings.NewReplacer(
			"baseten-switch-compass", "Auto(Compass)",
			"zai-org/GLM-5.3-Fast", "system.ai.glm-5-3-flash",
			"zai-org/GLM-5.3", "system.ai.glm-5-3",
			"moonshotai/Kimi-K3", "system.ai.kimi-k3",
			"gpt-6-astra", "system.ai.gpt-6-astra",
			"gpt-6.1-sol", "system.ai.gpt-6-1-sol",
		).Replace(id)
		id = strings.ReplaceAll(id, "system.ai.system.ai.", "system.ai.")
		if available[id] {
			return id
		}
		return "Auto(Compass)"
	}
	changed := false
	var provider, model string
	_ = json.Unmarshal(settings["defaultProvider"], &provider)
	_ = json.Unmarshal(settings["defaultModel"], &model)
	if provider == "baseten" {
		settings["defaultProvider"], _ = json.Marshal("databricks")
		settings["defaultModel"], _ = json.Marshal(translate(model))
		changed = true
	}
	var enabled []string
	if json.Unmarshal(settings["enabledModels"], &enabled) == nil {
		for i, id := range enabled {
			if strings.HasPrefix(id, "baseten/") {
				model := strings.TrimPrefix(id, "baseten/")
				if model == "*" {
					enabled[i] = "databricks/*"
				} else {
					enabled[i] = "databricks/" + translate(model)
				}
				changed = true
			}
		}
		if changed {
			settings["enabledModels"], _ = json.Marshal(enabled)
		}
	}
	if !changed {
		return raw, nil
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	return append(out, '\n'), err
}
