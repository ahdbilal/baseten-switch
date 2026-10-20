package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
	"github.com/basetenlabs/baseten-switch/gateway/internal/unity"
)

func cmdWorkspaceLogin(args []string) int {
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	host := fs.String("workspace", "", "Databricks workspace URL")
	profile := fs.String("profile", "", "Databricks CLI profile")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return 2
	}
	path, _ := resolveConfigPath()
	c, _ := unity.Load(path)
	if c == nil {
		saved, e := unity.SavedWorkspace()
		if e == nil {
			c = &saved
		}
	}
	if c == nil {
		c = &unity.Connection{}
	}
	if *host != "" {
		c.Workspace = strings.TrimRight(*host, "/")
		c.Profile = ""
	}
	if *profile != "" {
		c.Profile = *profile
	}
	if c.Workspace == "" {
		fmt.Fprint(os.Stderr, "Databricks workspace URL: ")
		s := bufio.NewScanner(os.Stdin)
		if !s.Scan() {
			return 1
		}
		c.Workspace = strings.TrimRight(strings.TrimSpace(s.Text()), "/")
	}
	if c.Profile == "" {
		c.Profile = "unity-gateway-" + strings.TrimPrefix(c.Workspace, "https://")
	}
	if e := c.Validate(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Signing in to %s (profile %s)\n", c.Workspace, c.Profile)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if _, e := c.Token(ctx, true); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	models, e := c.Models(ctx)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	// Only migrate our Compass file. ug configure is intentionally not invoked:
	// it also changes the user's shared harness configuration.
	cp := compass.Path(path)
	cfg, e := compass.Load(cp)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	available := map[string]bool{}
	for _, m := range models {
		available[m.Slug] = true
	}
	choose := func(preferred string) string {
		if available[preferred] {
			return preferred
		}
		return models[0].Slug
	}
	fields := []*string{&cfg.Router, &cfg.Default, &cfg.High, &cfg.MidHigh, &cfg.Mid, &cfg.Low}
	prefs := []string{"system.ai.glm-5-3-flash", "system.ai.glm-5-3", "system.ai.glm-5-3", "system.ai.glm-5-3", "system.ai.glm-5-3", "system.ai.kimi-k3"}
	for i, p := range fields {
		if !available[*p] {
			*p = choose(prefs[i])
		}
	}
	cfg.Comparison = cfg.High
	for client, model := range cfg.ManualModels {
		if !available[model] {
			cfg.ManualModels[client] = cfg.Default
		}
	}
	if cfg.ClaudeModel != "" && !compass.IsModel(cfg.ClaudeModel) && !available[cfg.ClaudeModel] {
		cfg.ClaudeModel = cfg.Default
	}
	// Preserve the previous provider's settings for an explicit rollback.
	if _, e := os.Stat(cp + ".before-unity"); os.IsNotExist(e) {
		if raw, readErr := os.ReadFile(cp); readErr == nil {
			if e = os.WriteFile(cp+".before-unity", raw, 0600); e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 1
			}
		}
	}
	if e = compass.Save(cp, cfg); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if e = unity.Save(path, *c); e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if state, pid := classifyPidfile(gatewayPidfilePath()); state == pidfileAlive {
		_ = signalRouter(pid)
	}
	fmt.Fprintf(os.Stdout, "Connected to %s. Discovered %d workspace models.\nCompass now uses workspace model IDs. Refresh the model picker and restart your client.\n", c.Workspace, len(models))
	return 0
}
