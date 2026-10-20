package gateway

import (
	"context"
	"unity-gateway/gateway/internal/unity"
	"os"
	"path/filepath"
	"testing"
)

func TestUnityLiveClassifier(t *testing.T) {
	path := os.Getenv("UNITY_LIVE_CONFIG")
	if path == "" {
		t.Skip("set UNITY_LIVE_CONFIG for an authenticated workspace smoke test")
	}
	c, e := unity.Load(path)
	if e != nil || c == nil {
		t.Fatal(e)
	}
	cfg := testConfig(t, "https://inference.gateway.invalid", "https://api.anthropic.com")
	cfg.ConfigPath = filepath.Join(t.TempDir(), "gateway.yaml")
	if e = unity.Save(cfg.ConfigPath, *c); e != nil {
		t.Fatal(e)
	}
	g, _, _ := newGateway(t, cfg, resolvedOpenAIDatabricks(t, "codex", "databricks"))
	tier, reason, e := g.classifyCompass(context.Background(), "system.ai.glm-5-3-flash", []byte(`{"input":"What is 2+2? Reply with only the number."}`))
	if e != nil {
		t.Fatal(e)
	}
	t.Log(tier, reason)
}
