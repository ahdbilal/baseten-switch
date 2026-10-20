package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unity-gateway/gateway/cmd/gateway"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/safefile"
)

type harnessStatus struct {
	Enabled    bool `json:"enabled"`
	Configured bool `json:"configured"`
}

func readHarnessStatus(name string) (harnessStatus, error) {
	path, _ := resolveConfigPath()
	switch name {
	case "codex-desktop":
		s := codexDesktopStore(path)
		st, e := s.Load()
		return harnessStatus{s.Active(st), st.Connected || st.ListenerOwned}, e
	case "claude-desktop":
		s, e := desktopStore(path)
		if e != nil {
			return harnessStatus{}, e
		}
		st, e := s.Load()
		return harnessStatus{s.Active(), st.Connected}, e
	case "pi":
		d := piDependencies()
		dir, e := resolvePiAgentDir(d.getenv("PI_CODING_AGENT_DIR"), d.homeDir)
		if e != nil {
			return harnessStatus{}, e
		}
		st, e := d.store.Status(context.Background(), dir)
		return harnessStatus{st.Healthy && st.Mode == "gateway", st.Installed}, e
	case "codex":
		a, e := newCodexAdapterFromEnv()
		if e != nil {
			return harnessStatus{}, e
		}
		b, exists, e := readCodexOverlay(a.overlayPath)
		managed := exists && a.overlayManaged(b)
		return harnessStatus{managed && a.clientEnabled, managed}, e
	case "claude-code":
		a, e := newClaudeAdapterFromEnv()
		if e != nil {
			return harnessStatus{}, e
		}
		root, _, e := readClaudeSettings(a.settingsPath)
		if e != nil {
			return harnessStatus{}, e
		}
		env, e := settingsEnv(root)
		if e != nil {
			return harnessStatus{}, e
		}
		owned, complete := a.claudeOnState(env)
		isolated, err := a.ugSettings()
		if err != nil {
			return harnessStatus{}, err
		}
		if isolated.Exists {
			owned, complete = true, true
		}
		f, _, e := loadGatewayConfigForAdapter()
		if e != nil {
			return harnessStatus{}, e
		}
		enabled := false
		for _, c := range f.Clients {
			if c.Name == a.clientName {
				enabled = c.Enabled
			}
		}
		return harnessStatus{complete && enabled, owned}, nil
	}
	return harnessStatus{}, fmt.Errorf("unknown harness %s", name)
}
func cmdHarness(args []string) int {
	if len(args) < 2 {
		return desktopError(fmt.Errorf("usage: unity-gateway harness status|on|off codex|codex-desktop|claude-code|claude-desktop|pi"))
	}
	action, name := args[0], args[1]
	if action == "status" {
		st, e := readHarnessStatus(name)
		if e != nil {
			return desktopError(e)
		}
		return desktopError(json.NewEncoder(os.Stdout).Encode(st))
	}
	if action != "on" && action != "off" {
		return desktopError(fmt.Errorf("action must be status, on, or off"))
	}
	switch name {
	case "codex-desktop":
		return cmdCodexDesktop([]string{action})
	case "claude-desktop":
		return cmdDesktop([]string{action})
	case "pi":
		if action == "off" {
			if code := cmdPi([]string{"uninstall"}); code != 0 {
				return code
			}
			d := piDependencies()
			dir, e := resolvePiAgentDir(d.getenv("PI_CODING_AGENT_DIR"), d.homeDir)
			if e != nil {
				return desktopError(e)
			}
			return desktopError(clearPiGatewaySelection(filepath.Join(dir, "settings.json")))
		}
		return cmdPi([]string{"gateway"})
	case "claude-code":
		if action == "on" {
			a, e := newClaudeAdapterFromEnv()
			if e != nil {
				return desktopError(e)
			}
			if e = config.SetClientScalars(a.configPath, a.clientName, map[string]string{"enabled": "true"}); e != nil {
				return desktopError(e)
			}
			if e = reloadHarnessListener(a.clientName); e != nil {
				return desktopError(e)
			}
			if state, pid := classifyPidfile(doorPidfilePath()); state == pidfileAlive {
				if e = signalRouter(pid); e != nil {
					return desktopError(e)
				}
			}
		}
		return cmdClaude([]string{action})
	case "codex":
		a, e := newCodexAdapterFromEnv()
		if e != nil {
			return desktopError(e)
		}
		a.out = os.Stdout
		if action == "on" {
			a.in = strings.NewReader("y\n")
			if code := a.on(); code != 0 {
				return code
			}
			return a.picker([]string{"sync"})
		}
		// Removing support removes the managed overlay and its catalog, not config.toml.
		ownedCatalog := false
		if _, e := a.readPickerCatalog(); e == nil {
			ownedCatalog = true
		}
		if code := a.off(); code != 0 {
			return code
		}
		if ownedCatalog {
			snap, e := safefile.Read(a.pickerPath())
			if e != nil {
				return desktopError(e)
			}
			if snap.Exists {
				return desktopError(snap.Remove())
			}
		}
		return 0
	}
	return desktopError(fmt.Errorf("unknown harness %s", name))
}

// Reload listeners without restarting the settings app hosting the toggle.
func reloadHarnessListener(name string) error {
	if state, pid := classifyPidfile(gatewayPidfilePath()); state == pidfileAlive {
		if err := signalRouter(pid); err != nil {
			return err
		}
		if !waitClientBound(envDefault("UNITY_GATEWAY_ADMIN_ADDR", gateway.DefaultAdminAddr), name, routeApplyTimeout) {
			return fmt.Errorf("%s listener did not start; check the router log and retry", name)
		}
	}
	return nil
}

// Pi must not keep a default pointing at the provider that was just removed.
func clearPiGatewaySelection(path string) error {
	snap, err := safefile.Read(path)
	if err != nil || !snap.Exists {
		return err
	}
	var settings map[string]json.RawMessage
	if err = json.Unmarshal(snap.Data, &settings); err != nil {
		return err
	}
	changed := false
	var provider string
	_ = json.Unmarshal(settings["defaultProvider"], &provider)
	if provider == piProviderID {
		delete(settings, "defaultProvider")
		delete(settings, "defaultModel")
		changed = true
	}
	var enabled []string
	if json.Unmarshal(settings["enabledModels"], &enabled) == nil {
		keep := []string{}
		for _, model := range enabled {
			if strings.HasPrefix(model, piProviderID+"/") {
				changed = true
			} else {
				keep = append(keep, model)
			}
		}
		if len(keep) == 0 {
			delete(settings, "enabledModels")
		} else {
			settings["enabledModels"], _ = json.Marshal(keep)
		}
	}
	if !changed {
		return nil
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	_, err = snap.Replace(append(data, '\n'), snap.Mode)
	return err
}
