import SwiftUI

struct CompassClientControls: View {
    @ObservedObject var state: BasetenSwitchState
    let client: String
    @State private var showAdvanced = false
    private var selection: CompassClientSelection? { state.compassSnapshot?.clients?[client] }
    private let fields = [("session_start", "Select at session start"), ("subagents", "Select for each subagent"), ("session_switching", "Switch within main session"), ("encourage_subagents", "Encourage subagents"), ("verbose", "Verbose CLI decisions")]
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Toggle("Enable Compass", isOn: Binding(
                get: { selection?.enabled ?? false },
                set: { enabled in Task { await state.setCompassEnabled(client, enabled: enabled) } }
            )).toggleStyle(.switch)
                .disabled(selection == nil || state.compassBusy || state.compassRefreshing || !state.canMutate)
            Text(client == "claude-code" ? "Automatically selects models when launched with baseten-switch compass run claude." : "Automatically selects models for new sessions. Restart the client to apply its saved default.")
                .font(.caption).foregroundStyle(.secondary)
            DisclosureGroup("Advanced options", isExpanded: $showAdvanced) {
                VStack(alignment: .leading, spacing: 12) {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Model choices").font(.subheadline.weight(.semibold))
                        CompassRoutingView(state: state)
                    }
                    Divider()
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Routing rules").font(.subheadline.weight(.semibold))
                        VStack(alignment: .leading, spacing: 8) {
                            ForEach(fields, id: \.0) { field in
                                Toggle(field.1, isOn: Binding(
                                    get: { state.compassSnapshot?.config.rules?.enabled(field.0) ?? (field.0 == "session_start" || field.0 == "subagents") },
                                    set: { value in Task { await state.setCompassRule(field.0, enabled: value) } }
                                )).toggleStyle(.switch)
                                    .disabled(state.compassBusy || state.compassRefreshing || !state.canMutate)
                            }
                        }.padding(.top, 8)
                    }
                }
                .padding(14)
                .background(Color.secondary.opacity(0.05), in: RoundedRectangle(cornerRadius: 8))
                .overlay(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 2).fill(Color.secondary.opacity(0.25)).frame(width: 3)
                }
                .padding(.leading, 14)
                .padding(.top, 10)
            }
            if client == "claude-code" {
                Text("OpenAI tiers require OPENAI_API_KEY in the gateway environment. Claude’s login is used only for Anthropic; Baseten tiers use Switch’s login.")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if let notice = state.compassNotice { Text(notice).font(.caption).foregroundStyle(.secondary) }
        }.task { await state.loadCompass() }
    }
}
