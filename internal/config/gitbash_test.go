package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGitBash(t *testing.T) {
	dir := t.TempDir()
	bash := filepath.Join(dir, "bash.exe")
	os.WriteFile(bash, []byte("x"), 0o755)
	if err := checkGitBash(bash); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"bash.exe", filepath.Join(dir, "cmd.exe"), filepath.Join(dir, "missing", "bash.exe"), dir} {
		if err := checkGitBash(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestGitBashInChildEnv(t *testing.T) {
	cl := Claude{GitBashPath: `C:\Git\bin\bash.exe`}
	env := strings.Join(cl.ChildEnvExtra(), "\n")
	if gitBashUsed != strings.Contains(env, "CLAUDE_CODE_GIT_BASH_PATH=") {
		t.Fatalf("used=%v env=%q", gitBashUsed, env)
	}
	if len(Claude{}.ChildEnvExtra()) != 0 {
		t.Fatal("nothing set, nothing added")
	}
}
