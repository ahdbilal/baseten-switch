import Foundation

enum ModelFamily: String, CaseIterable, Identifiable {
    case automatic = "Automatic"
    case openai = "OpenAI"
    case claude = "Anthropic · Claude"
    case gemini = "Google · Gemini"
    case glm = "GLM"
    case kimi = "Kimi"
    case deepseek = "DeepSeek"
    case qwen = "Qwen"
    case llama = "Meta · Llama"
    case mistral = "Mistral"
    case other = "Other models"
    var id: String { rawValue }

    static func classify(identifier: String, name: String) -> ModelFamily {
        let text = (identifier + " " + name).lowercased()
        if text.contains("compass") { return .automatic }
        if text.contains("claude") || text.contains("anthropic") { return .claude }
        if text.contains("gemini") || text.contains("gemma") { return .gemini }
        if text.contains("gpt") || text.contains("openai") || text.contains("codex")
            || text.range(of: #"(?:^|[./ -])o[134](?:$|[. -])"#, options: .regularExpression) != nil { return .openai }
        if text.contains("glm") { return .glm }
        if text.contains("kimi") || text.contains("moonshot") { return .kimi }
        if text.contains("deepseek") { return .deepseek }
        if text.contains("qwen") || text.contains("qwq") { return .qwen }
        if text.contains("llama") { return .llama }
        if text.contains("mistral") || text.contains("mixtral") || text.contains("codestral") { return .mistral }
        return .other
    }
}

struct ModelGroup<Model>: Identifiable {
    let family: ModelFamily
    let models: [Model]
    var id: ModelFamily { family }
}

func groupedModels<Model>(
    _ models: [Model], identifier: (Model) -> String, name: (Model) -> String,
    search: String = ""
) -> [ModelGroup<Model>] {
    let query = search.trimmingCharacters(in: .whitespacesAndNewlines)
    var buckets: [ModelFamily: [Model]] = [:]
    for model in models {
        let family = ModelFamily.classify(identifier: identifier(model), name: name(model))
        guard query.isEmpty || family.rawValue.localizedCaseInsensitiveContains(query)
            || identifier(model).localizedCaseInsensitiveContains(query)
            || name(model).localizedCaseInsensitiveContains(query) else { continue }
        buckets[family, default: []].append(model)
    }
    return ModelFamily.allCases.compactMap { family in
        guard let rows = buckets[family], !rows.isEmpty else { return nil }
        return ModelGroup(family: family, models: rows.sorted {
            let comparison = name($0).localizedStandardCompare(name($1))
            return comparison == .orderedSame ? identifier($0) < identifier($1) : comparison == .orderedAscending
        })
    }
}
