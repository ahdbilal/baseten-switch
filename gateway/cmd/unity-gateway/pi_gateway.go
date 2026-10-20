package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/migration"
	"unity-gateway/gateway/internal/safefile"
)

// Pi has its own loopback listener so telemetry and Compass pins remain separate.
func piGatewayConfig(raw []byte) ([]byte, string, error) {
	var f config.File
	if err := config.UnmarshalStrict(raw, &f); err != nil {
		return nil, "", err
	}
	for _, c := range f.Clients {
		if c.Name != "pi" {
			continue
		}
		host, _, err := net.SplitHostPort(c.BindAddr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || !c.Enabled || c.ProtocolShape != "openai" {
			return nil, "", fmt.Errorf("existing Pi client must be enabled with an OpenAI-shaped loopback listener")
		}
		return raw, "http://" + c.BindAddr, nil
	}
	for _, c := range f.Clients {
		if strings.HasSuffix(c.BindAddr, ":45274") {
			return nil, "", fmt.Errorf("Pi port 45274 is already assigned")
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, "", err
	}
	var clients *yaml.Node
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "clients" {
			clients = root.Content[i+1]
		}
	}
	if clients == nil || clients.Kind != yaml.SequenceNode {
		return nil, "", fmt.Errorf("gateway configuration has no clients list")
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("name: pi\nenabled: true\nbind_addr: 127.0.0.1:45274\nprotocol_shape: openai\ndefault_model: system.ai.glm-5-3\nfallback_route: openai\n"), &node); err != nil {
		return nil, "", err
	}
	clients.Content = append(clients.Content, node.Content[0])
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, "", err
	}
	var check config.File
	if err = config.UnmarshalStrict(data, &check); err != nil {
		return nil, "", err
	}
	if err = config.ValidateRoutingPolicy(&check); err != nil {
		return nil, "", err
	}
	return data, "http://127.0.0.1:45274", nil
}

func cmdPiGateway() int {
	deps := piDependencies()
	dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	models, err := deps.catalog.Models(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	enriched, err := deps.capabilities.Enrich(context.Background(), models)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	models = enriched.Models
	models = append([]piProviderModel{{ID: compass.Model, Name: compass.Model, ContextWindow: 128000, MaxTokens: 16384, Input: []string{"text"}, CapabilityKnown: true}}, models...)
	_, path, err := loadGatewayConfigForAdapter()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	snap, err := safefile.Read(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	data, url, err := piGatewayConfig(snap.Data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_, err = snap.Replace(data, 0600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	result, err := deps.store.Install(context.Background(), piInstallRequest{AgentDir: dir, Provider: piProviderSpec{
		ID: piProviderID, Name: "Databricks Unity Gateway", BaseURL: url, API: "openai-responses", APIKey: "unity-gateway-pi",
		Headers: map[string]string{"X-Unity-Gateway-Codex-Picker": "mixed-v1"}, Models: models,
	}})
	if err != nil {
		// Restore only if no intervening configuration edit occurred.
		current, readErr := safefile.Read(path)
		if readErr == nil && string(current.Data) == string(data) {
			_, _ = current.Replace(snap.Data, 0600)
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	if err := migration.PiSettings(dir, ids); err != nil {
		fmt.Fprintln(os.Stderr, "Pi settings migration:", err)
		return 1
	}
	fmt.Printf("Pi gateway provider installed in %s (%d models).\n", result.ModelsPath, result.ModelCount)
	if code := cmdRestart(nil); code != 0 {
		return code
	}
	fmt.Println("Start: pi --provider databricks --model 'Auto(Compass)'")
	return 0
}

func cmdPiStatusJSON() int {
	deps := piDependencies()
	dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	result, err := deps.store.Status(context.Background(), dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return writePiStatusJSON(result)
}
func writePiStatusJSON(result piStatusResult) int {
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}

func cmdPiVisibility(args []string) int {
	if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
		fmt.Fprintln(os.Stderr, "usage: unity-gateway pi visibility MODEL on|off")
		return 2
	}
	deps := piDependencies()
	dir, err := resolvePiAgentDir(deps.getenv("PI_CODING_AGENT_DIR"), deps.homeDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store, ok := deps.store.(*piFileStore)
	if !ok {
		fmt.Fprintln(os.Stderr, "Pi visibility is unavailable")
		return 1
	}
	if err := store.SetVisibility(dir, args[0], args[1] == "on"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Saved. Restart Pi or run /reload to update /model.")
	return 0
}
