import SwiftUI

struct TrafficView: View {
    @ObservedObject var store: TrafficStore
    @State private var selectedTab: TrafficTab = .cost
    @State private var grouping: TrafficGrouping = .model

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                status
                if selectedTab == .cost, let snapshot = store.snapshot, !snapshot.isEmpty {
                    TrafficBreakdownsView(snapshot: snapshot, grouping: grouping)
                        .id(grouping)
                }
                content
            }
            .padding(28)
            .frame(maxWidth: 900, alignment: .leading)
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

            groupingPicker
            viewPicker
        }
    }

    private var viewPicker: some View {
        Picker("View", selection: $selectedTab) {
            ForEach(TrafficTab.allCases) { tab in Text(tab.rawValue).tag(tab) }
        }.pickerStyle(.segmented).labelsHidden().frame(width: 200)
    }

    private var groupingPicker: some View {
        Picker("Traffic breakdown", selection: $grouping) {
            ForEach(TrafficGrouping.allCases) { group in Text(group.viewTitle).tag(group) }
        }.pickerStyle(.segmented).labelsHidden().frame(maxWidth: 360)
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
            if let lastUpdated = store.lastUpdated {
                Text("Updated \(lastUpdated.formatted(.relative(presentation: .named)))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
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

private struct TrafficCostView: View {
    let snapshot: TrafficAnalyticsSnapshot
    let grouping: TrafficGrouping

    private var rows: [TrafficCostRow] {
        switch grouping {
        case .provider: return snapshot.cost.providers
        case .model: return snapshot.cost.models
        case .harness: return snapshot.harnesses.map(\.costRow)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 24) {
            TrafficMetricCard(
                title: "Estimated Unity Gateway cost",
                value: trafficCurrency(
                    snapshot.cost.summary.actualDatabricksCostUSD),
                detail: tokenDescription(for: "Unity Gateway"),
                brand: .databricks)

            TrafficSectionCard(
                title: "Estimated cost by " + grouping.rawValue.lowercased(),
                description:
                    "Calculated from recorded usage and API rates. Your plan's charges may differ."
            ) {
                ActualSpendChart(rows: rows)
                TrafficActualSpendTable(
                    rows: rows,
                    collapsesLongLists: grouping == .model)
            }
        }
    }

    private func tokenDescription(for provider: String) -> String {
        snapshot.cost.providers.first {
            $0.provider.caseInsensitiveCompare(provider == "Unity Gateway" ? "Databricks" : provider) == .orderedSame
        }?.tokenUsage.cardDescription ?? "Tokens unavailable"
    }
}

private struct TrafficPerformanceView: View {
    let snapshot: TrafficAnalyticsSnapshot
    let grouping: TrafficGrouping

    private var rows: [TrafficPerformanceRow] {
        switch grouping {
        case .provider: return snapshot.performance.providers
        case .model: return snapshot.performance.models
        case .harness: return snapshot.harnesses.map(\.performanceRow)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                Text("\(rows.reduce(0) { $0 + $1.requests }.formatted()) requests")
                    .font(.title2.weight(.semibold))
                Spacer()
                Text("\(rows.count) \((rows.count == 1 ? grouping.rawValue : grouping.viewTitle).lowercased())")
                    .foregroundStyle(.secondary)
            }
            TrafficSectionCard(
                title: grouping.viewTitle + " · Performance",
                description: "Time to first token: lower is better. Output speed: higher is better."
            ) {
                PerformanceCharts(rows: rows)
                TrafficPerformanceTable(
                    rows: rows,
                    collapsesLongLists: grouping == .model)
                    .id(grouping)
            }
        }
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

private struct TrafficMetricCard: View {
    let title: String
    let value: String
    let detail: String
    let brand: TrafficBrand
    let detailBrand: TrafficBrand?

    init(
        title: String,
        value: String,
        detail: String,
        brand: TrafficBrand,
        detailBrand: TrafficBrand? = nil
    ) {
        self.title = title
        self.value = value
        self.detail = detail
        self.brand = brand
        self.detailBrand = detailBrand
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 7) {
            HStack(spacing: 7) {
                TrafficBrandMark(brand: brand, size: 14)
                Text(title)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Text(value)
                .font(.title2.weight(.semibold))
                .monospacedDigit()
            HStack(spacing: 5) {
                if let detailBrand {
                    TrafficBrandMark(brand: detailBrand, size: 12)
                }
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                    .help(detail)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            Color.secondary.opacity(0.07),
            in: RoundedRectangle(cornerRadius: 10))
    }
}

private struct TrafficSectionCard<Content: View>: View {
    let title: String
    let description: String
    @ViewBuilder let content: Content

    init(
        title: String,
        description: String,
        @ViewBuilder content: () -> Content
    ) {
        self.title = title
        self.description = description
        self.content = content()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.headline)
                Text(description)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            content
        }
        .padding(18)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(Color.secondary.opacity(0.045)))
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(Color.secondary.opacity(0.12), lineWidth: 1))
    }
}

private struct TrafficActualSpendTable: View {
    let rows: [TrafficCostRow]
    let collapsesLongLists: Bool
    @State private var isExpanded = false

    private var visibleRows: [TrafficCostRow] {
        Array(rows.prefix(
            trafficVisibleRowCount(
                totalCount: rows.count,
                isExpanded: isExpanded,
                collapsesLongLists: collapsesLongLists)))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(spacing: 0) {
                HStack(spacing: 16) {
                    trafficTableHeader("Name")
                        .frame(maxWidth: .infinity, alignment: .leading)
                    trafficTableHeader("Requests")
                        .frame(width: 65, alignment: .trailing)
                    trafficTableHeader(
                        rows.allSatisfy { $0.tokenBreakdown == nil }
                            ? "Input + output" : "Total tokens")
                        .frame(maxWidth: .infinity, alignment: .trailing)
                    trafficTableHeader("Estimated cost")
                        .frame(maxWidth: .infinity, alignment: .trailing)
                }
                .padding(.vertical, 7)

                Divider()

                ForEach(visibleRows) { row in
                    HStack(alignment: .top, spacing: 16) {
                        TrafficProviderLabel(
                            provider: row.provider,
                            label: row.label)
                            .frame(
                                maxWidth: .infinity,
                                alignment: .leading)
                        Text(row.requests.formatted()).monospacedDigit()
                            .frame(width: 65, alignment: .trailing)
                        TrafficTokenCell(usage: row.tokenUsage, label: row.label)
                            .frame(
                                maxWidth: .infinity,
                                alignment: .trailing)
                        VStack(alignment: .trailing, spacing: 1) {
                            Text(trafficCurrency(row.actualCostUSD))
                                .monospacedDigit()
                            if row.hasPartialCost {
                                Text(
                                    "Partial, \(row.unpricedRows) unpriced")
                                    .font(.caption2)
                                    .foregroundStyle(.orange)
                            }
                        }
                        .frame(
                            maxWidth: .infinity,
                            alignment: .trailing)
                    }
                    .padding(.vertical, 8)
                    .accessibilityElement(children: .contain)
                    .accessibilityLabel(
                        actualSpendAccessibilityLabel(row))

                    if row.id != visibleRows.last?.id {
                        Divider()
                    }
                }
            }

            trafficExpansionButton(
                totalCount: rows.count,
                isExpanded: $isExpanded,
                collapsesLongLists: collapsesLongLists,
                identifier: "traffic-actual-spend-expand")
        }
    }

    private func actualSpendAccessibilityLabel(
        _ row: TrafficCostRow
    ) -> String {
        var label =
            "\(row.label), \(row.tokenUsage.accessibilitySummary), "
            + "estimated cost \(trafficCurrency(row.actualCostUSD))"
        if row.hasPartialCost {
            label += ", partial cost, \(row.unpricedRows) unpriced"
        }
        return label
    }
}

private struct TrafficPerformanceTable: View {
    let rows: [TrafficPerformanceRow]
    let collapsesLongLists: Bool
    @State private var isExpanded = false

    private var visibleRows: [TrafficPerformanceRow] {
        Array(rows.prefix(
            trafficVisibleRowCount(
                totalCount: rows.count,
                isExpanded: isExpanded,
                collapsesLongLists: collapsesLongLists)))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            VStack(spacing: 0) {
                HStack(spacing: 16) {
                    trafficTableHeader("Name")
                        .frame(maxWidth: .infinity, alignment: .leading)
                    trafficTableHeader(
                        rows.allSatisfy { $0.tokenBreakdown == nil }
                            ? "Input + output" : "Total tokens")
                        .frame(maxWidth: .infinity, alignment: .trailing)
                    trafficTableHeader("Median TTFT")
                        .frame(maxWidth: .infinity, alignment: .trailing)
                    trafficTableHeader("Median output TPS")
                        .frame(maxWidth: .infinity, alignment: .trailing)
                }
                .padding(.vertical, 7)

                Divider()

                ForEach(visibleRows) { row in
                    HStack(alignment: .top, spacing: 16) {
                        VStack(alignment: .leading, spacing: 4) {
                            TrafficProviderLabel(provider: row.provider, label: row.label)
                            Text("\(row.requests.formatted()) requests")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                            .frame(
                                maxWidth: .infinity,
                                alignment: .leading)
                        TrafficTokenCell(usage: row.tokenUsage, label: row.label)
                            .frame(
                                maxWidth: .infinity,
                                alignment: .trailing)
                        VStack(alignment: .trailing, spacing: 4) {
                            Text(trafficMilliseconds(row.measuredMedianTTFTMS)).monospacedDigit()
                            Text("\(row.ttftSamples) measured")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                            .frame(
                                maxWidth: .infinity,
                                alignment: .trailing)
                        VStack(alignment: .trailing, spacing: 4) {
                            Text(trafficTPS(row.measuredMedianOutputTokensPerSecond)).monospacedDigit()
                            Text("\(row.outputTPSSamples) measured")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                            .frame(
                                maxWidth: .infinity,
                                alignment: .trailing)
                    }
                    .padding(.vertical, 8)
                    .accessibilityElement(children: .contain)
                    .accessibilityLabel(
                        performanceAccessibilityLabel(row))

                    if row.id != visibleRows.last?.id {
                        Divider()
                    }
                }
            }

            trafficExpansionButton(
                totalCount: rows.count,
                isExpanded: $isExpanded,
                collapsesLongLists: collapsesLongLists,
                identifier: "traffic-performance-expand")
        }
    }

    private func performanceAccessibilityLabel(
        _ row: TrafficPerformanceRow
    ) -> String {
        return
            "\(row.label), \(row.tokenUsage.accessibilitySummary), "
            + "median TTFT "
            + "\(trafficMilliseconds(row.measuredMedianTTFTMS)), "
            + "median output speed "
            + "\(trafficTPS(row.measuredMedianOutputTokensPerSecond))"
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
                Text(usage.value)
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
            if usage.isPartial {
                Text("Partial coverage")
                    .font(.caption2)
                    .foregroundStyle(.orange)
            } else if usage.breakdown == nil {
                Text("Input + output")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
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
