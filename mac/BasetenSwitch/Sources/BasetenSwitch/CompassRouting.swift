import Foundation
import SwiftUI

struct CompassConfiguration: Decodable {
    let router: String
    let comparison: String?
    let defaultModel: String
    let high: String
    let midHigh: String
    let mid: String
    let low: String
    enum CodingKeys: String, CodingKey {
        case router, high, mid, low, comparison
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
    let subagent: Bool
}
struct CompassSnapshot: Decodable {
    let config: CompassConfiguration
    let recent: [CompassRecentDecision]
    let usage: CompassUsage?
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
                                $0.slug != "Auto(Compass)"
                                    && $0.slug != "baseten-switch-compass"
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
                Text("Open Compass in the sidebar to see automatically updated session and subagent decisions.")
                    .font(.caption).foregroundStyle(.secondary)
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
    @ObservedObject var state: BasetenSwitchState

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("Compass").font(.largeTitle.weight(.semibold))
                Text("See which model Compass selected and why. Configure the routing model and tiers on the Codex page.")
                    .foregroundStyle(.secondary)
                if let snapshot = state.compassSnapshot {
                    HStack {
                        Text("Savings comparison model")
                        Spacer()
                        Picker("Savings comparison model", selection: Binding(
                            get: { snapshot.config.comparison ?? snapshot.config.high },
                            set: { model in Task { await state.setCompassModel("comparison", model: model) } }
                        )) {
                            let choices = state.codexPickerRows.filter {
                                $0.slug != "Auto(Compass)" && $0.slug != "baseten-switch-compass" && $0.slug != "baseten-switch-compat-v1"
                            }
                            let selected = snapshot.config.comparison ?? snapshot.config.high
                            if !choices.contains(where: { $0.slug == selected }) { Text(selected).tag(selected) }
                            ForEach(choices) { model in Text(model.displayName).tag(model.slug) }
                        }.labelsHidden().frame(width: 250)
                            .disabled(state.compassBusy || !state.canMutate)
                    }
                    if let usage = snapshot.usage { CompassUsageView(usage: usage) }
                    HStack {
                        Label("Recent sessions and subagents", systemImage: "location.north.circle")
                            .font(.subheadline.weight(.semibold))
                        Spacer()
                        Button("Refresh") { Task { await state.loadCompass() } }
                            .disabled(state.compassRefreshing || state.compassBusy)
                    }
                    Text("Updates every 5 seconds while this page is open. Each model stays fixed for its session. Up to 20 recent decisions; completed sessions may still appear.")
                        .font(.caption).foregroundStyle(.secondary)
                    if let updated = state.compassUpdatedAt {
                        Text("Last updated \(updated.formatted(date: .omitted, time: .standard))")
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                    if snapshot.recent.isEmpty {
                        Text("Choose Auto(Compass) in Codex and send a prompt to see the selected model here.")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    ForEach(snapshot.recent, id: \.stableID) { decision in
                        CompassDecisionRow(decision: decision)
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
                Text("Selection stays fixed for this session.").foregroundStyle(.secondary)
            }
            .font(.caption)
            .textSelection(.enabled)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 6)
        } label: {
            VStack(alignment: .leading, spacing: 5) {
                HStack {
                    Label("\(decision.client == "pi" ? "Pi" : "Codex") · \(decision.subagent ? "Subagent" : "Session")",
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
                if decision.tier == "default" {
                    Text(decision.reason).font(.caption).foregroundStyle(.orange)
                }
            }
        }
        .padding(10)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
    }
}

struct CompassUsage: Decodable {
    let repriced: Int?
    let priceSource: String?
    let baseline: String
    let requests: Int
    let priced: Int
    let compared: Int
    let actualUSD: Double
    let baselineUSD: Double
    let savedUSD: Double
    let partial: Bool
    let models: [CompassModelUsage]
    enum CodingKeys: String, CodingKey {
        case baseline, requests, priced, compared, partial, models, repriced
        case priceSource = "price_source"
        case actualUSD = "actual_usd"
        case baselineUSD = "baseline_usd"
        case savedUSD = "saved_usd"
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
private struct CompassUsageView: View {
    let usage: CompassUsage
    private func money(_ value: Double) -> String {
        value.formatted(.currency(code: "USD").precision(.fractionLength(4)))
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Usage and estimated savings · Last 24 hours").font(.headline)
            HStack(alignment: .top, spacing: 24) {
                metric("Requests", "\(usage.requests)")
                metric("Known model cost", usage.priced == 0 ? "Unavailable" : money(usage.actualUSD))
                metric("Estimated savings*", usage.compared == 0 ? "Unavailable" : money(usage.savedUSD))
            }
            Text("*Compared with using \(usage.baseline) for the same token usage, at cached API rates. Excludes routing-classifier cost; not subscription or billed savings. Negative values mean higher cost.")
                .font(.caption).foregroundStyle(.secondary)
            Text("\(usage.priced)/\(usage.requests) requests priced · \(usage.compared)/\(usage.requests) comparable. Missing prices or usage are excluded, never treated as free.")
                .font(.caption).foregroundStyle(.secondary)
            if let repriced = usage.repriced, repriced > 0 {
                Text("\(repriced) previously unpriced requests estimated using current available rates.")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if usage.priceSource?.hasPrefix("openai_official") == true {
                Link("OpenAI official API prices · reviewed Oct 11, 2026", destination: URL(string: "https://developers.openai.com/api/docs/pricing")!)
                    .font(.caption)
            }
            if usage.compared > 0 {
                Text("High-tier comparison cost: \(money(usage.baselineUSD))")
                    .font(.caption).foregroundStyle(.secondary)
            }
            if usage.partial {
                Label("Partial history: totals cover only readable recent records.", systemImage: "exclamationmark.triangle")
                    .font(.caption).foregroundStyle(.orange)
            }
            ForEach(usage.models, id: \.model) { model in
                HStack {
                    Text(model.model).textSelection(.enabled)
                    Spacer()
                    Text("\(model.requests) requests").foregroundStyle(.secondary)
                    Text(model.priced == 0 ? "Unpriced" : money(model.actualUSD))
                        .monospacedDigit()
                    if model.priced > 0 && model.priced < model.requests {
                        Text("Partial").font(.caption).foregroundStyle(.orange)
                    }
                }.font(.callout)
            }
            if usage.requests == 0 {
                Text("No recorded Compass requests in this window.").font(.caption).foregroundStyle(.secondary)
            }
        }
        .padding(16)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 10))
    }
    private func metric(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.title3.weight(.semibold)).monospacedDigit()
        }
    }
}
