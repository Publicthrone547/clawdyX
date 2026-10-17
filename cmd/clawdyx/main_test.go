package main

import (
	"os"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// TestMain keeps tests away from the real Keychain.
func TestMain(m *testing.M) {
	config.TokenVault = vault.NewMemory()
	os.Exit(m.Run())
}
