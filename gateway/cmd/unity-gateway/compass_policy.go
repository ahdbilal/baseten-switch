package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"unity-gateway/gateway/internal/compass"
)

func compassPolicyEdit(cfg compass.Config, args []string, available func(string) bool) (compass.Config, error) {
	if args[0] == "override" {
		client := args[1]
		if compass.Harness(client) == "" || (args[2] != "on" && args[2] != "off") {
			return cfg, fmt.Errorf("override requires a known client and on|off")
		}
		next := make(map[string]compass.Policy, len(cfg.Overrides))
		for k, v := range cfg.Overrides {
			next[k] = v
		}
		if args[2] == "on" {
			next[client] = cfg.ClientPolicy(client)
		} else {
			delete(next, client)
		}
		cfg.Overrides = next
		return cfg, cfg.Validate()
	}
	scope, id := args[1], args[2]
	if (scope == "harness" && id != "codex" && id != "claude" && id != "pi") || (scope == "client" && compass.Harness(id) == "") || (scope != "harness" && scope != "client") {
		return cfg, fmt.Errorf("invalid Compass policy scope")
	}
	var draft compass.Policy
	decoder := json.NewDecoder(strings.NewReader(args[3]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return cfg, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return cfg, fmt.Errorf("expected one policy")
	}
	current := cfg.HarnessPolicy(id)
	if scope == "client" {
		current = cfg.ClientPolicy(id)
	}
	// Reuse full-draft catalog and model validation; power belongs to this scope.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args[3]), &fields); err != nil {
		return cfg, err
	}
	delete(fields, "enabled")
	payload, err := json.Marshal(fields)
	if err != nil {
		return cfg, err
	}
	updated, err := compassSharedConfiguration(current.Config(), string(payload), available)
	if err != nil {
		return cfg, err
	}
	p := updated.Policy()
	p.Enabled = draft.Enabled
	if scope == "harness" {
		next := make(map[string]compass.Policy, len(cfg.Harnesses)+1)
		for k, v := range cfg.Harnesses {
			next[k] = v
		}
		next[id] = p
		cfg.Harnesses = next
	} else {
		next := make(map[string]compass.Policy, len(cfg.Overrides)+1)
		for k, v := range cfg.Overrides {
			next[k] = v
		}
		next[id] = p
		cfg.Overrides = next
	}
	return cfg, cfg.Validate()
}

func compassResolved(cfg compass.Config) map[string]compass.Config {
	result := map[string]compass.Config{}
	for _, client := range []string{"codex", "codex-desktop", "claude-code", "claude-desktop", "pi"} {
		result[client] = cfg.ForClient(client)
	}
	return result
}
