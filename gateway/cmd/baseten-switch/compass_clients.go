package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
	"github.com/basetenlabs/baseten-switch/gateway/internal/safefile"
)

type compassClientSelection struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

func (a *codexAdapter) compassClients() map[string]compassClientSelection {
	result := map[string]compassClientSelection{}
	if cfg, err := compass.Load(compass.Path(a.configPath)); err == nil {
		result["claude-code"] = compassClientSelection{compass.IsModel(cfg.ClaudeModel), cfg.ClaudeModel}
	}
	if raw, exists, err := readCodexOverlay(a.overlayPath); err == nil && exists {
		model := parseCodexOverlay(raw).model
		result["codex"] = compassClientSelection{compass.IsModel(model), model}
	}
	deps := piDependencies()
	if dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir); err == nil {
		if snap, err := safefile.Read(filepath.Join(dir, "settings.json")); err == nil {
			var settings struct {
				Provider string `json:"defaultProvider"`
				Model    string `json:"defaultModel"`
			}
			if !snap.Exists || json.Unmarshal(snap.Data, &settings) == nil {
				result["pi"] = compassClientSelection{settings.Provider == "baseten" && compass.IsModel(settings.Model), settings.Model}
			}
		}
	}
	return result
}

func (a *codexAdapter) selectCompassClient(client string, enabled bool, cfg *compass.Config) error {
	switch client {
	case "claude-code":
		cfg.ClaudeModel = cfg.ManualModels[client]
		if enabled {
			cfg.ClaudeModel = compass.Model
		}
		return nil
	case "codex":
		raw, exists, err := readCodexOverlay(a.overlayPath)
		if err != nil {
			return err
		}
		if !exists || !a.overlayManaged(raw) || parseCodexOverlay(raw).catalogPath != a.pickerPath() {
			return fmt.Errorf("connect Codex and sync its model picker first")
		}
		model := cfg.Default
		if cfg.ManualModels[client] != "" {
			model = cfg.ManualModels[client]
		}
		if enabled {
			model = compass.Model
		} else if !compass.IsModel(parseCodexOverlay(raw).model) {
			return nil
		}
		return a.writePickerProfile(raw, replaceCodexPickerModel(raw, model))
	case "pi":
		deps := piDependencies()
		dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir)
		if err != nil {
			return err
		}
		status, err := deps.store.Status(context.Background(), dir)
		if err != nil {
			return err
		}
		if !status.Healthy || status.Mode != "gateway" {
			return fmt.Errorf("connect Pi in gateway mode first")
		}
		model := ""
		for _, candidate := range status.Models {
			if compass.IsModel(candidate.ID) {
				continue
			}
			if model == "" || candidate.ID == cfg.Default || candidate.ID == cfg.ManualModels[client] {
				model = candidate.ID
			}
		}
		if enabled {
			model = compass.Model
		}
		if model == "" {
			return fmt.Errorf("refresh Pi models before disabling Compass")
		}
		snap, err := safefile.Read(filepath.Join(dir, "settings.json"))
		if err != nil {
			return err
		}
		data, err := piCompassSettings(snap.Data, enabled, model)
		if err != nil {
			return err
		}
		_, err = snap.Replace(data, 0600)
		return err
	default:
		return fmt.Errorf("client must be codex, pi, or claude-code")
	}
}

func piCompassSettings(raw []byte, enabled bool, model string) ([]byte, error) {
	settings := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return nil, fmt.Errorf("Pi settings: %w", err)
		}
	}
	if settings == nil {
		return nil, fmt.Errorf("Pi settings must be a JSON object")
	}
	current, _ := settings["defaultModel"].(string)
	if !enabled && (settings["defaultProvider"] != "baseten" || !compass.IsModel(current)) {
		if len(raw) == 0 {
			return []byte("{}\n"), nil
		}
		return raw, nil
	}
	settings["defaultProvider"] = "baseten"
	settings["defaultModel"] = model
	data, err := json.MarshalIndent(settings, "", "  ")
	return append(data, '\n'), err
}

func (a *codexAdapter) setCompassManual(client, model string, cfg *compass.Config) error {
	if !compass.ValidModel(model) {
		return fmt.Errorf("choose an explicit model")
	}
	selections := a.compassClients()
	switch client {
	case "claude-code":
		if !selections[client].Enabled {
			cfg.ClaudeModel = model
		}
	case "codex":
		catalog, err := a.readPickerCatalog()
		if err != nil {
			return err
		}
		found := false
		for _, m := range catalog.Models {
			if m["slug"] == model {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("choose a catalog model")
		}
		raw, exists, err := readCodexOverlay(a.overlayPath)
		if err != nil {
			return err
		}
		if !exists || !a.overlayManaged(raw) {
			return fmt.Errorf("connect Codex first")
		}
		if !selections[client].Enabled {
			if err := a.writePickerProfile(raw, replaceCodexPickerModel(raw, model)); err != nil {
				return err
			}
		}
	case "pi":
		deps := piDependencies()
		dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir)
		if err != nil {
			return err
		}
		status, err := deps.store.Status(context.Background(), dir)
		if err != nil {
			return err
		}
		if !status.Healthy {
			return fmt.Errorf("connect Pi first")
		}
		found := false
		for _, m := range status.Models {
			if m.ID == model {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("choose a Pi model")
		}
		if !selections[client].Enabled {
			snap, err := safefile.Read(filepath.Join(dir, "settings.json"))
			if err != nil {
				return err
			}
			data, err := piCompassSettings(snap.Data, true, model)
			if err != nil {
				return err
			}
			if _, err = snap.Replace(data, 0600); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("client must be codex, pi, or claude-code")
	}
	if cfg.ManualModels == nil {
		cfg.ManualModels = map[string]string{}
	}
	cfg.ManualModels[client] = model
	return nil
}
