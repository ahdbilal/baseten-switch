package gateway

import (
	"net/http"
	"strings"

	"unity-gateway/gateway/internal/version"
)

const switchVersionHeader = "X-Unity-Gateway-Version"

func applyDatabricksUsageHeaders(headers http.Header, route string) {
	if route != "databricks" {
		return
	}
	marker := "unity-gateway/" + version.Version
	if prior := strings.TrimSpace(headers.Get("User-Agent")); prior != "" {
		marker += " " + prior
	}
	headers.Set("User-Agent", marker)
	headers.Set(switchVersionHeader, version.Version)
}
