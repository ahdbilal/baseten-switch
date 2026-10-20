// Package ugruntime adapts the pinned official Unity Gateway configuration builders.
// It never runs ug configure/revert or lets the builder write harness settings.
package ugruntime

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const Revision = "e0958c1f6bf380ed7542d0f6d28e5577ad81cf0e"
const Source = "git+https://github.com/databricks/unity-gateway@" + Revision

//go:embed bridge.py
var bridge string

func Root() string {
	if path := os.Getenv("UNITY_GATEWAY_UG_RUNTIME"); path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "unity-gateway", "ug-runtime", Revision)
}

func Binary() (string, error) {
	p := filepath.Join(Root(), "bin", "ug")
	if st, err := os.Stat(p); err != nil || st.IsDir() || st.Mode()&0111 == 0 {
		return "", fmt.Errorf("pinned ug runtime is missing; run unity-gateway ug install")
	}
	return p, nil
}

type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Request struct {
	Client    string  `json:"client"`
	Workspace string  `json:"workspace"`
	Profile   string  `json:"profile"`
	BaseURL   string  `json:"base_url"`
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	Models    []Model `json:"models"`
}
type Result struct {
	Revision     string         `json:"revision"`
	Config       map[string]any `json:"config"`
	ProviderTOML string         `json:"provider_toml"`
}

func Render(ctx context.Context, request Request) (Result, error) {
	var result Result
	if _, err := Binary(); err != nil {
		return result, err
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(Root(), "bin", "python"), "-I", "-c", bridge, Revision)
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Join(Root(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		return result, fmt.Errorf("ug configuration builder failed; run unity-gateway ug install to repair the pinned runtime (%w)", err)
	}
	if len(out) > 4<<20 || json.Unmarshal(out, &result) != nil || result.Revision != Revision || result.Config == nil {
		return Result{}, fmt.Errorf("ug returned an incompatible configuration")
	}
	return result, nil
}

func Install(ctx context.Context) error {
	if _, err := Render(ctx, Request{Client: "check"}); err == nil {
		return nil
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return fmt.Errorf("install uv first, then run unity-gateway ug install")
	}
	root := Root()
	for _, args := range [][]string{{"venv", "--python", "3.12", "--allow-existing", root}, {"pip", "install", "--python", filepath.Join(root, "bin", "python"), "--reinstall-package", "unity-gateway", Source}} {
		cmd := exec.CommandContext(ctx, uv, args...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("install pinned ug: %w", err)
		}
	}
	_, err = Render(ctx, Request{Client: "check"})
	return err
}
