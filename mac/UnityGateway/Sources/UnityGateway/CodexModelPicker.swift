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
    var body: some View {
        ModelConfigurationCard {
            ClientDefaultRow(state: state, client: "codex", models: state.codexPickerRows.map { PiModelOption(id: $0.slug, name: $0.displayName) })
            Divider()
            ClientAutoOptions(state: state, client: "codex")
            Divider()
            ClientVisibleModels(
                models: state.codexPickerRows.map { PiModelOption(id: $0.slug, name: $0.displayName) },
                saved: Set(state.codexPickerRows.filter(\.visible).map(\.slug)),
                busy: state.codexPickerBusy || !state.canMutate,
                refresh: { await state.syncCodexPicker() },
                save: { await state.saveVisibleModels("codex", desired: $0) })
            if let notice = state.codexPickerNotice { Text(notice).font(.caption).foregroundStyle(.secondary) }
        }
        .task { await state.loadCodexPicker() }
        .accessibilityIdentifier("codex-model-picker")
    }
}
