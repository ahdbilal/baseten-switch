import Foundation
import SwiftUI

struct CodexPickerRow: Decodable, Identifiable {
    let slug: String
    let displayName: String
    let visibility: String
    var id: String { slug }
    var visible: Bool { visibility == "list" }
    enum CodingKeys: String, CodingKey {
        case slug, visibility
        case displayName = "display_name"
    }
}

struct CodexPickerDocument: Decodable {
    let models: [CodexPickerRow]
}

struct CodexModelPickerSectionView: View {
    @ObservedObject var state: UnityGatewayState
    @State private var search = ""

    var body: some View {
        RoutingSectionCard {
            HStack {
                Label("Model picker", systemImage: "list.bullet").font(.headline)
                Spacer()
                if state.codexPickerBusy { ProgressView().controlSize(.small) }
                Button("Refresh models") {
                    Task { await state.syncCodexPicker() }
                }.disabled(state.codexPickerBusy || !state.canMutate)
            }
        } content: {
            VStack(alignment: .leading, spacing: 12) {
                Text("Choose the models shown in /model. Restart Codex after changing visibility.")
                    .font(.callout).foregroundStyle(.secondary)
                Picker("Default model", selection: Binding(
                    get: { state.compassSnapshot?.config.manualModels?["codex"] ?? (state.compassSnapshot?.clients?["codex"]?.enabled == false ? state.compassSnapshot?.clients?["codex"]?.model : state.compassSnapshot?.config.defaultModel) ?? "" },
                    set: { model in Task { await state.setCompassDefault("codex", model: model) } }
                )) {
                    Text("Choose a manual default").tag("")
                    ForEach(state.codexPickerRows.filter { $0.slug != "Auto(Compass)" && $0.slug != "unity-gateway-compass" }) { row in
                        Text(row.displayName).tag(row.slug)
                    }
                }.disabled(state.compassBusy || !state.canMutate)
                TextField("Filter models", text: $search)
                    .textFieldStyle(.roundedBorder)
                ForEach(state.codexPickerRows.filter {
                    search.isEmpty || $0.displayName.localizedCaseInsensitiveContains(search)
                        || $0.slug.localizedCaseInsensitiveContains(search)
                }) { row in
                    Toggle(isOn: Binding(
                        get: { row.visible },
                        set: { visible in
                            Task { await state.setCodexPickerVisibility(row.slug, visible: visible) }
                        })) {
                            ClientModelLabel(name: row.displayName, identifier: row.slug)
                        }
                        .disabled(state.codexPickerBusy || !state.canMutate)
                }
                if let notice = state.codexPickerNotice {
                    Text(notice).font(.callout).foregroundStyle(.secondary)
                }
                Text(state.compassSnapshot?.config.rules?.verbose == true ? "Start Codex with: unity-gateway compass run --verbose codex -- --profile unity-gateway" : "Start Codex with: codex --profile unity-gateway")
                    .font(.caption).textSelection(.enabled)
            }
        }
        .task { await state.loadCodexPicker() }
        .accessibilityIdentifier("codex-model-picker")
    }
}
