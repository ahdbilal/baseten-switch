package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/basetenlabs/baseten-switch/gateway/cmd/gateway"
	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
	"github.com/basetenlabs/baseten-switch/gateway/internal/unity"
)

const codexPickerFilename = "baseten-switch.models.json"

type codexPickerCatalog struct {
	Mixed  bool             `json:"baseten_switch_mixed,omitempty"`
	Owner  string           `json:"baseten_switch_catalog"`
	Models []map[string]any `json:"models"`
}

func (a *codexAdapter) pickerPath() string {
	return filepath.Join(filepath.Dir(a.overlayPath), codexPickerFilename)
}

func (a *codexAdapter) readPickerCatalog() (*codexPickerCatalog, error) {
	raw, err := os.ReadFile(a.pickerPath())
	if err != nil {
		return nil, err
	}
	var catalog codexPickerCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, err
	}
	if catalog.Owner != "baseten-switch-v1" || len(catalog.Models) == 0 {
		return nil, fmt.Errorf("%s is not a Switch model catalog", a.pickerPath())
	}
	return &catalog, nil
}

func (a *codexAdapter) pickerCatalogSetting() string {
	if _, err := a.readPickerCatalog(); err != nil {
		return ""
	}
	return fmt.Sprintf("model_catalog_json = %q\n", a.pickerPath())
}

// Codex persists /model choices in the selected profile. Recognize only
// entries in our catalog, so off can still restore the pre-integration file.
func (a *codexAdapter) ownsPickerOverlay(raw []byte) bool {
	sh := parseCodexOverlay(raw)
	if sh.modelProvider != "baseten" || !sh.providerTable || sh.baseURL == "" ||
		sh.catalogPath != a.pickerPath() {
		return false
	}
	catalog, err := a.readPickerCatalog()
	if err != nil {
		return false
	}
	for _, m := range catalog.Models {
		if m["slug"] == sh.model {
			return true
		}
	}
	return false
}

func codexPickerEntry(id, name string, contextWindow, priority int, input []string) map[string]any {
	if len(input) == 0 {
		input = []string{"text"}
	}
	return map[string]any{
		"slug": id, "display_name": name, "description": "Served through Databricks Unity Gateway",
		"default_reasoning_level": "none", "supported_reasoning_levels": []map[string]string{{"effort": "none", "description": "Use the gateway's model reasoning policy"}},
		"shell_type": "unified_exec", "visibility": "list", "supported_in_api": true, "priority": priority,
		"availability_nux": nil, "upgrade": nil, "model_messages": nil,
		"base_instructions":                    "You are a coding assistant. Follow the user's instructions.",
		"supports_reasoning_summary_parameter": false, "default_reasoning_summary": "none",
		"support_verbosity": false, "default_verbosity": nil, "apply_patch_tool_type": nil,
		"truncation_policy": map[string]any{"mode": "bytes", "limit": 10000},
		"context_window":    contextWindow, "effective_context_window_percent": 95,
		"experimental_supported_tools": []string{}, "input_modalities": input,
	}
}

func buildCodexPickerCatalog(models []piProviderModel, defaultModel string) ([]byte, error) {
	defaultContext := 128000
	for _, m := range models {
		if m.ID == defaultModel {
			defaultContext = m.ContextWindow
		}
	}
	catalog := codexPickerCatalog{Mixed: true, Owner: "baseten-switch-v1", Models: []map[string]any{
		codexPickerEntry(compass.Model, "Auto(Compass)", defaultContext, 0, nil),
	}}
	seen := map[string]bool{}
	for _, m := range models {
		if (!strings.Contains(m.ID, "/") && !unity.IsModel(m.ID)) || strings.ContainsAny(m.ID, "\r\n\t ") || m.ContextWindow <= 0 || seen[m.ID] {
			return nil, fmt.Errorf("invalid or duplicate gateway model %q", m.ID)
		}
		seen[m.ID] = true
		catalog.Models = append(catalog.Models, codexPickerEntry(m.ID, "Unity Gateway · "+m.Name, m.ContextWindow, len(catalog.Models), m.Input))
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("Gateway model catalog contains no models")
	}
	raw, err := json.MarshalIndent(catalog, "", "  ")
	return append(raw, '\n'), err
}

// Replace only the top-level catalog key and preserve unrelated profile edits.
func codexPickerOverlay(raw []byte, path string) []byte {
	lines := strings.Split(string(raw), "\n")
	var out []string
	root := true
	inserted := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if root && strings.HasPrefix(trimmed, "[") {
			if path != "" {
				out = append(out, fmt.Sprintf("model_catalog_json = %q", path))
				inserted = true
			}
			root = false
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if root && ok && strings.TrimSpace(key) == "model_catalog_json" {
			continue
		}
		out = append(out, line)
	}
	if path != "" && !inserted {
		out = append(out, fmt.Sprintf("model_catalog_json = %q", path))
	}
	return []byte(strings.Join(out, "\n"))
}

func (a *codexAdapter) picker(args []string) int {
	if !validCodexPickerArgs(args) {
		fmt.Fprintln(a.out, "usage: baseten-switch codex picker sync|status|disable|list --json|show <model>|hide <model>")
		return 2
	}
	raw, exists, err := readCodexOverlay(a.overlayPath)
	if err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	if args[0] == "list" {
		catalog, err := a.readPickerCatalog()
		if err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		return writeCodexPickerJSON(a.out, catalog)
	}
	if args[0] == "status" {
		catalog, err := a.readPickerCatalog()
		if err != nil || !exists || parseCodexOverlay(raw).catalogPath != a.pickerPath() {
			fmt.Fprintln(a.out, "codex picker: disabled (run 'baseten-switch codex picker sync')")
			return 3
		}
		visible := 0
		for _, m := range catalog.Models {
			if m["visibility"] == "list" {
				visible++
			}
		}
		fmt.Fprintf(a.out, "codex picker: enabled (%d visible models; %s)\n", visible, a.pickerPath())
		return 0
	}
	if !exists || !a.overlayManaged(raw) {
		fmt.Fprintln(a.out, "codex picker: a managed Baseten profile is required; run 'baseten-switch codex on' first")
		return 1
	}
	sh := parseCodexOverlay(raw)
	if sh.catalogPath != "" && sh.catalogPath != a.pickerPath() {
		fmt.Fprintln(a.out, "codex picker: refusing to replace a user-configured model_catalog_json")
		return 1
	}
	if args[0] == "show" || args[0] == "hide" {
		catalog, err := a.readPickerCatalog()
		if err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		if err := setCodexPickerVisibility(catalog, args[1], args[0] == "show"); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		data, err := json.MarshalIndent(catalog, "", "  ")
		if err == nil {
			err = writeCodexOverlay(a.pickerPath(), append(data, '\n'))
		}
		if err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		fmt.Fprintln(a.out, "codex picker: visibility saved; restart Codex to apply")
		return 0
	}
	if args[0] == "disable" {
		updated := codexMixedProviderOverlay(codexPickerOverlay(raw, ""), false)
		if sh.model != gateway.CodexCompatibilityModel {
			updated = replaceCodexPickerModel(updated, gateway.CodexCompatibilityModel)
		}
		if err := a.writePickerProfile(raw, updated); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		fmt.Fprintln(a.out, "codex picker: disabled; restart Codex to apply (catalog retained)")
		return 0
	}
	var previous *codexPickerCatalog
	if _, err := os.Lstat(a.pickerPath()); err == nil {
		if catalog, err := a.readPickerCatalog(); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		} else {
			previous = catalog
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(a.out, err)
		return 1
	}
	deps := piDependencies()
	models, err := deps.catalog.Models(context.Background())
	if err != nil {
		fmt.Fprintf(a.out, "codex picker: %v\n", err)
		return 1
	}
	enriched, err := deps.capabilities.Enrich(context.Background(), models)
	if err == nil {
		models = enriched.Models
	} else {
		fmt.Fprintln(a.out, "codex picker: public capability lookup unavailable; using text-only defaults")
	}
	data, err := buildCodexPickerCatalog(models, a.modelSlug)
	if err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	var merged codexPickerCatalog
	if err := json.Unmarshal(data, &merged); err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	nativePath := filepath.Join(filepath.Dir(a.overlayPath), "models_cache.json")
	if native, err := os.ReadFile(nativePath); err == nil {
		if err := mergeCodexNativeCatalog(&merged, native); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(a.out, err)
		return 1
	}
	if merged.Mixed && codexPickerHasForeignHeaders(raw) {
		fmt.Fprintln(a.out, "codex picker: refusing to replace user-configured provider http_headers")
		return 1
	}
	data, err = json.MarshalIndent(merged, "", "  ")
	if err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	data = append(data, '\n')
	if previous != nil {
		var next codexPickerCatalog
		if err := json.Unmarshal(data, &next); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		preserveCodexPickerVisibility(previous, &next)
		data, err = json.MarshalIndent(next, "", "  ")
		if err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		data = append(data, '\n')
	}
	if err := writeCodexOverlay(a.pickerPath(), data); err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	if err := a.writePickerProfile(raw, migrateCodexCompassProfile(raw, a.pickerPath(), merged.Mixed)); err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	fmt.Fprintf(a.out, "codex picker: synced %d gateway models; restart 'codex --profile baseten' and open /model\n", len(models))
	return 0
}

func replaceCodexPickerModel(raw []byte, model string) []byte {
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			break
		}
		key, _, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(key) == "model" {
			lines[i] = fmt.Sprintf("model = %q", model)
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func (a *codexAdapter) writePickerProfile(raw, updated []byte) error {
	if err := writeCodexOverlay(a.overlayPath, updated); err != nil {
		return err
	}
	// Keep off's original backup and update its hash only when the profile had
	// not drifted before this operation. User edits retain drift protection.
	if bak, err := loadCodexBackup(a.backupPath); err == nil && bak != nil && bak.WrittenHash == sha256Hex(raw) {
		bak.WrittenHash = sha256Hex(updated)
		if err := saveCodexBackup(a.backupPath, bak); err != nil {
			fmt.Fprintf(a.out, "warning: update backup hash: %v\n", err)
		}
	}
	return nil
}

func validCodexPickerArgs(args []string) bool {
	if len(args) == 1 {
		return args[0] == "sync" || args[0] == "status" || args[0] == "disable"
	}
	return len(args) == 2 && ((args[0] == "list" && args[1] == "--json") || args[0] == "show" || args[0] == "hide")
}

func writeCodexPickerJSON(out io.Writer, catalog *codexPickerCatalog) int {
	rows := make([]map[string]any, 0, len(catalog.Models))
	for _, m := range catalog.Models {
		rows = append(rows, map[string]any{"slug": m["slug"], "display_name": m["display_name"], "visibility": m["visibility"]})
	}
	if err := json.NewEncoder(out).Encode(map[string]any{"models": rows}); err != nil {
		return 1
	}
	return 0
}

func setCodexPickerVisibility(catalog *codexPickerCatalog, id string, show bool) error {
	var target map[string]any
	visible := 0
	for _, m := range catalog.Models {
		if m["slug"] == id {
			target = m
		}
		if m["visibility"] == "list" {
			visible++
		}
	}
	if target == nil {
		return fmt.Errorf("unknown picker model %q; sync the catalog first", id)
	}
	if !show && target["visibility"] == "list" && visible <= 1 {
		return fmt.Errorf("keep at least one model visible in the picker")
	}
	if show {
		target["visibility"] = "list"
	} else {
		target["visibility"] = "hide"
	}
	return nil
}

func preserveCodexPickerVisibility(previous, next *codexPickerCatalog) {
	hidden := map[string]bool{}
	for _, m := range previous.Models {
		if m["visibility"] == "hide" {
			if id, ok := m["slug"].(string); ok {
				hidden[id] = true
				if compass.IsModel(id) {
					hidden[compass.Model] = true
				}
			}
		}
	}
	visible := 0
	for _, m := range next.Models {
		if id, ok := m["slug"].(string); ok && hidden[id] {
			m["visibility"] = "hide"
		} else {
			visible++
		}
	}
	// If every surviving entry was hidden, restore the gateway-default row.
	if visible == 0 && len(next.Models) > 0 {
		for _, m := range next.Models {
			if m["slug"] == compass.Model {
				m["visibility"] = "list"
				return
			}
		}
		next.Models[0]["visibility"] = "list"
	}
}

// Use the native account cache's complete metadata, including its instruction
// templates. No credentials are read or copied by catalog generation.
func mergeCodexNativeCatalog(catalog *codexPickerCatalog, raw []byte) error {
	path, _ := resolveConfigPath()
	if c, e := unity.Load(path); e != nil {
		return e
	} else if c != nil {
		return nil
	}
	var native struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(raw, &native); err != nil {
		return fmt.Errorf("read native Codex catalog: %w", err)
	}
	seen := map[string]bool{}
	for _, m := range catalog.Models {
		if id, ok := m["slug"].(string); ok {
			seen[id] = true
		}
	}
	var entries []map[string]any
	for _, m := range native.Models {
		id, ok := m["slug"].(string)
		if !ok || id == "" || strings.ContainsAny(id, "/\r\n\t ") || seen[id] {
			continue
		}
		seen[id] = true
		entries = append(entries, m)
	}
	if len(entries) > 0 {
		catalog.Mixed = true
		catalog.Models = append(entries, catalog.Models...)
	}
	return nil
}

const codexMixedPickerHeader = "X-Baseten-Switch-Codex-Picker"

func codexMixedProviderOverlay(raw []byte, mixed bool) []byte {
	lines := strings.Split(string(raw), "\n")
	var out []string
	provider := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			provider = trimmed == codexProviderTable
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if provider && ok && (strings.TrimSpace(key) == "requires_openai_auth" ||
			(strings.TrimSpace(key) == "http_headers" && strings.Contains(line, codexMixedPickerHeader))) {
			continue
		}
		out = append(out, line)
		if trimmed == codexProviderTable && mixed {
			out = append(out, "requires_openai_auth = true", "http_headers = { \""+codexMixedPickerHeader+"\" = \"mixed-v1\" }")
		}
	}
	return []byte(strings.Join(out, "\n"))
}

func codexPickerHasForeignHeaders(raw []byte) bool {
	provider := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			provider = trimmed == codexProviderTable
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if provider && ok && strings.TrimSpace(key) == "http_headers" && !strings.Contains(line, codexMixedPickerHeader) {
			return true
		}
	}
	return false
}

func migrateCodexCompassProfile(raw []byte, path string, mixed bool) []byte {
	updated := codexMixedProviderOverlay(codexPickerOverlay(raw, path), mixed)
	if parseCodexOverlay(raw).model == gateway.CodexCompatibilityModel || compass.IsModel(parseCodexOverlay(raw).model) {
		updated = replaceCodexPickerModel(updated, compass.Model)
	}
	return updated
}
