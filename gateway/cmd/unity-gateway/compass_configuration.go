package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"unity-gateway/gateway/internal/compass"
)

// Use the same local catalogs as the UI, including installations without Codex.
func (a *codexAdapter) compassKnownModels() map[string]bool {
	known := map[string]bool{}
	if catalog, err := a.readPickerCatalog(); err == nil {
		for _, model := range catalog.Models {
			if slug, ok := model["slug"].(string); ok {
				known[slug] = true
			}
		}
	}
	if st, err := codexDesktopStore(a.configPath).Load(); err == nil {
		for _, model := range st.Models {
			known[model.ID] = true
		}
	}
	if store, err := desktopStore(a.configPath); err == nil {
		if st, err := store.Load(); err == nil {
			for _, model := range st.Models {
				known[model.ID] = true
			}
		}
	}
	deps := piDependencies()
	if dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir); err == nil {
		if raw, err := os.ReadFile(filepath.Join(dir, piModelsFilename)); err == nil {
			var document struct {
				Providers map[string]struct {
					Models []struct {
						ID string `json:"id"`
					} `json:"models"`
				} `json:"providers"`
			}
			if json.Unmarshal(raw, &document) == nil {
				for _, model := range document.Providers[piProviderID].Models {
					known[model.ID] = true
				}
			}
		}
	}
	return known
}

// The sheet saves its draft in one write. Client selections, the master switch,
// and the analytics comparison are deliberately outside this payload.
func compassSharedConfiguration(current compass.Config, payload string, available func(string) bool) (compass.Config, error) {
	var draft struct {
		Router  string         `json:"router"`
		Default string         `json:"default"`
		High    string         `json:"high"`
		MidHigh string         `json:"mid_high"`
		Mid     string         `json:"mid"`
		Low     string         `json:"low"`
		Rules   *compass.Rules `json:"rules"`
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return current, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return current, fmt.Errorf("expected one Compass configuration")
	}
	if draft.Rules == nil {
		return current, fmt.Errorf("routing rules are required")
	}
	old := []string{current.Router, current.Default, current.High, current.MidHigh, current.Mid, current.Low}
	models := []string{draft.Router, draft.Default, draft.High, draft.MidHigh, draft.Mid, draft.Low}
	for i, model := range models {
		if model != old[i] && !available(model) {
			return current, fmt.Errorf("choose a model from the catalog: %s", model)
		}
	}
	updated := current
	updated.Router, updated.Default, updated.High = draft.Router, draft.Default, draft.High
	updated.MidHigh, updated.Mid, updated.Low = draft.MidHigh, draft.Mid, draft.Low
	updated.Rules = *draft.Rules
	if err := updated.Validate(); err != nil {
		return current, err
	}
	return updated, nil
}
