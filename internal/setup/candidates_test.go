package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeCandidatesFindTheNativeInstaller(t *testing.T) {
	home := t.TempDir()
	c := claudeCandidates(home)
	if len(c) == 0 {
		t.Fatal("no candidates")
	}
	// The native installer's location is tried on every OS. Only the
	// candidates under home are checked: the machine may have claude in a
	// system location.
	native := ""
	var inHome []string
	for _, p := range c {
		if filepath.Dir(p) == filepath.Join(home, ".local", "bin") {
			native = p
		}
		if strings.HasPrefix(p, home+string(filepath.Separator)) {
			inHome = append(inHome, p)
		}
	}
	if native == "" {
		t.Fatalf("no native installer location in %v", c)
	}
	if firstFile(inHome) != "" {
		t.Fatal("nothing installed yet")
	}
	os.MkdirAll(filepath.Dir(native), 0o755)
	os.WriteFile(native, []byte("x"), 0o755)
	if firstFile(inHome) != native {
		t.Fatal("installed binary not found")
	}
}
