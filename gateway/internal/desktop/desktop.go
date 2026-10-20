// Package desktop manages a dedicated Claude Desktop gateway configuration.
// The user's other profiles, MCP settings and signed-in account are preserved.
package desktop

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/safefile"
	"unity-gateway/gateway/internal/unity"
)

const Client = "claude-desktop"
const ProfileID = "a73adf05-20e8-4a49-bc38-d781649ba923"
const BaseURL = "http://127.0.0.1:45275"

type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type State struct {
	Models         []Model         `json:"models"`
	Hidden         map[string]bool `json:"hidden"`
	Default        string          `json:"default"`
	Connected      bool            `json:"connected"`
	PreviousID     json.RawMessage `json:"previous_id,omitempty"`
	PreviousMode   json.RawMessage `json:"previous_mode,omitempty"`
	PriorProfile   json.RawMessage `json:"prior_profile,omitempty"`
	WrittenProfile json.RawMessage `json:"written_profile,omitempty"`
}
type Store struct{ Dir, StatePath string }

func (s Store) Load() (State, error) {
	var st State
	raw, err := os.ReadFile(s.StatePath)
	if os.IsNotExist(err) {
		return State{Models: []Model{}, Hidden: map[string]bool{}}, nil
	}
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(raw, &st)
	return st, err
}
func (s Store) Save(st State) error { return writeJSON(s.StatePath, st) }
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	snap, err := safefile.Read(path)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = snap.Replace(append(raw, '\n'), 0600)
	return err
}
func readObject(path string) (*safefile.Snapshot, map[string]json.RawMessage, error) {
	snap, err := safefile.Read(path)
	if err != nil {
		return nil, nil, err
	}
	obj := map[string]json.RawMessage{}
	if snap.Exists {
		if err = json.Unmarshal(snap.Data, &obj); err != nil || obj == nil {
			return nil, nil, fmt.Errorf("invalid JSON object: %s", path)
		}
	}
	return snap, obj, nil
}
func set(obj map[string]json.RawMessage, key string, v any) { obj[key], _ = json.Marshal(v) }
func restore(obj map[string]json.RawMessage, key string, v json.RawMessage) {
	if len(v) == 0 {
		delete(obj, key)
	} else {
		obj[key] = v
	}
}
func (s Store) profilePath() string { return filepath.Join(s.Dir, "configLibrary", ProfileID+".json") }
func (s Store) metaPath() string    { return filepath.Join(s.Dir, "configLibrary", "_meta.json") }
func (s Store) configPath() string  { return filepath.Join(s.Dir, "claude_desktop_config.json") }

const ModelAliasPrefix = "claude-unity-model-"

// WireModel adapts Desktop's model-name filter. Catalogs, labels, settings and
// telemetry keep the actual model ID; only the Desktop profile uses an alias.
func WireModel(id string) string {
	if id == compass.DesktopModel || strings.Contains(strings.ToLower(id), "claude") {
		return id
	}
	return ModelAliasPrefix + hex.EncodeToString([]byte(id))
}

func ResolveModel(id string) (string, error) {
	if !strings.HasPrefix(id, ModelAliasPrefix) {
		return id, nil
	}
	raw, err := hex.DecodeString(strings.TrimPrefix(id, ModelAliasPrefix))
	model := string(raw)
	if err != nil || !unity.IsModel(model) || WireModel(model) != id {
		return "", fmt.Errorf("invalid Desktop model alias")
	}
	return model, nil
}
func (st State) Has(id string) bool {
	for _, m := range st.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}
func (st State) Profile() (map[string]any, error) {
	if !st.Has(st.Default) || st.Hidden[st.Default] {
		return nil, fmt.Errorf("select a visible Desktop default model")
	}
	models := []map[string]any{}
	for _, m := range st.Models {
		if m.ID != compass.DesktopModel && !unity.IsModel(m.ID) {
			return nil, fmt.Errorf("Desktop requires a workspace model: %s", m.ID)
		}
		if !st.Hidden[m.ID] {
			models = append(models, map[string]any{"name": WireModel(m.ID), "labelOverride": m.Name})
		}
	}
	for i, m := range models {
		if m["name"] == WireModel(st.Default) {
			models[0], models[i] = models[i], models[0]
			break
		}
	}
	return map[string]any{"inferenceProvider": "gateway", "inferenceGatewayBaseUrl": BaseURL, "inferenceGatewayApiKey": "unity-gateway-desktop", "inferenceGatewayAuthScheme": "bearer", "inferenceModels": models, "modelDiscoveryEnabled": false}, nil
}

// Apply changes three Desktop files with compare-and-swap and rollback. A durable
// receipt is saved first so interrupted setup remains recoverable by Disconnect.
func (s Store) Apply(st State) error {
	profile, err := st.Profile()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(s.Dir, "configLibrary"), 0700); err != nil {
		return err
	}
	ps, err := safefile.Read(s.profilePath())
	if err != nil {
		return err
	}
	old, err := s.Load()
	if err != nil {
		return err
	}
	if ps.Exists && (len(old.WrittenProfile) == 0 || !jsonEqual(ps.Data, old.WrittenProfile) && !jsonEqual(ps.Data, old.PriorProfile)) {
		return fmt.Errorf("Desktop gateway profile was edited outside Unity Gateway; preserving it")
	}
	ms, meta, err := readObject(s.metaPath())
	if err != nil {
		return err
	}
	cs, cfg, err := readObject(s.configPath())
	if err != nil {
		return err
	}
	st.PreviousID, st.PreviousMode = old.PreviousID, old.PreviousMode
	if !old.Connected {
		st.PreviousID = meta["appliedId"]
		st.PreviousMode = cfg["deploymentMode"]
	}
	var entries []map[string]json.RawMessage
	if raw := meta["entries"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &entries); err != nil {
			return err
		}
	}
	found := false
	for _, e := range entries {
		var id string
		_ = json.Unmarshal(e["id"], &id)
		if id == ProfileID {
			found = true
		}
	}
	if !found {
		e := map[string]json.RawMessage{}
		set(e, "id", ProfileID)
		set(e, "name", "Databricks Unity Gateway")
		entries = append(entries, e)
	}
	set(meta, "entries", entries)
	set(meta, "appliedId", ProfileID)
	// A hybrid pointer would override this local profile. Leave it intact and ask
	// the user to switch deployment modes explicitly instead of deleting it.
	if raw := meta["hybridPointer"]; len(raw) > 0 && string(raw) != "null" {
		return fmt.Errorf("Claude Desktop has an active organization profile; switch to a local configuration first")
	}
	set(cfg, "deploymentMode", "3p")
	raw, _ := json.MarshalIndent(profile, "", "  ")
	raw = append(raw, '\n')
	st.Connected = true
	st.PriorProfile = old.WrittenProfile
	st.WrittenProfile = raw
	if err = s.Save(st); err != nil {
		return err
	}
	if err = replaceAll([]*safefile.Snapshot{ps, ms, cs}, []any{profile, meta, cfg}); err != nil {
		return err
	}
	return nil
}
func jsonEqual(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	aa, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return bytes.Equal(aa, bb)
}
func replaceAll(snaps []*safefile.Snapshot, values []any) error {
	written := []*safefile.Snapshot{}
	for i, snap := range snaps {
		raw, err := json.MarshalIndent(values[i], "", "  ")
		if err != nil {
			return err
		}
		next, err := snap.Replace(append(raw, '\n'), 0600)
		if err != nil {
			for j := len(written) - 1; j >= 0; j-- {
				var e error
				if snaps[j].Exists {
					_, e = written[j].Replace(snaps[j].Data, 0600)
				} else {
					e = written[j].Remove()
				}
				if e != nil {
					return fmt.Errorf("%v; rollback needs attention: %w", err, e)
				}
			}
			return err
		}
		written = append(written, next)
	}
	return nil
}
func (s Store) Disconnect() error {
	st, err := s.Load()
	if err != nil || !st.Connected {
		return err
	}
	ms, meta, err := readObject(s.metaPath())
	if err != nil {
		return err
	}
	cs, cfg, err := readObject(s.configPath())
	if err != nil {
		return err
	}
	var applied string
	_ = json.Unmarshal(meta["appliedId"], &applied)
	if applied == ProfileID {
		restore(meta, "appliedId", st.PreviousID)
		var mode string
		_ = json.Unmarshal(cfg["deploymentMode"], &mode)
		if mode == "3p" {
			restore(cfg, "deploymentMode", st.PreviousMode)
		}
	}
	// Retain the inactive profile in Desktop's configuration library so existing
	// sessions and user edits remain recoverable. Reconnecting reuses our entry.
	if err = replaceAll([]*safefile.Snapshot{ms, cs}, []any{meta, cfg}); err != nil {
		return err
	}
	st.Connected = false
	return s.Save(st)
}
func (s Store) Active() bool {
	st, err := s.Load()
	if err != nil || !st.Connected {
		return false
	}
	_, meta, err := readObject(s.metaPath())
	if err != nil {
		return false
	}
	var applied string
	_ = json.Unmarshal(meta["appliedId"], &applied)
	_, cfg, err := readObject(s.configPath())
	if err != nil {
		return false
	}
	var mode string
	_ = json.Unmarshal(cfg["deploymentMode"], &mode)
	if applied != ProfileID || mode != "3p" {
		return false
	}
	profile, err := safefile.Read(s.profilePath())
	return err == nil && profile.Exists && jsonEqual(profile.Data, st.WrittenProfile) && (len(meta["hybridPointer"]) == 0 || string(meta["hybridPointer"]) == "null")
}
