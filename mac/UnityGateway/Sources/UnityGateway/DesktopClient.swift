import AppKit
import SwiftUI

struct DesktopClientStatus: Decodable {
    let models: [PiModelOption]
    let hidden: [String: Bool]?
    let `default`: String
    let connected: Bool
    let configured: Bool
    let base_url: String
    let directory: String
}

struct DesktopClientView: View {
    @ObservedObject var state: UnityGatewayState
    @State private var search = ""
    private var ready: Bool { state.desktopStatus?.connected == true }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                SwitchClientHeader(name: "Claude Desktop")
                HStack {
                    Label(ready ? "Connected to Databricks" : "Connect Claude Desktop to Databricks", systemImage: ready ? "checkmark.circle" : "info.circle")
                    Spacer()
                    if state.desktopBusy { ProgressView().controlSize(.small) }
                    Button(ready ? "Open Claude" : "Connect Claude Desktop") {
                        if ready { NSWorkspace.shared.open(URL(fileURLWithPath: "/Applications/Claude.app")) }
                        else { Task { await state.desktopCommand(["on"]) } }
                    }.disabled(state.desktopBusy || !state.canMutate)
                }
                CompassClientSection(state: state, client: "claude-desktop")
                RoutingSectionCard {
                    HStack {
                        Label("Model picker", systemImage: "list.bullet").font(.headline)
                        Spacer()
                        Button("Refresh models") { Task { await state.desktopCommand(["refresh"]) } }
                            .disabled(!ready || state.desktopBusy || !state.canMutate)
                    }
                } content: {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Choose models shown in Desktop. Auto(Compass) routes through your configured tiers. Restart Claude Desktop to apply.")
                            .font(.callout).foregroundStyle(.secondary)
                        Picker("Default model", selection: Binding(
                            get: { state.compassSnapshot?.config.manualModels?["claude-desktop"] ?? state.desktopStatus?.models.first(where: { $0.id != "claude-unity-compass" })?.id ?? "" },
                            set: { model in Task { await state.setCompassDefault("claude-desktop", model: model); await state.desktopCommand() } }
                        )) {
                            if state.desktopStatus?.models.isEmpty != false { Text("Connect to load models").tag("") }
                            ForEach(groupedModels((state.desktopStatus?.models ?? []).filter { $0.id != "claude-unity-compass" }, identifier: { $0.id }, name: { $0.name })) { group in
                                Section(group.family.rawValue) {
                                    ForEach(group.models) { model in Text(model.name).tag(model.id) }
                                }
                            }
                        }.disabled(!ready || state.desktopBusy || !state.canMutate)
                        TextField("Filter models", text: $search).textFieldStyle(.roundedBorder)
                        GroupedModelList(models: state.desktopStatus?.models ?? [], search: search,
                            identifier: { $0.id }, name: { $0.name }, selected: { state.desktopStatus?.hidden?[$0.id] != true }) { model in
                            Toggle(isOn: Binding(
                                get: { state.desktopStatus?.hidden?[model.id] != true },
                                set: { visible in Task { await state.desktopCommand(["visibility", model.id, visible ? "on" : "off"]) } }
                            )) { ClientModelLabel(name: model.name, identifier: model.id) }
                                .disabled(!ready || state.desktopBusy || !state.canMutate || model.id == state.desktopStatus?.default)
                        }
                    }
                }
                DisclosureGroup("Connection & setup") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(state.desktopStatus?.base_url ?? "http://127.0.0.1:45275").textSelection(.enabled)
                        Text("Uses a separate Desktop gateway profile. Organization-managed settings take precedence. Requests appear under Claude Desktop in Traffic.")
                        HStack {
                            Button("Refresh status") { Task { await state.desktopCommand() } }
                            if state.desktopStatus?.configured == true {
                                Button("Disconnect") { Task { await state.desktopCommand(["off"]) } }
                            }
                        }.disabled(state.desktopBusy || !state.canMutate)
                    }.font(.caption).foregroundStyle(.secondary).padding(.top, 8)
                }
                if let notice = state.desktopNotice { Text(notice).font(.caption).textSelection(.enabled) }
            }.padding(28).frame(maxWidth: 720, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }.task { await state.desktopCommand() }
    }
}
