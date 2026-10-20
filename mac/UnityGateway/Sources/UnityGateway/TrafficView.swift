import SwiftUI

struct TrafficView: View {
    @ObservedObject var store: TrafficStore
    @State private var selectedTab: TrafficTab = .cost
    @State private var grouping: TrafficGrouping = .model

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                header
                status
                if let snapshot = store.snapshot, !snapshot.isEmpty {
                    TrafficCompactSummary(snapshot: snapshot, tab: selectedTab)
                    controls
                }
                content
                footer
            }
            .padding(20)
            .frame(maxWidth: 1000, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .accessibilityIdentifier("traffic")
        .onAppear { store.start() }
        .onDisappear { store.stop() }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline) {
                Label("Traffic", systemImage: "chart.bar.xaxis")
                    .font(.title2.weight(.semibold))
                Spacer()
                rangePicker
                Button {
                    store.requestRefresh()
                } label: {
                    if store.isRefreshing {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Image(systemName: "arrow.clockwise")
                    }
                }
                .buttonStyle(.borderless)
                .disabled(store.isLoading || store.isRefreshing)
                .help("Refresh traffic data")
                .accessibilityLabel("Refresh traffic data")
            }

        }
    }

    private var controls: some View {
        ViewThatFits(in: .horizontal) {
            HStack {
                viewPicker
                Spacer(minLength: 16)
                groupingPicker
            }
            VStack(alignment: .leading, spacing: 8) {
                viewPicker
                groupingPicker
            }
        }
    }

    @ViewBuilder private var footer: some View {
        if let snapshot = store.snapshot, !snapshot.isEmpty {
            HStack {
                if let lastUpdated = store.lastUpdated {
                    Text("Updated \(lastUpdated.formatted(.relative(presentation: .named)))")
                }
                Spacer()
                Text(selectedTab == .cost ? "Cost is estimated" : "TTFT = time to first token")
            }.font(.caption).foregroundStyle(.secondary)
        }
    }

    private var viewPicker: some View {
        Picker("View", selection: $selectedTab) {
            ForEach(TrafficTab.allCases) { tab in Text(tab.rawValue).tag(tab) }
        }.pickerStyle(.segmented).labelsHidden().frame(width: 190)
    }

    private var groupingPicker: some View {
        Picker("Traffic breakdown", selection: $grouping) {
            ForEach(TrafficGrouping.allCases) { group in Text(group.rawValue).tag(group) }
        }.pickerStyle(.segmented).labelsHidden().frame(width: 275)
            .accessibilityIdentifier("traffic-breakdown-picker")
    }

    private var rangePicker: some View {
        Picker("Time range", selection: $store.range) {
            ForEach(TrafficRange.allCases) { range in
                Text(range.label).tag(range)
            }
        }
        .pickerStyle(.segmented)
        .labelsHidden()
        .frame(width: 150)
        .accessibilityLabel("Relative time range")
    }

    @ViewBuilder
    private var status: some View {
        if let snapshot = store.snapshot {
            if let message = trafficCollectionStatusMessage(
                snapshot.coverage) {
                trafficBanner(
                    message,
                    symbol: "pause.circle",
                    color: .secondary)
                    .accessibilityIdentifier(
                        "traffic-collection-paused")
            }
            if store.isStale {
                trafficBanner(
                    "Showing the last available traffic data. \(store.errorMessage ?? "Refresh failed.")",
                    symbol: "wifi.exclamationmark",
                    color: .orange)
            }
            if !snapshot.coverage.complete {
                trafficBanner(
                    partialHistoryMessage(snapshot.coverage),
                    symbol: "clock.badge.exclamationmark",
                    color: .orange)
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        if store.isLoading, store.snapshot == nil {
            VStack(spacing: 12) {
                ProgressView()
                Text("Loading traffic data…")
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, minHeight: 300)
        } else if let snapshot = store.snapshot {
            if snapshot.isEmpty {
                trafficEmptyState
            } else if selectedTab == .cost {
                TrafficCostView(snapshot: snapshot, grouping: grouping)
            } else {
                TrafficPerformanceView(
                    snapshot: snapshot,
                    grouping: grouping)
            }
        } else {
            trafficErrorState
        }
    }

    private var trafficEmptyState: some View {
        VStack(spacing: 12) {
            Image(systemName: "chart.bar.xaxis")
                .font(.system(size: 36))
                .foregroundStyle(.secondary)
            Text("No traffic in this range")
                .font(.title3.weight(.semibold))
            Text("Requests will appear here after Databricks Unity Gateway records eligible local traffic.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
                .frame(maxWidth: 420)
        }
        .frame(maxWidth: .infinity, minHeight: 300)
    }

    private var trafficErrorState: some View {
        VStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 34))
                .foregroundStyle(.orange)
            Text("Traffic is unavailable")
                .font(.title3.weight(.semibold))
            Text(store.errorMessage ?? "The gateway did not return traffic data.")
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
            Button("Try Again") {
                store.requestRefresh()
            }
        }
        .frame(maxWidth: .infinity, minHeight: 300)
    }

    private func partialHistoryMessage(_ coverage: TrafficCoverage) -> String {
        if coverage.reason.isEmpty {
            return "Partial history is available for the selected range."
        }
        return "Partial history: \(coverage.reason)"
    }
}

func trafficCollectionStatusMessage(
    _ coverage: TrafficCoverage
) -> String? {
    guard !coverage.collectionEnabled else { return nil }
    return "Collection paused. Retained traffic history remains available."
}

private struct TrafficCompactSummary: View {
    let snapshot: TrafficAnalyticsSnapshot
    let tab: TrafficTab

    private var summary: TrafficCompactCostSummary {
        TrafficCompactCostSummary(rows: snapshot.cost.providers)
    }
    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(alignment: .firstTextBaseline, spacing: 16) {
                if tab == .cost { cost }
                counts
            }
            VStack(alignment: .leading, spacing: 6) {
                if tab == .cost { cost }
                counts
            }
        }.font(.callout).foregroundStyle(.secondary)
    }
    private var cost: some View {
        HStack(alignment: .firstTextBaseline, spacing: 5) {
            Text(trafficCurrency(summary.value) + (summary.partial ? "*" : ""))
                .font(.title3.weight(.medium)).foregroundStyle(.primary).monospacedDigit()
            Text("est. cost")
        }.help(summary.partial ? "Known cost only; some requests have no price or complete usage." : "Estimated from recorded usage and API prices.")
    }
    private var counts: some View {
        HStack(spacing: 14) {
            Text("\(snapshot.coverage.requestRows.formatted()) requests")
            Text("\(snapshot.cost.models.count) models")
            Text("\(snapshot.harnesses.count) harnesses")
        }.monospacedDigit()
    }
}

private struct TrafficCostView: View {
    let snapshot: TrafficAnalyticsSnapshot
    let grouping: TrafficGrouping
    private var rows: [TrafficCostRow] { snapshot.costRows(groupedBy: grouping) }
    var body: some View {
        TrafficActualSpendTable(rows: rows, grouping: grouping, snapshot: snapshot)
            .id(grouping)
    }
}

private struct TrafficPerformanceView: View {
    let snapshot: TrafficAnalyticsSnapshot
    let grouping: TrafficGrouping
    var body: some View {
        TrafficPerformanceTable(rows: snapshot.performanceRows(groupedBy: grouping), grouping: grouping)
            .id(grouping)
            .accessibilityIdentifier("traffic-performance-" + grouping.rawValue.lowercased())
    }
}

struct TrafficPerformanceCardContent: Equatable, Identifiable {
    let title: String
    let value: String
    let detail: String
    let brand: TrafficBrand
    let detailBrand: TrafficBrand

    var id: String { title }
}

func trafficPerformanceCardContents(
    claude: TrafficPerformanceRow?,
    databricks: TrafficPerformanceRow?
) -> [TrafficPerformanceCardContent] {
    let databricksTTFT = databricks?.measuredMedianTTFTMS
    let claudeTTFT = claude?.measuredMedianTTFTMS
    let databricksTPS = databricks?.measuredMedianOutputTokensPerSecond
    let claudeTPS = claude?.measuredMedianOutputTokensPerSecond

    return [
        TrafficPerformanceCardContent(
            title: "Unity Gateway median TTFT",
            value: trafficMilliseconds(databricksTTFT),
            detail: trafficComparisonDetail(
                value: "Claude \(trafficMilliseconds(claudeTTFT))",
                comparison: trafficPerformanceSpeedComparison(
                    databricks: databricksTTFT,
                    claude: claudeTTFT,
                    lowerIsBetter: true)),
            brand: .databricks,
            detailBrand: .claude),
        TrafficPerformanceCardContent(
            title: "Unity Gateway median output speed",
            value: trafficTPS(databricksTPS),
            detail: trafficComparisonDetail(
                value: "Claude \(trafficTPS(claudeTPS))",
                comparison: trafficPerformanceSpeedComparison(
                    databricks: databricksTPS,
                    claude: claudeTPS,
                    lowerIsBetter: false)),
            brand: .databricks,
            detailBrand: .claude),
        TrafficPerformanceCardContent(
            title: databricks.map { $0.tokenBreakdown == nil } == true
                ? "Unity Gateway input + output" : "Unity Gateway total tokens",
            value: (databricks?.tokenUsage.value ?? "Unavailable")
                + (databricks?.tokenUsage.isPartial == true ? " · partial" : ""),
            detail: "Claude \(claude?.tokenUsage.cardDescription ?? "tokens unavailable")",
            brand: .databricks,
            detailBrand: .claude),
    ]
}

func trafficPerformanceSpeedComparison(
    databricks: Double?,
    claude: Double?,
    lowerIsBetter: Bool
) -> String? {
    guard let databricks, let claude,
          databricks.isFinite, claude.isFinite,
          databricks > 0, claude > 0 else {
        return nil
    }
    let ratio = lowerIsBetter
        ? claude / databricks
        : databricks / claude
    guard ratio.isFinite, ratio > 0 else { return nil }
    if abs(ratio - 1) < 0.05 {
        return "about the same"
    }
    if ratio > 1 {
        return ratio.formatted(
            .number.precision(.fractionLength(1))) + "× faster"
    }
    return (1 / ratio).formatted(
        .number.precision(.fractionLength(1))) + "× slower"
}

private func trafficComparisonDetail(
    value: String,
    comparison: String?
) -> String {
    guard let comparison else { return value }
    return "\(value) · \(comparison)"
}

private struct TrafficActualSpendTable: View {
    let rows: [TrafficCostRow]
    let grouping: TrafficGrouping
    let snapshot: TrafficAnalyticsSnapshot
    @State private var isExpanded = false
    @State private var expandedGroups: Set<String> = []
    private var visibleRows: [TrafficCostRow] {
        Array(rows.prefix(trafficVisibleRowCount(totalCount: rows.count,
            isExpanded: isExpanded, collapsesLongLists: grouping == .model)))
    }
    private var maximum: Double { rows.compactMap(\.actualCostUSD).max() ?? 0 }
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ViewThatFits(in: .horizontal) {
                table(tokens: true).frame(minWidth: 530)
                table(tokens: false)
            }
            trafficExpansionButton(totalCount: rows.count, isExpanded: $isExpanded,
                collapsesLongLists: grouping == .model, identifier: "traffic-actual-spend-expand")
        }.accessibilityIdentifier("traffic-compact-cost-table")
    }
    private func children(_ row: TrafficCostRow) -> [TrafficCostRow] {
        switch grouping {
        case .model: return []
        case .harness: return snapshot.harnesses.first { $0.costRow.id == row.id }?.models ?? []
        case .provider: return snapshot.cost.models.filter { $0.provider == row.provider }
        }
    }
    private func table(tokens: Bool) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 14) {
                trafficTableHeader(grouping.rawValue).frame(maxWidth: .infinity, alignment: .leading)
                trafficTableHeader("Requests").frame(width: 62, alignment: .trailing)
                if tokens { trafficTableHeader("Tokens").frame(width: 100, alignment: .trailing) }
                trafficTableHeader("Cost").frame(width: tokens ? 135 : 95, alignment: .trailing)
            }.padding(.vertical, 7)
            Divider()
            ForEach(visibleRows) { row in
                costRow(row, tokens: tokens, child: false)
                if expandedGroups.contains(row.id) {
                    ForEach(children(row).sorted { $0.requests > $1.requests }) { child in
                        costRow(child, tokens: tokens, child: true)
                    }
                }
                Divider()
            }
        }
    }
    private func costRow(_ row: TrafficCostRow, tokens: Bool, child: Bool) -> some View {
        HStack(spacing: 14) {
            HStack(spacing: 6) {
                if !child && !children(row).isEmpty {
                    Button {
                        if !expandedGroups.insert(row.id).inserted { expandedGroups.remove(row.id) }
                    } label: {
                        Image(systemName: expandedGroups.contains(row.id) ? "chevron.down" : "chevron.right")
                            .font(.caption2).frame(width: 10)
                    }.buttonStyle(.borderless)
                        .accessibilityLabel("\(expandedGroups.contains(row.id) ? "Collapse" : "Expand") \(row.label) models")
                }
                TrafficProviderLabel(provider: row.provider,
                    label: trafficCompactModelLabel(row.label, modelID: row.modelID))
                    .lineLimit(1).help(row.modelID ?? row.label)
            }.padding(.leading, child ? 16 : 0)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(row.requests.formatted()).frame(width: 62, alignment: .trailing)
            if tokens {
                TrafficTokenCell(usage: row.tokenUsage, label: row.label)
                    .frame(width: 100, alignment: .trailing)
            }
            HStack(spacing: 8) {
                if tokens, let value = row.actualCostUSD {
                    TrafficInlineBar(value: value, maximum: maximum).frame(width: 50)
                }
                Text(trafficCurrency(row.actualCostUSD) + (row.hasPartialCost ? "*" : ""))
                    .frame(maxWidth: .infinity, alignment: .trailing)
                    .help(row.actualCostUSD == nil ? "No matching price or complete usage" : row.hasPartialCost
                        ? "Known cost only; \(row.unpricedRows) requests unpriced" : "Estimated API cost")
            }.frame(width: tokens ? 135 : 95, alignment: .trailing)
        }.font(.callout).monospacedDigit().padding(.vertical, 9)
            .accessibilityElement(children: .contain)
    }
}

private struct TrafficInlineBar: View {
    let value: Double
    let maximum: Double
    var body: some View {
        GeometryReader { geometry in
            Capsule().fill(Color.accentColor.opacity(0.12))
                .overlay(alignment: .leading) {
                    Capsule().fill(Color.accentColor)
                        .frame(width: geometry.size.width * trafficBarFraction(value: value, maximum: maximum))
                }
        }.frame(height: 4).accessibilityHidden(true)
    }
}

private struct TrafficPerformanceTable: View {
    let rows: [TrafficPerformanceRow]
    let grouping: TrafficGrouping
    @State private var isExpanded = false
    private var visibleRows: [TrafficPerformanceRow] {
        Array(rows.prefix(trafficVisibleRowCount(totalCount: rows.count,
            isExpanded: isExpanded, collapsesLongLists: grouping == .model)))
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ViewThatFits(in: .horizontal) {
                table(tokens: true).frame(minWidth: 560)
                table(tokens: false)
            }
            trafficExpansionButton(totalCount: rows.count, isExpanded: $isExpanded,
                collapsesLongLists: grouping == .model, identifier: "traffic-performance-expand")
        }.accessibilityIdentifier("traffic-compact-performance-table")
    }
    private func table(tokens: Bool) -> some View {
        VStack(spacing: 0) {
            HStack(spacing: 14) {
                trafficTableHeader(grouping.rawValue).frame(maxWidth: .infinity, alignment: .leading)
                trafficTableHeader("Requests").frame(width: 62, alignment: .trailing)
                if tokens { trafficTableHeader("Tokens").frame(width: 100, alignment: .trailing) }
                trafficTableHeader("TTFT").frame(width: 68, alignment: .trailing)
                    .help("Median time to first token")
                trafficTableHeader("Tok/s").frame(width: 68, alignment: .trailing)
                    .help("Median output tokens per second")
            }.padding(.vertical, 7)
            Divider()
            ForEach(visibleRows) { row in
                HStack(spacing: 14) {
                    TrafficProviderLabel(provider: row.provider,
                        label: trafficCompactModelLabel(row.label, modelID: row.modelID))
                        .lineLimit(1).help(row.modelID ?? row.label)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Text(row.requests.formatted()).frame(width: 62, alignment: .trailing)
                    if tokens {
                        TrafficTokenCell(usage: row.tokenUsage, label: row.label)
                            .frame(width: 100, alignment: .trailing)
                    }
                    Text(trafficCompactSeconds(row.measuredMedianTTFTMS))
                        .frame(width: 68, alignment: .trailing)
                        .help("Median time to first token · \(row.ttftSamples) measured requests")
                    Text(row.measuredMedianOutputTokensPerSecond.map {
                        $0.formatted(.number.precision(.fractionLength(0...1)))
                    } ?? "—")
                        .frame(width: 68, alignment: .trailing)
                        .help("Median output tokens per second · \(row.outputTPSSamples) measured requests")
                }.font(.callout).monospacedDigit().padding(.vertical, 9)
                    .accessibilityElement(children: .contain)
                Divider()
            }
        }
    }
}

private struct TrafficTokenCell: View {
    let usage: TrafficTokenUsage
    let label: String
    @State private var showsDetails = false
    @FocusState private var detailsFocused: Bool

    var body: some View {
        VStack(alignment: .trailing, spacing: 2) {
            HStack(spacing: 5) {
                Text((usage.isUnavailable ? "—" : usage.value) + (usage.isPartial ? "*" : ""))
                    .help(usage.accessibilitySummary)
                    .monospacedDigit()
                    .accessibilityLabel(usage.accessibilitySummary)
                if let breakdown = usage.breakdown {
                    Button {
                        showsDetails.toggle()
                    } label: {
                        Image(systemName: "info.circle")
                    }
                    .buttonStyle(.borderless)
                    .focused($detailsFocused)
                    .help("Show token details")
                    .accessibilityLabel("Token details for \(label)")
                    .popover(isPresented: $showsDetails, arrowEdge: .bottom) {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Token details")
                                .font(.headline)
                            Text(label)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                            Grid(horizontalSpacing: 24, verticalSpacing: 8) {
                                detailRow("Uncached input", value: breakdown.inputTokens)
                                detailRow("Output", value: breakdown.outputTokens)
                                detailRow("Cache reads", value: breakdown.cacheReadInputTokens)
                                detailRow("Cache writes", value: breakdown.cacheWriteInputTokens)
                                Divider()
                                detailRow("Total tokens", value: usage.tokens)
                                    .fontWeight(.semibold)
                            }
                            Text(usage.coverageMessage ?? "")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            Button("Done") { showsDetails = false }
                                .keyboardShortcut(.cancelAction)
                                .frame(maxWidth: .infinity, alignment: .trailing)
                        }
                        .padding(18)
                        .frame(width: 320)
                    }
                    .onChange(of: showsDetails) { isPresented in
                        if !isPresented { detailsFocused = true }
                    }
                }
            }

        }
    }

    private func detailRow(_ title: String, value: Int64) -> some View {
        GridRow {
            Text(title)
                .frame(maxWidth: .infinity, alignment: .leading)
            Text(usage.isUnavailable ? "Unavailable" : value.formatted())
                .monospacedDigit()
        }
        .accessibilityElement(children: .contain)
    }
}

private let trafficCollapsedTableRowLimit = 8

func trafficVisibleRowCount(
    totalCount: Int,
    isExpanded: Bool,
    collapsesLongLists: Bool
) -> Int {
    guard collapsesLongLists, !isExpanded else {
        return max(0, totalCount)
    }
    return min(max(0, totalCount), trafficCollapsedTableRowLimit)
}

func trafficExpansionButtonTitle(
    totalCount: Int,
    isExpanded: Bool,
    collapsesLongLists: Bool
) -> String? {
    guard collapsesLongLists,
          totalCount > trafficCollapsedTableRowLimit else {
        return nil
    }
    if isExpanded {
        return "Show less"
    }
    return "Show \(totalCount - trafficCollapsedTableRowLimit) more"
}

@ViewBuilder
private func trafficExpansionButton(
    totalCount: Int,
    isExpanded: Binding<Bool>,
    collapsesLongLists: Bool,
    identifier: String
) -> some View {
    if let title = trafficExpansionButtonTitle(
        totalCount: totalCount,
        isExpanded: isExpanded.wrappedValue,
        collapsesLongLists: collapsesLongLists
    ) {
        Button(title) {
            isExpanded.wrappedValue.toggle()
        }
        .buttonStyle(.borderless)
        .accessibilityIdentifier(identifier)
    }
}

private func trafficTableHeader(_ title: String) -> some View {
    Text(title)
        .font(.caption.weight(.semibold))
        .foregroundStyle(.secondary)
        .accessibilityAddTraits(.isHeader)
}

private struct TrafficProviderLabel: View {
    let provider: String
    let label: String

    var body: some View {
        HStack(spacing: 7) {
            if ["codex", "codex-desktop", "pi", "claude-code", "claude-desktop"].contains(provider) {
                ClientBrandMark(clientName: provider).frame(width: 14, height: 14)
            } else if provider == "unknown" {
                Image(systemName: "questionmark.circle").frame(width: 14, height: 14)
            } else {
                TrafficBrandMark(brand: TrafficBrand.provider(provider), size: 14)
            }
            Text(label)
        }
    }
}

private func trafficBanner(
    _ text: String,
    symbol: String,
    color: Color
) -> some View {
    HStack(alignment: .top, spacing: 9) {
        Image(systemName: symbol)
            .foregroundStyle(color)
        Text(text)
            .font(.callout)
            .fixedSize(horizontal: false, vertical: true)
        Spacer()
    }
    .padding(11)
    .background(color.opacity(0.1), in: RoundedRectangle(cornerRadius: 9))
}

private func trafficMilliseconds(_ value: Double?) -> String {
    guard let value else { return "No data" }
    return value.formatted(
        .number.precision(.fractionLength(0...1))) + " ms"
}

private func trafficTPS(_ value: Double?) -> String {
    guard let value else { return "No data" }
    return value.formatted(
        .number.precision(.fractionLength(0...1))) + " tok/s"
}
