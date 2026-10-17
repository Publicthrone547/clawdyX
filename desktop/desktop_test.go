package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlistProgramAndHomeRel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if plistProgram() != "" {
		t.Fatal("no plist yet")
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, serviceLabel+".plist"), []byte(`<plist><dict><key>ProgramArguments</key><array>
    <string>`+home+`/Applications/clawdyX.app/Contents/Helpers/clawdyx</string><string>run</string></array></dict></plist>`), 0o644)
	if got := plistProgram(); got != home+"/Applications/clawdyX.app/Contents/Helpers/clawdyx" {
		t.Fatalf("program %q", got)
	}
	if got := homeRel(home + "/x/y"); got != "~/x/y" {
		t.Fatalf("homeRel %q", got)
	}
	if homeRel("/opt/x") != "/opt/x" {
		t.Fatal("paths outside home must stay as they are")
	}
}

func TestOpenOnlyKnownLinks(t *testing.T) {
	s := &SystemService{}
	if err := s.Open("file:///etc/passwd"); err == nil {
		t.Fatal("arbitrary URL accepted")
	}
	if err := s.Open("../../x"); err == nil {
		t.Fatal("arbitrary key accepted")
	}
	for k, u := range links {
		if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "x-apple.systempreferences:") {
			t.Errorf("%s: unexpected scheme %s", k, u)
		}
	}
}

func TestSanitizeRedactsTokensAndHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	tok := "123456789:" + strings.Repeat("A", 35)
	out := sanitize("token "+tok+" path "+home+"/Library/x", false, &WorkerService{})
	if strings.Contains(out, tok) || strings.Contains(out, home) || !strings.Contains(out, "~/Library/x") {
		t.Fatalf("not sanitized: %q", out)
	}
}

func TestWorkerCallsFailCleanlyWhenDown(t *testing.T) {
	w := &WorkerService{}
	if _, err := w.Status(); err != ErrWorkerDown {
		t.Fatalf("want ErrWorkerDown, got %v", err)
	}
}
