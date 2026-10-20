# Automatic model selection with Compass

Open the **Compass** tab and turn on **Enable Compass** beside **Configure**. Select **Auto(Compass)** in a connected client to use automatic routing. Requests go through the local Unity Gateway proxy and are served by the selected Databricks workspace.

The switch controls automatic routing across clients using Auto. Turning it off sends new Auto sessions and subagents to the configured fallback without calling the classifier. Existing sessions keep their model and stop reconsidering it. Manual model choices and saved routing rules are unchanged. Past usage and savings remain visible. Requests already in flight finish normally.

The equivalent CLI commands are `unity-gateway codex compass power on` and `unity-gateway codex compass power off`. Existing installations keep their previous routing behavior until you change the switch.

## Start a client

```sh
# Codex
unity-gateway codex on
unity-gateway codex picker sync
unity-gateway codex compass enable codex
codex --profile unity-gateway

# Pi
unity-gateway pi gateway
unity-gateway codex compass enable pi
pi --provider databricks --model 'Auto(Compass)'

# Claude Code
unity-gateway claude on
unity-gateway codex compass enable claude-code
unity-gateway compass run claude
```

Enable only the clients you use. Ordinary model selections bypass Compass. Model visibility and manual defaults belong to the model picker, independently of Compass.

## Models

Under **Compass → Configure**, choose the shared models and routing rules, then **Save**. **Cancel** discards the draft. Configuration is available even while Compass is off; saving does not turn it on. There are no per-client settings in this sheet.

| Choice | Initial workspace model |
| --- | --- |
| Router | GLM 5.3 Flash |
| Default / fallback | GLM 5.3 |
| High | GPT-6 Astra |
| Mid-high | GPT-6.1 Sol |
| Mid | GLM 5.3 |
| Low | Kimi K3 |

The classifier receives bounded task text and selects the least costly tier likely to handle it. These tier names express intended task complexity, not a guarantee of price ordering. Your configured models and their rates determine cost. Model IDs are fully qualified, for example `system.ai.gpt-6-astra`. Availability depends on your workspace and the client's API.

## Rules

The routing rules in **Configure** are independently selectable:

- **Session start:** classify a new session, then pin its selected model.
- **Subagents:** classify each new subagent separately when the harness supplies its identity.
- **Switch within main session:** reconsider below 25,000 observed tokens, then after 250,000 additional observed tokens since the last review. Switch only at a safe turn boundary when expected savings cover lost input-cache value, rebuilding the target cache, and classification. Missing prices, cache information, context capacity, or portable state can prevent switching.
- **Encourage subagents:** inject delegation guidance for substantial separable work. The client must support and expose a delegation tool. Trivial prompts intentionally do not need subagents.
- **Verbose:** show compact model decisions and reasons.

Use a fresh session after changing tier mappings. Existing pins stay stable unless the main-session switching rule allows a change.

## Observe decisions and savings

The **Compass** tab shows estimated savings, estimated cost, model mix, and recent decisions. Requests and traffic track all models served through the workspace, including GPT models.

```sh
unity-gateway compass watch
unity-gateway compass run --verbose codex -- --profile unity-gateway
```

Estimates use Databricks reference prices for the default tier models. Unknown prices or usage are excluded, never counted as free. See [pricing assumptions](../README.md#cost-estimates).

## Demo prompts

Use separate fresh sessions to compare task complexity:

> What is 2+2? Reply with the number only.

> Design a distributed job scheduler with crash recovery, deduplication, and fairness. Explain the failure modes and tradeoffs. Delegate independent reviews of recovery, deduplication, and fairness to available subagents, then reconcile their findings.

The second prompt explicitly requests delegation to make the demo less dependent on model choice. With encouragement enabled, a user does not have to request subagents, but an instruction cannot guarantee a tool call. Inspect actual decisions rather than assuming a prompt always selects a particular tier.
