package codexdesktop

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unity-gateway/gateway/internal/compass"
)

func fixture(t *testing.T, original string) (Store, State) {
	t.Helper()
	dir := t.TempDir()
	s := Store{Dir: dir, StatePath: filepath.Join(t.TempDir(), "state.json")}
	if original != "" {
		if e := os.WriteFile(s.ConfigPath(), []byte(original), 0600); e != nil {
			t.Fatal(e)
		}
	}
	st := State{Default: compass.Model, Models: []Model{{ID: compass.Model, Name: "Auto(Compass)"}, {ID: "system.ai.glm-5-3", Name: "GLM"}}, Hidden: map[string]bool{}}
	return s, st
}
func TestConnectRefreshDisconnectRestoresOriginal(t *testing.T) {
	original := "# My OpenAI setup\nmodel = 'gpt-6-sol' # keep comment\nmodel_reasoning_effort = \"high\"\n[mcp_servers.example]\ncommand = 'echo'\n[projects.'/tmp/example']\ntrust_level = 'trusted'\n"
	s, st := fixture(t, original)
	auth := []byte("unchanged auth fixture")
	os.WriteFile(filepath.Join(s.Dir, "auth.json"), auth, 0600)
	os.WriteFile(filepath.Join(s.Dir, "unity-gateway.config.toml"), []byte("separate CLI profile"), 0600)
	for i := 0; i < 2; i++ {
		if e := s.Apply(st, []byte(`{"models":[]}`)); e != nil {
			t.Fatal(e)
		}
		st, _ = s.Load()
		if !s.Active(st) {
			t.Fatal("not active")
		}
	}
	st.Default = "system.ai.glm-5-3"
	if e := s.Apply(st, []byte(`{"models":[]}`)); e != nil {
		t.Fatal(e)
	}
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(s.ConfigPath())
	if string(b) != original {
		t.Fatalf("did not restore bytes:\n%s", b)
	}
	if _, e := os.Stat(s.CatalogPath()); !os.IsNotExist(e) {
		t.Fatal("catalog retained")
	}
	b, _ = os.ReadFile(filepath.Join(s.Dir, "auth.json"))
	if !bytes.Equal(b, auth) {
		t.Fatal("auth changed")
	}
	b, _ = os.ReadFile(filepath.Join(s.Dir, "unity-gateway.config.toml"))
	if string(b) != "separate CLI profile" {
		t.Fatal("CLI profile changed")
	}
}
func TestDisconnectPreservesUnrelatedEditsAndNestedKeys(t *testing.T) {
	s, st := fixture(t, "model = 'gpt-6-sol'\n[project]\nmodel = 'nested'\n")
	if e := s.Apply(st, []byte("catalog")); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(s.ConfigPath())
	b = append(b, []byte("\n[mcp_servers.new]\ncommand = 'test' # user comment\n")...)
	os.WriteFile(s.ConfigPath(), b, 0600)
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(s.ConfigPath())
	for _, text := range []string{"gpt-6-sol", "model = 'nested'", "command = 'test' # user comment"} {
		if !strings.Contains(string(b), text) {
			t.Fatalf("lost %s: %s", text, b)
		}
	}
	if strings.Contains(string(b), Provider) {
		t.Fatal("provider remains")
	}
}
func TestNewConfigRemovedAndSymlinkPreserved(t *testing.T) {
	s, st := fixture(t, "")
	if e := s.Apply(st, []byte("catalog")); e != nil {
		t.Fatal(e)
	}
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(s.ConfigPath()); !os.IsNotExist(e) {
		b, _ := os.ReadFile(s.ConfigPath())
		t.Fatalf("new config remains: %q", b)
	}
	target := filepath.Join(t.TempDir(), "linked.toml")
	os.WriteFile(target, []byte("model = 'openai'\n"), 0600)
	os.Symlink(target, s.ConfigPath())
	if e := s.Apply(st, []byte("catalog")); e != nil {
		t.Fatal(e)
	}
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Readlink(s.ConfigPath()); e != nil {
		t.Fatal("severed symlink", e)
	}
}
func TestRejectForeignProviderAndPreserveExternalProviderChoice(t *testing.T) {
	s, st := fixture(t, "[model_providers."+Provider+"]\nbase_url = 'https://example.com'\n")
	if e := s.Apply(st, []byte("catalog")); e == nil {
		t.Fatal("adopted foreign provider")
	}
	s, st = fixture(t, "model = 'gpt-6-sol'\n")
	if e := s.Apply(st, []byte("catalog")); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(s.ConfigPath())
	b = bytes.Replace(b, []byte(`model_provider = "`+Provider+`"`), []byte(`model_provider = "other"`), 1)
	b = bytes.Replace(b, []byte(`model = "`+compass.Model+`"`), []byte(`model = "other-model"`), 1)
	os.WriteFile(s.ConfigPath(), b, 0600)
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(s.ConfigPath())
	if !bytes.Contains(b, []byte(`model_provider = "other"`)) || !bytes.Contains(b, []byte(`model = "other-model"`)) {
		t.Fatal("overwrote external choice")
	}
}
func TestPatchQuotedAndMultilineRoots(t *testing.T) {
	s, st := fixture(t, "'model' = '''gpt-6-sol''' # comment\nnotes = '''\nmodel = \"not-a-setting\"\n'''\n[project]\nmodel='nested'\n")
	if e := s.Apply(st, []byte("catalog")); e != nil {
		t.Fatal(e)
	}
	if e := s.Disconnect(); e != nil {
		t.Fatal(e)
	}
}

// A process may exit after saving recovery state but before replacing the catalog.
func TestDisconnectRecoversInterruptedRefresh(t *testing.T) {
	original := "model = 'openai-model'\n"
	s, st := fixture(t, original)
	if err := s.Apply(st, []byte("old catalog")); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Load()
	st.PreviousCatalog = st.Catalog
	st.Catalog = []byte("new catalog")
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	if err := s.Disconnect(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(s.ConfigPath())
	if string(got) != original {
		t.Fatal(string(got))
	}
	if _, err := os.Stat(s.CatalogPath()); !os.IsNotExist(err) {
		t.Fatal("managed catalog not removed")
	}
}

func TestConcurrentChangesLeaveConfigurationIntact(t *testing.T) {
	s, st := fixture(t, "model = 'openai-model'\n")
	if err := s.Apply(st, []byte("catalog")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.ConfigPath())
	unlock, err := s.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := s.Disconnect(); err == nil {
		t.Fatal("concurrent disconnect accepted")
	}
	if err := s.Apply(st, []byte("different")); err == nil {
		t.Fatal("concurrent apply accepted")
	}
	after, _ := os.ReadFile(s.ConfigPath())
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed")
	}
}
