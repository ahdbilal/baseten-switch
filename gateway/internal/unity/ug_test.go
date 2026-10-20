package unity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUGTokenUsesPinnedHelperAndExplicitBrowserLogin(t *testing.T) {
	root := t.TempDir()
	t.Setenv("UNITY_GATEWAY_UG_RUNTIME", root)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "arguments")
	t.Setenv("TEST_UG_ARGUMENTS", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_UG_ARGUMENTS\"\nprintf 'synthetic-token\\n'\n"
	if err := os.WriteFile(filepath.Join(bin, "ug"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	c := Connection{Workspace: "https://example.cloud.databricks.com", Profile: "profile with spaces"}
	for _, browser := range []bool{false, true} {
		token, err := c.Token(context.Background(), browser)
		if err != nil || token != "synthetic-token" {
			t.Fatal(err)
		}
		args, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(args), "--profile\nprofile with spaces\n") || strings.Contains(string(args), "--browser-login") != browser {
			t.Fatalf("unexpected helper arguments: %s", args)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "ug"), []byte("#!/bin/sh\necho synthetic-secret >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := c.Token(context.Background(), false)
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("helper diagnostics leaked: %v", err)
	}
}
