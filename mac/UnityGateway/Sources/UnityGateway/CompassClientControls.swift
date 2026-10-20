import SwiftUI

func compassHarness(_ client: String) -> String {
    if client.hasPrefix("codex") { return "codex" }
    if client.hasPrefix("claude") { return "claude" }
    return "pi"
}
func compassHarnessName(_ harness: String) -> String {
    harness == "pi" ? "Pi" : harness.capitalized
}
func isCompassModel(_ model: String) -> Bool {
    ["Auto(Compass)", "unity-gateway-compass", "claude-unity-compass"].contains(model)
}

struct SwitchClientHeader: View {
    @ObservedObject var state: UnityGatewayState
    let name: String
    let client: String
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 5) {
                    Text(name).font(.title.weight(.semibold))
                    Text("Model configuration").font(.callout).foregroundStyle(.secondary)
                }
                Spacer()
                Toggle("Gateway", isOn: Binding(
                    get: { state.harnessStatuses[client]?.configured == true },
                    set: { enabled in Task { await state.harnessCommand(client, enabled: enabled) } }
                )).toggleStyle(.switch)
                    .disabled(state.harnessStatuses[client] == nil || state.harnessBusy.contains(client) || !state.canMutate)
                    .help("Disable to restore this client's previous provider settings")
                if state.harnessBusy.contains(client) { ProgressView().controlSize(.small) }
            }
            if let notice = state.harnessNotices[client] {
                Text(notice).font(.caption).foregroundStyle(.secondary)
            }
        }
        .task { await state.harnessCommand(client); await state.loadCompass() }
    }
}

struct ModelConfigurationCard<Content: View>: View {
    @ViewBuilder let content: () -> Content
    var body: some View {
        VStack(alignment: .leading, spacing: 14, content: content)
            .padding(18)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color(nsColor: .separatorColor).opacity(0.5)))
    }
}

struct ClientDefaultRow: View {
    @ObservedObject var state: UnityGatewayState
    let client: String
    let models: [PiModelOption]
    var current: String = ""
    private var selection: String {
        let value = state.compassSnapshot?.clients?[client]?.model ?? current
        return isCompassModel(value) ? "Auto(Compass)" : value
    }
    var body: some View {
        HStack {
            Text("Default")
            Spacer()
            Picker("Default", selection: Binding(
                get: { selection },
                set: { value in Task { await state.selectClientModel(client, model: value) } }
            )) {
                Text("Auto").tag("Auto(Compass)")
                let choices = models.filter { !isCompassModel($0.id) }
                if !isCompassModel(selection) && !choices.contains(where: { $0.id == selection }) {
                    Text(selection.isEmpty ? "Choose model" : trafficCompactModelLabel(selection, modelID: selection)).tag(selection)
                }
                ForEach(groupedModels(choices, identifier: { $0.id }, name: { $0.name })) { group in
                    Section(group.family.rawValue) {
                        ForEach(group.models) { model in Text(model.name).tag(model.id) }
                    }
                }
            }.labelsHidden().frame(maxWidth: 270)
                .disabled(state.compassSnapshot == nil || state.compassBusy || !state.canMutate)
        }
    }
}

struct ClientAutoOptions: View {
    @ObservedObject var state: UnityGatewayState
    let client: String
    @State private var expanded = true
    @State private var editingShared = false
    @State private var editingOverride = false
    private var harness: String { compassHarness(client) }
    private var overridden: Bool { state.compassSnapshot?.config.overrides?[client] != nil }
    var body: some View {
        DisclosureGroup(isExpanded: $expanded) {
            VStack(alignment: .leading, spacing: 12) {
                Divider()
                HStack {
                    Toggle("Override for this client", isOn: Binding(
                        get: { overridden },
                        set: { enabled in Task { await state.setCompassOverride(client, enabled: enabled) } }
                    )).toggleStyle(.switch).controlSize(.small)
                    Spacer()
                    Button("Edit shared defaults") { editingShared = true }.buttonStyle(.link)
                }
                if let snapshot = state.compassSnapshot {
                    let policy = snapshot.policy(for: client)
                    HStack(alignment: .firstTextBaseline) {
                        Text("Auto routing \(policy.isEnabled ? "on" : "off") · " + [policy.high, policy.midHigh, policy.mid, policy.low].map { trafficCompactModelLabel($0, modelID: $0) }.joined(separator: " / "))
                            .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                        if overridden {
                            Spacer()
                            Button("Configure override") { editingOverride = true }.buttonStyle(.link)
                        }
                    }
                }
            }.padding(.top, 8)
        } label: {
            HStack {
                Text("Auto")
                Spacer()
                Text(overridden ? "Client override" : compassHarnessName(harness) + " defaults")
                    .font(.callout).foregroundStyle(.secondary)
            }
        }
        .disabled(state.compassBusy || state.compassSnapshot == nil || !state.canMutate)
        .sheet(isPresented: $editingShared) {
            if let snapshot = state.compassSnapshot {
                CompassConfigurationSheet(state: state, configuration: snapshot.shared(for: harness), harness: harness)
            }
        }
        .sheet(isPresented: $editingOverride) {
            if let snapshot = state.compassSnapshot {
                CompassConfigurationSheet(state: state, configuration: snapshot.config.overrides?[client] ?? snapshot.shared(for: harness), harness: harness, client: client)
            }
        }
    }
}

struct ClientModelLabel: View {
    let name: String
    let identifier: String
    var body: some View {
        Text(name).help(identifier).padding(.vertical, 3)
    }
}

struct ClientVisibleModels: View {
    let models: [PiModelOption]
    let saved: Set<String>
    var busy: Bool = false
    let refresh: () async -> Void
    let save: (Set<String>) async -> Bool
    @State private var draft: Set<String>?
    @State private var search = ""
    @State private var saving = false
    private var selection: Set<String> { draft ?? saved }
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Text("Visible models").font(.headline)
                Spacer()
                Button("All") { draft = Set(models.map(\.id)) }.buttonStyle(.link)
                Button("None") { draft = [] }.buttonStyle(.link)
                Button { Task { await refresh() } } label: { Image(systemName: "arrow.clockwise") }
                    .buttonStyle(.borderless).help("Refresh models").disabled(draft != nil)
            }
            TextField("Filter models", text: $search).textFieldStyle(.roundedBorder)
            if draft != nil {
                HStack {
                    Text(selection.isEmpty ? "Select at least one model." : "\(selection.count) selected")
                        .font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("Cancel") { draft = nil }
                    Button("Save") {
                        let desired = selection
                        saving = true
                        Task { if await save(desired) { draft = nil }; saving = false }
                    }.disabled(selection.isEmpty).buttonStyle(.borderedProminent)
                }
            }
            GroupedModelList(models: models, search: search, identifier: { $0.id }, name: { $0.name }, selected: { selection.contains($0.id) }) { model in
                Toggle(isOn: Binding(get: { selection.contains(model.id) }, set: { visible in
                    var next = selection
                    if visible { next.insert(model.id) } else { next.remove(model.id) }
                    draft = next
                })) { ClientModelLabel(name: model.name, identifier: model.id) }.toggleStyle(.checkbox)
            }
        }.disabled(busy || saving)
    }
}
