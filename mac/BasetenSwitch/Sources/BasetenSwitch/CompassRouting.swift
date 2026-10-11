import Foundation
import SwiftUI

struct CompassConfiguration: Decodable {
    let router: String
    let defaultModel: String
    let high: String
    let midHigh: String
    let mid: String
    let low: String
    enum CodingKeys: String, CodingKey {
        case router, high, mid, low
        case defaultModel = "default"
        case midHigh = "mid_high"
    }
    func model(for field: String) -> String {
        switch field {
        case "router": return router
        case "default": return defaultModel
        case "high": return high
        case "mid_high": return midHigh
        case "mid": return mid
        default: return low
        }
    }
}
struct CompassRecentDecision: Decodable {
    let model: String
    let tier: String
    let reason: String
    let subagent: Bool
}
struct CompassSnapshot: Decodable {
    let config: CompassConfiguration
    let recent: [CompassRecentDecision]
}

struct CompassRoutingView: View {
    @ObservedObject var state: BasetenSwitchState
    private let fields = [
        ("router", "Routing model"),
        ("default", "Default / fallback"),
        ("high", "High"),
        ("mid_high", "Mid-high"),
        ("mid", "Mid"),
        ("low", "Low"),
    ]
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Select Compass in Codex to choose a model automatically at the start of each session and new subagent.")
                .font(.callout).foregroundStyle(.secondary)
            if let snapshot = state.compassSnapshot {
                ForEach(fields, id: \.0) { field in
                    HStack {
                        Text(field.1)
                        Spacer()
                        Picker(field.1, selection: Binding(
                            get: { snapshot.config.model(for: field.0) },
                            set: { model in Task { await state.setCompassModel(field.0, model: model) } }
                        )) {
                            let choices = state.codexPickerRows.filter {
                                $0.slug != "baseten-switch-compass"
                                    && $0.slug != "baseten-switch-compat-v1"
                                    && (field.0 != "router" || $0.slug.contains("/"))
                            }
                            if !choices.contains(where: { $0.slug == snapshot.config.model(for: field.0) }) {
                                Text(snapshot.config.model(for: field.0))
                                    .tag(snapshot.config.model(for: field.0))
                            }
                            ForEach(choices) { model in
                                Text(model.displayName).tag(model.slug)
                            }
                        }
                        .labelsHidden().pickerStyle(.menu).frame(width: 250)
                        .disabled(state.compassBusy || !state.canMutate)
                    }
                }
                Text("The routing model assesses the prompt and context. If it cannot choose, Compass uses the default. Changes apply to new sessions and subagents; existing selections stay fixed.")
                    .font(.caption).foregroundStyle(.secondary)
                Divider()
                HStack {
                    Text("Recent selections").font(.subheadline.weight(.semibold))
                    Spacer()
                    Button("Refresh") { Task { await state.loadCompass() } }
                        .disabled(state.compassBusy)
                }
                if snapshot.recent.isEmpty {
                    Text("Start a session with Compass to see its choice here.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                ForEach(Array(snapshot.recent.enumerated()), id: \.offset) { _, decision in
                    VStack(alignment: .leading, spacing: 2) {
                        Text("\(decision.subagent ? "Subagent" : "Session") · \(decision.model)")
                            .font(.callout)
                        Text(decision.reason).font(.caption).foregroundStyle(.secondary)
                    }
                }
            }
            if state.compassBusy { ProgressView().controlSize(.small) }
            if let notice = state.compassNotice {
                Text(notice).font(.caption).foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .task {
            await state.loadCompass()
            if state.codexPickerRows.isEmpty { await state.loadCodexPicker() }
        }
        .accessibilityIdentifier("compass-model-routing")
    }
}
