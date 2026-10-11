import SwiftUI

struct CompassClientControls: View {
    @ObservedObject var state: BasetenSwitchState
    let client: String
    @State private var showAdvanced = false
    @State private var showModels = false
    @State private var showRules = false
    private var selection: CompassClientSelection? { state.compassSnapshot?.clients?[client] }
    private let fields = [("session_start", "Select at session start"), ("subagents", "Select for each subagent"), ("session_switching", "Switch within main session"), ("encourage_subagents", "Encourage subagents"), ("verbose", "Verbose CLI decisions")]
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Toggle("Enable Compass", isOn: Binding(
                get: { selection?.enabled ?? false },
                set: { enabled in Task { await state.setCompassEnabled(client, enabled: enabled) } }
            )).toggleStyle(.switch)
                .disabled(selection == nil || state.compassBusy || state.compassRefreshing || !state.canMutate)
            Text("Automatically selects models for new sessions. Restart the client to apply its saved default.")
                .font(.caption).foregroundStyle(.secondary)
            DisclosureGroup("Advanced options", isExpanded: $showAdvanced) {
                VStack(alignment: .leading, spacing: 12) {
                    DisclosureGroup("Compass · Auto routing", isExpanded: $showModels) {
                        CompassRoutingView(state: state).padding(.top, 8)
                    }
                    Divider()
                    DisclosureGroup("Routing rules", isExpanded: $showRules) {
                        VStack(alignment: .leading, spacing: 8) {
                            ForEach(fields, id: \.0) { field in
                                Toggle(field.1, isOn: Binding(
                                    get: { state.compassSnapshot?.config.rules?.enabled(field.0) ?? (field.0 == "session_start" || field.0 == "subagents") },
                                    set: { value in Task { await state.setCompassRule(field.0, enabled: value) } }
                                )).toggleStyle(.switch)
                                    .disabled(state.compassBusy || state.compassRefreshing || !state.canMutate)
                            }
                            DisclosureGroup("Rule details & cache protection") {
                                Text("Selection off uses the fallback. Main-session switching may reconsider below $1 or above $10, only when expected savings cover cache loss and routing cost. Missing usage, prices, or compatible conversation state keeps the model fixed. Subagent encouragement applies to new sessions; it cannot create unsupported tools. Verbose output is available through baseten-switch compass watch.")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                        }.padding(.top, 8)
                    }
                }.padding(.top, 10)
            }
            if let notice = state.compassNotice { Text(notice).font(.caption).foregroundStyle(.secondary) }
        }.task { await state.loadCompass() }
    }
}
