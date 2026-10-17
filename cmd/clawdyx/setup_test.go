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
)

func fakeClaude(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
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
		if got := workspaceName(in); got != want {
			t.Errorf("workspaceName(%q) = %q want %q", in, got, want)
		}
	}
}

func TestGenerateConfigIsValid(t *testing.T) {
	ws, bin := t.TempDir(), fakeClaude(t)
	data, err := json.MarshalIndent(generateConfig(111111111, ws, "Safari", bin), "", "  ")
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
	for _, p := range []string{cfgPath, envPath} {
		st, err := os.Stat(p)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("%s: want mode 0600, got %v (%v)", p, st, err)
		}
	}
	if st, _ := os.Stat(filepath.Dir(cfgPath)); st.Mode().Perm() != 0o700 {
		t.Fatalf("config dir mode %v want 0700", st.Mode().Perm())
	}
	c, err := config.Load(cfgPath, envPath)
	if err != nil {
		t.Fatalf("written files do not load: %v", err)
	}
	if c.Token != token || c.Telegram.OwnerUserID != 111111111 || c.Workspaces[0].Canonical == "" {
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
