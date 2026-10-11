package main

import (
	"encoding/json"
	"fmt"
	"github.com/basetenlabs/baseten-switch/gateway/internal/config"
	"github.com/basetenlabs/baseten-switch/gateway/internal/pricing"
	"github.com/basetenlabs/baseten-switch/gateway/internal/telemetry"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
)

func (a *codexAdapter) compass(args []string) int {
	if !(len(args) == 2 && args[0] == "status" && args[1] == "--json") && !(len(args) == 3 && args[0] == "set") {
		fmt.Fprintln(a.out, "usage: baseten-switch codex compass status --json | set router|default|high|mid_high|mid|low|comparison <model>")
		return 2
	}
	path := compass.Path(a.configPath)
	cfg, err := compass.Load(path)
	if err != nil {
		fmt.Fprintln(a.out, err)
		return 1
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
			if !strings.Contains(model, "/") {
				fmt.Fprintln(a.out, "choose a Baseten routing model")
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
	if err = json.NewEncoder(a.out).Encode(map[string]any{"config": cfg, "recent": compass.Recent(path), "usage": a.compassUsage(cfg)}); err != nil {
		return 1
	}
	return 0
}

func (a *codexAdapter) compassUsage(cfg compass.Config) compass.UsageSummary {
	p := pricing.New()
	for _, provider := range []string{"openai", "baseten"} {
		if raw, err := os.ReadFile(filepath.Join(filepath.Dir(a.configPath), "catalogs", provider+"-public.json")); err == nil {
			_ = p.ImportProviderCache(raw)
		}
	}
	route := "openai"
	if strings.Contains(cfg.ComparisonModel(), "/") {
		route = "baseten"
	}
	dir := config.DefaultTelemetryDir()
	gatewayConfig, configErr := config.Load(a.configPath)
	if configErr != nil {
		summary := compass.SummarizeUsage(nil, time.Now().UTC().Add(-24*time.Hour), cfg.ComparisonModel(), p.Quote(route, cfg.ComparisonModel()))
		summary.Partial = true
		return summary
	}
	if gatewayConfig.Global.TelemetryDir != "" {
		dir = config.ExpandPath(gatewayConfig.Global.TelemetryDir)
	}
	events, err := telemetry.ReadTailEvents(dir, 5000, 8<<20)
	summary := compass.SummarizeUsage(events, time.Now().UTC().Add(-24*time.Hour), cfg.ComparisonModel(), p.Quote(route, cfg.ComparisonModel()))
	summary.Partial = err != nil || len(events) >= 5000
	return summary
}
