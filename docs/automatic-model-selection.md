# Automatic model selection with Compass

Compass selects a model for a task using a routing model and the tiers you configure. It runs in the local Databricks Unity Gateway gateway and works with the fork's Codex, Pi, and Claude Code integrations.

Use a build from **ahdbilal/baseten-switch** containing these changes. Installing the upstream Homebrew release does not necessarily include this fork's Compass features. Install the matching CLI and Mac app together, then restart the gateway and your client.

## 1. Connect your client

Sign in to Baseten and start Switch if you have not already:

```sh
baseten-switch setup
baseten-switch up
```

In the Mac app, turn routing on and connect the client you want to use. The client page has **Compass above the model picker**. The picker controls visible models and the manual default independently of Compass.

### Codex

```sh
baseten-switch codex on
baseten-switch codex picker sync
baseten-switch codex compass enable codex
codex --profile baseten
```

In `/model`, choose **Auto(Compass)** if it is not already selected. Restart Codex after refreshing the picker. Native OpenAI models use Codex's own sign-in; Baseten models use Switch's Baseten credentials. Run normal Codex and sign in first if its OpenAI model cache is missing, then sync again.

`codex --provider baseten` is not a supported command; use `--profile baseten`.

### Pi

```sh
baseten-switch pi gateway
baseten-switch codex compass enable pi
pi --provider baseten --model 'Auto(Compass)'
```

In **Clients → Pi → Model picker**, check the models to show in `/model` and choose a manual default, just as in Codex. Run `/reload` or restart Pi after changing visibility. Refreshing models preserves your selections; keep at least one model selected. Visibility only changes the picker, not the models Compass may route to.

Gateway mode supplies routing and usage tracking. Direct Baseten mode bypasses Switch. To use OpenAI tiers through Pi, supply an OpenAI API key with Pi's `--api-key` option; otherwise configure Baseten models for every tier. Pi's separate login credentials are not forwarded automatically.

### Claude Code

Connect Claude explicitly; enabling Compass alone does not enable its gateway connection:

```sh
baseten-switch claude on
baseten-switch codex compass enable claude-code
baseten-switch compass run claude
```

Claude uses the same Compass controls. OpenAI tiers require `OPENAI_API_KEY` in the gateway process environment; Claude subscription credentials cannot authenticate OpenAI. Company-managed gateway/model policies must permit this integration. Switch does not bypass them.

## 2. Configure automatic selection

Enable **Compass** on the client page. Under **Advanced options → Model choices**, choose:

| Setting | Purpose |
| --- | --- |
| Routing model | Classifies the task; GLM 5.3 Fast is the default router. |
| Default / fallback | Used when classification fails or is disabled. |
| High | Your strongest model for demanding work. |
| Mid-high | A strong general coding/reasoning model. |
| Mid | A balanced model. |
| Low | An economical model for simple work. |

An example mapping is Astra / Sol / GLM 5.3 / Kimi, using models available to your account. Assign different models to tiers if you want different routing outcomes. A tier can still select the same model as another tier. Router choices are dynamic; a particular prompt does not guarantee a particular tier.

Choices and rules are shared across client pages. Start a fresh session after changing them. Explicit model selections bypass Compass. Subagents must inherit Auto(Compass) to receive their own routing decision.

## 3. Choose routing rules

Under **Advanced options → Routing rules**:

- **Select at session start:** classify a new main session. Enabled by default; disabling it uses the fallback.
- **Select for each subagent:** classify each identified child session separately. Enabled by default; disabling it uses the fallback for children.
- **Switch within main session:** optional token-triggered reviews with cache/cost protection. Disabled by default.
- **Encourage subagents:** optional instructions encouraging delegation of useful, bounded work. Disabled by default.
- **Verbose CLI decisions:** optional compact decision feed. Disabled by default.

The same rules can be changed from the CLI:

```sh
baseten-switch codex compass rule encourage_subagents on
baseten-switch codex compass rule session_switching on
baseten-switch codex compass rule verbose on
```

### Encouraging subagents

The prompt asks the model to delegate at least one useful bounded subtask on substantial work, unless the task is small, tightly sequential, or delegation would cost more than it saves. It asks the parent to continue useful work, review child results, and integrate them. Trivial work stays local.

**The user does not need to mention subagents.** The client must expose working subagent tools and permit their use. This is an instruction, not forced execution or a guarantee. Pi extensions must supply distinct child session IDs and `X-Pi-Parent-Session-Id` for child-specific routing.

Use normal saved Codex sessions for demos. In testing, `--ephemeral` runs could fail to spawn a child with “no thread with id”; a normal saved session worked.

### Switching within the main session

Reviews are eligible below **25,000 observed session tokens**, then after **250,000 additional observed tokens since the last review**. Counts include output and all input, including cached input counted once. These fixed thresholds replace the earlier dollar-based windows.

A review does not guarantee a switch. At a portable user-turn boundary, Switch compares keeping the current cache with cold input on the target, output costs, cache-write rates, and a routing allowance. It requires at least 20% and $0.10 projected savings over a conservative five-request forecast.

Missing recent usage or comparison prices, overlapping requests, active tool exchanges, encrypted/nonportable history, or unknown target capacity can keep the current model. At most two switches occur, separated by five completed requests. Interrupted or missing-usage requests disable the early window, but later reviews can recover once enough confirmed tokens accumulate. Older ledgers start a conservative lower-bound token counter rather than inventing missing history.

## 4. See the selected model and reason

Keep a separate terminal open:

```sh
baseten-switch compass watch --verbose
```

Or launch Codex with the decision feed:

```sh
baseten-switch compass run --verbose codex -- --profile baseten
```

The feed includes concurrent sessions for that client; identify the session/child in each message. A separate `watch` terminal is easier to read when a client redraws its screen.

In the Mac app:

- **Compass:** model mix, estimated savings, recent decisions, and reasons for keeping the current model.
- **Requests / Traffic:** actual served models, including native OpenAI traffic through the gateway.

Savings compare the same recorded tokens against the configured comparison model using API rates. They exclude classification overhead and are not actual invoice or subscription savings. Missing usage cannot be reconstructed by fetching prices. Portkey supplies fallback prices for supported exact OpenAI/Anthropic model IDs; Baseten prices come from its catalog.

## 5. Demo without explicitly asking for subagents

Enable **Encourage subagents** and start a fresh normal Codex session in this repository with Auto(Compass). Paste:

> Review gateway/internal/compass/switching.go and gateway/internal/pricing/portkey.go for correctness. Check two areas: token-based review eligibility and recovery after interrupted requests; and pricing accuracy, cache protection, and price-source metadata. Give me one combined report with up to three concrete findings, file references, and a short verdict. If you find no bugs, say so. Keep this review bounded to these files and their immediate helpers/tests. Do not modify files or run broad test suites.

This exact prompt was tested without any subagent mention: the parent spawned a child, waited for it, and produced a combined report in about three minutes. Both were independently routed to Baseten GLM 5.3. Different decisions may legitimately choose the same model.

For contrasting task difficulty, try the following with distinct high/low tier assignments:

> Review Compass's routing architecture for concurrency hazards, interrupted-request recovery, and cache-cost correctness. Before starting your analysis, delegate a separate clerical task to a subagent: list every Test function in gateway/internal/compass/switching_test.go, with its line number. Tell the child to only list names and lines, with no analysis or recommendations. Let it inherit Auto(Compass); do not specify a model. Continue the architecture review yourself, then combine both results.

This second prompt is a suggested demo, not a verified guarantee of different models. Parent/child selection is separate from changing the parent's model within a session.

## Validation and troubleshooting

- Real Codex Compass usage capture and automatic delegation without explicit user prompting have been verified.
- Controlled HTTP integration tests cover token-triggered switching, interruption recovery, warm-cache protection, and missing-usage holds.
- A successful model switch within a real CLI main session has **not** yet been verified. Encrypted history can legitimately block it.
- A real Claude CLI run on the development machine was blocked by managed policy; direct Messages protocol tests passed.
- Go checks and CLI smoke tests passed. The full native test gate is blocked on the development machine by missing XCTest; release app compilation and signature verification are separate checks.

If Auto(Compass) is missing, refresh the client's models and restart it. If it stays on a model, inspect the decision/hold reason rather than clearing the session accounting. If there is no delegation, confirm a fresh session, the enabled rule, available subagent tools, and a task with useful independent work.
