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
    @ObservedObject var state: BasetenSwitchState
    @State private var search = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Codex model picker").font(.headline)
                Spacer()
                if state.codexPickerBusy { ProgressView().controlSize(.small) }
                Button("Refresh models") {
                    Task { await state.syncCodexPicker() }
                }.disabled(state.codexPickerBusy || !state.canMutate)
            }
            Text("Choose the models shown in /model. Restart Codex after changing visibility.")
                .font(.callout).foregroundStyle(.secondary)
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
                        VStack(alignment: .leading, spacing: 2) {
                            Text(row.displayName)
                            Text(row.slug).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    .disabled(state.codexPickerBusy || !state.canMutate)
            }
            if let notice = state.codexPickerNotice {
                Text(notice).font(.callout).foregroundStyle(.secondary)
            }
            Text("Start Codex with: codex --profile baseten")
                .font(.caption).textSelection(.enabled)
        }
        .padding(16)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 10))
        .task { await state.loadCodexPicker() }
        .accessibilityIdentifier("codex-model-picker")
    }
}
