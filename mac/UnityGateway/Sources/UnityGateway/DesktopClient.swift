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
    @State private var selectionDraft: Set<String>?
    private var ready: Bool { state.desktopStatus?.connected == true }
    private var models: [PiModelOption] { state.desktopStatus?.models ?? [] }
    private var savedSelection: Set<String> {
        Set(models.filter { state.desktopStatus?.hidden?[$0.id] != true }.map(\.id))
    }
    private var selection: Set<String> { selectionDraft ?? savedSelection }
    private var selectionDefault: String {
        if let current = state.desktopStatus?.default, selection.contains(current) { return current }
        return models.first(where: { selection.contains($0.id) })?.id ?? ""
    }
    private var manualDefault: String {
        if let current = state.desktopStatus?.default, current != "claude-unity-compass" { return current }
        return state.compassSnapshot?.config.manualModels?["claude-desktop"]
            ?? models.first(where: { $0.id != "claude-unity-compass" })?.id ?? ""
    }
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
                            .disabled(!ready || state.desktopBusy || !state.canMutate || selectionDraft != nil)
                    }
                } content: {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Choose models shown in Desktop. Auto(Compass) routes through your configured tiers. Restart Claude Desktop to apply.")
                            .font(.callout).foregroundStyle(.secondary)
                        Picker("Default model", selection: Binding(
                            get: { manualDefault },
                            set: { model in Task { await state.setCompassDefault("claude-desktop", model: model); await state.desktopCommand() } }
                        )) {
                            if state.desktopStatus?.models.isEmpty != false { Text("Connect to load models").tag("") }
                            ForEach(groupedModels((state.desktopStatus?.models ?? []).filter { $0.id != "claude-unity-compass" }, identifier: { $0.id }, name: { $0.name })) { group in
                                Section(group.family.rawValue) {
                                    ForEach(group.models) { model in Text(model.name).tag(model.id) }
                                }
                            }
                        }.disabled(!ready || state.desktopBusy || !state.canMutate || selectionDraft != nil)
                        TextField("Filter models", text: $search).textFieldStyle(.roundedBorder)
                        HStack(spacing: 12) {
                            Button("Select all") { selectionDraft = Set(models.map(\.id)) }
                                .disabled(selection.count == models.count)
                            Button("Deselect all") { selectionDraft = [] }
                                .disabled(selection.isEmpty)
                            Spacer()
                            if selectionDraft != nil {
                                Button("Cancel") { selectionDraft = nil }
                                Button("Save selection") { Task { await saveSelection() } }
                                    .buttonStyle(.borderedProminent)
                                    .disabled(selection.isEmpty)
                            }
                        }.disabled(!ready || state.desktopBusy || !state.canMutate || models.isEmpty)
                        if selectionDraft != nil {
                            Text(selection.isEmpty ? "Select at least one model to save." : "Default: " + (models.first(where: { $0.id == selectionDefault })?.name ?? selectionDefault))
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        GroupedModelList(models: state.desktopStatus?.models ?? [], search: search,
                            identifier: { $0.id }, name: { $0.name }, selected: { selection.contains($0.id) }) { model in
                            Toggle(isOn: Binding(
                                get: { selection.contains(model.id) },
                                set: { visible in
                                    var next = selection
                                    if visible { next.insert(model.id) } else { next.remove(model.id) }
                                    selectionDraft = next
                                }
                            )) { ClientModelLabel(name: model.name, identifier: model.id) }
                                .disabled(!ready || state.desktopBusy || !state.canMutate)
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

    private func saveSelection() async {
        let desired = selection
        let defaultModel = selectionDefault
        let ids = models.filter { desired.contains($0.id) }.map(\.id)
        guard !ids.isEmpty else { return }
        await state.desktopCommand(["selection", defaultModel] + ids)
        if state.desktopStatus?.default == defaultModel && savedSelection == desired {
            selectionDraft = nil
        }
    }
}
