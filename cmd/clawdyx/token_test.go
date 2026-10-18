package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/vault"
)

func TestTokenDeleteRemovesOnlyThisConfigsToken(t *testing.T) {
	dir := t.TempDir()
	cfg, other := filepath.Join(dir, "config.json"), filepath.Join(dir, "other", "config.json")
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("TELEGRAM_BOT_TOKEN=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := vault.NewMemory()
	_ = st.Set(vault.TokenAccount(cfg), "mine")
	_ = st.Set(vault.TokenAccount(other), "theirs")
	var out strings.Builder
	if code := tokenCmd(st, cfg, env, []string{"delete"}, &out); code != exitOK {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if _, err := st.Get(vault.TokenAccount(cfg)); err == nil {
		t.Fatal("this config's token is still stored")
	}
	if v, _ := st.Get(vault.TokenAccount(other)); v != "theirs" {
		t.Fatal("another config's token was touched")
	}
	if _, err := os.Stat(env); !os.IsNotExist(err) {
		t.Fatal("the token file is still there")
	}
	out.Reset()
	if code := tokenCmd(st, cfg, env, []string{"delete"}, &out); code != exitOK || !strings.Contains(out.String(), "no stored bot token") {
		t.Fatalf("second delete: %d %q", code, out.String())
	}
	if code := tokenCmd(st, cfg, env, nil, &out); code != exitConfig {
		t.Fatal("no subcommand was accepted")
	}
}
