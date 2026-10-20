import SwiftUI

struct CompassConfigurationSheet: View {
    @ObservedObject var state: UnityGatewayState
    @Environment(\.dismiss) private var dismiss
    @State private var models: [String: String]
    @State private var rules: [String: Bool]
    @State private var saving = false
    @State private var saveError: String?

    private static let modelFields = [
        ("router", "Router"), ("default", "Fallback"), ("high", "High"),
        ("mid_high", "Mid-high"), ("mid", "Mid"), ("low", "Low")
    ]
    private static let ruleFields = [
        ("session_start", "Session start"), ("subagents", "Subagents"),
        ("session_switching", "Mid-session switching"),
        ("encourage_subagents", "Encourage subagents"), ("verbose", "Verbose output")
    ]

    init(state: UnityGatewayState, configuration: CompassConfiguration) {
        self.state = state
        _models = State(initialValue: Dictionary(uniqueKeysWithValues: Self.modelFields.map {
            ($0.0, configuration.model(for: $0.0))
        }))
        _rules = State(initialValue: Dictionary(uniqueKeysWithValues: Self.ruleFields.map {
            ($0.0, configuration.rules?.enabled($0.0) ?? ($0.0 == "session_start" || $0.0 == "subagents"))
        }))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Configure Compass").font(.title2.weight(.semibold))
            Text("Shared routing defaults").font(.callout).foregroundStyle(.secondary)
            Divider()
            ForEach(Self.modelFields, id: \.0) { field in
                HStack {
                    Text(field.1)
                    Spacer()
                    Picker(field.1, selection: Binding(
                        get: { models[field.0] ?? "" },
                        set: { models[field.0] = $0 }
                    )) {
                        let choices = state.compassPickerRows.filter {
                            $0.slug != "Auto(Compass)" && $0.slug != "unity-gateway-compass"
                                && $0.slug != "claude-unity-compass" && $0.slug != "unity-gateway-compat-v1"
                                && (field.0 != "router" || $0.slug.contains("/") || $0.slug.split(separator: ".").count >= 3)
                        }
                        let selected = models[field.0] ?? ""
                        if !choices.contains(where: { $0.slug == selected }) {
                            Text(trafficCompactModelLabel(selected, modelID: selected)).tag(selected)
                        }
                        ForEach(groupedModels(choices, identifier: { $0.slug }, name: { $0.displayName })) { group in
                            Section(group.family.rawValue) {
                                ForEach(group.models) { model in Text(model.displayName).tag(model.slug) }
                            }
                        }
                    }.labelsHidden().frame(width: 260)
                }
            }
            .disabled(saving || !state.canMutate)
            Divider()
            Text("Routing rules").font(.headline)
            LazyVGrid(columns: [GridItem(.flexible(), alignment: .leading), GridItem(.flexible(), alignment: .leading)], alignment: .leading, spacing: 10) {
                ForEach(Self.ruleFields, id: \.0) { field in
                    Toggle(field.1, isOn: Binding(
                        get: { rules[field.0] ?? false },
                        set: { rules[field.0] = $0 }
                    )).toggleStyle(.checkbox)
                }
            }.disabled(saving || !state.canMutate)
            Text("Applies to clients using Auto. Manual model choices stay unchanged.")
                .font(.caption).foregroundStyle(.secondary)
            if let saveError { Text(saveError).font(.caption).foregroundStyle(.red) }
            HStack {
                if saving { ProgressView().controlSize(.small) }
                Spacer()
                Button("Cancel") { dismiss() }.keyboardShortcut(.cancelAction).disabled(saving)
                Button("Save") {
                    saving = true
                    saveError = nil
                    Task {
                        let success = await state.saveCompassConfiguration(models: models, rules: rules)
                        saving = false
                        if success { dismiss() }
                        else { saveError = "Could not save. Your draft is kept; refresh the model catalog and retry." }
                    }
                }.keyboardShortcut(.defaultAction).disabled(saving || state.compassBusy || !state.canMutate)
            }
        }
        .padding(22)
        .frame(width: 480)
        .interactiveDismissDisabled(saving)
        .task {
            // Read catalogs only; opening Configure must not connect a client.
            await state.loadPi()
            await state.codexDesktopCommand()
            await state.desktopCommand()
        }
        .accessibilityIdentifier("compass-configuration")
    }
}
