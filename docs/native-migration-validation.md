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
