package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsFolders(t *testing.T) {
	home := `C:\Users\Иван Петров`
	t.Setenv("APPDATA", `D:\Roaming`)
	t.Setenv("LOCALAPPDATA", `D:\Local`)
	if got := configDir(home); got != `D:\Roaming\clawdyx` {
		t.Fatal(got)
	}
	if got := defaultStateDir(home); got != `D:\Local\clawdyx` {
		t.Fatal(got)
	}
	// Unset or relative variables fall back to the profile folders.
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", `relative\x`)
	if got := configDir(home); got != filepath.Join(home, "AppData", "Roaming", "clawdyx") {
		t.Fatal(got)
	}
	if got := defaultLogDir(home); got != filepath.Join(home, "AppData", "Local", "clawdyx", "logs") {
		t.Fatal(got)
	}
	if got := legacyConfigPath(home); got != filepath.Join(home, ".config", "clawdyx", "config.json") {
		t.Fatal(got)
	}
	if !pathEqual(`C:\Users\A\x`, `c:\users\a\X`) {
		t.Fatal("NTFS paths compare without case")
	}
}

func TestDefaultPathUsesTheOldFileUntilMoved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAWDYX_CONFIG", "")
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	std := filepath.Join(home, "AppData", "Roaming", "clawdyx", "config.json")
	if DefaultPath() != std {
		t.Fatalf("no config yet: %s", DefaultPath())
	}
	old := filepath.Join(home, ".config", "clawdyx", "config.json")
	os.MkdirAll(filepath.Dir(old), 0o700)
	os.WriteFile(old, []byte("{}"), 0o600)
	if DefaultPath() != old || DefaultEnvPath() != filepath.Join(filepath.Dir(old), ".env") {
		t.Fatalf("0.4.x layout: %s %s", DefaultPath(), DefaultEnvPath())
	}
	if moved, from, err := MigrateDefault(DefaultPath()); !moved || from != old || err != nil {
		t.Fatalf("moved=%v from=%s err=%v", moved, from, err)
	}
	if DefaultPath() != std {
		t.Fatalf("after the move: %s", DefaultPath())
	}
}
