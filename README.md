# Databricks Unity Gateway desktop

A local desktop app and CLI that connects Codex, Pi, and Claude Code to your Databricks workspace. Compass automatically selects a workspace model at session and subagent boundaries. The app provides model pickers, routing controls, request history, model mix, and estimated savings.

The app changes each enabled client's configuration to use a loopback proxy. It does not intercept unrelated network traffic. Inference and classification use Unity Gateway in your selected Databricks workspace.

## Install and sign in

Download the matching macOS app and CLI from [Releases](https://github.com/ahdbilal/baseten-switch/releases), extract the ZIP, then run `bash install.sh` from the extracted directory. Apple Silicon is supported by the packaged build.

Install the dependencies if needed:

```sh
brew install databricks/tap/databricks
uv tool install git+https://github.com/databricks/unity-gateway
```

Then connect your workspace:

```sh
unity-gateway setup
unity-gateway up --install
```

You can also enter the workspace URL and select **Sign in** in the app. Authentication uses your Databricks workspace profile; individual model-provider keys are unnecessary.

## Clients

| Client | Configure | Start |
| --- | --- | --- |
| Codex | `unity-gateway codex on` then `unity-gateway codex picker sync` | `codex --profile unity-gateway` |
| Pi | `unity-gateway pi gateway` | `pi --provider databricks --model 'Auto(Compass)'` |
| Claude Code | `unity-gateway claude on` | `unity-gateway compass run claude` |

Enable **Compass** on the client page. The independent model picker controls visible models and the manual default. **Compass → Advanced options** contains tier mappings and routing rules. All three client pages share these controls; their model inventories follow their supported APIs.

Pi discovers workspace models supporting Responses, Anthropic Messages, or Chat Completions, and selects the API per model. Use **Pi → Refresh models** (or `unity-gateway pi gateway`) after upgrading. A fresh `pi` session uses the migrated Databricks default; resumed sessions can retain their previous provider selection. Old defaults and model filters are backed up before migration.

For compact routing messages alongside the client:

```sh
unity-gateway compass run --verbose codex -- --profile unity-gateway
```

See the [Compass guide](docs/automatic-model-selection.md) and [workspace connection guide](docs/databricks-workspace.md).

## Upgrading

Install the app and CLI from the same release, restart the gateway, then restart your clients. `unity-gateway update check --refresh` checks published desktop releases.

The installer migrates an existing workspace connection and routing preferences into `~/.config/unity-gateway`. Original configuration and old request history are retained at the previous location for rollback. Old provider credentials, price caches, and pinned session decisions are not imported. Client pickers are refreshed for the new profile.

## Cost estimates

Compass records usage and estimates cost using Databricks reference list prices for its default models: GLM 5.3 Flash, GLM 5.3, Kimi K3, GPT-6 Astra, and GPT-6.1 Sol. Estimates assume standard serving, global processing and $0.07/DBU; negotiated rates, region uplifts, priority pricing, and invoices can differ. Unavailable prices or token counts remain unknown. Long-context Astra/Sol pricing is conservatively unavailable above 128k input tokens until its applicable threshold is known.

Main-session switching uses early/late token gates (below 25k observed tokens, then after each additional 250k), and cache rebuilding checks. Missing prices or unsafe conversation state can still prevent a switch. Subagent encouragement is an instruction to the client model; the client must expose a working delegation tool.

## Development

```sh
./scripts/build.sh gateway
go -C gateway test ./...
./scripts/build-menubar.sh
```

The executable is `gateway/bin/unity-gateway`; the macOS source is `mac/UnityGateway`. The Go module is `unity-gateway/gateway`. Local state defaults to `~/.config/unity-gateway`; `UNITY_GATEWAY_CONFIG_PATH` selects an isolated configuration.
