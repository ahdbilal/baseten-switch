import Foundation

struct TrafficCompactCostSummary: Equatable {
    let value: Double?
    let partial: Bool

    init(rows: [TrafficCostRow]) {
        let known = rows.compactMap(\.actualCostUSD).filter { $0.isFinite }
        value = known.isEmpty ? nil : known.reduce(0, +)
        partial = !known.isEmpty && rows.contains {
            $0.unpricedRows > 0 || ($0.actualCostUSD == nil && $0.requests > 0)
        }
    }
}

extension TrafficAnalyticsSnapshot {
    func costRows(groupedBy grouping: TrafficGrouping) -> [TrafficCostRow] {
        let rows: [TrafficCostRow]
        switch grouping {
        case .model: rows = cost.models
        case .harness: rows = harnesses.map(\.costRow)
        case .provider: rows = cost.providers
        }
        return rows.sorted { $0.requests == $1.requests ? $0.id < $1.id : $0.requests > $1.requests }
    }

    func performanceRows(groupedBy grouping: TrafficGrouping) -> [TrafficPerformanceRow] {
        let rows: [TrafficPerformanceRow]
        switch grouping {
        case .model: rows = performance.models
        case .harness: rows = harnesses.map(\.performanceRow)
        case .provider: rows = performance.providers
        }
        return rows.sorted { $0.requests == $1.requests ? $0.id < $1.id : $0.requests > $1.requests }
    }
}

func trafficCompactModelLabel(_ label: String, modelID: String?) -> String {
    guard let modelID, !modelID.isEmpty else { return label }
    // Prefer a supplied human name. Only format IDs and legacy ID-like labels.
    guard label == modelID || label.hasPrefix("system.ai.") else { return label }
    var name = modelID.hasPrefix("system.ai.") ? String(modelID.dropFirst(10)) : modelID
    name = name.replacingOccurrences(of: "(?<=\\d)[ _-](?=\\d)", with: ".", options: .regularExpression)
    name = name.replacingOccurrences(of: "_", with: " ").replacingOccurrences(of: "-", with: " ")
    return name.split(separator: " ").map { word in
        switch word.lowercased() {
        case "gpt": return "GPT"
        case "glm": return "GLM"
        case "deepseek": return "DeepSeek"
        default: return word.prefix(1).uppercased() + word.dropFirst()
        }
    }.joined(separator: " ")
}

func trafficBarFraction(value: Double, maximum: Double) -> Double {
    guard value.isFinite, maximum.isFinite, maximum > 0 else { return 0 }
    return min(1, max(0, value / maximum))
}

func trafficCompactSeconds(_ milliseconds: Double?) -> String {
    guard let milliseconds, milliseconds.isFinite, milliseconds >= 0 else { return "—" }
    if milliseconds > 0, milliseconds < 10 { return "<0.01s" }
    return (milliseconds / 1000).formatted(.number.precision(.fractionLength(0...2))) + "s"
}
