import Foundation
import SwiftUI

struct CompassRules: Decodable {
    let session_start: Bool?
    let subagents: Bool?
    let session_switching: Bool?
    let encourage_subagents: Bool?
    let verbose: Bool?
    func enabled(_ field: String) -> Bool {
        switch field {
        case "session_start": return session_start ?? true
        case "subagents": return subagents ?? true
        case "session_switching": return session_switching ?? false
        case "encourage_subagents": return encourage_subagents ?? false
        default: return verbose ?? false
        }
    }
}
struct CompassConfiguration: Decodable {
    let rules: CompassRules?
    let manualModels: [String: String]?
    let router: String
    let comparison: String?
    let defaultModel: String
    let high: String
    let midHigh: String
    let mid: String
    let low: String
    enum CodingKeys: String, CodingKey {
        case router, high, mid, low, comparison, rules
        case manualModels = "manual_models"
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
    let id: String?
    let client: String?
    let router: String
    let createdAt: String
    enum CodingKeys: String, CodingKey {
        case id, client, router, model, tier, reason, subagent
        case reviewReason = "review_reason"
        case createdAt = "created_at"
    }
    var stableID: String { id ?? "\(createdAt)-\(model)-\(subagent)" }
    var date: Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.date(from: createdAt) ?? ISO8601DateFormatter().date(from: createdAt)
    }
    var tierLabel: String {
        switch tier {
        case "default": return "Fallback"
        case "mid_high": return "Mid-high"
        default: return tier.capitalized
        }
    }
    let model: String
    let tier: String
    let reason: String
    let reviewReason: String?
    let subagent: Bool
}
struct CompassClientSelection: Decodable {
    let enabled: Bool
    let model: String
}
struct CompassSnapshot: Decodable {
    let clients: [String: CompassClientSelection]?
    let config: CompassConfiguration
    let recent: [CompassRecentDecision]
    let usage: CompassUsage?
}

struct CompassRoutingView: View {
    @ObservedObject var state: UnityGatewayState
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
                                $0.slug != "Auto(Compass)"
                                    && $0.slug != "unity-gateway-compass"
                                    && $0.slug != "unity-gateway-compat-v1"
                                    && (field.0 != "router" || $0.slug.contains("/") || $0.slug.split(separator: ".").count >= 3)
                            }
                            if !choices.contains(where: { $0.slug == snapshot.config.model(for: field.0) }) {
                                Text(snapshot.config.model(for: field.0))
                                    .tag(snapshot.config.model(for: field.0))
                            }
                            ForEach(groupedModels(choices, identifier: { $0.slug }, name: { $0.displayName })) { group in
                                Section(group.family.rawValue) {
                                    ForEach(group.models) { model in Text(model.displayName).tag(model.slug) }
                                }
                            }
                        }
                        .labelsHidden().pickerStyle(.menu).frame(width: 250)
                        .disabled(state.compassBusy || !state.canMutate)
                    }
                }

            }
            if state.compassBusy || (state.compassRefreshing && state.compassSnapshot == nil) {
                ProgressView().controlSize(.small)
            }
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

struct CompassActivityView: View {
    @ObservedObject var state: UnityGatewayState
    @State private var showDecisions = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("Compass").font(.largeTitle.weight(.semibold))
                Text("Savings and model usage at a glance")
                    .foregroundStyle(.secondary)
                if let snapshot = state.compassSnapshot {
                    HStack {
                        Picker("Period", selection: $state.compassPeriod) {
                            Text("Last 24 hours").tag("24h")
                            Text("7 days").tag("7d")
                            Text("30 days").tag("30d")
                        }.frame(maxWidth: 190)
                        Picker("Client", selection: $state.compassClient) {
                            Text("All clients").tag("all")
                            Text("Pi").tag("pi")
                            Text("Codex").tag("codex")
                            Text("Claude Code").tag("claude-code")
                        }.frame(maxWidth: 150)
                        Spacer()
                        Picker("Savings comparison model", selection: Binding(
                            get: { snapshot.config.comparison ?? snapshot.config.high },
                            set: { model in Task { await state.setCompassModel("comparison", model: model) } }
                        )) {
                            let choices = state.codexPickerRows.filter {
                                $0.slug != "Auto(Compass)" && $0.slug != "unity-gateway-compass" && $0.slug != "unity-gateway-compat-v1"
                            }
                            let selected = snapshot.config.comparison ?? snapshot.config.high
                            if !choices.contains(where: { $0.slug == selected }) { Text(selected).tag(selected) }
                            ForEach(groupedModels(choices, identifier: { $0.slug }, name: { $0.displayName })) { group in
                                Section(group.family.rawValue) {
                                    ForEach(group.models) { model in Text(model.displayName).tag(model.slug) }
                                }
                            }
                        }.frame(maxWidth: 300)
                            .disabled(state.compassBusy || !state.canMutate)
                    }
                    .disabled(state.compassRefreshing || state.compassBusy)
                    if let usage = snapshot.usage { CompassUsageView(usage: usage) }
                    DisclosureGroup("Recent routing decisions", isExpanded: $showDecisions) {
                    HStack {
                        Label("Recent sessions and subagents", systemImage: "location.north.circle")
                            .font(.subheadline.weight(.semibold))
                        Spacer()
                        Button("Refresh") { Task { await state.loadCompass() } }
                            .disabled(state.compassRefreshing || state.compassBusy)
                    }
                    Text("Updates every 5 seconds while this page is open. Main-session switches require the enabled rule and a successful cost/cache check. Up to 20 recent decisions; completed sessions may still appear.")
                        .font(.caption).foregroundStyle(.secondary)
                    if let updated = state.compassUpdatedAt {
                        Text("Last updated \(updated.formatted(date: .omitted, time: .standard))")
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                    if snapshot.recent.isEmpty {
                        Text("Choose Auto(Compass) in Pi, Codex, or Claude Code and send a prompt to see the selected model here.")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    ForEach(snapshot.recent.filter { state.compassClient == "all" || ($0.client ?? "codex") == state.compassClient }, id: \.stableID) { decision in
                        CompassDecisionRow(decision: decision)
                    }
                    }
                } else if state.compassRefreshing {
                    ProgressView("Loading routing decisions…")
                }
                if let notice = state.compassNotice {
                    Text(notice).font(.caption).foregroundStyle(.secondary)
                }
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .task {
            await state.loadCompass()
            if state.codexPickerRows.isEmpty { await state.loadCodexPicker() }
            while !Task.isCancelled {
                do { try await Task.sleep(nanoseconds: 5_000_000_000) }
                catch { break }
                guard !Task.isCancelled else { break }
                await state.loadCompass()
            }
        }
        .onChange(of: state.compassPeriod) { _ in Task { await state.loadCompass() } }
        .onChange(of: state.compassClient) { _ in Task { await state.loadCompass() } }
        .accessibilityIdentifier("compass-activity")
    }
}

private struct CompassDecisionRow: View {
    let decision: CompassRecentDecision
    @State private var expanded = false

    var body: some View {
        DisclosureGroup(isExpanded: $expanded) {
            VStack(alignment: .leading, spacing: 6) {
                Text(decision.reason)
                Text("Routing model: \(decision.router)").foregroundStyle(.secondary)
            }
            .font(.caption)
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 6)
        } label: {
            VStack(alignment: .leading, spacing: 5) {
                HStack {
                    Label("\(decision.client == "pi" ? "Pi" : (decision.client == "claude-code" ? "Claude Code" : "Codex")) · \(decision.subagent ? "Subagent" : "Session")",
                          systemImage: decision.subagent ? "arrow.triangle.branch" : "bubble.left")
                    if let id = decision.id {
                        Text(String(id.prefix(8))).font(.caption.monospaced()).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Text(decision.tierLabel)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(decision.tier == "default" ? Color.orange : Color.accentColor)
                    if let date = decision.date {
                        Text(date.formatted(date: .abbreviated, time: .standard))
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                }
                Text(decision.model).font(.callout.weight(.medium)).textSelection(.enabled)
                if let review = decision.reviewReason, !review.isEmpty {
                    Text(review).font(.caption).foregroundStyle(.secondary).lineLimit(2)
                } else if decision.tier == "default" {
                    Text(decision.reason).font(.caption).foregroundStyle(.orange)
                }
            }
        }
        .padding(10)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
    }
}

struct CompassUsage: Decodable {
    let incompleteUsage: Int?
    let missingPrices: Int?
    let repriced: Int?
    let priceSource: String?
    let baseline: String
    let requests: Int
    let priced: Int
    let compared: Int
    let actualUSD: Double
    let baselineUSD: Double
    let savedUSD: Double
    let comparedActualUSD: Double?
    let partial: Bool
    let models: [CompassModelUsage]
    enum CodingKeys: String, CodingKey {
        case incompleteUsage = "incomplete_usage"
        case missingPrices = "missing_prices"
        case baseline, requests, priced, compared, partial, models, repriced
        case priceSource = "price_source"
        case actualUSD = "actual_usd"
        case baselineUSD = "baseline_usd"
        case savedUSD = "saved_usd"
        case comparedActualUSD = "compared_actual_usd"
    }
}
struct CompassModelUsage: Decodable {
    let model: String
    let requests: Int
    let priced: Int
    let actualUSD: Double
    enum CodingKeys: String, CodingKey {
        case model, requests, priced
        case actualUSD = "actual_usd"
    }
}
