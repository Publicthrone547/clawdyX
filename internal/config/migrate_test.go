package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/vault"
)

func writeLegacy(t *testing.T, dir string, withEnv bool) (cfg, tok string) {
	t.Helper()
	cfg = filepath.Join(dir, ".config", "clawdyx", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte(`{"telegram":{"owner_user_id":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tok = "123456789:" + strings.Repeat("M", 35)
	if withEnv {
		if err := os.WriteFile(EnvPathFor(cfg), []byte("TELEGRAM_BOT_TOKEN="+tok+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	} else if err := TokenVault.Set(vault.TokenAccount(cfg), tok); err != nil {
		t.Fatal(err)
	}
	return cfg, tok
}

func TestMigrateLegacyMovesFilesAndToken(t *testing.T) {
	dir := t.TempDir()
	old, tok := writeLegacy(t, dir, false)
	nu := filepath.Join(dir, "AppData", "Roaming", "clawdyx", "config.json")

	moved, err := MigrateLegacy(nu, old)
	if err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if b, err := os.ReadFile(nu); err != nil || !strings.Contains(string(b), "owner_user_id") {
		t.Fatalf("config not at the new place: %v", err)
	}
	if got, err := StoredToken(nu); err != nil || got != tok {
		t.Fatalf("token not re-keyed: %v", err)
	}
	if _, err := StoredToken(old); err == nil {
		t.Fatal("the old vault item must be removed")
	}
	if _, err := os.Stat(filepath.Dir(old)); !os.IsNotExist(err) {
		t.Fatalf("the empty old folder must be removed: %v", err)
	}
	if from := TakeMigratedMarker(nu); from != old {
		t.Fatalf("marker %q", from)
	}
	if TakeMigratedMarker(nu) != "" {
		t.Fatal("the marker is read once")
	}

	// Idempotent: nothing left to move.
	if moved, err := MigrateLegacy(nu, old); err != nil || moved {
		t.Fatalf("second run moved=%v err=%v", moved, err)
	}
}

func TestMigrateLegacyMovesEnvFile(t *testing.T) {
	dir := t.TempDir()
	old, tok := writeLegacy(t, dir, true)
	nu := filepath.Join(dir, "new", "config.json")
	if moved, err := MigrateLegacy(nu, old); err != nil || !moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	got, err := LoadToken(EnvPathFor(nu))
	if err != nil || got != tok {
		t.Fatalf(".env not moved: %v", err)
	}
	if _, err := os.Stat(EnvPathFor(old)); !os.IsNotExist(err) {
		t.Fatal("old .env left behind")
	}
}

func TestMigrateLegacyNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	old, _ := writeLegacy(t, dir, false)
	nu := filepath.Join(dir, "new", "config.json")
	os.MkdirAll(filepath.Dir(nu), 0o700)
	os.WriteFile(nu, []byte("{}"), 0o600)
	if moved, err := MigrateLegacy(nu, old); err != nil || moved {
		t.Fatalf("moved=%v err=%v", moved, err)
	}
	if b, _ := os.ReadFile(nu); string(b) != "{}" {
		t.Fatal("existing config replaced")
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatal("old config must stay when nothing moved")
	}
	if _, err := StoredToken(old); err != nil {
		t.Fatal("old token must stay when nothing moved")
	}
}

func TestMigrateDefaultSkipsOverrides(t *testing.T) {
	t.Setenv("CLAWDYX_CONFIG", filepath.Join(t.TempDir(), "x.json"))
	if moved, _, err := MigrateDefault(LegacyPath()); moved || err != nil {
		t.Fatal("CLAWDYX_CONFIG must skip the migration")
	}
	t.Setenv("CLAWDYX_CONFIG", "")
	if moved, _, err := MigrateDefault(filepath.Join(t.TempDir(), "other.json")); moved || err != nil {
		t.Fatal("another -config must skip the migration")
	}
}

func TestEnvPathFollowsConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dev", "config.json")
	t.Setenv("CLAWDYX_CONFIG", p)
	if DefaultPath() != p || DefaultEnvPath() != filepath.Join(filepath.Dir(p), ".env") {
		t.Fatalf("%s %s", DefaultPath(), DefaultEnvPath())
	}
}
