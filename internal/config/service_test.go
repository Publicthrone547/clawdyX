package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceLabel(t *testing.T) {
	t.Setenv("CLAWDYX_SERVICE_LABEL", "")
	if ServiceLabel() != DefaultServiceLabel {
		t.Fatal(ServiceLabel())
	}
	t.Setenv("CLAWDYX_SERVICE_LABEL", "local.clawdyx.dev")
	if ServiceLabel() != "local.clawdyx.dev" {
		t.Fatal(ServiceLabel())
	}
	for _, bad := range []string{"../x", "a b", `x\y`, "-x"} {
		t.Setenv("CLAWDYX_SERVICE_LABEL", bad)
		if ServiceLabel() != DefaultServiceLabel {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFoldersOfAnInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	state := filepath.Join(dir, "st")
	os.WriteFile(p, []byte(`{"state_dir":`+jsonStr(state)+`,"bogus":1}`), 0o600)
	s, l := Folders(p)
	if s != state || l == "" {
		t.Fatal(s, l)
	}
	if s, l := Folders(filepath.Join(dir, "missing.json")); s == "" || l == "" {
		t.Fatal("defaults for a missing file")
	}
}
