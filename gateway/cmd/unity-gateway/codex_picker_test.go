package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pickerTestCatalog struct{}

func (pickerTestCatalog) Models(context.Context) ([]piProviderModel, error) {
	return []piProviderModel{{ID: "example/model-a", Name: "Model A", ContextWindow: 65536}}, nil
}

func TestCodexPickerCatalogRoutingAndCompatibilityMetadata(t *testing.T) {
	raw, err := buildCodexPickerCatalog([]piProviderModel{{ID: "example/model-a", Name: "Model A", ContextWindow: 65536, Input: []string{"text", "image"}}}, "example/model-a")
	if err != nil {
		t.Fatal(err)
	}
	var catalog codexPickerCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Models) != 2 || catalog.Models[1]["slug"] != "example/model-a" || catalog.Models[0]["context_window"] != float64(65536) {
		t.Fatalf("catalog = %s", raw)
	}
	for _, model := range catalog.Models {
		if model["visibility"] != "list" || model["support_verbosity"] != false || model["supports_reasoning_summary_parameter"] != false {
			t.Fatalf("unsafe metadata: %+v", model)
		}
	}
}

func TestCodexPickerRejectsNonRoutingModelIDs(t *testing.T) {
	for _, id := range []string{"gpt-6-astra", "example/with space", "example/model\n"} {
		if _, err := buildCodexPickerCatalog([]piProviderModel{{ID: id, ContextWindow: 1000}}, ""); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
}

func TestCodexPickerOverlayPreservesSettingsAndProviderTable(t *testing.T) {
	raw := []byte("# user comment\nmodel_reasoning_effort = \"low\"\nmodel_catalog_json = \"old.json\"\n\n[model_providers.databricks]\nbase_url = \"http://localhost:1\"\n")
	got := string(codexPickerOverlay(raw, "/tmp/catalog.json"))
	if strings.Count(got, "model_catalog_json") != 1 || !strings.Contains(got, "model_reasoning_effort = \"low\"") || !strings.Contains(got, "# user comment") || !strings.Contains(got, "base_url = \"http://localhost:1\"") {
		t.Fatalf("profile = %s", got)
	}
	if strings.Contains(string(codexPickerOverlay([]byte(got), "")), "model_catalog_json") {
		t.Fatal("disable kept catalog setting")
	}
}

func TestCodexPickerSyncAndOffRestoreOriginalProfile(t *testing.T) {
	a, out := testCodexAdapter(t)
	writeOverlayFile(t, a, codexTestStaleOverlay)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	oldDeps := piDependencies
	t.Cleanup(func() { piDependencies = oldDeps })
	piDependencies = func() piAdapterDependencies {
		return piAdapterDependencies{catalog: pickerTestCatalog{}, capabilities: pickerTestCapabilities{}}
	}
	if a.picker([]string{"sync"}) != 0 {
		t.Fatal(out.String())
	}
	if a.picker([]string{"status"}) != 0 {
		t.Fatal(out.String())
	}
	raw := fileBytes(t, a.overlayPath)
	if parseCodexOverlay(raw).catalogPath != a.pickerPath() {
		t.Fatalf("profile = %s", raw)
	}
	if a.off() != 0 {
		t.Fatal(out.String())
	}
	if string(fileBytes(t, a.overlayPath)) != codexTestStaleOverlay {
		t.Fatal("off failed to restore original")
	}
}

type pickerTestCapabilities struct{}

func (pickerTestCapabilities) Enrich(_ context.Context, models []piProviderModel) (piCapabilityEnrichment, error) {
	return piCapabilityEnrichment{Models: models}, nil
}

func TestCodexPickerSelectedModelRemainsOwned(t *testing.T) {
	a, _ := testCodexAdapter(t)
	if a.on() != 0 {
		t.Fatal("on failed")
	}
	data, _ := buildCodexPickerCatalog([]piProviderModel{{ID: "example/model-a", Name: "A", ContextWindow: 1000}}, "example/model-a")
	if err := os.WriteFile(a.pickerPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	raw := codexPickerOverlay(a.desiredOverlay(), a.pickerPath())
	raw = replaceCodexPickerModel(raw, "example/model-a")
	if !a.overlayManaged(raw) {
		t.Fatalf("selected model not owned: %s", raw)
	}
	raw = []byte(strings.Replace(string(raw), "example/model-a", "foreign/model", 1))
	if a.overlayManaged(raw) {
		t.Fatal("claimed unknown model")
	}
}

func TestCodexPickerDoesNotOverwriteForeignCatalogOrProfile(t *testing.T) {
	a, out := testCodexAdapter(t)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	foreign := []byte(`{"models":[]}`)
	if err := os.WriteFile(a.pickerPath(), foreign, 0600); err != nil {
		t.Fatal(err)
	}
	before := fileBytes(t, a.overlayPath)
	if a.picker([]string{"sync"}) == 0 {
		t.Fatal("overwrote foreign catalog")
	}
	if string(fileBytes(t, a.pickerPath())) != string(foreign) || string(fileBytes(t, a.overlayPath)) != string(before) {
		t.Fatal("foreign state changed")
	}
}

func TestCodexPickerDisableAfterSelectionReturnsToManagedDefault(t *testing.T) {
	a, out := testCodexAdapter(t)
	if a.on() != 0 {
		t.Fatal(out.String())
	}
	data, _ := buildCodexPickerCatalog([]piProviderModel{{ID: "example/model-a", Name: "A", ContextWindow: 1000}}, "example/model-a")
	if err := os.WriteFile(a.pickerPath(), data, 0600); err != nil {
		t.Fatal(err)
	}
	raw := replaceCodexPickerModel(codexPickerOverlay(a.desiredOverlay(), a.pickerPath()), "example/model-a")
	if err := os.WriteFile(a.overlayPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if a.picker([]string{"disable"}) != 0 {
		t.Fatal(out.String())
	}
	updated := fileBytes(t, a.overlayPath)
	if !a.overlayManaged(updated) || parseCodexOverlay(updated).catalogPath != "" {
		t.Fatalf("disable left unmanaged profile: %s", updated)
	}
	if a.off() != 0 {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(a.overlayPath); !os.IsNotExist(err) {
		t.Fatal("off left selected-model profile installed")
	}
}

func TestCodexPickerVisibilitySurvivesRefresh(t *testing.T) {
	raw, _ := buildCodexPickerCatalog([]piProviderModel{{ID: "example/a", Name: "A", ContextWindow: 1000}}, "example/a")
	var previous, next codexPickerCatalog
	_ = json.Unmarshal(raw, &previous)
	_ = json.Unmarshal(raw, &next)
	if err := setCodexPickerVisibility(&previous, "example/a", false); err != nil {
		t.Fatal(err)
	}
	preserveCodexPickerVisibility(&previous, &next)
	if next.Models[1]["visibility"] != "hide" {
		t.Fatal("refresh lost hidden model")
	}
	if err := setCodexPickerVisibility(&previous, "Auto(Compass)", false); err == nil {
		t.Fatal("allowed empty picker")
	}
}

func TestCodexPickerMergesNativeMetadataWithoutOpenAIAuth(t *testing.T) {
	raw, _ := buildCodexPickerCatalog([]piProviderModel{{ID: "example/a", Name: "A", ContextWindow: 1000}}, "example/a")
	var catalog codexPickerCatalog
	_ = json.Unmarshal(raw, &catalog)
	native := []byte(`{"models":[{"slug":"gpt-test","display_name":"OpenAI test","visibility":"list","base_instructions":"native instructions","support_verbosity":true},{"slug":"bad/slug"}]}`)
	if err := mergeCodexNativeCatalog(&catalog, native); err != nil {
		t.Fatal(err)
	}
	if !catalog.Mixed || len(catalog.Models) != 3 || catalog.Models[0]["base_instructions"] != "native instructions" || catalog.Models[0]["support_verbosity"] != true {
		t.Fatalf("native metadata lost: %+v", catalog)
	}
	a, _ := testCodexAdapter(t)
	profile := codexMixedProviderOverlay(a.desiredOverlay(), true)
	if !strings.Contains(string(profile), "requires_openai_auth = false") || strings.Count(string(profile), codexMixedPickerHeader) != 1 {
		t.Fatalf("profile %s", profile)
	}
	if string(codexMixedProviderOverlay(profile, true)) != string(profile) {
		t.Fatal("mixed auth is not idempotent")
	}
	disabled := codexMixedProviderOverlay(profile, false)
	if strings.Contains(string(disabled), codexMixedPickerHeader) || strings.Contains(string(disabled), "requires_openai_auth") {
		t.Fatal("disable retained mixed routing")
	}
}

func TestCodexPickerProtectsCustomProviderHeaders(t *testing.T) {
	a, _ := testCodexAdapter(t)
	raw := append(a.desiredOverlay(), []byte("http_headers = { \"User-Header\" = \"value\" }\n")...)
	if !codexPickerHasForeignHeaders(raw) {
		t.Fatal("custom headers unprotected")
	}
	if codexPickerHasForeignHeaders(codexMixedProviderOverlay(a.desiredOverlay(), true)) {
		t.Fatal("claimed own headers foreign")
	}
}

func TestCodexPickerMigratesLegacyDefaultToCompass(t *testing.T) {
	a, _ := testCodexAdapter(t)
	raw := migrateCodexCompassProfile(a.desiredOverlay(), a.pickerPath(), true)
	if parseCodexOverlay(raw).model != "Auto(Compass)" {
		t.Fatalf("legacy default retained: %s", raw)
	}
	manual := replaceCodexPickerModel(a.desiredOverlay(), "gpt-test")
	if parseCodexOverlay(migrateCodexCompassProfile(manual, a.pickerPath(), true)).model != "gpt-test" {
		t.Fatal("explicit model choice overwritten")
	}
	data, _ := buildCodexPickerCatalog([]piProviderModel{{ID: "example/model", ContextWindow: 1000}}, "example/model")
	if strings.Contains(string(data), "unity-gateway-compat-v1") || !strings.Contains(string(data), "Auto(Compass)") {
		t.Fatal("old compatibility entry retained")
	}
}

func TestCodexPickerRenamesExistingCompassSelection(t *testing.T) {
	a, _ := testCodexAdapter(t)
	raw := replaceCodexPickerModel(a.desiredOverlay(), "unity-gateway-compass")
	got := migrateCodexCompassProfile(raw, a.pickerPath(), true)
	if parseCodexOverlay(got).model != "Auto(Compass)" {
		t.Fatal("Compass selection not renamed")
	}
}

func TestWorkspacePickerProfileStatusRecognizesCompass(t *testing.T) {
	dir := t.TempDir()
	a := &codexAdapter{overlayPath: filepath.Join(dir, codexOverlayName), gatewayPorts: map[string]bool{"45271": true}, out: io.Discard}
	catalog := codexPickerCatalog{Owner: "unity-gateway-v1", Models: []map[string]any{{"slug": "Auto(Compass)"}}}
	raw, _ := json.Marshal(catalog)
	if e := os.WriteFile(a.pickerPath(), raw, 0600); e != nil {
		t.Fatal(e)
	}
	profile := fmt.Sprintf("model_provider = \"databricks\"\nmodel = \"Auto(Compass)\"\nmodel_catalog_json = %q\n[model_providers.databricks]\nbase_url = \"http://127.0.0.1:45271/v1\"\n", a.pickerPath())
	if e := os.WriteFile(a.overlayPath, []byte(profile), 0600); e != nil {
		t.Fatal(e)
	}
	if code := a.status(io.Discard); code != 0 {
		t.Fatalf("workspace picker reported disconnected: %d", code)
	}
}
