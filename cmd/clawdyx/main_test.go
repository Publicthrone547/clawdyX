package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// TestMain keeps tests away from the real Keychain and the real config.
// The memory vault stands in for the system keyring, so the keyring counts
// as available even on a Linux machine without one.
func TestMain(m *testing.M) {
	config.TokenVault = vault.NewMemory()
	vault.Available = func() bool { return true }
	os.Setenv("CLAWDYX_CONFIG", filepath.Join(os.TempDir(), "clawdyx-test-never-created", "config.json"))
	os.Exit(m.Run())
}
