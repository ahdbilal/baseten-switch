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

	"github.com/basetenlabs/baseten-switch/gateway/internal/compass"
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
		fmt.Fprintln(os.Stderr, "usage: baseten-switch compass watch [--verbose] | run [--verbose] codex|pi [-- client arguments]")
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
	_, configPath, err := loadGatewayConfigForAdapter()
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
	if len(rest) == 0 || (rest[0] != "codex" && rest[0] != "pi") {
		fmt.Fprintln(os.Stderr, "choose codex or pi")
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
	if force || cfg.Rules.Verbose {
		go compassWatch(ctx, path, client, force)
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
