package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiDisconnectRemovesOnlyGatewaySelection(t *testing.T) {
	for _, provider := range []string{"databricks", "openai"} {
		t.Run(provider, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			raw := `{"defaultProvider":"` + provider + `","defaultModel":"chosen","enabledModels":["databricks/*","openai/*"],"theme":"dark"}`
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if err := clearPiGatewaySelection(path); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			got := string(b)
			if strings.Contains(got, "databricks") || !strings.Contains(got, "openai/*") || !strings.Contains(got, "dark") {
				t.Fatal(got)
			}
			if strings.Contains(got, "defaultModel") != (provider == "openai") {
				t.Fatal(got)
			}
		})
	}
}
