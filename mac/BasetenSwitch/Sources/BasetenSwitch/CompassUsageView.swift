import SwiftUI

struct CompassUsageView: View {
    let usage: CompassUsage
    @State private var costMix = false
    private var comparedCost: Double { usage.comparedActualUSD ?? (usage.baselineUSD - usage.savedUSD) }
    private var modelCount: Int { usage.models.filter { !$0.model.isEmpty }.count }
    private var mixTotal: Double { costMix ? usage.actualUSD : Double(usage.requests) }
    private func money(_ value: Double) -> String {
        value.formatted(.currency(code: "USD").precision(.fractionLength(2...4)))
    }
    private func modelName(_ model: String) -> String {
        model.isEmpty ? "Unknown model" : model.components(separatedBy: "/").last ?? model
    }
    private func color(_ model: String) -> Color {
        let key = model.lowercased()
        if key.contains("kimi") { return .teal }
        if key.contains("glm") { return .blue }
        if key.contains("sol") { return .purple }
        if key.contains("astra") { return .orange }
        return .gray
    }
    private func mixValue(_ model: CompassModelUsage) -> Double {
        costMix ? model.actualUSD : Double(model.requests)
    }
    private var savingsDetail: String {
        guard usage.compared > 0, usage.baselineUSD > 0 else { return "No comparable priced usage" }
        let percent = abs(usage.savedUSD / usage.baselineUSD).formatted(.percent.precision(.fractionLength(0)))
        return "\(percent) \(usage.savedUSD >= 0 ? "less" : "more") than \(modelName(usage.baseline))"
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: 12) { metrics }.fixedSize(horizontal: true, vertical: false)
                VStack(alignment: .leading, spacing: 12) { metrics }
            }.frame(maxWidth: .infinity, alignment: .leading)
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: 16) {
                    modelMix.frame(width: 340)
                    comparison.frame(width: 280)
                }
                VStack(alignment: .leading, spacing: 16) { modelMix; comparison }
            }
            coverage
        }
        .accessibilityIdentifier("compass-overview")
    }
    @ViewBuilder private var metrics: some View {
        metric("Estimated savings", value: usage.compared == 0 ? "Unavailable" : money(usage.savedUSD), detail: savingsDetail)
        metric("Estimated cost", value: usage.priced == 0 ? "Unavailable" : money(usage.actualUSD), detail: "\(usage.priced) priced / \(usage.requests) requests")
        metric("Models used", value: "\(modelCount)", detail: usage.models.first.map { "Most used: \(modelName($0.model))" } ?? "No recorded usage")
    }
    private func metric(_ title: String, value: String, detail: String) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title).font(.caption).foregroundStyle(.secondary)
            Text(value).font(.title.weight(.semibold)).monospacedDigit()
            Text(detail).font(.caption).foregroundStyle(.secondary).lineLimit(2)
        }
        .frame(minWidth: 150, maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
    }
    private var modelMix: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Model mix").font(.headline)
                    Text(costMix ? "Share of known estimated cost" : "Share of requests")
                        .font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Picker("Measure", selection: $costMix) {
                    Text("Requests").tag(false)
                    Text("Cost").tag(true)
                }.labelsHidden().frame(width: 110)
            }
            if mixTotal > 0 {
                GeometryReader { geometry in
                    HStack(spacing: 0) {
                        ForEach(usage.models.filter { mixValue($0) > 0 }, id: \.model) { model in
                            Rectangle().fill(color(model.model))
                                .frame(width: max(0, geometry.size.width * mixValue(model) / mixTotal))
                                .help("\(modelName(model.model)): \((mixValue(model) / mixTotal).formatted(.percent.precision(.fractionLength(0))))")
                        }
                    }.clipShape(RoundedRectangle(cornerRadius: 5))
                }.frame(height: 24)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(costMix ? "Model shares of known cost; values listed below" : "Model shares of requests; values listed below")
            } else {
                Text(costMix ? "No priced usage available." : "No Compass requests in this period.")
                    .font(.callout).foregroundStyle(.secondary)
            }
            ForEach(usage.models, id: \.model) { model in
                HStack(spacing: 8) {
                    RoundedRectangle(cornerRadius: 2).fill(color(model.model)).frame(width: 9, height: 9)
                        .accessibilityHidden(true)
                    Text(modelName(model.model)).lineLimit(1).truncationMode(.middle).help(model.model)
                    Spacer(minLength: 4)
                    Text(mixTotal > 0 && (!costMix || model.priced > 0)
                         ? (mixValue(model) / mixTotal).formatted(.percent.precision(.fractionLength(0))) : "—")
                        .foregroundStyle(.secondary).monospacedDigit()
                    Text(costMix ? (model.priced == 0 ? "Unpriced" : money(model.actualUSD)) : "\(model.requests)")
                        .monospacedDigit()
                }.font(.callout)
            }
            if costMix && usage.priced < usage.requests {
                Text("Unpriced requests are excluded from cost shares.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }.padding(18).frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
    }
    private var comparison: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Cost comparison").font(.headline)
            Text("Same-token API estimate · \(usage.compared) comparable requests")
                .font(.caption).foregroundStyle(.secondary)
            if usage.compared > 0 {
                costBar(modelName(usage.baseline) + " only", value: usage.baselineUSD, tint: .secondary)
                costBar("Compass", value: comparedCost, tint: .teal)
                Text("Estimated \(usage.savedUSD >= 0 ? "reduction" : "increase"): \(money(abs(usage.savedUSD)))")
                    .font(.caption).foregroundStyle(.secondary)
            } else {
                Text("Comparison unavailable until token usage and both model prices are available.")
                    .font(.callout).foregroundStyle(.secondary)
            }
        }.padding(18).frame(maxWidth: .infinity, alignment: .leading)
            .background(Color(nsColor: .controlBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
    }
    private func costBar(_ label: String, value: Double, tint: Color) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(label).lineLimit(1).truncationMode(.middle).help(label)
                Spacer()
                Text(money(value)).monospacedDigit()
            }.font(.callout)
            GeometryReader { geometry in
                let ceiling = max(usage.baselineUSD, comparedCost)
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 4).fill(Color.secondary.opacity(0.10))
                    RoundedRectangle(cornerRadius: 4).fill(tint)
                        .frame(width: ceiling > 0 ? max(0, geometry.size.width * value / ceiling) : 0)
                }
            }.frame(height: 16).accessibilityHidden(true)
        }
    }
    private var coverage: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("Pricing coverage: \(usage.priced)/\(usage.requests) requests · \(usage.compared) comparable")
            Text("Excludes routing overhead. Same-token API estimates, not subscription or billed savings. Missing prices or usage are never treated as free.")
            if let missing = usage.incompleteUsage, missing > 0 {
                Text("\(missing) requests lack complete token usage; new prices cannot recover those costs.")
            }
            if let missing = usage.missingPrices, missing > 0 {
                Text("\(missing) requests lack matching prices.")
            }
            if let repriced = usage.repriced, repriced > 0 {
                Text("\(repriced) previously unpriced requests estimated using current available rates.")
            }
            if usage.priceSource == "portkey_pricing" {
                Link("Portkey pricing", destination: URL(string: "https://portkey.ai/models")!)
            }
            if usage.priceSource?.hasPrefix("openai_official") == true {
                Link("OpenAI prices · reviewed Oct 11, 2026", destination: URL(string: "https://developers.openai.com/api/docs/pricing")!)
            }
            if usage.partial {
                Label("Partial history: totals cover only readable recent records, not the full selected period.", systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.orange)
            }
        }.font(.caption).foregroundStyle(.secondary)
    }
}
