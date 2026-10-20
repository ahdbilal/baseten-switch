# Unity Gateway gateway

This Go module builds the `unity-gateway` CLI, local router, admin API, and
front door. See the repository [README.md](../README.md) for installation and
[TESTING.md](../TESTING.md) for validation.

## Build

From the repository root:

```sh
scripts/build.sh
```

Or build the CLI directly:

```sh
cd gateway
go build -o bin/unity-gateway ./cmd/unity-gateway
```

## Start a development instance

```sh
bin/unity-gateway config init
bin/unity-gateway up
bin/unity-gateway status
```

Use the adapters instead of editing harness configuration by hand:

```sh
bin/unity-gateway claude on
bin/unity-gateway codex on
```

The generated configuration lives at
`~/.config/unity-gateway/gateway.yaml`. Runtime state, logs, and telemetry
use the same configuration directory.

The gateway reads the current Databricks CLI profile. It supports both OAuth and
API-key profiles and reports the active type through
`GET /v1/admin/auth/status`. The separate `DATABRICKS_API_KEY` fallback requires
`UNITY_GATEWAY_API_KEY_FALLBACK=1`; a selected profile takes precedence.

## Main commands

```text
up, down, restart, status
on, off
config init, config reset
claude on|off|status|subagents|route|reasoning|picker
codex on|off|status|route|reasoning
whoami, auth login, doctor, spend
gateway start|stop|restart|status
door
```

Run `unity-gateway <command> --help` for the current flags and environment
overrides.

`claude picker` manages the ordered `model_picker` projection stored beside
the Claude client's `model_aliases`. The gateway config owns desired alias
membership and order. The Claude adapter generates presentation, safely
projects rows into user settings, and reports policy or runtime visibility
without weakening managed Claude configuration.
When no picker configuration exists, `claude picker enable` creates an enabled
picker with no rows. Add models explicitly with `claude picker add <slug>`;
enable does not copy `model_aliases` into the picker. If
`add <slug> --dry-run --json` reports multiple alias choices, repeat it with
`--alias <alias>`.
Converting an existing Claude picker from replacement mode requires the
explicit `--convert-replacement-mode` flag on `enable` or `sync`.

## Module layout

```text
cmd/unity-gateway/   CLI and lifecycle commands
cmd/gateway/          router, admin API, and request handling
internal/auth/        Databricks CLI credential reader and OAuth transport
internal/config/      configuration parsing and editing
internal/door/        front-door configuration
internal/proxy/       upstream request and response relay
internal/telemetry/   segmented request telemetry
internal/translate/   protocol translation
```

## Development checks

From the repository root, run the full gate:

```sh
scripts/check.sh
```

For a quick package-only pass:

```sh
cd gateway
go test ./...
go vet ./...
```

The full gate remains required because it also builds the Swift app and runs
isolated lifecycle and credential-store smoke tests.
