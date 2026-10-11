package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
)

// Claude session metadata varies by release. Accept explicit session identity
// only; never use the user/account alone as a session pin.
func claudeCompassRequest(r *http.Request, body []byte) *http.Request {
	var e struct {
		Metadata struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(body, &e)
	session := r.Header.Get("Thread-Id")
	if session == "" {
		session = r.Header.Get("Session-Id")
	}
	if session == "" {
		var identity struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(e.Metadata.UserID), &identity) == nil {
			session = identity.SessionID
		}
		if session == "" {
			if i := strings.LastIndex(e.Metadata.UserID, "_session_"); i >= 0 {
				session = e.Metadata.UserID[i+len("_session_"):]
			}
		}
	}
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	if session != "" {
		// Distinct subagents within a parent session must never share its pin.
		if agent := r.Header.Get(subagentAgentIDHeader); agent != "" {
			session += "/agent/" + agent
		}
		clone.Header.Set("Thread-Id", session)
	}
	return clone
}

func claudeCompassContextBody(body []byte) []byte {
	var e map[string]json.RawMessage
	if json.Unmarshal(body, &e) != nil {
		return nil
	}
	raw, _ := json.Marshal(map[string]json.RawMessage{"input": e["messages"]})
	return raw
}

// Cross-model switching is conservative: only portable, text-only history.
// Tool results, images and signed thinking remain on the selected model.
func claudeCompassReviewBoundary(body []byte) (bool, string) {
	var e struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Speed       json.RawMessage `json:"speed"`
		ServiceTier json.RawMessage `json:"service_tier"`
	}
	if json.Unmarshal(body, &e) != nil || len(e.Messages) == 0 || len(e.Speed) > 0 || len(e.ServiceTier) > 0 {
		return false, ""
	}
	if e.Messages[len(e.Messages)-1].Role != "user" {
		return false, ""
	}
	for _, m := range e.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return false, ""
		}
		var text string
		if json.Unmarshal(m.Content, &text) == nil {
			continue
		}
		var parts []struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(m.Content, &parts) != nil || len(parts) == 0 {
			return false, ""
		}
		for _, p := range parts {
			if p.Type != "text" {
				return false, ""
			}
		}
	}
	h := sha256.Sum256(claudeCompassContextBody(body))
	return true, hex.EncodeToString(h[:])
}

func claudeCompassDelegation(body []byte) []byte {
	var e map[string]json.RawMessage
	if json.Unmarshal(body, &e) != nil {
		return body
	}
	var text string
	if len(e["system"]) == 0 || json.Unmarshal(e["system"], &text) == nil {
		if !strings.Contains(text, compassDelegationPrompt) {
			text += "\n\n" + compassDelegationPrompt
		}
		e["system"], _ = json.Marshal(text)
	} else {
		var blocks []json.RawMessage
		if json.Unmarshal(e["system"], &blocks) != nil {
			return body
		}
		for _, raw := range blocks {
			var block struct {
				Text string `json:"text"`
			}
			if json.Unmarshal(raw, &block) == nil && strings.Contains(block.Text, compassDelegationPrompt) {
				return body
			}
		}
		raw, _ := json.Marshal(map[string]string{"type": "text", "text": compassDelegationPrompt})
		e["system"], _ = json.Marshal(append(blocks, raw))
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return body
	}
	return raw
}
