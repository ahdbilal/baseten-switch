// Package unity connects the local client proxy to Databricks Unity Gateway.
// OAuth credentials remain owned by ug and the Databricks CLI.
package unity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"unity-gateway/gateway/internal/ugruntime"
)

type Connection struct {
	Workspace string `json:"workspace"`
	Profile   string `json:"profile"`
}

func Path(config string) string { return filepath.Join(filepath.Dir(config), "unity-gateway.json") }
func (c Connection) Validate() error {
	u, e := url.Parse(c.Workspace)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("workspace must be an HTTPS origin, for example https://your-workspace.cloud.databricks.com")
	}
	if strings.TrimSpace(c.Profile) == "" {
		return errors.New("Databricks profile is required")
	}
	return nil
}
func Load(config string) (*Connection, error) {
	raw, e := os.ReadFile(Path(config))
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var c Connection
	if e = json.Unmarshal(raw, &c); e != nil {
		return nil, e
	}
	return &c, c.Validate()
}
func Save(config string, c Connection) error {
	if e := c.Validate(); e != nil {
		return e
	}
	c.Workspace = strings.TrimRight(c.Workspace, "/")
	path := Path(config)
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	raw, _ := json.MarshalIndent(c, "", "  ")
	f, e := os.CreateTemp(filepath.Dir(path), ".unity-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(append(raw, '\n'))
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(f.Name(), path)
}
func SavedWorkspace() (Connection, error) {
	home, _ := os.UserHomeDir()
	raw, e := os.ReadFile(filepath.Join(home, ".ucode", "state.json"))
	if e != nil {
		return Connection{}, e
	}
	var s struct {
		Current    string `json:"current_workspace"`
		Workspaces map[string]struct {
			Profile string `json:"profile"`
			Custom  struct {
				Profile string `json:"profile"`
			} `json:"custom_oauth"`
		} `json:"workspaces"`
	}
	if e = json.Unmarshal(raw, &s); e != nil {
		return Connection{}, e
	}
	w := s.Workspaces[s.Current]
	p := w.Profile
	if w.Custom.Profile != "" {
		p = w.Custom.Profile
	}
	c := Connection{s.Current, p}
	return c, c.Validate()
}
func Binary() (string, error) {
	return ugruntime.Binary()
}

// Token never logs credentials. Only explicit interactive login forwards the
// helper's login instructions to the user's terminal.
func (c Connection) Token(ctx context.Context, browser bool) (string, error) {
	if e := c.Validate(); e != nil {
		return "", e
	}
	bin, e := Binary()
	if e != nil {
		return "", e
	}
	args := []string{"auth-token", "--host", c.Workspace, "--profile", c.Profile}
	if browser {
		args = append(args, "--browser-login")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	if browser {
		cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	}
	out, e := cmd.Output()
	if e != nil {
		return "", errors.New("Databricks authentication failed; run unity-gateway auth login")
	}
	t := strings.TrimSpace(string(out))
	if t == "" || strings.ContainsAny(t, "\r\n ") {
		return "", errors.New("invalid Databricks credential response")
	}
	return t, nil
}
func (c Connection) Endpoint(kind string) string {
	base := strings.TrimRight(c.Workspace, "/")
	switch kind {
	case "responses":
		return base + "/ai-gateway/codex/v1/responses"
	case "messages":
		return base + "/ai-gateway/anthropic/v1/messages"
	case "models":
		return base + "/ai-gateway/codex/v1/models"
	default:
		return base + "/ai-gateway/mlflow/v1/chat/completions"
	}
}

type Transport struct {
	Connection Connection
	Base       http.RoundTripper
	Token      func(context.Context) (string, error)
}

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	if e := t.Connection.Validate(); e != nil {
		return nil, e
	}
	u, _ := url.Parse(t.Connection.Workspace)
	if r.URL.Scheme != u.Scheme || r.URL.Host != u.Host {
		return nil, errors.New("refusing to send workspace credential to another origin")
	}
	token, e := t.Token(r.Context())
	if e != nil {
		return nil, e
	}
	out := r.Clone(r.Context())
	out.Header = r.Header.Clone()
	for _, h := range []string{"Authorization", "X-Api-Key", "ChatGPT-Account-Id", "Cookie", "Proxy-Authorization"} {
		out.Header.Del(h)
	}
	out.Header.Set("Authorization", "Bearer "+token)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(out)
	if err == nil && resp.StatusCode == 401 {
		recordHealth(t.Connection, "refresh_failed")
		tokens.Lock()
		delete(tokens.values, t.Connection)
		tokens.Unlock()
	}
	return resp, err
}

var tokens struct {
	sync.Mutex
	values map[Connection]struct {
		token string
		until time.Time
	}
}

func (c Connection) cachedToken(ctx context.Context) (string, error) {
	tokens.Lock()
	defer tokens.Unlock()
	v := tokens.values[c]
	if time.Now().Before(v.until) {
		return v.token, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	token, e := c.Token(ctx, false)
	if e != nil {
		recordHealth(c, "refresh_failed")
		return "", e
	}
	recordHealth(c, "ok")
	if tokens.values == nil {
		tokens.values = make(map[Connection]struct {
			token string
			until time.Time
		})
	}
	tokens.values[c] = struct {
		token string
		until time.Time
	}{token, time.Now().Add(30 * time.Second)}
	return token, nil
}
func (c Connection) Client() *http.Client {
	return &http.Client{Transport: Transport{Connection: c, Token: c.cachedToken}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type Model struct {
	Slug          string   `json:"slug"`
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	ContextWindow int      `json:"context_window"`
	APIs          []string `json:"supported_api_types,omitempty"`
	MaxOutput     int      `json:"max_output_tokens,omitempty"`
}

func IsModel(model string) bool {
	return len(strings.Split(model, ".")) >= 3 && !strings.ContainsAny(model, " /\t\r\n\x00")
}

var healthState struct {
	sync.Mutex
	values map[Connection]string
}

func recordHealth(c Connection, status string) {
	healthState.Lock()
	defer healthState.Unlock()
	if healthState.values == nil {
		healthState.values = map[Connection]string{}
	}
	healthState.values[c] = status
}
func (c Connection) Health() string {
	if c.Validate() != nil {
		return "signed_out"
	}
	healthState.Lock()
	defer healthState.Unlock()
	if h := healthState.values[c]; h != "" {
		return h
	}
	return "unknown"
}
