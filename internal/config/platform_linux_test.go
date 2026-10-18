package config

import (
	"path/filepath"
	"testing"
)

func TestXDGFolders(t *testing.T) {
	home := "/home/jöhn doe"
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	t.Setenv("XDG_STATE_HOME", "/tmp/x")
	if configDir(home) != "/cfg/clawdyx" || defaultStateDir(home) != "/tmp/x/clawdyx" || defaultLogDir(home) != "/tmp/x/clawdyx/logs" {
		t.Fatal(configDir(home), defaultStateDir(home), defaultLogDir(home))
	}
	// Relative values are ignored, as the XDG Base Directory rules say.
	t.Setenv("XDG_CONFIG_HOME", "rel")
	t.Setenv("XDG_STATE_HOME", "")
	if configDir(home) != filepath.Join(home, ".config", "clawdyx") || defaultStateDir(home) != filepath.Join(home, ".local", "state", "clawdyx") {
		t.Fatal(configDir(home), defaultStateDir(home))
	}
	if legacyConfigPath(home) != "" || pathEqual("/a/B", "/a/b") {
		t.Fatal("linux has no legacy path and compares case-sensitively")
	}
}
