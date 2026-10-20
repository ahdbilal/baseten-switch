package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"unity-gateway/gateway/internal/config"
)

func TestPiGatewayConfigAddsDedicatedClientAndPreservesExisting(t *testing.T) {
	raw, err := os.ReadFile("../../../config/gateway.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data, url, err := piGatewayConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://127.0.0.1:45274/v1" || !strings.Contains(string(data), "codex ships parked") {
		t.Fatal("wrong endpoint or comments lost")
	}
	var before, after config.File
	if err = config.UnmarshalStrict(raw, &before); err != nil {
		t.Fatal(err)
	}
	if err = config.UnmarshalStrict(data, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Clients) != len(before.Clients)+1 || after.Clients[len(after.Clients)-1].Name != "pi" {
		t.Fatal("Pi not added")
	}
	first, _ := json.Marshal(before.Clients)
	second, _ := json.Marshal(after.Clients[:len(before.Clients)])
	if string(first) != string(second) {
		t.Fatal("existing clients modified")
	}
	again, _, err := piGatewayConfig(data)
	if err != nil || string(again) != string(data) {
		t.Fatal("not idempotent", err)
	}

}
func TestPiGatewayProviderCarriesSessionAffinityWithoutSecret(t *testing.T) {
	raw, err := marshalPiProvider(piProviderSpec{ID: "databricks", API: "openai-responses", APIKey: "unity-gateway-pi", BaseURL: "http://127.0.0.1:45274/v1", Models: []piProviderModel{{ID: "Auto(Compass)", Name: "Auto(Compass)", ContextWindow: 128000, MaxTokens: 16384, Input: []string{"text"}}}})
	if err != nil {
		t.Fatal(err)
	}
	var p piProviderJSON
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.Models[0].Compat["sessionAffinityFormat"] != "openai" || p.APIKey != "unity-gateway-pi" {
		t.Fatal("missing session affinity or wrong local token")
	}
}
