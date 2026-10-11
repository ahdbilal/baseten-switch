package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCompassUsageFilters(t *testing.T) {
	command, window, client, err := compassUsageFilters([]string{"status", "--json", "--period", "7d", "--client", "pi"})
	if err != nil || len(command) != 2 || command[0] != "status" || window != 7*24*time.Hour || client != "pi" {
		t.Fatalf("unexpected filters: %v %v %s %v", command, window, client, err)
	}
	_, window, client, err = compassUsageFilters([]string{"set", "comparison", "gpt-6-astra"})
	if err != nil || window != 24*time.Hour || client != "all" {
		t.Fatal("legacy defaults changed")
	}
	for _, args := range [][]string{{"--period"}, {"--client", "other"}, {"--period", "999d"}} {
		if _, _, _, err := compassUsageFilters(args); err == nil {
			t.Fatalf("accepted invalid filter: %v", args)
		}
	}
}

func TestPiCompassSettingsPreserveUserFields(t *testing.T) {
	empty, err := piCompassSettings(nil, false, "fallback")
	if err != nil || !json.Valid(empty) {
		t.Fatal("disable created invalid empty settings")
	}

	raw := []byte(`{"theme":"dark","defaultProvider":"other","defaultModel":"manual","extensions":["custom"]}`)
	updated, err := piCompassSettings(raw, true, "Auto(Compass)")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err = json.Unmarshal(updated, &values); err != nil {
		t.Fatal(err)
	}
	if values["theme"] != "dark" || values["defaultModel"] != "Auto(Compass)" || len(values["extensions"].([]any)) != 1 {
		t.Fatal("lost unrelated settings")
	}
	unchanged, err := piCompassSettings(raw, false, "fallback")
	if err != nil || string(unchanged) != string(raw) {
		t.Fatal("changed unmanaged selection")
	}
	if _, err = piCompassSettings([]byte(`null`), true, "Auto(Compass)"); err == nil {
		t.Fatal("accepted null settings")
	}
}
func TestCompassTerminalTextStripsControls(t *testing.T) {
	if strings.ContainsAny(compactCompassText("hello\x1b[31m\nworld"), "\x1b\n") {
		t.Fatal("terminal control characters leaked")
	}
}
