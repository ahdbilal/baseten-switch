import SwiftUI

struct TrafficBreakdownsView: View {
    let snapshot: TrafficAnalyticsSnapshot
    @State private var showAllModels = false

    private var models: [TrafficCostRow] {
        snapshot.cost.models.sorted {
            $0.requests == $1.requests ? $0.id < $1.id : $0.requests > $1.requests
        }
    }
    private var total: Int { models.reduce(0) { $0 + $1.requests } }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("\(total.formatted()) requests").font(.title2.weight(.semibold))
                Spacer()
                Text("\(models.count) models · \(snapshot.harnesses.count) harnesses")
                    .foregroundStyle(.secondary)
            }
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: 16) {
                    modelCard.frame(minWidth: 250)
                    harnessCard.frame(minWidth: 250)
                }
                VStack(spacing: 16) { modelCard; harnessCard }
            }
        }
        .accessibilityIdentifier("traffic-breakdowns")
    }

    private var modelCard: some View {
        TrafficBreakdownCard(title: "Models", subtitle: "Actual models used · share of requests") {
            ForEach(Array(models.prefix(showAllModels ? models.count : 5))) { row in
                TrafficMixRow(row: row, total: total, tint: .accentColor)
            }
            if models.count > 5 {
                Button(showAllModels ? "Show fewer" : "Show all \(models.count) models") {
                    showAllModels.toggle()
                }.buttonStyle(.plain).foregroundStyle(Color.accentColor)
            }
        }
    }

    private var harnessCard: some View {
        TrafficBreakdownCard(title: "Harnesses", subtitle: "Expand a harness to see its models") {
            if snapshot.harnesses.isEmpty {
                Text("Harness breakdown unavailable. Update the gateway and refresh.")
                    .font(.caption).foregroundStyle(.secondary)
            }
            ForEach(snapshot.harnesses) { harness in
                DisclosureGroup {
                    VStack(alignment: .leading, spacing: 12) {
                        ForEach(harness.models.sorted { $0.requests > $1.requests }) { row in
                            TrafficMixRow(row: row, total: harness.cost.requests, tint: .teal)
                        }
                    }.padding(.leading, 10).padding(.top, 10)
                } label: {
                    TrafficMixRow(row: harness.costRow, total: total, tint: .teal)
                }
                .accessibilityIdentifier("traffic-harness-\(harness.client)")
            }
        }
    }
}

private struct TrafficBreakdownCard<Content: View>: View {
    let title: String
    let subtitle: String
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text(title).font(.headline)
                Text(subtitle).font(.caption).foregroundStyle(.secondary)
            }
            content
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background(Color.secondary.opacity(0.045), in: RoundedRectangle(cornerRadius: 12))
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(Color.secondary.opacity(0.12)))
    }
}

private struct TrafficMixRow: View {
    let row: TrafficCostRow
    let total: Int
    let tint: Color
    private var share: Double { total > 0 ? Double(row.requests) / Double(total) : 0 }
    private var cost: String {
        trafficCurrency(row.actualCostUSD) + (row.hasPartialCost ? " partial" : "")
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(row.label).font(.subheadline.weight(.medium)).lineLimit(1).help(row.modelID ?? row.label)
                Spacer(minLength: 4)
                Text(share.formatted(.percent.precision(.fractionLength(0))))
                    .font(.caption.monospacedDigit()).foregroundStyle(.secondary)
            }
            GeometryReader { geometry in
                Capsule().fill(tint.opacity(0.12))
                    .overlay(alignment: .leading) {
                        Capsule().fill(tint).frame(width: geometry.size.width * min(1, max(0, share)))
                    }
            }.frame(height: 5).accessibilityHidden(true)
            Text("\(row.requests.formatted()) requests · \(row.tokenUsage.cardDescription) · \(cost)")
                .font(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
    }
}
