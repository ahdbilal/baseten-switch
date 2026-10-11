# Baseten Switch

Baseten Switch is a local macOS app and gateway that routes supported AI
coding harnesses between their native providers and models served on Baseten.
One global switch controls Baseten routing, and per-client mappings select the
model that serves each request.

> **Beta:** Baseten Switch is under active development. Interfaces,
> configuration, and behavior may change between releases. The current macOS
> build is ad hoc signed and not yet notarized by Apple, so first launch may
> require approval under System Settings → Privacy & Security.

The first public release supports macOS 13 or newer on Apple Silicon and Intel.

## Quick start

```sh
brew install basetenlabs/baseten/baseten-switch
baseten-switch setup
baseten-switch up --install
baseten-switch claude on
baseten-switch doctor --probe
```

The fully qualified Homebrew command adds Baseten's public tap and installs
both Baseten Switch and its Baseten CLI dependency. It requires no GitHub
login, separate `brew tap`, second install command, or local compiler. The beta
release includes a universal macOS artifact for Apple Silicon and Intel.

`setup` verifies the Baseten CLI and its current credential, opens
`baseten auth login` when needed, and creates the initial configuration.
It never overwrites an existing configuration. `up --install` installs the
user launch agents, starts the local gateway, installs Baseten Switch.app in
`~/Applications`, and opens the app. `claude on` connects new Claude Code
sessions to the gateway. The final command checks the complete request path
with a small live request.

By default, Baseten Switch uses the current
[Baseten CLI profile](https://docs.baseten.co/reference/cli/baseten/auth) for
authentication. Both browser OAuth and API-key profiles created by
`baseten auth login` work with the gateway. Baseten controls each API key's
permissions and resource access; Switch treats the key as an opaque
credential.

To keep Switch authentication independent of CLI account changes, open
**Overview → Authentication** and save an API key. Switch uses that key until
you replace or remove it. Removing it restores CLI authentication.

If macOS blocks the app's first launch, open **System Settings → Privacy &
Security**, scroll to **Security**, and click **Open Anyway**. This control
appears after a blocked launch attempt. A managed Mac may prohibit the
override.

## Use the Mac app

The menu bar app is the primary interface for daily use. It provides:

- the global Baseten routing switch;
- Claude Code and Codex model configuration;
- current health, active fallback, and authentication status;
- recent traffic, performance, and spend views;
- actions to start the system and run it at login.

After an upgrade, `baseten-switch up` adopts the new CLI and app version.
Run `baseten-switch menubar` when you only need to install, refresh, or reopen
the app from the current Homebrew package.

The app and CLI update the same configuration. You can use either interface
without maintaining separate state.

## Pi

Open **Pi** under **Clients** in the Mac app and choose **Connect / refresh
models**. Each client page places Compass above its model picker. The default
model is saved separately from Compass; enabling Compass selects Auto(Compass)
for new sessions, and disabling it restores the manual default when available.
Restart the client to apply its default. Explicit model arguments take precedence.
Compass settings apply to both Pi and Codex. Or run:

```sh
baseten-switch pi gateway
pi --provider baseten --model 'Auto(Compass)'
```

Gateway mode adds a dedicated Pi listener at `127.0.0.1:45274`, restarts the
local gateway, and uses Switch's existing Baseten authentication. It safely
updates only the managed `baseten` provider in Pi's `models.json`; other
providers and Pi login accounts are preserved. Refresh models by running the
same command again. `pi status --json` exposes setup health to the app.

Compass pins one choice per Pi session. New or forked sessions get separate
choices. Third-party subagent extensions must pass distinct Pi session IDs and the `X-Pi-Parent-Session-Id` header to apply the subagent-specific rule; unmarked sessions use the session-start rule.
The **Compass** tab identifies Pi selections and includes its usage/savings;
**Traffic** and **Requests** include Pi gateway traffic. Direct mode bypasses
Switch and cannot provide this tracking.

OpenAI Compass tiers require an OpenAI API key passed explicitly to Pi:

```sh
pi --provider baseten --model 'Auto(Compass)' --api-key "$OPENAI_API_KEY"
```

The custom provider does not forward Pi's separate `/login` credentials.
Without an API key, use Baseten models for Compass tiers (configured on the
Pi or Codex page). Native selections without a key fail with an actionable error;
no native key is stored by the installer. The gateway provider's default
credential is a non-secret local placeholder. `pi uninstall` removes the
managed provider; the idle gateway listener remains configured.

Install the direct Baseten provider for
[Pi](https://github.com/badlogic/pi-mono) without starting Baseten Switch:

```sh
brew install basetenlabs/baseten/baseten-switch
export BASETEN_API_KEY="<your-api-key>"
baseten-switch pi install
pi --provider baseten --model <model-slug>
```

`pi install` reads the visible model catalog through the Baseten CLI and adds
only the direct `baseten` provider to Pi. It joins exact model IDs with the
public [models.dev](https://models.dev/) catalog for reasoning and text/image
input capabilities. Models without an exact capability match use conservative
defaults and are reported during installation. The API key remains in the
environment; Pi's configuration contains only a `$BASETEN_API_KEY` reference.
The command does not create gateway configuration, install launch agents,
start the router or front door, or change another harness profile.

The Homebrew formula contains the optional Mac app archive, but this flow does
not install the app in `~/Applications` or open it. Manage the direct provider
without running local services:

```sh
baseten-switch pi status
baseten-switch pi uninstall
```

## Claude Code

`baseten-switch claude on` saves the previous Claude Code environment values,
points new sessions at Baseten Switch, enables deferred tool loading, and
enables Claude Code's attribution block for compatibility with Anthropic's
Auto mode checks. Restart Claude Code after enabling or disabling the integration.

New installations also append the configured Baseten rows to Claude Code's
`/model` picker. Claude continues to own and update its native model choices.
The picker rows use stable aliases from `model_aliases`, so selection and
routing refer to the same model identity.

Useful controls:

```sh
baseten-switch on
baseten-switch off
baseten-switch claude status
baseten-switch claude route
baseten-switch claude route sonnet zai-org/GLM-5.2
baseten-switch claude route sonnet native
baseten-switch claude subagents zai-org/GLM-5.2
baseten-switch claude picker status
baseten-switch claude picker enable
baseten-switch claude picker add <baseten-model-slug> --dry-run --json
baseten-switch claude picker add <baseten-model-slug> --alias <alias>
baseten-switch claude picker remove <alias>
baseten-switch claude picker sync
```

`on` and `off` change one global routing switch. Saved model mappings remain
editable while routing is off. A Claude family can map to `native`, a
configured alias, or a Baseten model slug. Run
`baseten-switch claude route <family> default` to remove a family override.
On an existing installation without picker configuration,
`claude picker enable` enables an empty picker. Add each Baseten model you
want to expose with `claude picker add`; enabling the picker does not add
routing aliases as picker rows.
Picker add, remove, and reorder operations do not delete routing aliases or
change a running Claude Code session. Reopen `/model` after a change, and
restart Claude Code if the picker has not refreshed.

To restore the Claude Code setting that existed before setup:

```sh
baseten-switch claude off
```

## Codex CLI

Codex support is opt-in. Install
[Codex CLI](https://github.com/openai/codex), start Baseten Switch,
and create its managed profile:

```sh
baseten-switch codex on
baseten-switch codex route zai-org/GLM-5.2
baseten-switch codex status
codex --profile baseten
```

The first `codex on` may request permission to enable the parked Codex
listener. Baseten Switch writes `~/.codex/baseten.config.toml`; it does not
modify `~/.codex/config.toml`. Start Codex without `--profile baseten` to use
native OpenAI routing.

Remove the managed profile and restore any file it replaced:

```sh
baseten-switch codex off
```

To select account models from Codex's `/model` picker:

```sh
baseten-switch codex picker sync
codex --profile baseten
# Open /model and select a Baseten entry.
```

Sync requires an authenticated `baseten` CLI and writes a local Codex model
catalog beside the managed profile. When Codex's `models_cache.json` is
available, sync includes its native OpenAI entries with their original metadata.
The combined profile uses Codex's existing authentication: Codex refreshes its
credentials and sends them only to the local gateway. Native IDs route to
OpenAI (ChatGPT sign-in uses the official Codex Responses endpoint); Baseten
slugs route to Baseten, with native credentials stripped. No login tokens are
copied into the model catalog. Run plain `codex` once if its native cache is
missing, then sync again to include OpenAI models.

Run sync again to refresh account models.
Restart Codex after syncing. The catalog disables unsupported verbosity and
reasoning-summary parameters for Baseten entries. Explicit Baseten choices route to that Baseten
model. **Auto(Compass)** replaces the old compatibility/default entry.
Sync migrates an existing compatibility-model selection to Compass while
preserving explicit OpenAI or Baseten model choices.

Compass uses **GLM-5.3 Fast** to classify the first prompt and bounded recent
conversation context once per Codex thread. Each subagent thread gets its own
decision. The default tiers are Astra (high), Sol (mid-high), GLM-5.3 (mid),
and Kimi K3 (low). Failed or invalid classification uses the configurable
default, initially GLM-5.3. Missing thread identity also uses that default.

Open **Codex → Compass · Auto routing** in the Mac app to edit the routing
model, default/fallback, and four tier assignments. Saved choices apply to
new sessions and subagents. Existing decisions remain fixed across turns and
gateway restarts. Settings live in `compass.json` beside `gateway.yaml`;
`compass-sessions/` stores hashed thread keys and decisions, never request
bodies. The **Compass** sidebar page, directly below **Requests**, refreshes
up to 20 recent decisions every five seconds while open. It shows the actual
model, tier, session/subagent identifier, and selection time; expand a row for
the reason and routing model. Fallbacks are highlighted. These are saved
selections, not a live running/finished status. The page also groups recorded
Compass traffic by actual model over the last 24 hours, with estimated token
cost and same-token savings against a selectable comparison model (initially the high-tier model). Comparison
uses cached API rates and only requests with complete usage and both prices.
A reviewed October 11, 2026 [OpenAI official pricing](https://developers.openai.com/api/docs/pricing)
fallback fills missing Astra, GPT-6.1 Sol, and GPT-6 Luna prices, including
cached input, Fast mode, and the above-272K prompt surcharge. Previously
unpriced Compass requests can be estimated with newly available rates; the UI
counts these separately and stored telemetry is never rewritten. Other
unpriced requests are excluded. It excludes classifier overhead and does not
represent ChatGPT subscription or billed savings. History reads are bounded
to the latest 5,000 gateway events / 8 MiB, with partial coverage indicated.
Traffic includes OpenAI alongside Claude and Baseten in model/provider cost
and performance views. The routing
call sends bounded task context to the configured Baseten routing model and
adds a small inference request before the first answer. Its token cost is
not currently included in the gateway's main request telemetry.

```sh
baseten-switch codex compass status --json
baseten-switch codex compass set router zai-org/GLM-5.3-Fast
baseten-switch codex compass set default zai-org/GLM-5.3
```

In the local Mac app, open **Codex → Codex model picker** to search models
and toggle which appear. Click **Refresh models** to sync the catalog. Hidden
choices stay hidden across refreshes. Hiding a model does not reroute an
already-running session; restart Codex to apply the visible list.

CLI controls are also available:

```sh
baseten-switch codex picker hide zai-org/GLM-5.2
baseten-switch codex picker show zai-org/GLM-5.2
baseten-switch codex picker list --json
```

Use `baseten-switch codex picker status` to inspect the catalog, or
`baseten-switch codex picker disable` to restore the compatibility model and
remove the profile's catalog setting. Restart Codex after disabling it.
The catalog file is retained. This picker integration targets Codex CLI;
it does not change the Codex desktop app's model menu.

Without the picker, select the upstream model using `baseten-switch codex
route`. Avoid overriding the compatibility model with an OpenAI model ID:
its metadata can cause Codex to send parameters Baseten does not support.

## Status and troubleshooting

```sh
baseten-switch status
baseten-switch status --verbose
baseten-switch doctor
baseten-switch doctor --probe
```

`status` summarizes the router, front door, Mac app, authentication, and
client routing state. `doctor` inspects the same path without changing it and
prints the first failure with a concrete fix. `doctor --fix` can apply
supported repairs after confirmation.

Baseten Switch stores configuration, local state, logs, and telemetry under
`~/.config/baseten-switch/`. The primary logs are:

```text
~/.config/baseten-switch/logs/router.log
~/.config/baseten-switch/logs/door.log
```

Run `baseten-switch auth login` if the CLI credential expires, is rotated,
or needs to change. The command delegates authentication to the Baseten CLI,
reloads the gateway, and prints the current identity. `status` identifies the
selected profile as OAuth or API key. The gateway also watches the selected
CLI profile for login, logout, rotation, and profile changes.

A key saved in Switch takes precedence over CLI authentication. Manage it in
**Overview → Authentication**, or read a key from a local file through stdin:

```bash
baseten-switch auth api-key set < /path/to/key-file
baseten-switch auth api-key remove
```

On macOS, Switch stores the key in Keychain, separately from Baseten CLI
credentials, with a distinct entry for each configuration path. Existing
owner-only `.api-key` files migrate to Keychain; Switch verifies the stored key
before removing the file. Other operating systems retain the owner-only file
beside the configuration. Keys are never included in gateway YAML or status
responses.

A saved key works without a CLI login. If Keychain is locked, access is denied,
or a saved credential cannot be read, Switch reports an error instead of
substituting CLI credentials. Restore access or replace/remove the saved key to
recover. Saving checks the key's format; provider permissions are checked when
it is used.

`BASETEN_API_KEY` is a separate environment fallback, not the selected CLI
profile. Switch uses it only when `BASETEN_SWITCH_API_KEY_FALLBACK=1` and no
saved key or selected profile credential is available. A saved key takes
precedence over a selected OAuth or API-key profile, followed by the explicitly
enabled environment fallback.

### Why are Claude Code Auto permission checks failing?

Run `baseten-switch doctor`. If it reports disabled attribution, run
`baseten-switch doctor --fix`, approve the repair, and restart Claude Code.

## Upgrade

Packaged releases check for a newer version once a day. The Mac app shows an
update notice in its main window and a blue marker with an **Update Available**
action in its menu bar. **How to Update** opens release notes and copyable
installation instructions. Updates are installed manually; checking never
restarts the gateway or changes routing.

The CLI can check explicitly, including when the gateway is stopped:

```sh
baseten-switch update check
baseten-switch update check --refresh
baseten-switch update check --json
baseten-switch update automatic off
baseten-switch update automatic on
```

Checks use the public Homebrew formula version and confirm its matching
published GitHub release, including numeric beta releases. They require no
GitHub or Baseten credentials. Successful results are cached for 24 hours;
failed checks preserve the last confirmed release and back off before retrying.
`--refresh` forces a manual check, even when automatic checks are off.

Interactive `status`, `up`, and `doctor` commands can show a short update hint
on stderr. Piped output, JSON, help, and version output stay unchanged. Automatic
checks do not run for development builds or the separate Preview app/runtime.
Versions shipped before release discovery was added need one normal upgrade
before they can discover subsequent releases.

For Homebrew installations:

```sh
brew update && brew upgrade baseten-switch
baseten-switch up
baseten-switch doctor
```

`up` leaves healthy current components alone and moves stale components to the
new binary and app. Homebrew remains the canonical public install and upgrade
channel.

Finish active coding requests before running `up` to adopt newly installed
components. The update window detects a Homebrew or Nix CLI installation before
showing package-manager commands; other installations use the release's
installation guidance.

## Uninstall

Inspect the exact removal first:

```sh
baseten-switch uninstall --dry-run
```

Then remove managed harness settings, processes, launch agents, runtime
residue, and the Mac app:

```sh
baseten-switch uninstall
brew uninstall baseten-switch
```

The default uninstall retains configuration, telemetry, logs, and backups.
To remove those files as well, use this instead of the standard uninstall:

```sh
baseten-switch uninstall --purge --yes
brew uninstall baseten-switch
```

Uninstall never removes Baseten CLI credentials or keychain entries. If the
app's Start at Login item prevents safe bundle removal, the command prints the
manual action required in macOS System Settings.

## Privacy and trust

Baseten Switch binds its services to the local loopback interface. It receives
harness requests and credentials because it is in the selected request path.
Request content leaves the machine only for the upstream chosen by the active
routing policy. Baseten credentials go only to Baseten, and native credentials
go only to their matching native provider.

Baseten-routed inference requests identify Switch and its version in the
standard `User-Agent` header and `X-Baseten-Switch-Version`. Switch does not
add a user or installation identifier. Switch does not add these markers to
native-provider requests.

### Compatibility routing

Switch may route narrowly identified compatibility requests to a harness's
native provider even when ordinary requests are mapped to Baseten. For example,
recognized Claude Code Auto permission checks are sent to Anthropic and labeled
`Auto check` in the Requests view. These checks require credentials accepted
by Anthropic, and a native failure is returned directly instead of being
retried through Baseten. Other requests continue to follow the configured
routing policy.

Local telemetry contains request metadata, not prompts, responses,
credentials, headers, or request bodies. Disable future records by setting
`telemetry_enabled: false` in `gateway.yaml`, then reload the configuration.
Delete existing records from
`~/.config/baseten-switch/telemetry/`.

Request and response body capture is a separate, disabled-by-default local
feature. If an operator explicitly enables `global.trace_capture`, Switch can
store exact bodies in a separate private trace store. Those bodies can contain
prompts, responses, reasoning content, tool data, source code, credentials,
personal data, and regulated data. Switch does not upload captured traces.

Public model-catalog refreshes from models.dev send no credential or
user-derived data. The unauthenticated administration API binds to loopback
and must never be exposed on a network interface.

## Configuration

The generated configuration is
`~/.config/baseten-switch/gateway.yaml`. Prefer the Mac app or typed CLI
commands over direct edits; routing changes hot-reload without a restart.

See [config/schema.md](config/schema.md) for every field and
[config/gateway.example.yaml](config/gateway.example.yaml) for the generated
shape. The Baseten CLI owns selected-profile credentials. If you explicitly
enable the environment fallback, store `BASETEN_API_KEY` and
`BASETEN_SWITCH_API_KEY_FALLBACK=1` in
`~/.config/baseten-switch/env`, which must use mode `0600`, rather than in
`gateway.yaml`.

## Build and test

The Go module in `gateway/` builds the CLI, router, administration API, and
front door. The Swift package in `mac/BasetenSwitch/` builds the native menu
bar app without external Swift packages.

```sh
scripts/build.sh
scripts/check.sh
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution and dependency rules.
See [TESTING.md](TESTING.md) for test layers and isolation requirements.

## Nix source build

The Nix flake is an alternate source build for Linux and Apple Silicon macOS.
It does not include the signed Mac app or install the Baseten CLI dependency:

```sh
nix profile install github:basetenlabs/baseten-switch#baseten-switch
```

Upgrade and restart the locally running components:

```sh
nix profile upgrade --refresh baseten-switch
baseten-switch up
```

Homebrew is the supported path for the complete macOS product.

## License

Baseten Switch is available under the [MIT License](LICENSE).

### Advanced Compass rules

Under **Compass → Advanced options**, **Auto routing** contains the six model
selectors and **Routing rules** contains compact toggles. Picker visibility
remains independent. Rules and routing tiers are shared by Pi, Codex, and Claude Code.
Existing session-start and subagent classification default to on; disabling one
uses the fallback for new applicable sessions. Session switching, subagent
encouragement and verbose output default to off in older and new configurations.

Main-session switching considers a change only below $1 or above $10 of recorded
main-session model cost. It holds at $1 through $10. There are no threshold
controls in the UI. A conservative five-request forecast includes cold target
input, cache-write pricing where available, output and a routing-call allowance,
while crediting the current model's observed cached prefix. It requires at least
20% and $0.10 expected net savings. These are estimates, not billing guarantees.
Unknown accounting/pricing, old sessions lacking complete spend history, nonstandard
speed, overlapping requests, incomplete tool exchanges, opaque stored responses,
images/files or unknown target context limits keep the current model. Reviews
are deduplicated per user input; late reviews require $1 additional spend,
and at most two switches occur, separated by five completed requests.

If a selected Baseten model returns HTTP 429 or 5xx before a response starts,
Compass may retry once using its configured Baseten default/fallback model.
The existing 429/5xx fallback switches still apply. A completed retry pins that
model for subsequent session turns and appears in traffic as a fallback.
Native OpenAI selections and opaque/encrypted conversation state are not
rerouted this way. This availability recovery is separate from optional
cost-based session switching.

Subagent encouragement adds a stable, bounded instruction only to newly pinned
main sessions. It does not add subagent tools, override user instructions, or
promise that the harness will delegate.

Use the optional terminal decision feed:

```sh
baseten-switch compass watch --verbose
baseten-switch compass run --verbose codex -- --profile baseten
baseten-switch compass run --verbose pi -- --provider baseten --model 'Auto(Compass)'
```

Without `--verbose`, the run wrapper follows the Verbose CLI rule. Messages go
to stderr and never enter the conversation. The feed labels client and session;
it includes new decisions from concurrent sessions of that client, not just the
launched process. Interactive clients may redraw over wrapper output; `watch`
in a separate terminal provides a persistent view. This is a local wrapper,
not a patch to Codex's or Pi's native slash commands.

For automation, `codex compass rule <name> on|off` accepts `session_start`,
`subagents`, `session_switching`, `encourage_subagents`, and `verbose`.
`codex compass enable|disable codex|pi` changes the saved selection.
`codex compass default codex|pi <model>` saves the manual default.
Status supports `--period 24h|7d|30d --client all|codex|pi`. History remains
bounded; the overview labels partial retention/coverage explicitly.

### Claude Compass and client layout

Codex, Pi, and Claude Code show Compass above their model picker. Advanced
opens one inset panel containing model choices and routing rules. Picker defaults
remain outside Compass; Claude's existing curated picker and manual family routes
remain available. Pi lists its provider models and manual default.

Claude Compass preferences are launch defaults owned by Switch. They do not enable
Claude's gateway connection or modify Claude's settings. Connect Claude explicitly
when needed, then launch it through Switch:

```sh
baseten-switch codex compass enable claude-code
baseten-switch compass run claude
```

Use `baseten-switch codex compass default claude-code sonnet` for the manual default
and `baseten-switch codex compass disable claude-code` to restore it. Explicit
`--model` arguments take precedence. `--verbose` works with the same launcher.
Claude's session metadata and agent header provide separate session/subagent pins;
requests without a session identity use the fallback. Agents must inherit the
Compass model to use Compass; an explicitly selected agent model stays explicit.

Claude Messages use the gateway's existing provider translation and usage capture.
OpenAI Compass tiers require `OPENAI_API_KEY` in the **gateway process environment**;
Claude's subscription credentials are never sent to OpenAI. Baseten tiers use the
existing Switch login. Main-session re-routing shares the cost/cache safeguards,
and only text-only Claude history can switch; tools, images, and signed thinking
keep the current selection. The Compass dashboard includes Claude usage.

For Baseten's DeepSeek V4.1 Responses models, Switch removes unsupported response
`include` extensions (such as encrypted reasoning and logprobs). GLM, native OpenAI,
and input history are unchanged. DeepSeek V4.1 also omits unsupported optional
hosted web-search tools; client functions, namespaces, and apply_patch are preserved.
Explicitly required hosted search fails clearly instead of being silently removed.

Managed Claude installations may enforce a company gateway and a model allowlist.
Those policies override local Compass settings; Switch does not bypass them.
An administrator must permit the local gateway and Compass model before that
Claude installation can use this integration.
