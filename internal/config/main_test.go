package config

import (
	"os"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// TestMain keeps tests away from the real Keychain.
func TestMain(m *testing.M) {
	TokenVault = vault.NewMemory()
	os.Exit(m.Run())
}
