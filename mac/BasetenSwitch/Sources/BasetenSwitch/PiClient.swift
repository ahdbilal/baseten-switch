import SwiftUI

struct PiClientStatus: Decodable {
    let Mode: String
    let ModelsPath: String
    let ModelCount: Int
    let Installed: Bool
    let Healthy: Bool
    let Detail: String
}

struct PiClientView: View {
    @ObservedObject var state: BasetenSwitchState
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Label("Pi", systemImage: "terminal").font(.largeTitle.weight(.semibold))
                Text("Connect Pi through Baseten Switch to use Auto(Compass) and track model usage in Compass, Traffic, and Requests.")
                    .foregroundStyle(.secondary)
                if let status = state.piStatus {
                    Label(status.Installed ? (status.Healthy ? "Configured · \(status.Mode)" : "Configuration needs attention") : "Not configured",
                          systemImage: status.Healthy ? "checkmark.circle" : "info.circle")
                    if status.Installed { Text("\(status.ModelCount) models · \(status.ModelsPath)").font(.caption).textSelection(.enabled) }
                    if !status.Detail.isEmpty { Text(status.Detail).font(.caption).foregroundStyle(.orange) }
                }
                HStack {
                    Button("Connect / refresh models") { Task { await state.configurePi() } }
                        .buttonStyle(.borderedProminent).disabled(state.piBusy || !state.canMutate)
                    Button("Refresh status") { Task { await state.loadPi() } }.disabled(state.piBusy)
                    if state.piStatus?.Installed == true {
                        Button("Remove Pi provider") { Task { await state.configurePi(remove: true) } }
                            .disabled(state.piBusy || !state.canMutate)
                    }
                    if state.piBusy { ProgressView().controlSize(.small) }
                }
                Text("Connecting restarts the local gateway to enable Pi's listener. Existing requests may be interrupted. Your other Pi providers and login accounts are preserved.")
                    .font(.caption).foregroundStyle(.secondary)
                Divider()
                Text("Start Pi").font(.headline)
                Text("pi --provider baseten --model 'Auto(Compass)'")
                    .font(.system(.body, design: .monospaced)).textSelection(.enabled)
                Text("Baseten requests use Switch's existing login. For OpenAI Compass tiers, supply your OpenAI API key with --api-key \"$OPENAI_API_KEY\". Pi's separate /login accounts are not forwarded by this custom provider. You can instead configure all Compass tiers to Baseten models on the Codex page.")
                    .font(.callout).foregroundStyle(.secondary)
                Text("Compass chooses once per Pi session. New or forked Pi sessions receive separate choices. Third-party subagent extensions need distinct Pi session IDs.")
                    .font(.caption).foregroundStyle(.secondary)
                if let notice = state.piNotice { Text(notice).font(.callout).foregroundStyle(.secondary) }
            }.padding(24).frame(maxWidth: .infinity, alignment: .leading)
        }.task { await state.loadPi() }
    }
}
