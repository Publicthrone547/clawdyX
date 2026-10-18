package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGitBash(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir) // no git.exe on PATH
	t.Setenv("ProgramFiles", dir)
	t.Setenv("ProgramW6432", "")
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("LOCALAPPDATA", "")
	if DetectGitBash() != "" {
		t.Fatal("found without Git for Windows")
	}
	bash := filepath.Join(dir, "Git", "bin", "bash.exe")
	os.MkdirAll(filepath.Dir(bash), 0o755)
	os.WriteFile(bash, []byte("x"), 0o755)
	if got := DetectGitBash(); got != bash {
		t.Fatalf("got %q", got)
	}
}
