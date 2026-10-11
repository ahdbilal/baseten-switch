// Package compass owns the configuration and session pins for automatic Codex routing.
package compass

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/basetenlabs/baseten-switch/gateway/internal/safefile"
)

const Model = "Auto(Compass)"
const LegacyModel = "baseten-switch-compass"

func IsModel(id string) bool { return id == Model || id == LegacyModel }

type Rules struct {
	SessionStart       *bool `json:"session_start,omitempty"`
	Subagents          *bool `json:"subagents,omitempty"`
	SessionSwitching   bool  `json:"session_switching"`
	EncourageSubagents bool  `json:"encourage_subagents"`
	Verbose            bool  `json:"verbose"`
}

func (r Rules) Select(subagent bool) bool {
	flag := r.SessionStart
	if subagent {
		flag = r.Subagents
	}
	return flag == nil || *flag
}

type Config struct {
	Rules        Rules             `json:"rules"`
	ManualModels map[string]string `json:"manual_models,omitempty"`
	Comparison   string            `json:"comparison,omitempty"`
	Router       string            `json:"router"`
	Default      string            `json:"default"`
	High         string            `json:"high"`
	MidHigh      string            `json:"mid_high"`
	Mid          string            `json:"mid"`
	Low          string            `json:"low"`
}

func Defaults() Config {
	return Config{Router: "zai-org/GLM-5.3-Fast", Default: "zai-org/GLM-5.3", High: "gpt-6-astra", MidHigh: "gpt-6.1-sol", Mid: "zai-org/GLM-5.3", Low: "moonshotai/Kimi-K3"}
}
func Path(gatewayPath string) string { return filepath.Join(filepath.Dir(gatewayPath), "compass.json") }
func ValidModel(id string) bool {
	return id != "" && !IsModel(id) && id != "baseten-switch-compat-v1" && !strings.ContainsAny(id, " \t\r\n\x00")
}
func (c Config) Validate() error {
	for client, model := range c.ManualModels {
		if (client != "codex" && client != "pi") || !ValidModel(model) {
			return fmt.Errorf("invalid manual model")
		}
	}
	if c.Comparison != "" && !ValidModel(c.Comparison) {
		return fmt.Errorf("invalid comparison model")
	}
	for _, id := range []string{c.Router, c.Default, c.High, c.MidHigh, c.Mid, c.Low} {
		if !ValidModel(id) {
			return fmt.Errorf("invalid Compass model %q", id)
		}
	}
	if !strings.Contains(c.Router, "/") {
		return fmt.Errorf("the routing model must be a Baseten model")
	}
	return nil
}
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Defaults(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err = json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func Save(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	snap, err := safefile.Read(path)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	_, err = snap.Replace(append(raw, '\n'), 0600)
	return err
}
func (c Config) ComparisonModel() string {
	if c.Comparison != "" {
		return c.Comparison
	}
	return c.High
}

func (c Config) Target(tier string) string {
	switch tier {
	case "high":
		return c.High
	case "mid_high":
		return c.MidHigh
	case "mid":
		return c.Mid
	case "low":
		return c.Low
	default:
		return ""
	}
}

type Decision struct {
	EncourageSubagents bool      `json:"encourage_subagents,omitempty"`
	ReviewReason       string    `json:"review_reason,omitempty"`
	Client             string    `json:"client,omitempty"`
	ID                 string    `json:"id,omitempty"`
	Tier               string    `json:"tier"`
	Model              string    `json:"model"`
	Reason             string    `json:"reason"`
	Router             string    `json:"router"`
	Subagent           bool      `json:"subagent"`
	CreatedAt          time.Time `json:"created_at"`
}

func SessionKey(client, account, thread string) string {
	h := sha256.Sum256([]byte(client + "\x00" + account + "\x00" + thread))
	return hex.EncodeToString(h[:])
}
func sessionPath(configPath, key string) string {
	return filepath.Join(filepath.Dir(configPath), "compass-sessions", key+".json")
}
func LoadDecision(configPath, key string) (Decision, error) {
	var d Decision
	raw, err := os.ReadFile(sessionPath(configPath, key))
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal(raw, &d); err != nil {
		return d, err
	}
	if !ValidModel(d.Model) {
		return d, fmt.Errorf("invalid saved Compass decision")
	}
	return d, nil
}
func saveDecision(configPath, key string, d Decision) error {
	path := sessionPath(configPath, key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	snap, err := safefile.Read(path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = snap.Replace(raw, 0600)
	return err
}
func Recent(configPath string) []Decision {
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(configPath), "compass-sessions"))
	out := []Decision{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if d, err := LoadDecision(configPath, strings.TrimSuffix(e.Name(), ".json")); err == nil {
				d.ID = strings.TrimSuffix(e.Name(), ".json")
				out = append(out, d)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

type flight struct {
	done     chan struct{}
	decision Decision
	err      error
}
type Router struct {
	accountingMu     sync.Mutex
	active           map[string]int
	accountingFailed map[string]bool
	mu               sync.Mutex
	pending          map[string]*flight
}

// Pick deduplicates concurrent requests for a thread. Decisions are persisted
// without request text so gateway restarts do not trigger reclassification.
func (s *Router) Pick(ctx context.Context, path, key string, c Config, subagent bool, classify func(context.Context) (string, string, error), clients ...string) (Decision, error) {
	if key == "" {
		return Decision{Tier: "default", Model: c.Default, Router: c.Router, Reason: "No thread identity; using default", Subagent: subagent, CreatedAt: time.Now().UTC()}, nil
	}
	cacheKey := path + ":" + key
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]*flight{}
	}
	if f := s.pending[cacheKey]; f != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		case <-f.done:
			return f.decision, f.err
		}
	}
	f := &flight{done: make(chan struct{})}
	s.pending[cacheKey] = f
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, cacheKey); close(f.done); s.mu.Unlock() }()
	if d, err := LoadDecision(path, key); err == nil {
		f.decision = d
		return d, nil
	} else if !os.IsNotExist(err) {
		f.err = err
		return Decision{}, err
	}
	var tier, reason string
	var err error
	if c.Rules.Select(subagent) {
		tier, reason, err = classify(ctx)
	} else {
		reason = "Automatic selection disabled; using fallback"
	}
	if ctx.Err() != nil {
		f.err = ctx.Err()
		return Decision{}, f.err
	}
	model := c.Target(tier)
	if err != nil || model == "" {
		tier = "default"
		model = c.Default
		if c.Rules.Select(subagent) {
			reason = "Routing unavailable; using default model"
		}
	}
	if len([]rune(reason)) > 240 {
		reason = string([]rune(reason)[:240])
	}
	client := ""
	if len(clients) > 0 {
		client = clients[0]
	}
	d := Decision{EncourageSubagents: c.Rules.EncourageSubagents, Client: client, Tier: tier, Model: model, Reason: reason, Router: c.Router, Subagent: subagent, CreatedAt: time.Now().UTC()}
	if err = saveDecision(path, key, d); err != nil {
		f.err = err
		return Decision{}, err
	}
	f.decision = d
	return d, nil
}
