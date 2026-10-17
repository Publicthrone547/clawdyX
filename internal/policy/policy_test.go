package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

func ws(t *testing.T) *config.Workspace {
	dir := t.TempDir()
	canon, _ := filepath.EvalSymlinks(dir)
	return &config.Workspace{Name: "w", Path: dir, Canonical: canon,
		AllowedCommands: []string{"Bash(go test *)"}, DeniedCommands: []string{"Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"}}
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func TestObserveFlags(t *testing.T) {
	f, err := For(config.ModeObserve, ws(t), config.Claude{}, "/Users/u", false)
	if err != nil {
		t.Fatal(err)
	}
	if f.PermissionMode != "dontAsk" || len(f.Allowed) != 0 {
		t.Fatalf("observe: %+v", f)
	}
	for _, want := range []string{"Edit", "Write", "NotebookEdit", "WebFetch", "WebSearch", "Bash(rm *)", "Read(//Users/u/.ssh/**)", "Read(//Users/u/.claude/**)"} {
		if !has(f.Disallowed, want) {
			t.Errorf("observe missing deny %s", want)
		}
	}
	var s map[string]map[string]any
	if err := json.Unmarshal([]byte(f.SettingsJSON), &s); err != nil {
		t.Fatal(err)
	}
	if s["permissions"]["disableBypassPermissionsMode"] != "disable" || s["permissions"]["blockReadsOutsideWorkingDirectories"] != true {
		t.Fatalf("settings: %s", f.SettingsJSON)
	}
}

func TestAssistedFlags(t *testing.T) {
	f, err := For(config.ModeAssisted, ws(t), config.Claude{ExtraDisallowedTools: []string{"Bash(make deploy *)"}}, "/Users/u", false)
	if err != nil {
		t.Fatal(err)
	}
	if f.PermissionMode != "acceptEdits" || !has(f.Allowed, "Bash(go test *)") || has(f.Disallowed, "Edit") {
		t.Fatalf("assisted: %+v", f)
	}
	if !has(f.Disallowed, "Bash(make deploy *)") || !has(f.Disallowed, "WebFetch") {
		t.Fatal("assisted must keep deny rules")
	}
}

func TestNoBypassAnywhere(t *testing.T) {
	for _, m := range []string{config.ModeObserve, config.ModeAssisted} {
		f, _ := For(m, ws(t), config.Claude{}, "/Users/u", false)
		joined := f.PermissionMode + strings.Join(f.Allowed, " ")
		if strings.Contains(joined, "bypass") {
			t.Fatalf("bypass leaked in %s", m)
		}
	}
	if _, err := For(config.ModeAutomation, ws(t), config.Claude{}, "/Users/u", false); err == nil {
		t.Fatal("automation must be disabled unless the config enables it")
	}
	f, err := For(config.ModeAutomation, ws(t), config.Claude{}, "/Users/u", true)
	if err != nil || f.PermissionMode != "auto" {
		t.Fatalf("automation: %+v %v", f, err)
	}
	for _, want := range []string{"Bash(rm *)", "Bash(sudo *)", "Bash(git push *)", "WebFetch", "Read(//Users/u/.ssh/**)"} {
		if !has(f.Disallowed, want) {
			t.Errorf("automation missing deny %s", want)
		}
	}
	if has(f.Disallowed, "WebSearch") {
		t.Error("automation should allow WebSearch")
	}
	if obs, _ := For(config.ModeObserve, ws(t), config.Claude{}, "/Users/u", true); !has(obs.Disallowed, "WebSearch") {
		t.Error("observe must still deny WebSearch")
	}
	if !strings.Contains(f.SettingsJSON, "disableBypassPermissionsMode") || strings.Contains(f.SettingsJSON, "disableAutoMode") {
		t.Fatalf("automation settings: %s", f.SettingsJSON)
	}
	if _, err := For("root", ws(t), config.Claude{}, "/Users/u", false); err == nil {
		t.Fatal("unknown mode must fail")
	}
	w := ws(t)
	w.AllowedCommands = []string{"Bash"}
	if _, err := For(config.ModeAssisted, w, config.Claude{}, "/Users/u", false); err == nil {
		t.Fatal("bare Bash allow must be rejected")
	}
}

func TestWithin(t *testing.T) {
	w := ws(t)
	inside := filepath.Join(w.Canonical, "a")
	os.MkdirAll(inside, 0o755)
	if _, err := Within(w.Canonical, inside); err != nil {
		t.Fatalf("inside rejected: %v", err)
	}
	if _, err := Within(w.Canonical, filepath.Join(w.Canonical, "..")); err == nil {
		t.Fatal("parent accepted")
	}
	if _, err := Within(w.Canonical, w.Canonical+"-sibling"); err == nil {
		t.Fatal("missing sibling accepted")
	}
	outside := t.TempDir()
	link := filepath.Join(w.Canonical, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Within(w.Canonical, link); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err := Within(w.Canonical, "relative"); err == nil {
		t.Fatal("relative accepted")
	}
}

func TestCheckWorkspaceDirDetectsSwap(t *testing.T) {
	w := ws(t)
	if _, err := CheckWorkspaceDir(w); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	link := filepath.Join(t.TempDir(), "ws")
	os.Symlink(other, link)
	w.Path = link
	if _, err := CheckWorkspaceDir(w); err == nil {
		t.Fatal("swapped workspace accepted")
	}
}

func TestDisplayPath(t *testing.T) {
	if DisplayPath("/a/b/c", []string{"/a/b"}) != "/a/b/c" || DisplayPath("/a/bc", []string{"/a/b"}) != "<outside workspace>" {
		t.Fatal("display path")
	}
}
