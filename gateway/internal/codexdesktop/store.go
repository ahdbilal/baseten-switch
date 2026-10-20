// Package codexdesktop owns only the desktop gateway provider and selected
// model keys. OpenAI credentials, projects, tools and CLI profiles are untouched.
package codexdesktop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"syscall"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
	"unity-gateway/gateway/internal/compass"
	"unity-gateway/gateway/internal/safefile"
)

const Client = "codex-desktop"
const Provider = "unity_gateway_desktop"
const BaseURL = "http://127.0.0.1:45276/v1"
const catalogName = "unity-gateway-desktop.models.json"

var keys = []string{"model", "model_provider", "model_catalog_json", "model_reasoning_effort"}

type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type State struct {
	ListenerOwned   bool              `json:"listener_owned"`
	Connected       bool              `json:"connected"`
	Models          []Model           `json:"models"`
	Hidden          map[string]bool   `json:"hidden"`
	Default         string            `json:"default"`
	Original        []byte            `json:"original,omitempty"`
	Existed         bool              `json:"existed"`
	Previous        map[string]string `json:"previous,omitempty"`
	Written         map[string]string `json:"written,omitempty"`
	ProviderBlock   string            `json:"provider_block,omitempty"`
	Catalog         []byte            `json:"catalog,omitempty"`
	PreviousCatalog []byte            `json:"previous_catalog,omitempty"`
}
type Store struct{ Dir, StatePath string }

func (s Store) ConfigPath() string  { return filepath.Join(s.Dir, "config.toml") }
func (s Store) CatalogPath() string { return filepath.Join(s.Dir, catalogName) }
func (s Store) Load() (State, error) {
	st := State{Models: []Model{}, Hidden: map[string]bool{}}
	b, e := os.ReadFile(s.StatePath)
	if os.IsNotExist(e) {
		return st, nil
	}
	if e != nil {
		return st, e
	}
	e = json.Unmarshal(b, &st)
	return st, e
}
func save(path string, v any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	snap, e := safefile.Read(path)
	if e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	_, e = snap.Replace(append(b, '\n'), 0600)
	return e
}
func (s Store) Save(st State) error { return save(s.StatePath, st) }
func (st State) Has(id string) bool {
	for _, m := range st.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}
func parse(raw []byte) (map[string]any, error) {
	d := map[string]any{}
	e := toml.Unmarshal(raw, &d)
	return d, e
}
func stringValues(d map[string]any) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range keys {
		if v, ok := d[k]; ok {
			str, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("Codex %s must be a string", k)
			}
			out[k] = str
		}
	}
	return out, nil
}

// Patch only root scalar values using the parser's byte ranges. Quoted keys,
// multiline strings, comments, and nested similarly named keys are preserved.
func patchRoots(raw []byte, values map[string]*string) ([]byte, error) {
	if _, e := parse(raw); e != nil {
		return nil, e
	}
	type edit struct {
		start, end int
		value      string
	}
	var edits []edit
	seen := map[string]bool{}
	root := true
	var p unstable.Parser
	p.Reset(raw)
	for p.NextExpression() {
		n := p.Expression()
		if n.Kind == unstable.Table || n.Kind == unstable.ArrayTable {
			root = false
		}
		if !root || n.Kind != unstable.KeyValue {
			continue
		}
		it := n.Key()
		if !it.Next() {
			continue
		}
		key := it.Node()
		name := string(key.Data)
		if it.Next() {
			continue
		}
		value, wanted := values[name]
		if !wanted {
			continue
		}
		seen[name] = true
		v := n.Value()
		if v.Kind != unstable.String {
			return nil, fmt.Errorf("Codex %s must be a string", name)
		}
		start, end := int(v.Raw.Offset), int(v.Raw.Offset+v.Raw.Length)
		if value != nil {
			edits = append(edits, edit{start, end, strconv.Quote(*value)})
		} else {
			start = int(key.Raw.Offset)
			for end < len(raw) && raw[end] != '\n' {
				end++
			}
			if end < len(raw) {
				end++
			}
			edits = append(edits, edit{start, end, ""})
		}
	}
	if e := p.Error(); e != nil {
		return nil, e
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := append([]byte(nil), raw...)
	for _, e := range edits {
		out = append(append(append([]byte(nil), out[:e.start]...), []byte(e.value)...), out[e.end:]...)
	}
	for _, k := range keys {
		if value, ok := values[k]; ok && !seen[k] && value != nil {
			out = append([]byte(k+" = "+strconv.Quote(*value)+"\n"), out...)
		}
	}
	_, e := parse(out)
	return out, e
}
func providerBlock() string {
	return "\n# Unity Gateway: Codex desktop provider\n[model_providers." + Provider + "]\nname = \"Databricks Unity Gateway\"\nbase_url = \"" + BaseURL + "\"\nwire_api = \"responses\"\nrequires_openai_auth = false\nsupports_websockets = false\n"
}
func (s Store) Active(st State) bool {
	snap, e := safefile.Read(s.ConfigPath())
	if e != nil || !snap.Exists || !st.Connected {
		return false
	}
	d, e := parse(snap.Data)
	if e != nil {
		return false
	}
	providers, _ := d["model_providers"].(map[string]any)
	p, _ := providers[Provider].(map[string]any)
	return d["model_provider"] == Provider && p["base_url"] == BaseURL && d["model_catalog_json"] == s.CatalogPath()
}

// SelectedModel follows model changes made in Codex's own picker.
func (s Store) SelectedModel(st State) string {
	if s.Active(st) {
		if snap, e := safefile.Read(s.ConfigPath()); e == nil {
			if d, e := parse(snap.Data); e == nil {
				if model, ok := d["model"].(string); ok && st.Has(model) {
					return model
				}
			}
		}
	}
	return st.Default
}

// Serialize recovery records with the config/catalog transaction across processes.
func (s Store) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.StatePath), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.StatePath+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another Codex desktop configuration change is in progress; retry: %w", err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func (s Store) Apply(st State, catalog []byte) error {
	unlock, lockErr := s.lock()
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	if !st.Has(st.Default) || st.Hidden[st.Default] {
		return fmt.Errorf("select a visible default model")
	}
	if e := os.MkdirAll(s.Dir, 0700); e != nil {
		return e
	}
	old, e := s.Load()
	if e != nil {
		return e
	}
	snap, e := safefile.Read(s.ConfigPath())
	if e != nil {
		return e
	}
	d, e := parse(snap.Data)
	if e != nil {
		return e
	}
	vals, e := stringValues(d)
	if e != nil {
		return e
	}
	raw := snap.Data
	if old.Connected {
		if !s.Active(old) {
			return fmt.Errorf("Codex desktop configuration changed; disable the integration before reconnecting")
		}
		if !bytes.Contains(raw, []byte(old.ProviderBlock)) {
			return fmt.Errorf("Codex desktop provider was edited; disable the integration before reconnecting")
		}
		raw = bytes.Replace(raw, []byte(old.ProviderBlock), nil, 1)
		st.Original, st.Existed, st.Previous = old.Original, old.Existed, old.Previous
	} else {
		providers, _ := d["model_providers"].(map[string]any)
		if _, exists := providers[Provider]; exists {
			return fmt.Errorf("provider %s already exists; preserving it", Provider)
		}
		cs, e := safefile.Read(s.CatalogPath())
		if e != nil {
			return e
		}
		if cs.Exists {
			return fmt.Errorf("desktop catalog already exists without ownership state; preserving it")
		}
		st.Original, st.Existed, st.Previous = append([]byte(nil), snap.Data...), snap.Exists, vals
	}
	st.Written = map[string]string{"model": st.Default, "model_provider": Provider, "model_catalog_json": s.CatalogPath(), "model_reasoning_effort": "none"}
	patch := map[string]*string{}
	for k, v := range st.Written {
		v := v
		patch[k] = &v
	}
	raw, e = patchRoots(raw, patch)
	if e != nil {
		return e
	}
	st.ProviderBlock = providerBlock()
	raw = append(raw, []byte(st.ProviderBlock)...)
	if _, e = parse(raw); e != nil {
		return e
	}
	cs, e := safefile.Read(s.CatalogPath())
	if e != nil {
		return e
	}
	if cs.Exists && !bytes.Equal(cs.Data, old.Catalog) {
		return fmt.Errorf("desktop catalog changed outside Unity Gateway")
	}
	st.Connected = true
	st.Catalog = catalog
	st.PreviousCatalog = old.Catalog
	// Persist recovery data before touching either Codex-owned file.
	if e = s.Save(st); e != nil {
		return e
	}
	if _, e = cs.Replace(catalog, 0600); e != nil {
		return e
	}
	_, e = snap.Replace(raw, 0600)
	if e != nil {
		return e
	}
	st.PreviousCatalog = nil
	return s.Save(st)
}
func (s Store) Disconnect() error {
	unlock, lockErr := s.lock()
	if lockErr != nil {
		return lockErr
	}
	defer unlock()
	st, e := s.Load()
	if e != nil || !st.Connected {
		return e
	}
	snap, e := safefile.Read(s.ConfigPath())
	if e != nil {
		return e
	}
	if snap.Exists {
		d, e := parse(snap.Data)
		if e != nil {
			return e
		}
		current, e := stringValues(d)
		if e != nil {
			return e
		}
		raw := snap.Data
		if bytes.Contains(raw, []byte(st.ProviderBlock)) {
			raw = bytes.Replace(raw, []byte(st.ProviderBlock), nil, 1)
		} else {
			// Codex may reformat TOML. Remove only our provider if its value still
			// matches what we installed; preserve every other semantic setting.
			providers, _ := d["model_providers"].(map[string]any)
			expected, _ := parse([]byte(st.ProviderBlock))
			ep := expected["model_providers"].(map[string]any)
			if p, ok := providers[Provider]; ok {
				if !reflect.DeepEqual(p, ep[Provider]) {
					return fmt.Errorf("desktop provider was edited; preserving configuration for manual review")
				}
				delete(providers, Provider)
			}
			if len(providers) == 0 {
				delete(d, "model_providers")
			}
			raw, e = toml.Marshal(d)
			if e != nil {
				return e
			}
		}
		patch := map[string]*string{}
		for _, k := range keys {
			cur, exists := current[k]
			written := st.Written[k]
			owned := exists && cur == written
			if k == "model" && current["model_provider"] == Provider && (st.Has(cur) || compass.IsModel(cur)) {
				owned = true
			}
			if !exists || owned {
				if v, ok := st.Previous[k]; ok {
					v := v
					patch[k] = &v
				} else {
					patch[k] = nil
				}
			}
		}
		raw, e = patchRoots(raw, patch)
		if e != nil {
			return e
		}
		original, _ := parse(st.Original)
		restored, _ := parse(raw)
		if reflect.DeepEqual(original, restored) {
			raw = st.Original
		}
		if !st.Existed && len(bytes.TrimSpace(raw)) == 0 && !snap.FinalLinked {
			e = snap.Remove()
		} else {
			_, e = snap.Replace(raw, snap.Mode)
		}
		if e != nil {
			return e
		}
	}
	cs, e := safefile.Read(s.CatalogPath())
	if e != nil {
		return e
	}
	if cs.Exists {
		if !bytes.Equal(cs.Data, st.Catalog) && !bytes.Equal(cs.Data, st.PreviousCatalog) {
			return fmt.Errorf("desktop catalog was edited; configuration restored but catalog needs manual review")
		}
		if e = cs.Remove(); e != nil {
			return e
		}
	}
	st.Connected = false
	st.Original = nil
	st.Previous = nil
	st.Written = nil
	st.ProviderBlock = ""
	st.Catalog = nil
	st.PreviousCatalog = nil
	return s.Save(st)
}
