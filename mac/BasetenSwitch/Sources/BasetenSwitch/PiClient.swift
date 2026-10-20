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
    @ObservedObject var state: BasetenSwitchState
    @State private var search = ""
    private var manualModel: String {
        state.compassSnapshot?.config.manualModels?["pi"]
            ?? (state.compassSnapshot?.clients?["pi"]?.enabled == false ? state.compassSnapshot?.clients?["pi"]?.model : nil)
            ?? state.compassSnapshot?.config.defaultModel ?? ""
    }
    private var launchCommand: String {
        let model = state.compassSnapshot?.clients?["pi"]?.enabled == true ? "Auto(Compass)" : manualModel
        let prefix = state.compassSnapshot?.config.rules?.verbose == true ? "baseten-switch compass run --verbose pi --" : "pi"
        return prefix + " --provider baseten --model '" + model.replacingOccurrences(of: "'", with: "'\"'\"'") + "'"
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                SwitchClientHeader(name: "Pi")
                if state.piStatus?.Healthy != true {
                    Label("Connect Pi to use Compass and track requests.", systemImage: "info.circle")
                    Button("Connect Pi") { Task { await state.configurePi() } }
                        .buttonStyle(.borderedProminent).disabled(state.piBusy || !state.canMutate)
                }
                CompassClientSection(state: state, client: "pi")
                RoutingSectionCard {
                    HStack {
                        Label("Model picker", systemImage: "list.bullet").font(.headline)
                        Spacer()
                        if state.piBusy { ProgressView().controlSize(.small) }
                        Button("Refresh models") { Task { await state.configurePi() } }
                            .disabled(state.piBusy || !state.canMutate)
                    }
                } content: {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Choose the models shown in /model. Restart Pi or run /reload after changing visibility.")
                            .font(.callout).foregroundStyle(.secondary)
                        Picker("Default model", selection: Binding(
                            get: { manualModel },
                            set: { model in Task { await state.setCompassDefault("pi", model: model) } }
                        )) {
                            Text("Choose a manual default").tag("")
                            ForEach((state.piStatus?.Models ?? []).filter { $0.id != "Auto(Compass)" && $0.id != "baseten-switch-compass" }) { model in
                                Text(model.name).tag(model.id)
                            }
                        }.disabled(state.piStatus?.Healthy != true || state.piBusy || state.compassBusy || !state.canMutate)
                        TextField("Filter models", text: $search).textFieldStyle(.roundedBorder)
                        ForEach((state.piStatus?.Models ?? []).filter { search.isEmpty || $0.name.localizedCaseInsensitiveContains(search) || $0.id.localizedCaseInsensitiveContains(search) }) { model in
                            Toggle(isOn: Binding(
                                get: { state.piStatus?.Hidden?[model.id] != true },
                                set: { visible in Task { await state.setPiPickerVisibility(model.id, visible: visible) } }
                            )) {
                                ClientModelLabel(name: model.name, identifier: model.id)
                            }
                            .disabled(state.piStatus?.Healthy != true || state.piBusy || !state.canMutate)
                        }
                        Text("Start Pi: " + launchCommand).font(.caption).textSelection(.enabled)
                    }
                }
                DisclosureGroup("Connection & setup") {
                    VStack(alignment: .leading, spacing: 10) {
                        if let status = state.piStatus {
                            Text(status.Healthy ? "Connected · \(status.ModelCount) models" : status.Detail)
                            Text(status.ModelsPath).font(.caption).textSelection(.enabled)
                        }
                        HStack {
                            Button("Refresh status") { Task { await state.loadPi() } }.disabled(state.piBusy)
                            if state.piStatus?.Installed == true {
                                Button("Remove Pi provider") { Task { await state.configurePi(remove: true) } }
                                    .disabled(state.piBusy || !state.canMutate)
                            }
                        }
                        Text("Refreshing models restarts the gateway and may interrupt active requests. Other Pi providers are preserved.")
                        Text("OpenAI tiers require an API key supplied to Pi with --api-key. Unity Gateway uses Switch’s login. Subagent extensions need distinct session IDs.")
                    }.font(.caption).foregroundStyle(.secondary).padding(.top, 8)
                }
                if let notice = state.piNotice { Text(notice).font(.caption).foregroundStyle(.secondary) }
            }.padding(28).frame(maxWidth: 720, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }.task { await state.loadPi() }
    }
}
