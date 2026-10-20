# Testing

How Unity Gateway is tested and how to run each layer.

## Strategy

Testing runs in three layers, ordered from cheapest to most realistic:

1. **Unit and integration tests** (Go and Swift): hermetic, run on every change.
2. **The `check.sh` gate**: build, vet, full test suite, plus live smoke tests
   on scratch ports. This is the required gate for every change set.
3. **Fresh-install simulation** (`tests/fresh-install/`): a clean Debian
   container exercises the source-build lifecycle and verifies the full
   install-to-first-request path.

An additional opt-in live contract test validates API-key profile
authentication against Databricks. The normal gate skips this networked lane.

## Layer 1: unit and integration tests

### Go

```sh
cd gateway && go test ./...
```

| Package | Covers |
|---|---|
| `cmd/unity-gateway` | CLI surface, clean config reset, global mutation CAS/journal/reconciliation, Claude settings adapter, and diagnostics |
| `cmd/gateway` | Routing engine, global gate and resolver status, fallback waterfall, upstream discovery, subagent gate, TTFT timing, reload behavior, and admin security |
| `internal/*` | Analytics aggregation and indexing, config editing, OAuth and credential isolation, door circuit breaker, pidfiles, sanitization, telemetry, protocol translation, and usage accounting |

The only production package with no tests is `internal/version`.

### Swift (menubar app)

Tests under `mac/UnityGateway/Tests/UnityGatewayTests/` cover
`DisplayTests`, `DoorStatusTests`, `LoginItemTests`, `PopupDisplayTests`,
`RuntimeCoordinationTests`, `StatsTests`, and `TrafficTests`. Run with:

```sh
cd mac/UnityGateway && swift test
```

These cover display formatting, stats parsing, login-item policy,
isolated runtime environments, runtime-trust
interlocks, binary lookup, icon sizing, window-presentation state, and
routing mutation tokens, receipts, timeouts, and reconciliation.
Traffic tests also cover analytics decoding, range requests, stale replay,
request coalescing, and navigation. Nothing drives live AppKit behavior
(status item, native menu, window controls, or route-flip shell-outs).

Release validation also covers the packaged app, accessibility, and
performance.

Release discovery tests use hermetic HTTP servers and temporary cache roots to
exercise the canonical formula/release agreement, numeric versions, daily
caching, explicit refresh, failure/rate-limit backoff, bounded responses, and
credential-free requests. CLI tests protect human versus machine output and
the automatic-check preference. Swift update tests exercise the same JSON
contract, stale app versus upgraded CLI detection, single-flight checks,
cancellation, scheduling, install instructions, and tracked-menu stability.

The existing side-effect-free native fixtures include `update-available` and
`update-current` for both the main window and menu. They demonstrate synthetic
release states without network requests or installation; use an isolated
macOS QA environment to inspect native layout, focus, copy actions, and the
shared instructions window. Fixture appearance does not establish a live
public-feed check or a packaged daily check.

## Layer 2: the `check.sh` gate

```sh
scripts/check.sh            # full gate
scripts/check.sh --offline  # skips only the network smoke (step 5)
```

Required before calling any change set done, including work delegated to
agents. Steps, in order:

1. `gofmt` on changed files (ratchet: ignores pre-existing drift).
2. `go build ./...`, `go vet ./...`, `go test ./...`.
3. Build the `unity-gateway` binary.
4. Verify the scratch ports are free. Defaults are 28081-28083, 28182-28183,
   and 28786-28787; `UNITY_GATEWAY_CHECK_*_PORT` variables override them for parallel
   worktrees.
5. Smoke: `whoami --refresh` against the configured credential store, forcing
   a live `/v1/users/me` call. Skipped by `--offline` or when no local auth
   exists.
6. Smoke: throwaway gateway boot on scratch ports; asserts `/healthz` and two
   expected preflight warnings.
7. Smoke: throwaway door boot against a dead router; asserts `/doorz` reports
   tripped.
8. Smoke: `up`, `status`, idempotent `up`, `down`, `status` round trip on
   scratch ports with `UNITY_GATEWAY_LAUNCHD=off` and a fully sandboxed env. Never
   touches the live gateway.
9. Validate the API-key profile live-test script in dry-run mode. The live
   request runs only when explicitly enabled with a test key.

All smoke steps isolate themselves with the env seams listed below; the gate
never edits `~/.config/unity-gateway` or touches a running production gateway.

## Layer 3: fresh-install simulation (`tests/fresh-install/`)

A clean Debian container installs the binary, writes config and a 0600 env
file, runs `unity-gateway up`, then verifies that a real request through the
front door returns 200 from Databricks and that telemetry records the request.

```sh
cd tests/fresh-install
DATABRICKS_API_KEY=... ./run.sh    # keyed mode: real routed request
./run.sh --no-key               # keyless: asserts 503 needs-login, then a
                                # SIGHUP route flip to a local stub
```

The API key enters only via `docker run -e`, never a image layer. Not
simulated: macOS keychain OAuth, the menubar app, Gatekeeper/notarization,
and real launchd installs.

## Opt-in API-key profile contract

This test creates a temporary Databricks CLI API-key profile, starts scratch
router and door processes, and sends `GET /v1/models` plus one minimal Chat
Completions request through the door. It verifies that admin status reports a
healthy API-key profile, the environment fallback is disabled, and telemetry
attributes inference to Databricks.

Use an API key that may list models and invoke the selected model. The key is
read only from the environment and is never printed. The model defaults to
the public example model; override it when the key has access to a different
model.

```sh
UNITY_GATEWAY_TEST_API_KEY='<test-key>' \
UNITY_GATEWAY_TEST_MODEL='zai-org/GLM-5.2' \
scripts/tests/test_api_key_profile_live.sh
```

To include the lane in the full gate:

```sh
UNITY_GATEWAY_RUN_API_KEY_PROFILE_LIVE=1 \
UNITY_GATEWAY_TEST_API_KEY='<test-key>' \
UNITY_GATEWAY_TEST_MODEL='zai-org/GLM-5.2' \
scripts/check.sh
```

The script requires `databricks`, `curl`, `go`, and `lsof`. It uses a temporary
`DATABRICKS_CONFIG_DIR`, insecure storage inside that temporary directory, an
empty Switch environment file, scratch ports, and scratch pidfiles. It does
not read the normal CLI profile store or operate on installed Switch
processes. On exit it stops only the process IDs it started and removes only
its validated temporary directory.

Before cleanup, the script scans its output, logs, telemetry, and scratch
state for the exact test key. It excludes the temporary CLI credential file,
which intentionally contains the key.

## Writing tests: isolation seams

### Disposable VM cleanup

When testing in a virtual machine, include its disk in the test cleanup plan.
Stopping a VM does not delete its disk. Keep an environment needed for pending
manual review, then make an explicit retention or deletion decision when that
review finishes or is canceled.

- Identify the exact VM and the test task that owns it. Preserve required test
  evidence and guest-only work before removal.
- Complete fixture restoration and release test-session ownership before
  stopping the VM. Unresolved recovery is a reason to retain it and report a
  blocker, not to bypass the cleanup checks.
- Obtain approval covering the exact VMs before deleting their disks. Protect
  running tests, pending manual reviews, other tasks' environments and reusable
  base images. Age or stopped state alone does not establish that a VM is unused.
- Use the VM manager's supported deletion command for the approved targets.
  Avoid wildcard pruning. Verify removal and actual host free space afterward.
- At test closeout, report deleted VMs and storage reclaimed. For each retained
  VM, record an owner, retention reason and next review date. Review these records
  and available disk space before allocating more test VMs.

### Runtime isolation

Production code honors env overrides so tests and scratch instances never
touch real state. Any test that boots a component must set the relevant ones:

| Seam | Purpose |
|---|---|
| `UNITY_GATEWAY_CONFIG_PATH` | config file location |
| `UNITY_GATEWAY_ADMIN_ADDR` | admin API address |
| `UNITY_GATEWAY_CLAUDE_SETTINGS`, `UNITY_GATEWAY_BACKUP_DIR` | Claude settings.json adapter targets |
| `UNITY_GATEWAY_LAUNCHD=off` | disable launchd supervision entirely |
| `UNITY_GATEWAY_GATEWAY_PIDFILE`, `UNITY_GATEWAY_DOOR_PIDFILE` | pidfile locations |
| `UNITY_GATEWAY_GATEWAY_LOG`, `UNITY_GATEWAY_DOOR_LOG` | process log locations |
| `UNITY_GATEWAY_TELEMETRY_DIR` | segmented telemetry store location |
| `UNITY_GATEWAY_OAUTH_PROFILE` | named Databricks CLI profile; unset follows the CLI's current profile (point at a nonexistent name to run credential-less) |
| `DATABRICKS_CONFIG_DIR` | Databricks CLI profile-store directory; live credential tests must use a scratch directory. |
| `UNITY_GATEWAY_AUTH_NO_KEYRING=1` | prevent a scratch test from consulting the system keyring. |
| `UNITY_GATEWAY_MENUBAR_APP` | menubar bundle path override |

Hard rules for contributors and agents:

- Never point a test at the real `~/.claude/settings.json` or
  `~/.config/unity-gateway`; always go through the seams above.
- Never restart or kill a running production gateway from a test.
- Gate every change set with `scripts/check.sh`. `go test ./...` alone does
  not exercise the live refresh path.
- New scripts and flows must be executed at least once (or have a `--dry-run`
  wired into `check.sh`) before being handed to a user.
