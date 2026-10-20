import SwiftUI

struct GroupedModelList<Model: Identifiable, Row: View>: View {
    let models: [Model]
    let search: String
    let identifier: (Model) -> String
    let name: (Model) -> String
    let selected: (Model) -> Bool
    @ViewBuilder let row: (Model) -> Row
    @State private var expanded: Set<ModelFamily> = [.automatic]

    private var searching: Bool { !search.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
    private var groups: [ModelGroup<Model>] {
        groupedModels(models, identifier: identifier, name: name, search: search)
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !models.isEmpty {
                Text("\(models.count) models · \(models.filter(selected).count) selected")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if groups.isEmpty && searching {
                Text("No models match your search.").font(.callout).foregroundStyle(.secondary)
            }
            ForEach(groups) { group in
                DisclosureGroup(isExpanded: Binding(
                    get: { searching || expanded.contains(group.family) },
                    set: { if $0 { expanded.insert(group.family) } else { expanded.remove(group.family) } }
                )) {
                    LazyVStack(alignment: .leading, spacing: 8) {
                        ForEach(group.models) { model in row(model) }
                    }.padding(.leading, 12).padding(.top, 10)
                } label: {
                    HStack {
                        Text(group.family.rawValue).font(.subheadline.weight(.semibold))
                        Spacer()
                        Text("\(group.models.filter(selected).count)/\(group.models.count) selected")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                }
                .padding(.vertical, 5)
                Divider()
            }
        }
    }
}
