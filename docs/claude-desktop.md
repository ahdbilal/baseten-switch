# Claude Desktop with Databricks Unity Gateway

Claude Desktop has a separate gateway configuration from Claude Code. Connect it from **Claude Desktop** in the Unity Gateway sidebar, then quit and reopen Claude. You can also run:

```sh
unity-gateway desktop on
```

The integration creates a named Databricks profile in Claude Desktop's local configuration library. It preserves other profiles and MCP settings. Workspace authentication stays in Unity Gateway; the Desktop configuration contains only a local placeholder credential. The listener binds to `127.0.0.1:45275`.

## Models and Compass

The model picker includes all workspace models that support the Messages API, plus **Auto(Compass)**. This includes compatible OpenAI and open models alongside Claude. Unity Gateway groups choices by model family; choose which models appear and select a manual default. Changes apply after restarting Desktop. After upgrading from 0.8.4, click **Refresh models** or run `unity-gateway desktop refresh` to expand the saved catalog.

Use **Deselect all**, tick the models you want, then **Save selection**. **Select all** includes every catalog model, even when the list is filtered. These edits are a draft until saved; **Cancel** restores the saved selection. At least one model must be selected to save. The current default stays if selected; otherwise the first selected model becomes the default, shown before saving. Removing Auto(Compass) when it is the default switches Desktop to the displayed manual default.

Desktop filters model IDs. Auto(Compass) therefore uses the gateway route alias `claude-unity-compass`. Individual non-Claude choices use a `claude-unity-model-…` alias in the Desktop profile, while their visible labels keep the real model name. The gateway resolves these aliases to the exact workspace model before forwarding; they do not invoke Compass or silently substitute a different model. These are routing aliases, **not a claim that the selected upstream model is Claude**. Traffic and usage records retain actual model IDs. Manual defaults and visibility settings also use actual IDs.

Enable Compass from the Desktop page; its advanced model choices and rules are shared with the other clients. Desktop has separate session pins and usage attribution. Session identity must be present in the request metadata or headers. Missing identity uses the existing conservative routing behavior; unsupported tool/history boundaries prevent mid-session switching.

The alias is a compatibility technique verified against the locally tested Claude Desktop 3.1623.0 build, not an Anthropic-supported cross-provider integration. Future Desktop versions may reject it. The upstream model must support the workspace Messages API and the client's tool requests. An available catalog entry does not guarantee that its workspace deployment is currently routable.

## Status and removal

```sh
unity-gateway desktop status --json
unity-gateway desktop refresh
unity-gateway desktop default system.ai.claude-sonnet-4-6
unity-gateway desktop visibility system.ai.claude-haiku-4-5 off
unity-gateway desktop off
```

Disconnect restores the prior active profile and deployment mode if this integration still owns that selection. It preserves a newer user selection and keeps the inactive gateway profile for recovery. Quit and reopen Claude afterward. Configuration outside Unity Gateway is never silently overwritten; if its profile has changed, resolve that edit before reconnecting.

Organization-managed configuration takes precedence over local profiles. Devices with an active hybrid organization configuration must switch to a local profile before connecting. The connector currently manages macOS Desktop configuration.

## Validation

The integration is tested for profile/default/visibility updates, preservation of existing settings, conflict detection, a dedicated listener, alias resolution, and separate parent/subagent/session pins. Live testing additionally checks Desktop's actual picker, a response through Databricks, and tool use. Test the chosen Compass router and tier deployments in your workspace before a demo: an upstream `Could not route request` response cannot be fixed by changing the Desktop alias.

Official setup reference: [Claude Desktop gateway configuration](https://claude.com/docs/third-party/claude-desktop/gateway).
