package gateway

import (
	"net/http"
	"testing"

	"unity-gateway/gateway/internal/version"
)

func TestApplyDatabricksUsageHeaders(t *testing.T) {
	headers := http.Header{
		"User-Agent":               {"claude-code/2.1"},
		"X-Unity-Gateway-Version": {"forged"},
	}

	applyDatabricksUsageHeaders(headers, "databricks")

	if got, want := headers.Get("User-Agent"), "unity-gateway/"+version.Version+" claude-code/2.1"; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
	if got := headers.Get(switchVersionHeader); got != version.Version {
		t.Fatalf("%s = %q, want %q", switchVersionHeader, got, version.Version)
	}
}

func TestApplyDatabricksUsageHeadersWithoutInboundUserAgent(t *testing.T) {
	headers := http.Header{}

	applyDatabricksUsageHeaders(headers, "databricks")

	if got, want := headers.Get("User-Agent"), "unity-gateway/"+version.Version; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
}

func TestApplyDatabricksUsageHeadersLeavesNativeRoutesUnchanged(t *testing.T) {
	for _, route := range []string{"anthropic", "openai"} {
		t.Run(route, func(t *testing.T) {
			headers := http.Header{
				"User-Agent":               {"harness/1.0"},
				"X-Unity-Gateway-Version": {"inbound"},
			}

			applyDatabricksUsageHeaders(headers, route)

			if got := headers.Get("User-Agent"); got != "harness/1.0" {
				t.Fatalf("User-Agent = %q, want unchanged", got)
			}
			if got := headers.Get(switchVersionHeader); got != "inbound" {
				t.Fatalf("%s = %q, want unchanged", switchVersionHeader, got)
			}
		})
	}
}
