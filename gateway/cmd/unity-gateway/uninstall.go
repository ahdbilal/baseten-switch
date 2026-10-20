package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"unity-gateway/gateway/internal/config"
	"unity-gateway/gateway/internal/launchd"
	"unity-gateway/gateway/internal/tracecapture"
)

type uninstallOptions struct {
	dryRun bool
	purge  bool
	yes    bool
}

type uninstallStep struct {
	description string
	run         func() error
}

// cmdUninstall removes only the current Unity Gateway installation. It does
// not know about pre-release names or paths, and it never touches credentials
// owned by the Databricks CLI.
func cmdUninstall(args []string) int {
	opts, err := parseUninstallOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return runUninstall(opts, defaultUninstallSteps(), os.Stderr)
}

func parseUninstallOptions(args []string) (uninstallOptions, error) {
	var opts uninstallOptions
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			opts.dryRun = true
		case "--purge":
			opts.purge = true
		case "--yes":
			opts.yes = true
		default:
			return uninstallOptions{}, fmt.Errorf("unknown flag for uninstall: %s (usage: unity-gateway uninstall [--dry-run] [--purge --yes])", arg)
		}
	}
	if opts.yes && !opts.purge {
		return uninstallOptions{}, errors.New("uninstall: --yes is valid only with --purge")
	}
	if opts.purge && !opts.yes && !opts.dryRun {
		return uninstallOptions{}, errors.New("uninstall: --purge requires explicit confirmation with --yes")
	}
	return opts, nil
}

func runUninstall(opts uninstallOptions, steps []uninstallStep, out io.Writer) int {
	failures := 0
	var activeTracePaths tracecapture.RuntimePaths
	path, _ := resolveConfigPath()
	activeTracePaths, activeTracePathsErr := tracecapture.ResolveRuntimePaths(config.ExpandPath(path))
	defaultTracePaths, defaultTracePathsErr := tracecapture.ResolveRuntimePaths(config.ExpandPath(config.DefaultPath()))
	distinctDefaultRuntime := activeTracePathsErr != nil || defaultTracePathsErr != nil ||
		(defaultTracePaths.TraceDir != activeTracePaths.TraceDir || defaultTracePaths.ExportDir != activeTracePaths.ExportDir)
	retainedExports := tracecapture.ExportStatus{}
	retainedExportsKnown := false
	if activeTracePathsErr == nil {
		if status, inspectErr := tracecapture.InspectRuntimeExports(activeTracePaths); inspectErr == nil {
			retainedExports = status
			retainedExportsKnown = true
		}
	}
	if opts.purge {
		if activeTracePathsErr != nil {
			fmt.Fprintf(out, "uninstall: resolve active trace runtime before uninstall: %v\n", activeTracePathsErr)
			failures++
		} else if opts.dryRun {
			fmt.Fprintf(out, "would permanently remove active trace store %s and export store %s\n", activeTracePaths.TraceDir, activeTracePaths.ExportDir)
		}
		if distinctDefaultRuntime {
			if defaultTracePathsErr != nil {
				fmt.Fprintf(out, "uninstall: resolve default trace runtime before broad purge: %v\n", defaultTracePathsErr)
				failures++
			} else if opts.dryRun {
				fmt.Fprintf(out, "would permanently remove distinct default trace store %s and export store %s\n", defaultTracePaths.TraceDir, defaultTracePaths.ExportDir)
			}
		}
	}
	for _, step := range steps {
		if opts.dryRun {
			fmt.Fprintf(out, "would %s\n", step.description)
			continue
		}
		if err := step.run(); err != nil {
			fmt.Fprintf(out, "uninstall: %s: %v\n", step.description, err)
			failures++
		}
	}
	if opts.purge {
		activeTracePurgeSucceeded := activeTracePathsErr == nil
		if !opts.dryRun && activeTracePathsErr == nil {
			if err := purgeActiveTraceRuntime(activeTracePaths); err != nil {
				fmt.Fprintf(out, "uninstall: purge active trace runtime: %v\n", err)
				failures++
				activeTracePurgeSucceeded = false
			}
		}
		if !opts.dryRun && activeTracePurgeSucceeded && distinctDefaultRuntime {
			if defaultTracePathsErr != nil {
				activeTracePurgeSucceeded = false
			} else if err := purgeActiveTraceRuntime(defaultTracePaths); err != nil {
				fmt.Fprintf(out, "uninstall: purge default trace runtime before broad product purge: %v\n", err)
				failures++
				activeTracePurgeSucceeded = false
			}
		}
		root := databricksSwitchDataRoot()
		if opts.dryRun {
			fmt.Fprintf(out, "would permanently remove current product data root %s\n", root)
		} else if !activeTracePurgeSucceeded {
			fmt.Fprintln(out, "uninstall: skipped broad product data purge because validated trace-store removal did not succeed")
		} else if err := purgeUnityGatewayDataRoot(root); err != nil {
			fmt.Fprintf(out, "uninstall: purge %s: %v\n", root, err)
			failures++
		}
	}
	if opts.dryRun {
		fmt.Fprintln(out, "dry run complete; no changes made")
		return 0
	}
	if failures != 0 {
		fmt.Fprintf(out, "uninstall completed with %d incomplete step(s)\n", failures)
		return 1
	}
	if opts.purge {
		fmt.Fprintln(out, "Unity Gateway uninstalled and current product data purged")
	} else {
		if retainedExportsKnown {
			fmt.Fprintf(out, "Unity Gateway uninstalled; config, secrets, telemetry, captured traces, trace packages, logs, and backups were retained (%d package(s), %d package bytes, %d quarantine item(s), %d quarantine bytes)\n", retainedExports.PackageCount, retainedExports.PackageBytes, retainedExports.QuarantineCount, retainedExports.QuarantineBytes)
		} else {
			fmt.Fprintln(out, "Unity Gateway uninstalled; config, secrets, telemetry, captured traces, trace packages, logs, and backups were retained; retained trace package totals were unavailable")
		}
	}
	return 0
}

func purgeActiveTraceRuntime(paths tracecapture.RuntimePaths) error {
	return errors.Join(
		tracecapture.RemoveRuntimeTraceStore(paths),
		tracecapture.RemoveRuntimeExportStore(paths),
	)
}

func defaultUninstallSteps() []uninstallStep {
	root := databricksSwitchDataRoot()
	return []uninstallStep{
		{description: "restore Codex desktop's previous provider and remove its managed catalog", run: func() error {
			path, _ := resolveConfigPath()
			return codexDesktopStore(path).Disconnect()
		}},
		{
			description: "restore the previous Claude Desktop profile when Unity Gateway still owns the selection",
			run: func() error {
				path, _ := resolveConfigPath()
				if _, err := os.Stat(filepath.Join(filepath.Dir(path), "claude-desktop.json")); os.IsNotExist(err) {
					return nil
				}
				store, err := desktopStore(path)
				if err != nil {
					return err
				}
				return store.Disconnect()
			},
		},
		{
			description: "restore Claude Code settings only when Unity Gateway ownership can be proven",
			run:         uninstallClaude,
		},
		{
			description: "restore or remove the Codex overlay only when Unity Gateway ownership can be proven",
			run:         uninstallCodex,
		},
		{
			description: "boot out and remove the current Unity Gateway launchd agents",
			run:         uninstallLaunchd,
		},
		{
			description: "stop the current Unity Gateway door and router",
			run:         uninstallProcesses,
		},
		{
			description: "quit the current Unity Gateway menubar app",
			run:         uninstallQuitMenubar,
		},
		{
			description: "remove current Unity Gateway runtime residue",
			run: func() error {
				return removeRuntimeResidue(root)
			},
		},
		{
			description: "verify the current Unity Gateway menubar app can be safely unregistered and removed, otherwise require manual removal",
			run:         uninstallManagedApp,
		},
	}
}

func uninstallClaude() error {
	settings := envDefault("UNITY_GATEWAY_CLAUDE_SETTINGS", homeJoin(".claude", "settings.json"))
	backupRoot := envDefault("UNITY_GATEWAY_BACKUP_DIR", filepath.Join(databricksSwitchDataRoot(), "backups"))
	backup := claudeBackupPath(backupRoot, settings)
	// Harness settings may intentionally be a symlink. The adapter restores
	// through a validated safe-file snapshot; the internal backup itself must
	// remain an ordinary file.
	if err := rejectSymlinkTargets(backup); err != nil {
		return err
	}
	configPath, _ := resolveConfigPath()
	overlayExists := pathExists(filepath.Join(filepath.Dir(configPath), "claude-ug.settings.json"))
	if !pathExists(settings) && !pathExists(backup) && !overlayExists {
		fmt.Fprintln(os.Stderr, "claude: no settings or backup to restore")
		return nil
	}
	a, err := newClaudeAdapterFromEnv()
	if err != nil {
		if !pathExists(backup) && !overlayExists {
			fmt.Fprintf(os.Stderr, "claude: config unavailable; leaving settings unchanged because ownership cannot be proven (%v)\n", err)
			return nil
		}
		return fmt.Errorf("managed backup exists but ownership cannot be checked: %w", err)
	}
	return restoreWithRetainedBackup(backup, a.off)
}

func uninstallCodex() error {
	codexHome := envDefault("UNITY_GATEWAY_CODEX_HOME", homeJoin(".codex"))
	overlay := filepath.Join(codexHome, codexOverlayName)
	backupRoot := envDefault("UNITY_GATEWAY_BACKUP_DIR", filepath.Join(databricksSwitchDataRoot(), "backups"))
	backup := codexBackupPath(backupRoot, overlay)
	if err := rejectSymlinkTargets(overlay, backup); err != nil {
		return err
	}
	if !pathExists(overlay) && !pathExists(backup) {
		fmt.Fprintln(os.Stderr, "codex: no overlay or backup to restore")
		return nil
	}
	a, err := newCodexAdapterFromEnv()
	if err != nil {
		if !pathExists(backup) {
			fmt.Fprintf(os.Stderr, "codex: config unavailable; leaving overlay unchanged because ownership cannot be proven (%v)\n", err)
			return nil
		}
		return fmt.Errorf("managed backup exists but ownership cannot be checked: %w", err)
	}
	return restoreWithRetainedBackup(backup, a.off)
}

func restoreWithRetainedBackup(backupPath string, off func() int) error {
	var backup []byte
	if pathExists(backupPath) {
		var err error
		backup, err = os.ReadFile(backupPath)
		if err != nil {
			return fmt.Errorf("read backup before restore: %w", err)
		}
	}
	var errs []error
	if off() != 0 {
		errs = append(errs, errors.New("adapter restore failed"))
	}
	if backup != nil && !pathExists(backupPath) {
		retained, err := writeRetainedBackup(backupPath, backup)
		if err != nil {
			errs = append(errs, fmt.Errorf("retain consumed backup: %w", err))
		} else {
			fmt.Fprintf(os.Stderr, "uninstall: retained consumed backup at %s\n", retained)
		}
	}
	return errors.Join(errs...)
}

func writeRetainedBackup(activePath string, content []byte) (string, error) {
	base := activePath + ".uninstall-retained"
	for i := 0; ; i++ {
		path := base
		if i > 0 {
			path = fmt.Sprintf("%s.%d", base, i)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(content); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(path)
			return "", err
		}
		return path, nil
	}
}

func rejectSymlinkTargets(paths ...string) error {
	for _, path := range paths {
		st, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink %s", path)
		}
	}
	return nil
}

func uninstallLaunchd() error {
	if menubarGOOS != "darwin" {
		return nil
	}
	var errs []error
	for _, label := range []string{launchd.DoorLabel, launchd.RouterLabel} {
		if _, err := launchd.Bootout(launchdRunner, label); err != nil {
			errs = append(errs, fmt.Errorf("%s bootout: %w", label, err))
		}
		path := plistPathFor(label)
		if err := removeExactNonSymlink(path); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func uninstallProcesses() error {
	var doorAddrsResolved, routerAddrs []string
	if lc, err := resolveLifecycle(); err == nil {
		doorAddrsResolved = doorAddrs(lc.doorSpecs)
		routerAddrs = []string{lc.adminAddr}
	}
	var errs []error
	if stopComponent("door", doorPidfilePath(), doorAddrsResolved, doorHealthPath, doorHealthMarker) != 0 {
		errs = append(errs, errors.New("door did not stop cleanly"))
	}
	if stopComponent("router", gatewayPidfilePath(), routerAddrs, routerHealthPath, routerHealthMarker) != 0 {
		errs = append(errs, errors.New("router did not stop cleanly"))
	}
	return errors.Join(errs...)
}

func uninstallQuitMenubar() error {
	if menubarGOOS != "darwin" || !menubarRunning() {
		return nil
	}
	if err := quitMenubar(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "menubar: quit")
	return nil
}

func uninstallManagedApp() error {
	if menubarGOOS != "darwin" {
		return nil
	}
	appPath := homeJoin("Applications", menubarAppName)
	st, err := os.Lstat(appPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink %s", appPath)
	}
	return fmt.Errorf("manual action required: open %s, turn off Start at Login, quit the app, then remove %s; the CLI cannot safely unregister another app's SMAppService login item", appPath, appPath)
}

func databricksSwitchDataRoot() string {
	return homeJoin(".config", "unity-gateway")
}

func removeRuntimeResidue(root string) error {
	if err := validateUnityGatewayDataRoot(root); err != nil {
		return err
	}
	var errs []error
	for _, name := range []string{"door.pid", "gateway.pid", "gateway.config-path"} {
		path := filepath.Join(root, name)
		if err := removeExactNonSymlink(path); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func purgeUnityGatewayDataRoot(root string) error {
	if err := validateUnityGatewayDataRoot(root); err != nil {
		return err
	}
	st, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink data root %s", root)
	}
	return os.RemoveAll(root)
}

func validateUnityGatewayDataRoot(root string) error {
	want := filepath.Clean(homeJoin(".config", "unity-gateway"))
	if filepath.Clean(root) != want {
		return fmt.Errorf("refusing unexpected data root %s (expected %s)", root, want)
	}
	parent := filepath.Dir(want)
	if st, err := os.Lstat(parent); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing data root beneath symlink %s", parent)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func removeExactNonSymlink(path string) error {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to remove symlink")
	}
	if st.IsDir() {
		return errors.New("refusing to remove directory")
	}
	return os.Remove(path)
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
