# Official ug backend

The desktop app uses the official [Databricks Unity Gateway](https://github.com/databricks/unity-gateway) package for workspace authentication and client configuration builders. This version pins commit `e0958c1f6bf380ed7542d0f6d28e5577ad81cf0e` rather than depending on whichever `ug` happens to be on PATH.

```sh
unity-gateway ug install
unity-gateway ug status
```

The installer requires `uv` and creates a Python 3.12 environment under `~/.local/share/unity-gateway/ug-runtime/<revision>`. It leaves a separately installed global `ug` untouched. The bridge verifies the package's installed Git revision before using its builders. An incompatible or missing runtime stops configuration changes with a repair command.

## Request path

`Client → local proxy (Compass + telemetry) → Databricks workspace`

The official builders produce each client's provider/authentication settings. The app changes their base URL to its dedicated loopback listener, adds its routing marker and model catalog, then applies only its owned configuration. The generated authentication helpers and the proxy use the pinned `ug auth-token`; explicit sign-in lets ug handle browser login. Tokens are not copied into client configuration files.

The app keeps its existing proxy because it owns session and subagent routing, cache-aware switching, request history, and usage estimates. Upstream smart routing is disabled while rendering these configurations so a second classifier is not introduced.

## Client configuration boundaries

| Client | Configuration and removal |
| --- | --- |
| Codex CLI | Dedicated `unity-gateway.config.toml` overlay; launch with `codex --profile unity-gateway`. Removing support removes/restores the app-owned overlay. |
| Claude Code | Dedicated `claude-ug.settings.json`; launch with `unity-gateway compass run claude`. Plain `claude` keeps its normal connection. Existing model-picker rows remain narrowly managed with backup/restore in the user's settings. |
| Pi | App-owned `databricks` provider in `models.json`; other providers are preserved. |
| Claude Desktop | Gateway profile in the separate `Claude-3p` directory; prior profile state is backed up. |
| Codex Desktop | An owned provider and selected root keys in shared `config.toml`; previous values are restored on disable. Unprofiled Codex CLI launches share those defaults while enabled. |

The app does not call broad `ug configure` or `ug revert`. Existing ownership and conflict checks prevent overwriting settings changed by the user. This isolates configurations where the client supports it; it cannot turn Codex Desktop's shared settings file into an independent profile.

Reconnect an enabled integration once after upgrading to regenerate its settings with ug. Disabled integrations stay disabled. Restart the affected client afterward. Older configurations remain usable through the proxy until regenerated.

## Development validation

Ordinary Go tests exercise ownership, rollback, routing, and rendering failure handling. To additionally exercise the actual pinned Python builders against temporary client directories:

```sh
UNITY_GATEWAY_TEST_UG_RUNTIME="$HOME/.local/share/unity-gateway/ug-runtime/e0958c1f6bf380ed7542d0f6d28e5577ad81cf0e" \
  go -C gateway test ./internal/ugruntime ./cmd/unity-gateway -run UG
```

These tests do not authenticate or send inference requests. They validate generated configuration and restoration without modifying live client files.
