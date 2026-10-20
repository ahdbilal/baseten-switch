package unity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var modelCache struct {
	sync.Mutex
	values map[Connection]struct {
		models []Model
		at     time.Time
	}
}

func (c Connection) getJSON(ctx context.Context, path string, out any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(c.Workspace, "/")+path, nil)
	if e != nil {
		return e
	}
	resp, e := c.Client().Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("workspace catalog returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

// AllModels uses Unity Catalog capability metadata rather than guessing an API
// from a model's name. Pagination failures never publish a partial inventory.
func (c Connection) AllModels(ctx context.Context) ([]Model, error) {
	modelCache.Lock()
	cached := modelCache.values[c]
	modelCache.Unlock()
	if time.Since(cached.at) < time.Minute {
		return append([]Model(nil), cached.models...), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	models := map[string]Model{}
	token := ""
	seen := map[string]bool{}
	for page := 0; page < 100; page++ {
		q := url.Values{"parent": {"schemas/system.ai"}, "page_size": {"100"}}
		if token != "" {
			q.Set("page_token", token)
		}
		var d struct {
			Services []struct {
				Name string   `json:"name"`
				APIs []string `json:"supported_api_types"`
			} `json:"model_services"`
			Next string `json:"next_page_token"`
		}
		if e := c.getJSON(ctx, "/api/2.1/unity-catalog/model-services?"+q.Encode(), &d); e != nil {
			return nil, e
		}
		for _, m := range d.Services {
			id := strings.TrimPrefix(m.Name, "model-services/")
			if !IsModel(id) {
				continue
			}
			models[id] = Model{Slug: id, ID: id, DisplayName: DisplayName(id), ContextWindow: 128000, MaxOutput: 16384, APIs: m.APIs}
		}
		if d.Next == "" {
			break
		}
		if seen[d.Next] || page == 99 {
			return nil, fmt.Errorf("invalid or excessive workspace model pagination")
		}
		seen[d.Next] = true
		token = d.Next
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("workspace has no available model services")
	}
	out := make([]Model, 0, len(models))
	for _, m := range models {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	modelCache.Lock()
	if modelCache.values == nil {
		modelCache.values = map[Connection]struct {
			models []Model
			at     time.Time
		}{}
	}
	modelCache.values[c] = struct {
		models []Model
		at     time.Time
	}{out, time.Now()}
	modelCache.Unlock()
	return append([]Model(nil), out...), nil
}
func (m Model) Supports(kind string) bool {
	want := map[string]string{"responses": "codex/v1/responses", "messages": "anthropic/v1/messages", "chat": "mlflow/v1/chat/completions"}[kind]
	for _, a := range m.APIs {
		if a == want {
			return true
		}
	}
	return false
}
func (c Connection) ModelsFor(ctx context.Context, kind string) ([]Model, error) {
	models, e := c.AllModels(ctx)
	if e != nil {
		return nil, e
	}
	out := []Model{}
	for _, m := range models {
		if m.Supports(kind) {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("workspace has no models supporting %s", kind)
	}
	return out, nil
}
func (c Connection) Models(ctx context.Context) ([]Model, error) {
	return c.ModelsFor(ctx, "responses")
}

func DisplayName(id string) string {
	switch id {
	case "system.ai.glm-5-3":
		return "GLM 5.3"
	case "system.ai.glm-5-3-flash":
		return "GLM 5.3 Flash"
	case "system.ai.gpt-6-astra":
		return "GPT-6 Astra"
	case "system.ai.gpt-6-1-sol":
		return "GPT-6.1 Sol"
	case "system.ai.kimi-k3":
		return "Kimi K3"
	}
	return strings.TrimPrefix(id, "system.ai.")
}
