package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// TestMain keeps tests away from the real Keychain and the real config.
func TestMain(m *testing.M) {
	config.TokenVault = vault.NewMemory()
	os.Setenv("CLAWDYX_CONFIG", filepath.Join(os.TempDir(), "clawdyx-test-never-created", "config.json"))
	os.Exit(m.Run())
}
