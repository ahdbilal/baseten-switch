import SwiftUI

struct PiModelOption: Decodable, Identifiable {
    let id: String
    let name: String
}

struct PiClientStatus: Decodable {
    let Models: [PiModelOption]?
    let Hidden: [String: Bool]?
    let Mode: String
    let ModelsPath: String
    let ModelCount: Int
    let Installed: Bool
    let Healthy: Bool
    let Detail: String
}

struct PiClientView: View {
    @ObservedObject var state: UnityGatewayState
    @State private var search = ""
    private var manualModel: String {
        state.compassSnapshot?.config.manualModels?["pi"]
            ?? (state.compassSnapshot?.clients?["pi"]?.enabled == false ? state.compassSnapshot?.clients?["pi"]?.model : nil)
            ?? state.compassSnapshot?.config.defaultModel ?? ""
    }
    private var launchCommand: String {
        let model = state.compassSnapshot?.clients?["pi"]?.enabled == true ? "Auto(Compass)" : manualModel
        let prefix = state.compassSnapshot?.policy(for: "pi").rules?.verbose == true ? "unity-gateway compass run --verbose pi --" : "pi"
        return prefix + " --provider databricks --model '" + model.replacingOccurrences(of: "'", with: "'\"'\"'") + "'"
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                SwitchClientHeader(state: state, name: "Pi", client: "pi")
                ModelConfigurationCard {
                    ClientDefaultRow(state: state, client: "pi", models: state.piStatus?.Models ?? [], current: manualModel)
                    Divider()
                    ClientAutoOptions(state: state, client: "pi")
                    Divider()
                    ClientVisibleModels(models: state.piStatus?.Models ?? [],
                        saved: Set((state.piStatus?.Models ?? []).filter { state.piStatus?.Hidden?[$0.id] != true }.map(\.id)),
                        busy: state.piBusy || !state.canMutate,
                        refresh: { await state.configurePi() },
                        save: { await state.saveVisibleModels("pi", desired: $0) })
                }.disabled(state.piStatus?.Healthy != true)
                DisclosureGroup("Connection & setup") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("Start Pi: " + launchCommand).textSelection(.enabled)
                        if let status = state.piStatus {
                            Text(status.Healthy ? "Connected · \(status.ModelCount) models" : status.Detail)
                            Text(status.ModelsPath).font(.caption).textSelection(.enabled)
                        }
                        HStack {
                            Button("Refresh status") { Task { await state.loadPi() } }.disabled(state.piBusy)

                        }
                        Text("Refreshing models reloads the gateway and may interrupt active requests. Other Pi providers are preserved.")
                        Text("All tiers use your Databricks workspace sign-in. Subagent extensions need distinct session IDs.")
                    }.font(.caption).foregroundStyle(.secondary).padding(.top, 8)
                }
                if let notice = state.piNotice { Text(notice).font(.caption).foregroundStyle(.secondary) }
            }.padding(28).frame(maxWidth: 900, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }.task { await state.loadPi() }
    }
}
