package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/platform"
)

func fakeBinary(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude"+platform.ExeSuffix)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func baseJSON(t *testing.T, owner string, ws string, extra string) string {
	t.Helper()
	bin := fakeBinary(t)
	return `{"telegram":{"owner_user_id":` + owner + `},
	"claude":{"binary":` + jsonStr(bin) + extra + `},
	"workspaces":[{"name":"ws","path":` + jsonStr(ws) + `,"denied_commands":["Bash(rm *)","Bash(sudo *)","Bash(git push *)"]}],
	"default_workspace":"ws"}`
}

func TestParseValid(t *testing.T) {
	ws := t.TempDir()
	c, err := Parse([]byte(baseJSON(t, "111111111", ws, "")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Claude.MaxConcurrentJobs != 2 || c.Claude.DefaultEffort != "auto" || c.Claude.JobTimeout.Minutes() != 15 {
		t.Fatalf("defaults not applied: %+v", c.Claude)
	}
	canon, _ := filepath.EvalSymlinks(ws)
	if c.Workspaces[0].Canonical != canon {
		t.Fatalf("canonical path %q want %q", c.Workspaces[0].Canonical, canon)
	}
}

func TestParseRejects(t *testing.T) {
	ws := t.TempDir()
	cases := map[string]string{
		"zero owner":       baseJSON(t, "0", ws, ""),
		"negative owner":   baseJSON(t, "-7", ws, ""),
		"owner list":       baseJSON(t, "[111111111]", ws, ""),
		"owner wildcard":   baseJSON(t, `"*"`, ws, ""),
		"owner username":   baseJSON(t, `"@someone"`, ws, ""),
		"missing owner":    strings.Replace(baseJSON(t, "111111111", ws, ""), `"owner_user_id":111111111`, `"poll_timeout_seconds":30`, 1),
		"bad effort":       baseJSON(t, "111111111", ws, `,"default_effort":"ultra"`),
		"too many jobs":    baseJSON(t, "111111111", ws, `,"max_concurrent_jobs":9`),
		"unknown field":    baseJSON(t, "111111111", ws, `,"dangerously_skip_permissions":true`),
		"missing ws":       baseJSON(t, "111111111", filepath.Join(ws, "nope"), ""),
		"relative ws":      baseJSON(t, "111111111", "relative/path", ""),
		"budget too small": baseJSON(t, "111111111", ws, `,"max_budget_usd":0.5`),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Fatalf("expected rejection")
			}
		})
	}
}

func TestMinimumDenyList(t *testing.T) {
	ws := t.TempDir()
	doc := strings.Replace(baseJSON(t, "111111111", ws, ""), `"Bash(rm *)",`, "", 1)
	if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "Bash(rm *)") {
		t.Fatalf("expected deny-list error, got %v", err)
	}
}

func TestSymlinkWorkspaceResolved(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	symlink(t, real, link)
	c, err := Parse([]byte(baseJSON(t, "111111111", link, "")))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	if c.Workspaces[0].Canonical != want {
		t.Fatalf("got %q want %q", c.Workspaces[0].Canonical, want)
	}
}

func TestLoadTokenAndPermissions(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	good := "TELEGRAM_BOT_TOKEN=123456789:" + strings.Repeat("A", 35) + "\n"
	if err := os.WriteFile(env, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(env); err == nil && !platform.IsWindows {
		t.Fatal("expected permission error for 0644 env file")
	}
	if err := os.Chmod(env, 0o600); err != nil {
		t.Fatal(err)
	}
	tok, err := LoadToken(env)
	if err != nil || !strings.HasPrefix(tok, "123456789:") {
		t.Fatalf("token load failed: %v", err)
	}
	if err := os.WriteFile(env, []byte("TELEGRAM_BOT_TOKEN=not-a-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(env); err == nil || strings.Contains(err.Error(), "not-a-token") {
		t.Fatalf("expected format error without echoing value, got %v", err)
	}
	if _, err := LoadToken(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestAnyPositiveOwnerAccepted(t *testing.T) {
	ws := t.TempDir()
	c, err := Parse([]byte(baseJSON(t, "42", ws, "")))
	if err != nil {
		t.Fatalf("a positive owner id must be accepted: %v", err)
	}
	if c.Telegram.OwnerUserID != 42 {
		t.Fatalf("owner id %d want 42", c.Telegram.OwnerUserID)
	}
}

func TestAllowlistValidation(t *testing.T) {
	good := []AllowedUser{{ID: 111111111, Role: RoleOwner, Label: "me"}, {ID: 222222222, Role: RoleUser, Label: "Ann (phone)"}}
	u, err := NormalizeAllowlist(0, good)
	if err != nil || len(u) != 2 || u[0].Role != RoleOwner {
		t.Fatalf("%v %+v", err, u)
	}
	// owner listed second is moved first
	u, _ = NormalizeAllowlist(0, []AllowedUser{good[1], good[0]})
	if u[0].ID != 111111111 {
		t.Fatal("owner must come first")
	}
	if u, err := NormalizeAllowlist(111111111, nil); err != nil || len(u) != 1 || u[0].Role != RoleOwner {
		t.Fatalf("legacy owner field: %v %+v", err, u)
	}
	bad := map[string][]AllowedUser{
		"empty and no owner": nil,
		"no owner":           {{ID: 1, Role: RoleUser}},
		"two owners":         {{ID: 1, Role: RoleOwner}, {ID: 2, Role: RoleOwner}},
		"duplicate":          {{ID: 1, Role: RoleOwner}, {ID: 1, Role: RoleUser}},
		"zero":               {{ID: 0, Role: RoleOwner}},
		"negative":           {{ID: -5, Role: RoleOwner}},
		"bad role":           {{ID: 1, Role: "admin"}},
		"bad label":          {{ID: 1, Role: RoleOwner, Label: "<script>"}},
	}
	for name, l := range bad {
		if _, err := NormalizeAllowlist(0, l); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := NormalizeAllowlist(333, good); err == nil {
		t.Error("conflicting owner_user_id accepted")
	}
	many := []AllowedUser{{ID: 1, Role: RoleOwner}}
	for i := 2; i <= MaxAllowedUsers+1; i++ {
		many = append(many, AllowedUser{ID: int64(i), Role: RoleUser})
	}
	if _, err := NormalizeAllowlist(0, many); err == nil {
		t.Error("too many users accepted")
	}
	tg := Telegram{AllowedUsers: good}
	if tg.Role(222222222) != RoleUser || tg.Role(9) != "" {
		t.Fatal("role lookup")
	}
}

func TestParseAllowedUsers(t *testing.T) {
	ws := t.TempDir()
	doc := strings.Replace(baseJSON(t, "0", ws, ""), `"owner_user_id":0`, `"allowed_users":[{"id":222222222,"role":"user"},{"id":111111111,"role":"owner","label":"me"}]`, 1)
	c, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if c.Telegram.OwnerUserID != 111111111 || len(c.Telegram.AllowedUsers) != 2 {
		t.Fatalf("%+v", c.Telegram)
	}
}

// jsonStr quotes a path for a JSON document (Windows paths have backslashes).
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// symlink creates a symbolic link or skips the test where the OS refuses
// (Windows without Developer Mode or administrator rights).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if platform.IsWindows {
			t.Skipf("symbolic links are not available: %v", err)
		}
		t.Fatal(err)
	}
}
