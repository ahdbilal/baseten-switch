package gateway

import (
	"fmt"
	"net/http"
	"time"

	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/proxy"
	"unity-gateway/gateway/internal/unity"
)

// An invalid saved connection fails closed; it must never fall back to Databricks.
func (g *Gateway) unityConnection() *unity.Connection {
	c, e := unity.Load(g.runtimeConfig().ConfigPath)
	if e != nil || (c == nil && g.runtimeConfig().WorkspaceOnly) {
		return &unity.Connection{}
	}
	return c
}
func (g *Gateway) unityAttempt(cl *clientListener, r *http.Request, body []byte, kind, model string) (upstreamAttempt, error) {
	c := g.unityConnection()
	if c == nil {
		return upstreamAttempt{}, fmt.Errorf("Unity Gateway is not configured")
	}
	if e := c.Validate(); e != nil {
		return upstreamAttempt{}, e
	}
	if cl.cfg.globalRoutingOff() {
		return upstreamAttempt{}, routingDisabledModelError{model: model}
	}
	requested := proxy.RewriteModelInBody(body, "").RequestedModel
	if model == "" {
		if target, ok := cl.cfg.ModelAliases[requested]; ok {
			model = target
		}
	}
	if requested == CodexCompatibilityModel && model == "" {
		cfg, e := compass.Load(compass.Path(g.runtimeConfig().ConfigPath))
		if e != nil {
			return upstreamAttempt{}, e
		}
		model = cfg.Default
	}
	if model == "" && !unityModel(requested) {
		if pin := resolveModelRoutePin(cl.cfg, requested); pin.forcedModel != "" {
			model = pin.forcedModel
		}
		if model == "" {
			model = cl.cfg.DefaultModel
		}
	}
	res := proxy.RewriteModelInBody(body, model)
	// Workspace models are fully qualified catalog identifiers. Never silently
	// send a legacy vendor slug to a different provider.
	if !unityModel(res.UpstreamModel) {
		return upstreamAttempt{}, fmt.Errorf("choose a Unity Gateway workspace model; %q is not a workspace model", res.UpstreamModel)
	}
	if g.runtimeConfig().WorkspaceOnly {
		models, e := c.ModelsFor(r.Context(), kind)
		if e != nil {
			return upstreamAttempt{}, e
		}
		compatible := false
		for _, m := range models {
			if m.ID == res.UpstreamModel {
				compatible = true
				break
			}
		}
		if !compatible {
			return upstreamAttempt{}, fmt.Errorf("workspace model %s does not support this client's %s API", res.UpstreamModel, kind)
		}
	}
	headers, e := proxy.BuildUpstreamHeaders(r.Header, proxy.UpstreamModeOAuth, "")
	if e != nil {
		return upstreamAttempt{}, e
	}
	headers.Del("ChatGPT-Account-Id")
	headers.Del("X-Unity-Gateway-Codex-Picker")
	snapshot := g.pricing.Capture()
	return upstreamAttempt{route: "databricks", kind: kind, url: c.Endpoint(kind), res: res, headers: headers, client: c.Client(), modelForCost: res.UpstreamModel, catalogSnapshot: snapshot, telemetryAttempt: captureTelemetryAttemptV1(snapshot, time.Now(), "databricks", res.UpstreamModel)}, nil
}
func unityModel(model string) bool { return unity.IsModel(model) }
func (g *Gateway) unityAuthStatus() map[string]any {
	c := g.unityConnection()
	health := c.Health()
	signed := true
	if e := c.Validate(); e != nil {
		health = "signed_out"
		signed = false
	}
	return map[string]any{"signed_in": signed, "auth_type": "oauth", "health": health, "profile": c.Profile, "workspace": c.Workspace, "source": "databricks", "saved_api_key": false, "fallback_enabled": false, "fallback_in_use": false, "revision": g.credentialRevision()}
}
func (g *Gateway) unityModelCatalog(w http.ResponseWriter, r *http.Request) {
	models, e := g.unityConnection().ModelsFor(r.Context(), "messages")
	if e != nil {
		writeModelCatalogJSON(w, r.Method, modelCatalogResponse{State: "error", Models: []modelCatalogModel{}, Error: e.Error()})
		return
	}
	out := make([]modelCatalogModel, 0, len(models))
	for _, m := range models {
		out = append(out, modelCatalogModel{Slug: m.Slug, DisplayName: m.DisplayName, ContextTokens: int64(m.ContextWindow)})
	}
	writeModelCatalogJSON(w, r.Method, modelCatalogResponse{State: "ready", Models: out, FetchedAt: time.Now().UTC().Format(time.RFC3339)})
}
