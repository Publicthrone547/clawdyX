//go:build !windows

package main

import (
	"os"
	"path/filepath"
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
