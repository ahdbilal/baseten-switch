import SwiftUI

struct CompassConfigurationSheet: View {
    @ObservedObject var state: UnityGatewayState
    @Environment(\.dismiss) private var dismiss
    let client: String?
    let chooseHarness: Bool
    @State private var harness: String
    @State private var enabled: Bool
    @State private var drafts: [String: Draft] = [:]
    private struct Draft { var models: [String: String]; var rules: [String: Bool]; var enabled: Bool }
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

    init(state: UnityGatewayState, configuration: CompassConfiguration, harness: String = "codex", client: String? = nil, chooseHarness: Bool = false) {
        self.state = state
        self.client = client
        self.chooseHarness = chooseHarness
        _harness = State(initialValue: harness)
        let configuration = chooseHarness ? (state.compassSnapshot?.shared(for: harness) ?? configuration) : configuration
        _enabled = State(initialValue: configuration.isEnabled)
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
            HStack {
                if chooseHarness {
                    Picker("Harness", selection: Binding(get: { harness }, set: { target in
                        drafts[harness] = Draft(models: models, rules: rules, enabled: enabled)
                        harness = target
                        if let draft = drafts[target] { models = draft.models; rules = draft.rules; enabled = draft.enabled }
                        else if let config = state.compassSnapshot?.shared(for: target) {
                            models = Dictionary(uniqueKeysWithValues: Self.modelFields.map { ($0.0, config.model(for: $0.0)) })
                            rules = Dictionary(uniqueKeysWithValues: Self.ruleFields.map { ($0.0, config.rules?.enabled($0.0) ?? ($0.0 == "session_start" || $0.0 == "subagents")) })
                            enabled = config.isEnabled
                        }
                    })) {
                        Text("Codex").tag("codex"); Text("Claude").tag("claude"); Text("Pi").tag("pi")
                    }.labelsHidden().pickerStyle(.segmented)
                } else {
                    Text(client == nil ? compassHarnessName(harness) + " shared defaults" : "Override for this client").foregroundStyle(.secondary)
                }
                Spacer()
                Toggle("Auto routing", isOn: $enabled).toggleStyle(.switch).controlSize(.small)
            }.disabled(saving)
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
            Text(client == nil ? "Shared by " + (harness == "codex" ? "Codex CLI and Desktop." : harness == "claude" ? "Claude Code and Desktop." : "Pi sessions.") : "Applies only to this client.")
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
                        var pending = chooseHarness ? drafts : [:]
                        pending[harness] = Draft(models: models, rules: rules, enabled: enabled)
                        var success = true
                        for key in pending.keys.sorted() {
                            guard let draft = pending[key] else { continue }
                            if !(await state.saveCompassPolicy(scope: client == nil ? "harness" : "client", id: client ?? key, enabled: draft.enabled, models: draft.models, rules: draft.rules)) { success = false; break }
                        }
                        saving = false
                        if success { dismiss() }
                        else { saveError = "Could not finish saving. Your draft is kept; retry to apply the remaining settings." }
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
