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

func TestSystemdUnitAndProgram(t *testing.T) {
	for label, want := range map[string]string{"local.clawdyx": "clawdyx", "local.clawdyx.dev": "clawdyx-dev", "mybot": "mybot"} {
		if got := SystemdUnit(label); got != want {
			t.Errorf("%s → %s want %s", label, got, want)
		}
	}
	unit := "[Service]\nExecStart=\"/home/jöhn doe/bin/clawdyx\" run -config \"/x/my \\\"odd\\\" 100%% $$c.json\"\n"
	if got := UnitProgram(unit); got != "/home/jöhn doe/bin/clawdyx" {
		t.Fatal(got)
	}
	if got := UnitProgram("ExecStart=/usr/bin/clawdyx run\n"); got != "/usr/bin/clawdyx" {
		t.Fatal(got)
	}
	if UnitProgram("[Unit]\n") != "" || UnitProgram("ExecStart=\"unterminated") != "" {
		t.Fatal("no program")
	}
}
