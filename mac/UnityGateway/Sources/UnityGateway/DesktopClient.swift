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
    var client = "claude-desktop"
    private var isCodex: Bool { client == "codex-desktop" }
    private var name: String { isCodex ? "Codex Desktop" : "Claude Desktop" }
    private var status: DesktopClientStatus? { isCodex ? state.codexDesktopStatus : state.desktopStatus }
    private var busy: Bool { isCodex ? state.codexDesktopBusy : state.desktopBusy }
    private var notice: String? { isCodex ? state.codexDesktopNotice : state.desktopNotice }
    private var compassModel: String { isCodex ? "Auto(Compass)" : "claude-unity-compass" }
    private func command(_ args: [String] = ["status", "--json"]) async {
        if isCodex { await state.codexDesktopCommand(args) } else { await state.desktopCommand(args) }
    }
    @State private var search = ""
    @State private var selectionDraft: Set<String>?
    private var ready: Bool { status?.connected == true && !state.harnessBusy.contains(client) }
    private var models: [PiModelOption] { status?.models ?? [] }
    private var savedSelection: Set<String> {
        Set(models.filter { status?.hidden?[$0.id] != true }.map(\.id))
    }
    private var selection: Set<String> { selectionDraft ?? savedSelection }
    private var selectionDefault: String {
        if let current = status?.default, selection.contains(current) { return current }
        return models.first(where: { selection.contains($0.id) })?.id ?? ""
    }
    private var manualDefault: String {
        if let current = status?.default, current != compassModel { return current }
        return state.compassSnapshot?.config.manualModels?[client]
            ?? models.first(where: { $0.id != compassModel })?.id ?? ""
    }
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                SwitchClientHeader(name: name)
                HarnessConnectionSection(state: state, client: client)
                CompassClientSection(state: state, client: client).disabled(!ready)
                RoutingSectionCard {
                    HStack {
                        Label("Model picker", systemImage: "list.bullet").font(.headline)
                        Spacer()
                        Button("Refresh models") { Task { await command(["refresh"]) } }
                            .disabled(!ready || busy || !state.canMutate || selectionDraft != nil)
                    }
                } content: {
                    VStack(alignment: .leading, spacing: 12) {
                        Text("Choose models shown in Desktop. Auto(Compass) routes through your configured tiers. Restart the desktop app to apply.")
                            .font(.callout).foregroundStyle(.secondary)
                        Picker("Default model", selection: Binding(
                            get: { manualDefault },
                            set: { model in Task { await state.setCompassDefault(client, model: model); await command() } }
                        )) {
                            if status?.models.isEmpty != false { Text("Connect to load models").tag("") }
                            ForEach(groupedModels((status?.models ?? []).filter { $0.id != compassModel }, identifier: { $0.id }, name: { $0.name })) { group in
                                Section(group.family.rawValue) {
                                    ForEach(group.models) { model in Text(model.name).tag(model.id) }
                                }
                            }
                        }.disabled(!ready || busy || !state.canMutate || selectionDraft != nil)
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
                        }.disabled(!ready || busy || !state.canMutate || models.isEmpty)
                        if selectionDraft != nil {
                            Text(selection.isEmpty ? "Select at least one model to save." : "Default: " + (models.first(where: { $0.id == selectionDefault })?.name ?? selectionDefault))
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        GroupedModelList(models: status?.models ?? [], search: search,
                            identifier: { $0.id }, name: { $0.name }, selected: { selection.contains($0.id) }) { model in
                            Toggle(isOn: Binding(
                                get: { selection.contains(model.id) },
                                set: { visible in
                                    var next = selection
                                    if visible { next.insert(model.id) } else { next.remove(model.id) }
                                    selectionDraft = next
                                }
                            )) { ClientModelLabel(name: model.name, identifier: model.id) }
                                .disabled(!ready || busy || !state.canMutate)
                        }
                    }
                }
                DisclosureGroup("Connection & setup") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text(status?.base_url ?? (isCodex ? "http://127.0.0.1:45276/v1" : "http://127.0.0.1:45275")).textSelection(.enabled)
                        Text(isCodex ? "Uses ~/.codex/config.toml. Unprofiled CLI launches share these defaults. Disable restores the previous settings and preserves your OpenAI sign-in. Restart Codex and start a new chat to apply." : "Uses a separate Desktop gateway profile. Organization-managed settings take precedence. Requests appear under Claude Desktop in Traffic.")
                        HStack {
                            Button("Refresh status") { Task { await command() } }

                        }.disabled(busy || !state.canMutate)
                    }.font(.caption).foregroundStyle(.secondary).padding(.top, 8)
                }
                if let notice = notice { Text(notice).font(.caption).textSelection(.enabled) }
            }.padding(28).frame(maxWidth: 720, alignment: .leading)
                .frame(maxWidth: .infinity, alignment: .topLeading)
        }.task { await command() }
    }

    private func saveSelection() async {
        let desired = selection
        let defaultModel = selectionDefault
        let ids = models.filter { desired.contains($0.id) }.map(\.id)
        guard !ids.isEmpty else { return }
        await command(["selection", defaultModel] + ids)
        if status?.default == defaultModel && savedSelection == desired {
            selectionDraft = nil
        }
    }
}
