package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/desktop"
	"unity-gateway/gateway/internal/safefile"
	"unity-gateway/gateway/internal/ugruntime"
	"unity-gateway/gateway/internal/unity"
)

func desktopStore(configPath string) (desktop.Store, error) {
	dir := os.Getenv("UNITY_GATEWAY_CLAUDE_DESKTOP_DIR")
	if dir == "" {
		if runtime.GOOS != "darwin" {
			return desktop.Store{}, fmt.Errorf("Claude Desktop setup currently supports macOS")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return desktop.Store{}, err
		}
		dir = filepath.Join(home, "Library", "Application Support", "Claude-3p")
	}
	if !filepath.IsAbs(dir) {
		return desktop.Store{}, fmt.Errorf("Desktop directory must be absolute")
	}
	store := desktop.Store{Dir: dir, StatePath: filepath.Join(filepath.Dir(configPath), "claude-desktop.json"), RenderProfile: func(st desktop.State) (map[string]any, error) {
		c, err := unity.Load(configPath)
		if err != nil {
			return nil, err
		}
		if c == nil {
			return nil, fmt.Errorf("sign in to a Databricks workspace first")
		}
		profile, err := st.Profile()
		if err != nil {
			return nil, err
		}
		models := []ugruntime.Model{}
		for _, row := range profile["inferenceModels"].([]map[string]any) {
			models = append(models, ugruntime.Model{ID: row["name"].(string), Name: row["labelOverride"].(string)})
		}
		r, err := c.RenderClient(context.Background(), ugruntime.Request{Client: "claude-desktop", BaseURL: desktop.BaseURL, Models: models})
		return r.Config, err
	}}
	if c, err := unity.Load(configPath); err == nil && c == nil {
		store.RenderProfile = nil
	}
	return store, nil
}

func desktopGatewayConfig(raw []byte, model string) ([]byte, error) {
	var f config.File
	if err := config.UnmarshalStrict(raw, &f); err != nil {
		return nil, err
	}
	for _, c := range f.Clients {
		if c.Name == desktop.Client {
			if !c.Enabled || c.ProtocolShape != "anthropic" || c.BindAddr != "127.0.0.1:45275" {
				return nil, fmt.Errorf("existing Desktop listener must use enabled Anthropic loopback port 45275")
			}
			return raw, nil
		}
		if strings.HasSuffix(c.BindAddr, ":45275") {
			return nil, fmt.Errorf("Desktop port 45275 is already assigned")
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == "clients" {
			if root.Content[i+1].Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("clients must be a list")
			}
			var node yaml.Node
			if err := node.Encode(map[string]any{"name": desktop.Client, "enabled": true, "bind_addr": "127.0.0.1:45275", "protocol_shape": "anthropic", "default_model": model, "fallback_route": "anthropic"}); err != nil {
				return nil, err
			}
			root.Content[i+1].Content = append(root.Content[i+1].Content, &node)
			data, err := yaml.Marshal(&doc)
			if err != nil {
				return nil, err
			}
			if err = config.UnmarshalStrict(data, &f); err != nil {
				return nil, err
			}
			return data, config.ValidateRoutingPolicy(&f)
		}
	}
	return nil, fmt.Errorf("gateway configuration has no clients list")
}

func cmdDesktop(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: unity-gateway desktop on|off|status --json|refresh|default <model>|visibility <model> on|off|selection <default> <model>...")
		return 2
	}
	path, _ := resolveConfigPath()
	store, err := desktopStore(path)
	if err != nil {
		return desktopError(err)
	}
	st, err := store.Load()
	if err != nil {
		return desktopError(err)
	}
	switch args[0] {
	case "status":
		return desktopError(json.NewEncoder(os.Stdout).Encode(map[string]any{"models": st.Models, "hidden": st.Hidden, "default": st.Default, "connected": store.Active(), "configured": st.Connected, "base_url": desktop.BaseURL, "directory": store.Dir}))
	case "off":
		return desktopError(store.Disconnect())
	case "on", "refresh":
		c, err := unity.Load(path)
		if err != nil {
			return desktopError(err)
		}
		if c == nil {
			return desktopError(fmt.Errorf("sign in to a Databricks workspace first"))
		}
		models, err := c.ModelsFor(context.Background(), "messages")
		if err != nil {
			return desktopError(err)
		}
		st.Models = []desktop.Model{{ID: compass.DesktopModel, Name: "Auto(Compass)"}}
		for _, m := range models {
			st.Models = append(st.Models, desktop.Model{ID: m.ID, Name: m.DisplayName})
		}
		if len(st.Models) == 1 {
			return desktopError(fmt.Errorf("workspace has no Messages models available"))
		}
		if !st.Has(st.Default) {
			st.Default = st.Models[0].ID

		}
		if st.Hidden == nil {
			st.Hidden = map[string]bool{}
		}
		delete(st.Hidden, st.Default)
		snap, err := safefile.Read(path)
		if err != nil {
			return desktopError(err)
		}
		listenerDefault := st.Default
		if compass.IsModel(listenerDefault) {
			listenerDefault = st.Models[1].ID
		}
		raw, err := desktopGatewayConfig(snap.Data, listenerDefault)
		if err != nil {
			return desktopError(err)
		}
		written, err := snap.Replace(raw, 0600)
		if err != nil {
			return desktopError(err)
		}
		if err = store.Apply(st); err != nil {
			_, restoreErr := written.Replace(snap.Data, 0600)
			if restoreErr != nil {
				err = fmt.Errorf("%w; gateway rollback: %v", err, restoreErr)
			}
			return desktopError(err)
		}
		if err := reloadHarnessListener("claude-desktop"); err != nil {
			return desktopError(err)
		}
		fmt.Println("Connected Claude Desktop. Quit and reopen Claude to load the Databricks profile.")
		return 0
	case "default":
		if len(args) != 2 || !st.Has(args[1]) {
			return desktopError(fmt.Errorf("choose a Desktop catalog model"))
		}
		st.Default = args[1]
		if st.Hidden == nil {
			st.Hidden = map[string]bool{}
		}
		delete(st.Hidden, st.Default)
	case "selection":
		if len(args) < 3 {
			return desktopError(fmt.Errorf("selection requires a default and at least one selected model"))
		}
		st, err = st.WithSelection(args[1], args[2:])
		if err != nil {
			return desktopError(err)
		}
	case "visibility":
		if len(args) != 3 || !st.Has(args[1]) || (args[2] != "on" && args[2] != "off") {
			return desktopError(fmt.Errorf("visibility requires a catalog model and on or off"))
		}
		if args[1] == st.Default && args[2] == "off" {
			return desktopError(fmt.Errorf("choose another default before hiding this model"))
		}
		if st.Hidden == nil {
			st.Hidden = map[string]bool{}
		}
		st.Hidden[args[1]] = args[2] == "off"
	default:
		return desktopError(fmt.Errorf("unknown desktop command"))
	}
	if !store.Active() {
		return desktopError(fmt.Errorf("connect Claude Desktop first"))
	}
	return desktopError(store.Apply(st))
}
func desktopError(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
