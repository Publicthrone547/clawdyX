package telegram

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitBashCheck(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("ProgramFiles", dir)
	t.Setenv("ProgramW6432", "")
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("LOCALAPPDATA", "")
	if c := gitBashCheck(""); c.OK || c.Fix == "" {
		t.Fatalf("missing: %+v", c)
	}
	bash := filepath.Join(dir, "Git", "bin", "bash.exe")
	os.MkdirAll(filepath.Dir(bash), 0o755)
	os.WriteFile(bash, []byte("x"), 0o755)
	if c := gitBashCheck(""); !c.OK || c.Detail != bash {
		t.Fatalf("found: %+v", c)
	}
	if c := gitBashCheck(filepath.Join(dir, "nope", "bash.exe")); c.OK {
		t.Fatalf("a wrong setting must fail: %+v", c)
	}
}
