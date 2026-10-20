package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"unity-gateway/gateway/internal/compass"
)

func compactCompassText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > 140 {
		r = r[:140]
	}
	return string(r)
}
func compassWatch(ctx context.Context, path, client string, force bool) {
	seen := map[string]string{}
	for _, d := range compass.Recent(path) {
		seen[d.ID] = d.CreatedAt.String() + d.Model + d.ReviewReason
	}
	fmt.Fprintln(os.Stderr, "◈ Compass · Auto · waiting for decisions (client/session feed)")
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			cfg, err := compass.Load(path)
			if err != nil || (!force && !cfg.Rules.Verbose) {
				continue
			}
			for _, d := range compass.Recent(path) {
				if client != "" && d.Client != client {
					continue
				}
				stamp := d.CreatedAt.String() + d.Model + d.ReviewReason
				if seen[d.ID] == stamp {
					continue
				}
				seen[d.ID] = stamp
				reason := d.Reason
				if d.ReviewReason != "" {
					reason = d.ReviewReason
				}
				id := d.ID
				if len(id) > 6 {
					id = id[:6]
				}
				fmt.Fprintf(os.Stderr, "◈ Compass [%s/%s] → %s · %s\n", compactCompassText(d.Client), id, compactCompassText(d.Model), compactCompassText(reason))
			}
		}
	}
}
func cmdCompass(args []string) int {
	if len(args) == 0 || (args[0] != "watch" && args[0] != "run") {
		fmt.Fprintln(os.Stderr, "usage: unity-gateway compass watch [--verbose] | run [--verbose] codex|pi|claude [-- client arguments]")
		return 2
	}
	force := false
	rest := []string{}
	for i, arg := range args[1:] {
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if arg == "--verbose" {
			force = true
		} else {
			rest = append(rest, arg)
		}
	}
	gatewayConfig, configPath, err := loadGatewayConfigForAdapter()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	path := compass.Path(configPath)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if args[0] == "watch" {
		if len(rest) > 0 {
			return 2
		}
		compassWatch(ctx, path, "", force)
		return 0
	}
	if len(rest) == 0 || (rest[0] != "codex" && rest[0] != "pi" && rest[0] != "claude") {
		fmt.Fprintln(os.Stderr, "choose codex, pi, or claude")
		return 2
	}
	client := rest[0]
	clientArgs := rest[1:]
	if len(clientArgs) > 0 && clientArgs[0] == "--" {
		clientArgs = clientArgs[1:]
	}
	cfg, err := compass.Load(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if client == "claude" {
		clientArgs = claudeCompassLaunchArgs(clientArgs, cfg.ClaudeModel)
		adapter, adapterErr := newClaudeAdapterFromEnv()
		if adapterErr != nil {
			fmt.Fprintln(os.Stderr, adapterErr)
			return 1
		}
		var isolated bool
		clientArgs, isolated, err = adapter.ugLaunchArgs(clientArgs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if compass.IsModel(claudeLaunchModel(clientArgs)) {
			enabled := false
			for _, c := range gatewayConfig.Clients {
				if c.Name == "claude-code" {
					enabled = c.Enabled
				}
			}
			root, _, readErr := readClaudeSettings(adapter.settingsPath)
			env, envErr := settingsEnv(root)
			if !enabled || (!isolated && (readErr != nil || envErr != nil || env[claudeManagedEnvKey] != adapter.desiredURL())) {
				fmt.Fprintln(os.Stderr, "Connect Claude first with 'unity-gateway claude on'; its connection has not been changed.")
				return 1
			}
			if override := os.Getenv(claudeManagedEnvKey); override != "" && override != adapter.desiredURL() {
				fmt.Fprintln(os.Stderr, "ANTHROPIC_BASE_URL overrides the Switch connection; clear it before launching Compass.")
				return 1
			}
		}
	}
	feedClient := client
	if client == "claude" {
		feedClient = "claude-code"
	}
	if force || cfg.Rules.Verbose {
		go compassWatch(ctx, path, feedClient, force)
	}
	cmd := exec.Command(client, clientArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return e.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Explicit CLI selection takes precedence over the saved launch default.
func claudeCompassLaunchArgs(args []string, model string) []string {
	if model == "" {
		return args
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--model" || strings.HasPrefix(arg, "--model=") {
			return args
		}
	}
	return append([]string{"--model", model}, args...)
}

func claudeLaunchModel(args []string) string {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--model" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--model=") {
			return strings.TrimPrefix(arg, "--model=")
		}
	}
	return ""
}
