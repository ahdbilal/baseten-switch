# Workspace connection

The desktop app connects clients through a local proxy to Databricks Unity Gateway. It saves the workspace URL and Databricks profile in `~/.config/unity-gateway/unity-gateway.json`. Tokens stay with `ug` and the Databricks CLI.

```sh
unity-gateway auth login --workspace https://YOUR-WORKSPACE.cloud.databricks.com --profile unity-demo
unity-gateway whoami
```

Without arguments, login uses the saved app connection, then the workspace selected in `ug`. Browser authentication opens when the credential needs renewal. Custom OAuth profiles saved by `ug` are respected.

Model discovery reads the paginated Unity Catalog model-services inventory and its `supported_api_types`. Codex and Pi use models supporting the Codex Responses API. Claude uses models supporting Anthropic Messages. Astra and Sol are workspace models when available; no direct OpenAI credentials are required.

Requests use these workspace endpoints:

| Purpose | Endpoint |
| --- | --- |
| Codex and Pi | `/ai-gateway/codex/v1/responses` |
| Claude Code | `/ai-gateway/anthropic/v1/messages` |
| Compass classifier | `/ai-gateway/mlflow/v1/chat/completions` |

The proxy preserves tool definitions, tool results, reasoning, and streaming. It removes client authorization, API keys, cookies, and ChatGPT account headers before attaching workspace OAuth. It rejects redirects to another origin. When the local router is unavailable, the door returns an error rather than contacting a different provider.

Select a new workspace in the app, refresh the client pickers, and start fresh client sessions. Model access and supported APIs can differ by workspace.

Cost and savings figures are reference estimates, not billed charges. See the [pricing assumptions](../README.md#cost-estimates). The external-provider catalog does not supply the rate for Databricks pay-per-token foundation models and is not substituted for it.
