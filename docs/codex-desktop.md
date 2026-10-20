# Codex Desktop

## Enable

1. Sign in to a Databricks workspace in Unity Gateway.
2. Open **Clients → Codex Desktop**.
3. Turn on **Enable Unity Gateway**.
4. Keep **Enable Compass** on for automatic selection, or turn it off and choose a default model.
5. Use the grouped **Model picker** to select visible workspace models. Select all, deselect all, and filtering are available.
6. Quit and reopen Codex, then start a new chat.

Codex Desktop connects to the local Responses endpoint at `http://127.0.0.1:45276/v1`. The gateway authenticates to your Databricks workspace. Requests appear under **Codex Desktop** in Traffic, Requests, and Compass. The selected workspace model must support the Responses API and the tools used by Codex.

## Disable and return to OpenAI

Turn off **Enable Unity Gateway** on the Codex Desktop page. This restores the prior model, provider, catalog, and reasoning settings where Unity Gateway still owns them, removes the managed provider and catalog, and removes its dedicated listener. It preserves OpenAI credentials, history, projects, MCP servers, and unrelated edits. Changed files that cannot be safely identified as managed are preserved with an actionable error.

Quit and reopen Codex and start a new chat. Existing chats can retain their previous model/provider. If OpenAI was your prior provider, new chats use that setup again. No new OpenAI sign-in is needed unless your existing credentials have expired.

**Enable Compass** only changes automatic model selection. It does not disconnect the client. The global routing switch likewise does not remove client configuration. Use the client’s **Enable Unity Gateway** switch to disconnect it.

## Shared configuration

The Codex app reads `~/.codex/config.toml`, as described in [OpenAI’s gateway setup guide](https://learn.chatgpt.com/docs/enterprise/connect-to-a-gateway). Ordinary Codex CLI launches also read that file, so they inherit the desktop defaults while enabled. Explicit CLI profiles remain separate; Unity Gateway does not overwrite its existing CLI overlay when connecting Desktop. Requests that use the desktop endpoint are attributed to Codex Desktop, including unprofiled CLI launches using those defaults.

`CODEX_HOME` (or `UNITY_GATEWAY_CODEX_HOME` for the integration command) can point to a different directory. Use the directory your desktop app actually loads.

## CLI

```sh
unity-gateway harness on codex-desktop
unity-gateway harness status codex-desktop
unity-gateway harness off codex-desktop
```

Detailed model controls:

```sh
unity-gateway codex-desktop refresh
unity-gateway codex-desktop status --json
unity-gateway codex-desktop default system.ai.glm-5-3
unity-gateway codex compass enable codex-desktop
unity-gateway codex compass disable codex-desktop
```

The same `harness on|off|status` commands accept `codex`, `claude-code`, `pi`, and `claude-desktop`. Their UI pages all expose **Enable Unity Gateway** above Compass and the model picker. Removing support restores or removes only managed configuration; it does not erase a harness installation or account.

## Validation

Configuration tests cover repeat enable/refresh/disable, exact restoration, unrelated user edits, credentials and CLI profile preservation, symlinked files, foreign provider rejection, and interrupted refresh recovery. Integration tests cover listener isolation and Compass/manual selection.

The installed Codex app’s bundled app-server engine was also tested in an isolated home against a real workspace: initialization, configuration read, model discovery, Compass-selected streaming response, tool use, telemetry attribution, and disabling back to the original OpenAI configuration. The running desktop app’s own configuration was not changed by this validation.
