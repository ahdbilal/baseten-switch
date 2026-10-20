import Foundation
import XCTest
@testable import UnityGateway

final class TrafficCompactTests: XCTestCase {
    private func snapshot(_ raw: String) throws -> TrafficAnalyticsSnapshot {
        var root = try JSONSerialization.jsonObject(with: Data(raw.utf8)) as! [String: Any]
        if var cost = root["cost"] as? [String: Any] {
            cost["summary"] = ["actual_claude_cost_usd": 0, "actual_databricks_cost_usd": 0,
                "estimated_native_cost_for_databricks_usd": 0, "saved_usd": 0, "saved_percent": 0]
            cost["savings"] = ["by_databricks_model": [], "mappings": []]
            if cost["models"] == nil { cost["models"] = [] }
            root["cost"] = cost
        }
        func performance(_ row: [String: Any]) -> [String: Any] {
            var result = row
            for key in ["requests", "tokens", "ttft_samples", "output_tps_samples"] where result[key] == nil {
                result[key] = 0
            }
            return result
        }
        if var metrics = root["performance"] as? [String: Any] {
            for key in ["models", "providers"] {
                metrics[key] = (metrics[key] as? [[String: Any]] ?? []).map(performance)
            }
            root["performance"] = metrics
        }
        if let harnesses = root["harnesses"] as? [[String: Any]] {
            root["harnesses"] = harnesses.map { harness in
                var h = harness
                h["performance"] = performance(h["performance"] as! [String: Any])
                return h
            }
        }
        return try JSONDecoder().decode(TrafficAnalyticsSnapshot.self,
            from: JSONSerialization.data(withJSONObject: root))
    }

    func testTotalIncludesEveryProviderAndPreservesUnknownCosts() throws {
        let data = try snapshot(#"{"cost":{"providers":[{"provider":"Databricks","requests":3,"actual_cost_usd":1.25},{"provider":"OpenAI","requests":2,"actual_cost_usd":2.5},{"provider":"Anthropic","requests":1,"actual_cost_usd":null}]}}"#)
        let total = TrafficCompactCostSummary(rows: data.cost.providers)
        XCTAssertEqual(total.value, 3.75)
        XCTAssertTrue(total.partial)
        XCTAssertNil(TrafficCompactCostSummary(rows: [data.cost.providers[2]]).value)
        var free = data.cost.providers[0]
        free.actualCostUSD = 0
        XCTAssertEqual(TrafficCompactCostSummary(rows: [free]).value, 0)
        XCTAssertFalse(TrafficCompactCostSummary(rows: [free]).partial)
    }

    func testEachGroupingUsesItsOwnCostAndMeasuredPerformance() throws {
        let data = try snapshot(#"{"cost":{"providers":[{"provider":"Databricks","requests":9}],"models":[{"provider":"Databricks","model_id":"glm","requests":3},{"provider":"Databricks","model_id":"kimi","requests":6}]},"performance":{"providers":[{"provider":"Databricks","requests":9,"ttft_samples":9,"median_ttft_ms":500}],"models":[{"provider":"Databricks","model_id":"glm","requests":3,"ttft_samples":3,"median_ttft_ms":800}]},"harnesses":[{"client":"pi","models":[],"cost":{"provider":"pi","requests":4},"performance":{"provider":"pi","requests":4,"ttft_samples":0,"median_ttft_ms":0}}]}"#)
        XCTAssertEqual(data.costRows(groupedBy: .model).map(\.modelID), ["kimi", "glm"])
        XCTAssertEqual(data.costRows(groupedBy: .provider).first?.requests, 9)
        XCTAssertEqual(data.costRows(groupedBy: .harness).first?.label, "Pi")
        XCTAssertEqual(data.performanceRows(groupedBy: .model).first?.measuredMedianTTFTMS, 800)
        XCTAssertEqual(data.performanceRows(groupedBy: .provider).first?.measuredMedianTTFTMS, 500)
        XCTAssertNil(data.performanceRows(groupedBy: .harness).first?.measuredMedianTTFTMS)
    }

    func testCompactLabelsPreserveVersionsAndSuppliedNames() {
        XCTAssertEqual(trafficCompactModelLabel("system.ai.glm 5 3", modelID: "system.ai.glm-5-3"), "GLM 5.3")
        XCTAssertEqual(trafficCompactModelLabel("system.ai.gpt-6-1-sol", modelID: "system.ai.gpt-6-1-sol"), "GPT 6.1 Sol")
        XCTAssertEqual(trafficCompactModelLabel("My production model", modelID: "system.ai.glm-5-3"), "My production model")
        XCTAssertEqual(trafficCompactModelLabel("Codex Desktop", modelID: nil), "Codex Desktop")
        XCTAssertEqual(trafficBarFraction(value: 2, maximum: 4), 0.5)
        XCTAssertEqual(trafficBarFraction(value: 0, maximum: 0), 0)
        XCTAssertEqual(trafficBarFraction(value: .nan, maximum: 4), 0)
        XCTAssertEqual(trafficBarFraction(value: 5, maximum: 4), 1)
        XCTAssertEqual(trafficCompactSeconds(nil), "—")
        XCTAssertEqual(trafficCompactSeconds(0), "0s")
        XCTAssertEqual(trafficCompactSeconds(2), "<0.01s")
    }
}
