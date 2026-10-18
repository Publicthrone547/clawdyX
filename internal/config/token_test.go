package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/vault"
)

// errStore fails every call with err.
type errStore struct{ err error }

func (s errStore) Get(string) (string, error) { return "", s.err }
func (s errStore) Set(string, string) error   { return s.err }
func (s errStore) Delete(string) error        { return s.err }
func (s errStore) Kind() string               { return "fake" }

func writeConfig(t *testing.T) (cfg, env string) {
	t.Helper()
	dir := t.TempDir()
	cfg, env = filepath.Join(dir, "config.json"), filepath.Join(dir, ".env")
	if err := os.WriteFile(cfg, []byte(baseJSON(t, "111111111", t.TempDir(), "")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("TELEGRAM_BOT_TOKEN=123456789:"+strings.Repeat("E", 35)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg, env
}

func TestLoadFallsBackOnlyWhenNoKeyringExists(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	old := TokenVault
	defer func() { TokenVault = old }()
	cfg, env := writeConfig(t)

	TokenVault = errStore{vault.ErrUnavailable}
	c, err := Load(cfg, env)
	if err != nil || c.TokenSource != "file" || !c.NoKeyring {
		t.Fatalf("no keyring: %v %+v", err, c)
	}

	TokenVault = errStore{errors.New("org.freedesktop.Secret.Error.IsLocked")}
	if _, err := Load(cfg, env); err == nil || !strings.Contains(err.Error(), "token store failed") {
		t.Fatalf("a locked or broken store must fail closed, got %v", err)
	}

	TokenVault = errStore{vault.ErrNotFound}
	if c, err := Load(cfg, env); err != nil || c.TokenSource != "file" || c.NoKeyring {
		t.Fatalf("no item: %v", err)
	}
}
