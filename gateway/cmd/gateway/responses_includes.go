package gateway

import (
	"encoding/json"
	"fmt"
	"unity-gateway/gateway/internal/responsescompat"
	"strings"
)

// Databricks's DeepSeek V4.1 endpoint rejects include extensions. Remove only the
// requested response extras, never encrypted input history or native requests.
func stripDatabricksResponseIncludes(body []byte, model string) []byte {
	if !strings.HasPrefix(strings.ToLower(model), "deepseek-ai/deepseek-v4.1") {
		return body
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return body
	}
	if _, ok := envelope["include"]; !ok {
		return body
	}
	delete(envelope, "include")
	updated, err := json.Marshal(envelope)
	if err != nil {
		return body
	}
	return updated
}

// DeepSeek V4.1 rejects hosted web search while accepting client function,
// namespace, and apply_patch tools. Keep those client tools unchanged.
func stripDeepSeekHostedSearch(body []byte, model string) ([]byte, []string, error) {
	if !strings.HasPrefix(strings.ToLower(model), "deepseek-ai/deepseek-v4.1") {
		return body, nil, nil
	}
	var e struct {
		ToolChoice struct {
			Type string `json:"type"`
		} `json:"tool_choice"`
	}
	_ = json.Unmarshal(body, &e)
	if e.ToolChoice.Type == "web_search" || e.ToolChoice.Type == "web_search_preview" {
		return nil, nil, fmt.Errorf("DeepSeek V4.1 does not support required web_search; select a model that supports hosted web search")
	}
	next, removed := responsescompat.StripToolTypes(body, []string{"web_search", "web_search_preview"})
	return next, removed, nil
}
