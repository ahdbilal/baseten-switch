# Databricks workspace connection

The desktop app connects Codex, Pi, and Claude Code to a Databricks workspace.
The local proxy still provides Compass routing and request telemetry. Inference
runs on Unity Gateway; the proxy does not intercept unrelated traffic.

## Sign in

Install the Databricks CLI and [Unity Gateway](https://github.com/databricks/unity-gateway)
(`uv tool install git+https://github.com/databricks/unity-gateway`). In the app,
enter a workspace URL under **Databricks workspace** and choose **Sign in**.
Leaving the field blank reuses the saved workspace.

CLI equivalent:

```sh
baseten-switch auth login --workspace https://YOUR-WORKSPACE.cloud.databricks.com --profile unity-demo
```

Without arguments, login reuses the app's saved connection, then `ug`'s current
workspace. Existing custom OAuth profiles saved by `ug` are respected. If the
credential needs renewal, the Databricks CLI opens browser authentication.
Tokens are fetched with `ug auth-token`; the app saves only workspace and profile
in `unity-gateway.json` alongside `gateway.yaml`. It does not copy tokens into
client configuration. `ug configure` is not invoked because it also edits shared
client settings.

## Models and Compass

Refresh the picker after login, then restart your client. For Codex:

```sh
baseten-switch codex picker sync
baseten-switch on
codex --profile baseten
```

Select **Auto(Compass)**. Compass uses the workspace's GLM 5.3 Flash when available
as its classifier. Legacy tier choices migrate to models actually present in
the workspace; the previous Compass configuration is backed up as
`compass.json.before-unity`. In a workspace without Astra or Sol, those models
are not offered as direct OpenAI fallbacks. Choose tier mappings under Compass's
advanced options.

The shared picker currently uses the Codex-compatible workspace catalog. This
provides common choices for all three clients; additional Anthropic-only models
are not yet included in that shared picker. Model names use fully qualified
identifiers such as `system.ai.glm-5-3`.

## Request flow

1. Your client sends requests to its configured local proxy port.
2. Compass chooses a model on session/subagent start when enabled.
3. The proxy attaches a fresh workspace credential and forwards native payloads:
   - Responses: `/ai-gateway/codex/v1/responses`
   - Anthropic: `/ai-gateway/anthropic/v1/messages`
   - Classifier/chat completions: `/ai-gateway/mlflow/v1/chat/completions`
4. Responses stream back through the proxy; usage and routing decisions remain local.

Client API keys, ChatGPT account headers, and cookies are removed before forwarding.
Redirects cannot forward workspace credentials to a different host. Baseten-specific
tool stripping and request translation do not run on workspace requests.

Workspace pricing is not inferred from Baseten prices. Missing prices remain unknown;
cache-protected mid-session switching stays conservative when pricing is unavailable.
Subagent encouragement remains a prompt; the harness must expose subagent tools.

## Verify

```sh
baseten-switch whoami
baseten-switch compass watch
```

Start a fresh client session and ask “What is 2+2?” Check that the decision shows
a `system.ai.*` model. Use a fresh session for a harder task to compare selections.

The executable, `baseten` client profile/provider ID, and physical app bundle path
retain their old names so existing installations can upgrade without losing settings.
