package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlistProgram(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if PlistProgram("local.clawdyx") != "" {
		t.Fatal("no plist yet")
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "local.clawdyx.plist"), []byte(`<plist><dict><key>ProgramArguments</key><array>
    <string>`+home+`/Applications/clawdyX.app/Contents/Helpers/clawdyx</string><string>run</string></array></dict></plist>`), 0o644)
	if got := PlistProgram("local.clawdyx"); got != home+"/Applications/clawdyX.app/Contents/Helpers/clawdyx" {
		t.Fatalf("program %q", got)
	}
}
