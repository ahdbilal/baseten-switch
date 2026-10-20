# Native workspace migration validation

Validated October 20, 2026, on Apple Silicon macOS.

- Full Go test suite passed, including protocol preservation, session pinning, subagent identities, cache protection, usage accounting, configuration ownership, and restore behavior.
- Added workspace-specific checks for authenticated catalog pagination/API filtering, price isolation between workspaces, migration without credential copying or overwriting new state, and door failure without cross-provider traffic.
- Added a cache/cost check using Databricks tier prices; unknown long-context rates prevent a switch.
- macOS app built for arm64 and x86_64 with the macOS 26.5 SDK. Ad-hoc signature verified.
- Live workspace requests succeeded through local Codex/Pi Responses and Claude Messages listeners. All chose Kimi K3 for a simple task and recorded token usage and estimated cost.
- Actual Codex and Pi CLI sessions completed through the local gateway.
- Live same-session test: Astra completed a distributed scheduler design; the next simple arithmetic request switched to Kimi K3 after the cache/cost gate. Both responses completed.
- Migration was tested against a temporary copy of the existing configuration. It restored the Astra/Sol preferences saved before the earlier restricted model-catalog migration.

Full Claude Code CLI validation on this Mac is blocked by machine-managed settings that force another Databricks workspace endpoint. Test requests returned 401 there without reaching the local listener. Those settings were not changed. The local Anthropic protocol path is verified separately; a client permitted to use the local endpoint is required for end-to-end Claude Code operation.

Cost figures are reference list-price estimates for the default tier models, not invoices. Unknown models and unsupported long-context pricing remain unpriced. The UI and guide state the assumptions. Old request history remains at the previous configuration location; it is not relabeled as Databricks traffic.

## Pi catalog and desktop discovery patch (0.8.1)

Pi now imports all workspace models with a supported conversational API, chooses the API per model, and preserves those overrides when models are hidden or refreshed. Codex retains its Responses-only inventory. Legacy Pi defaults and filters are migrated with a settings backup. The desktop app resolves the direct-install CLI paths as well as Homebrew formula paths, and distinguishes unavailable Pi status from an unconfigured client.

Live Pi tests completed with GPT-6.1 Sol, Claude Haiku 4.5, Gemini 3.5 Flash, and the migrated default Compass selection. The workspace exposed 67 compatible models plus Auto(Compass). Chat Completions configuration omits unsupported stream options; Anthropic configuration disables unsupported eager tool input streaming. The full Go suite and universal macOS build passed.
