package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeCandidatesFindTheNativeInstaller(t *testing.T) {
	home := t.TempDir()
	c := claudeCandidates(home)
	if len(c) == 0 {
		t.Fatal("no candidates")
	}
	// The native installer's location is tried first on every OS.
	if filepath.Dir(c[0]) != filepath.Join(home, ".local", "bin") {
		t.Fatalf("first candidate %s", c[0])
	}
	if firstFile(c) != "" {
		t.Fatal("nothing installed yet")
	}
	os.MkdirAll(filepath.Dir(c[0]), 0o755)
	os.WriteFile(c[0], []byte("x"), 0o755)
	if firstFile(c) != c[0] {
		t.Fatal("installed binary not found")
	}
}
