package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/platform"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

func fakeClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude"+platform.ExeSuffix)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWorkspaceName(t *testing.T) {
	cases := map[string]string{
		"/Users/me/projects/telegram_bot":       "telegram_bot",
		"/Users/me/My Project!":                 "my-project",
		"/tmp/-weird-":                          "weird",
		"/":                                     "workspace",
		"/x/" + strings.Repeat("a", 40):         strings.Repeat("a", 32),
		"/x/" + strings.Repeat("a", 31) + "-bb": strings.Repeat("a", 31),
	}
	for in, want := range cases {
		if got := setup.WorkspaceName(in); got != want {
			t.Errorf("workspaceName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestGenerateConfigIsValid(t *testing.T) {
	ws, bin := t.TempDir(), fakeClaude(t)
	data, err := json.MarshalIndent(setup.GenerateConfig(111111111, ws, "Safari", bin), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Parse(data)
	if err != nil {
		t.Fatalf("generated config rejected: %v", err)
	}
	if c.Telegram.OwnerUserID != 111111111 || c.Workspaces[0].Path != ws || c.DefaultWorkspace != c.Workspaces[0].Name || c.Claude.Binary != bin {
		t.Fatalf("unexpected config: %+v", c)
	}
	if strings.Contains(string(data), "i_know_what_i_am_doing") {
		t.Fatal("the deny-list opt-out must not be emitted by default")
	}
	if !strings.Contains(c.Claude.ChildPath, filepath.Dir(bin)) {
		t.Fatalf("child PATH must include the claude directory: %s", c.Claude.ChildPath)
	}
}

func TestSetupNonInteractive(t *testing.T) {
	old := getMeFunc
	getMeFunc = func(context.Context, string) (string, error) { return "testbot", nil }
	defer func() { getMeFunc = old }()
	t.Setenv("TELEGRAM_BOT_TOKEN", "")

	dir, ws, bin := t.TempDir(), t.TempDir(), fakeClaude(t)
	cfgPath, envPath := filepath.Join(dir, "cfg", "config.json"), filepath.Join(dir, "cfg", ".env")
	token := "123456789:" + strings.Repeat("A", 35)
	opts := setupOptions{OwnerID: 111111111, Workspace: ws, Browser: "Safari", ClaudeBinary: bin, TokenStdin: true, NoCheck: true}

	var out bytes.Buffer
	if code := runSetup(cfgPath, envPath, opts, strings.NewReader(token+"\n"), &out); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if strings.Contains(out.String(), token) {
		t.Fatal("the token was echoed")
	}
	if st, err := os.Stat(cfgPath); err != nil || (!platform.IsWindows && st.Mode().Perm() != 0o600) {
		t.Fatalf("%s: want mode 0600, got %v (%v)", cfgPath, st, err)
	}
	if fileExists(envPath) {
		t.Fatal("by default the token must go to the Keychain, not a file")
	}
	if tok, err := config.StoredToken(cfgPath); err != nil || tok != token {
		t.Fatalf("token not in the vault: %v", err)
	}
	if st, _ := os.Stat(filepath.Dir(cfgPath)); !platform.IsWindows && st.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode %v want 0700", st.Mode().Perm())
	}
	c, err := config.Load(cfgPath, envPath)
	if err != nil {
		t.Fatalf("written files do not load: %v", err)
	}
	if c.Token != token || c.TokenSource != "keychain" || c.Telegram.OwnerUserID != 111111111 || c.Workspaces[0].Canonical == "" {
		t.Fatalf("unexpected loaded config: owner %d", c.Telegram.OwnerUserID)
	}

	// A second non-interactive run must not replace the stored token silently.
	out.Reset()
	if code := runSetup(cfgPath, envPath, opts, strings.NewReader(token+"\n"), &out); code != exitConfig {
		t.Fatalf("expected refusal without -force, got %d:\n%s", code, out.String())
	}
	// With -force both files are regenerated.
	opts.Force = true
	out.Reset()
	if code := runSetup(cfgPath, envPath, opts, strings.NewReader(token+"\n"), &out); code != exitOK {
		t.Fatalf("forced rerun failed %d:\n%s", code, out.String())
	}
}

func TestSetupRejectsBadInputWithoutWriting(t *testing.T) {
	old := getMeFunc
	getMeFunc = func(context.Context, string) (string, error) { return "testbot", nil }
	defer func() { getMeFunc = old }()
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	ws, bin := t.TempDir(), fakeClaude(t)

	cases := map[string]struct {
		opts  setupOptions
		stdin string
	}{
		"malformed token": {setupOptions{OwnerID: 1, Workspace: ws, ClaudeBinary: bin, TokenStdin: true, NoCheck: true}, "not-a-token\n"},
		"zero owner":      {setupOptions{Workspace: ws, ClaudeBinary: bin, TokenStdin: true, NoCheck: true}, "123456789:" + strings.Repeat("A", 35) + "\n"},
		"bad workspace":   {setupOptions{OwnerID: 1, Workspace: filepath.Join(ws, "missing"), ClaudeBinary: bin, TokenStdin: true, NoCheck: true}, "123456789:" + strings.Repeat("A", 35) + "\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath, envPath := filepath.Join(dir, "config.json"), filepath.Join(dir, ".env")
			var out bytes.Buffer
			if code := runSetup(cfgPath, envPath, tc.opts, strings.NewReader(tc.stdin), &out); code != exitConfig {
				t.Fatalf("exit %d want %d:\n%s", code, exitConfig, out.String())
			}
			if fileExists(cfgPath) || fileExists(envPath) {
				t.Fatal("nothing must be written when input is rejected")
			}
		})
	}
}

func TestSetupTokenFileAndMigration(t *testing.T) {
	old := getMeFunc
	getMeFunc = func(context.Context, string) (string, error) { return "testbot", nil }
	defer func() { getMeFunc = old }()
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	dir, ws, bin := t.TempDir(), t.TempDir(), fakeClaude(t)
	cfgPath, envPath := filepath.Join(dir, "config.json"), filepath.Join(dir, ".env")
	token := "123456789:" + strings.Repeat("B", 35)
	var out bytes.Buffer
	opts := setupOptions{OwnerID: 111111111, Workspace: ws, ClaudeBinary: bin, TokenStdin: true, TokenFile: true, NoCheck: true}
	if code := runSetup(cfgPath, envPath, opts, strings.NewReader(token+"\n"), &out); code != exitOK {
		t.Fatalf("%d %s", code, out.String())
	}
	if st, err := os.Stat(envPath); err != nil || (!platform.IsWindows && st.Mode().Perm() != 0o600) {
		t.Fatal("token file missing or not private")
	}
	if c, err := config.Load(cfgPath, envPath); err != nil || c.TokenSource != "file" {
		t.Fatalf("load: %v", err)
	}
	out.Reset()
	if code := runSetup(cfgPath, envPath, setupOptions{MigrateToken: true}, strings.NewReader(""), &out); code != exitOK {
		t.Fatalf("migrate: %d %s", code, out.String())
	}
	if fileExists(envPath) || strings.Contains(out.String(), token) {
		t.Fatal("file kept or token echoed")
	}
	c, err := config.Load(cfgPath, envPath)
	if err != nil || c.TokenSource != "keychain" || c.Token != token {
		t.Fatalf("after migration: %v %+v", err, c)
	}
	// The environment variable wins (development).
	t.Setenv("TELEGRAM_BOT_TOKEN", "987654321:"+strings.Repeat("C", 35))
	if c, _ := config.Load(cfgPath, envPath); c.TokenSource != "env" {
		t.Fatal("env must win")
	}
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	// Migrating again without a file fails without touching the vault.
	if code := runSetup(cfgPath, envPath, setupOptions{MigrateToken: true}, strings.NewReader(""), &out); code == exitOK {
		t.Fatal("migration without a file succeeded")
	}
	if tok, _ := config.StoredToken(cfgPath); tok != token {
		t.Fatal("vault changed")
	}
}
