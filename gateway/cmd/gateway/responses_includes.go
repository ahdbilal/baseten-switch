package gateway

import (
	"encoding/json"
	"strings"
)

// Baseten's DeepSeek V4.1 endpoint rejects include extensions. Remove only the
// requested response extras, never encrypted input history or native requests.
func stripBasetenResponseIncludes(body []byte, model string) []byte {
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
