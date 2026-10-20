package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unity-gateway/gateway/internal/safefile"
	"unity-gateway/gateway/internal/ugruntime"
	"unity-gateway/gateway/internal/unity"
)

func (a *claudeAdapter) ugSettingsPath() string {
	if a.configPath == "" {
		return filepath.Join(filepath.Dir(a.backupPath), "claude-ug.settings.json")
	}
	return filepath.Join(filepath.Dir(a.configPath), "claude-ug.settings.json")
}

// A separate receipt protects edits and permits recovery if a write is interrupted.
func (a *claudeAdapter) ugSettings() (*safefile.Snapshot, error) {
	s, err := safefile.Read(a.ugSettingsPath())
	if err != nil || !s.Exists {
		return s, err
	}
	raw, err := os.ReadFile(a.ugSettingsPath() + ".receipt")
	var hashes []string
	if err != nil || json.Unmarshal(raw, &hashes) != nil {
		return nil, fmt.Errorf("Claude ug overlay has no ownership receipt; preserving it")
	}
	for _, hash := range hashes {
		if hash == sha256Hex(s.Data) {
			return s, nil
		}
	}
	return nil, fmt.Errorf("Claude ug overlay was edited outside the app; preserving it")
}

func (a *claudeAdapter) ugOn(c unity.Connection) int {
	lock, err := a.acquireSettingsMutationLock()
	if err != nil {
		return desktopError(err)
	}
	// Render and validate before changing any existing connection.
	r, err := c.RenderClient(context.Background(), ugruntime.Request{Client: "claude-code", BaseURL: a.desiredURL()})
	if err != nil {
		lock.close()
		return desktopError(err)
	}
	s, err := a.ugSettings()
	if err != nil {
		lock.close()
		return desktopError(err)
	}
	raw, err := json.MarshalIndent(r.Config, "", "  ")
	if err != nil {
		lock.close()
		return desktopError(err)
	}
	raw = append(raw, '\n')
	receipt, err := safefile.Read(a.ugSettingsPath() + ".receipt")
	if err != nil {
		lock.close()
		return desktopError(err)
	}
	hashes, _ := json.Marshal([]string{sha256Hex(raw), sha256Hex(s.Data)})
	// Validate the new overlay first, then remove only legacy app-owned wiring
	// under the same settings lock before making the isolated connection active.
	if code := a.offLegacyLocked(); code != 0 {
		lock.close()
		return code
	}
	if err = os.MkdirAll(filepath.Dir(a.ugSettingsPath()), 0700); err == nil {
		_, err = receipt.Replace(hashes, 0600)
	}
	if err == nil {
		_, err = s.Replace(raw, 0600)
	}
	lock.close()
	if err != nil {
		return desktopError(err)
	}
	fmt.Fprintln(a.out, "Claude connected using ug configuration in an isolated settings file. Start: unity-gateway compass run claude")
	return 0
}

func (a *claudeAdapter) removeUGSettings() error {
	lock, err := a.acquireSettingsMutationLock()
	if err != nil {
		return err
	}
	defer lock.close()
	s, err := a.ugSettings()
	if err != nil {
		return err
	}
	if s.Exists {
		if err = s.Remove(); err != nil {
			return err
		}
	}
	receipt, err := safefile.Read(a.ugSettingsPath() + ".receipt")
	if err != nil {
		return err
	}
	if receipt.Exists {
		return receipt.Remove()
	}
	return nil
}

func (a *claudeAdapter) ugLaunchArgs(args []string) ([]string, bool, error) {
	s, err := a.ugSettings()
	if err != nil || !s.Exists {
		return args, false, err
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--settings" || strings.HasPrefix(arg, "--settings=") {
			return nil, false, fmt.Errorf("the gateway launcher supplies --settings; remove that argument or launch Claude directly for your own setup")
		}
	}
	return append([]string{"--settings", a.ugSettingsPath()}, args...), true, nil
}
