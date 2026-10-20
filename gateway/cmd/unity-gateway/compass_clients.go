package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/safefile"
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
	if store, err := desktopStore(a.configPath); err == nil && store.Active() {
		if st, err := store.Load(); err == nil {
			result["claude-desktop"] = compassClientSelection{compass.IsModel(st.Default), st.Default}
		}
	}
	store := codexDesktopStore(a.configPath)
	if st, err := store.Load(); err == nil && store.Active(st) {
		model := store.SelectedModel(st)
		result["codex-desktop"] = compassClientSelection{compass.IsModel(model), model}
	}
	deps := piDependencies()
	if dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir); err == nil {
		if snap, err := safefile.Read(filepath.Join(dir, "settings.json")); err == nil {
			var settings struct {
				Provider string `json:"defaultProvider"`
				Model    string `json:"defaultModel"`
			}
			if !snap.Exists || json.Unmarshal(snap.Data, &settings) == nil {
				result["pi"] = compassClientSelection{settings.Provider == "databricks" && compass.IsModel(settings.Model), settings.Model}
			}
		}
	}
	return result
}

func (a *codexAdapter) selectCompassClient(client string, enabled bool, cfg *compass.Config) error {
	switch client {
	case "codex-desktop":
		store := codexDesktopStore(a.configPath)
		st, err := store.Load()
		if err != nil {
			return err
		}
		if !store.Active(st) {
			return fmt.Errorf("enable Codex desktop first")
		}
		model := cfg.ManualModels[client]
		if enabled {
			model = compass.Model
		} else if !st.Has(model) {
			for _, m := range st.Models {
				if !compass.IsModel(m.ID) {
					model = m.ID
					break
				}
			}
		}
		if !st.Has(model) {
			return fmt.Errorf("refresh Codex desktop models first")
		}
		st.Default = model
		delete(st.Hidden, model)
		return applyCodexDesktop(store, st)

	case "claude-desktop":
		store, err := desktopStore(a.configPath)
		if err != nil {
			return err
		}
		if !store.Active() {
			return fmt.Errorf("connect Claude Desktop first")
		}
		st, err := store.Load()
		if err != nil {
			return err
		}
		model := cfg.ManualModels[client]
		if enabled {
			model = compass.DesktopModel
		} else if !st.Has(model) {
			for _, m := range st.Models {
				if !compass.IsModel(m.ID) {
					model = m.ID
					break
				}
			}
		}
		if !st.Has(model) {
			return fmt.Errorf("refresh Desktop models first")
		}
		st.Default = model
		delete(st.Hidden, model)
		return store.Apply(st)
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
		model := cfg.ForClient(client).Default
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
		model := compassManualCandidate(status.Models, cfg.ForClient(client).Default, cfg.ManualModels[client])
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
		return fmt.Errorf("client must be codex, codex-desktop, pi, claude-code, or claude-desktop")
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
	if !enabled && (settings["defaultProvider"] != "databricks" || !compass.IsModel(current)) {
		if len(raw) == 0 {
			return []byte("{}\n"), nil
		}
		return raw, nil
	}
	settings["defaultProvider"] = "databricks"
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
	case "codex-desktop":
		store := codexDesktopStore(a.configPath)
		st, err := store.Load()
		if err != nil {
			return err
		}
		if !store.Active(st) {
			return fmt.Errorf("enable Codex desktop first")
		}
		if !st.Has(model) {
			return fmt.Errorf("choose a Codex desktop catalog model")
		}
		if !selections[client].Enabled {
			st.Default = model
			delete(st.Hidden, model)
			if err = applyCodexDesktop(store, st); err != nil {
				return err
			}
		}

	case "claude-desktop":
		store, err := desktopStore(a.configPath)
		if err != nil {
			return err
		}
		if !store.Active() {
			return fmt.Errorf("connect Claude Desktop first")
		}
		st, err := store.Load()
		if err != nil {
			return err
		}
		if !st.Has(model) {
			return fmt.Errorf("choose a Desktop catalog model")
		}
		if !selections[client].Enabled {
			st.Default = model
			delete(st.Hidden, model)
			if err = store.Apply(st); err != nil {
				return err
			}
		}

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
		return fmt.Errorf("client must be codex, codex-desktop, pi, claude-code, or claude-desktop")
	}
	if cfg.ManualModels == nil {
		cfg.ManualModels = map[string]string{}
	}
	cfg.ManualModels[client] = model
	return nil
}

// Explicit selection wins regardless of the order of the model catalog.
func compassManualCandidate(models []piModelJSON, fallback, manual string) string {
	first := ""
	for _, model := range models {
		if compass.IsModel(model.ID) {
			continue
		}
		if model.ID == manual {
			return manual
		}
		if first == "" || model.ID == fallback {
			first = model.ID
		}
	}
	return first
}
