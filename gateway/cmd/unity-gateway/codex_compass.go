package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/pricing"
	"unity-gateway/gateway/internal/telemetry"
	"unity-gateway/gateway/internal/unity"

	"unity-gateway/gateway/internal/compass"
)

// Presentation filters never alter routing or persistent model settings.
func compassUsageFilters(args []string) ([]string, time.Duration, string, error) {
	window, client := 24*time.Hour, "all"
	var command []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--period" && args[i] != "--client" {
			command = append(command, args[i])
			continue
		}
		flag := args[i]
		i++
		if i == len(args) {
			return nil, 0, "", fmt.Errorf("%s requires a value", flag)
		}
		if flag == "--client" {
			client = args[i]
			if client != "all" && client != "pi" && client != "codex" && client != "claude-code" && client != "claude-desktop" {
				return nil, 0, "", fmt.Errorf("client must be all, pi, codex, claude-code, or claude-desktop")
			}
		} else {
			switch args[i] {
			case "24h":
				window = 24 * time.Hour
			case "7d":
				window = 7 * 24 * time.Hour
			case "30d":
				window = 30 * 24 * time.Hour
			default:
				return nil, 0, "", fmt.Errorf("period must be 24h, 7d, or 30d")
			}
		}
	}
	return command, window, client, nil
}

func (a *codexAdapter) compass(args []string) int {
	args, window, client, filterErr := compassUsageFilters(args)
	if filterErr != nil {
		fmt.Fprintln(a.out, filterErr)
		return 2
	}
	if !(len(args) == 2 && args[0] == "status" && args[1] == "--json") && !(len(args) == 3 && (args[0] == "set" || args[0] == "rule" || args[0] == "default")) && !(len(args) == 2 && (args[0] == "enable" || args[0] == "disable")) {
		fmt.Fprintln(a.out, "usage: unity-gateway codex compass status --json [--period 24h|7d|30d] [--client all|pi|codex|claude-code] | set <field> <model> | rule <name> on|off | enable|disable codex|pi|claude-code|claude-desktop | default codex|pi|claude-code|claude-desktop <model>")
		return 2
	}
	path := compass.Path(a.configPath)
	cfg, err := compass.Load(path)
	if err != nil {
		fmt.Fprintln(a.out, err)
		return 1
	}
	if args[0] == "rule" {
		if args[2] != "on" && args[2] != "off" {
			fmt.Fprintln(a.out, "rule value must be on or off")
			return 2
		}
		value := args[2] == "on"
		switch args[1] {
		case "session_start":
			cfg.Rules.SessionStart = &value
		case "subagents":
			cfg.Rules.Subagents = &value
		case "session_switching":
			cfg.Rules.SessionSwitching = value
		case "encourage_subagents":
			cfg.Rules.EncourageSubagents = value
		case "verbose":
			cfg.Rules.Verbose = value
		default:
			fmt.Fprintln(a.out, "unknown Compass rule")
			return 2
		}
		if err := compass.Save(path, cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	}
	if args[0] == "default" {
		if err := a.setCompassManual(args[1], args[2], &cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
		if err := compass.Save(path, cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	}
	if args[0] == "enable" || args[0] == "disable" {
		selection := a.compassClients()[args[1]]
		if args[0] == "enable" && !selection.Enabled && compass.ValidModel(selection.Model) {
			if cfg.ManualModels == nil {
				cfg.ManualModels = map[string]string{}
			}
			cfg.ManualModels[args[1]] = selection.Model
		}
		if err := a.selectCompassClient(args[1], args[0] == "enable", &cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	}
	if args[0] == "enable" || args[0] == "disable" {
		if err := compass.Save(path, cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	}
	if args[0] == "set" {
		model := args[2]
		catalog, err := a.readPickerCatalog()
		if err != nil {
			fmt.Fprintln(a.out, "sync the Codex picker first:", err)
			return 1
		}
		found := false
		for _, m := range catalog.Models {
			if m["slug"] == model {
				found = true
			}
		}
		if !found || !compass.ValidModel(model) {
			fmt.Fprintln(a.out, "choose a model from the Codex catalog")
			return 1
		}
		switch args[1] {
		case "router":
			if !strings.Contains(model, "/") && !unity.IsModel(model) {
				fmt.Fprintln(a.out, "choose a Databricks routing model")
				return 1
			}
			cfg.Router = model
		case "default":
			cfg.Default = model
		case "comparison":
			cfg.Comparison = model
		case "high":
			cfg.High = model
		case "mid_high":
			cfg.MidHigh = model
		case "mid":
			cfg.Mid = model
		case "low":
			cfg.Low = model
		default:
			fmt.Fprintln(a.out, "unknown Compass setting")
			return 2
		}
		if err = compass.Save(path, cfg); err != nil {
			fmt.Fprintln(a.out, err)
			return 1
		}
	}
	if err = json.NewEncoder(a.out).Encode(map[string]any{"config": cfg, "clients": a.compassClients(), "recent": compass.Recent(path), "usage": a.compassUsage(cfg, window, client)}); err != nil {
		return 1
	}
	return 0
}

func (a *codexAdapter) compassUsage(cfg compass.Config, window time.Duration, client string) compass.UsageSummary {
	p := pricing.NewWorkspace()
	if c, err := unity.Load(a.configPath); err == nil && c != nil {
		_ = p.ReplaceWorkspaceModels([]string{cfg.Router, cfg.Default, cfg.High, cfg.MidHigh, cfg.Mid, cfg.Low, cfg.ComparisonModel()}, c.Workspace, time.Now().UTC())
	}
	route := "openai"
	if strings.Contains(cfg.ComparisonModel(), "/") || unity.IsModel(cfg.ComparisonModel()) {
		route = "databricks"
	}
	dir := config.DefaultTelemetryDir()
	gatewayConfig, configErr := config.Load(a.configPath)
	if configErr != nil {
		summary := compass.SummarizeUsage(nil, time.Now().UTC().Add(-window), cfg.ComparisonModel(), p.Quote(route, cfg.ComparisonModel()), p.Capture())
		summary.Partial = true
		return summary
	}
	if gatewayConfig.Global.TelemetryDir != "" {
		dir = config.ExpandPath(gatewayConfig.Global.TelemetryDir)
	}
	events, err := telemetry.ReadTailEvents(dir, 5000, 8<<20)
	partial := err != nil || len(events) >= 5000
	if client != "all" {
		filtered := make([]telemetry.EventV1, 0, len(events))
		for _, event := range events {
			if event.Client == client {
				filtered = append(filtered, event)
			}
		}
		events = filtered
	}
	summary := compass.SummarizeUsage(events, time.Now().UTC().Add(-window), cfg.ComparisonModel(), p.Quote(route, cfg.ComparisonModel()), p.Capture())
	summary.Partial = partial
	return summary
}
