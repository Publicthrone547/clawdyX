package telegram

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
)

func writeConfig(t *testing.T, h *harness) string {
	t.Helper()
	c := *h.cfg
	c.Claude.JobTimeout = config.Duration{Duration: time.Minute}
	ws := append([]config.Workspace(nil), c.Workspaces...)
	ws[0].DeniedCommands = append([]string{}, config.MinimumDenied...)
	c.Workspaces = ws
	if _, err := config.Parse(mustJSON(t, &c)); err != nil {
		t.Fatalf("test config invalid: %v", err)
	}
	b, err := json.MarshalIndent(&c, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	h.app.ConfigPath = p
	return p
}

func TestAllowlistEditAndDetection(t *testing.T) {
	h := start(t)
	p := writeConfig(t, h)
	c := withControl(t, h)
	ctx := context.Background()
	// Detection is off until armed: a stranger's message is not recorded.
	h.tg.push(msg(member, member, "private", "hi"))
	time.Sleep(400 * time.Millisecond)
	var dr control.DetectResult
	_ = c.Call(ctx, "telegram.detect.result", nil, &dr)
	if len(dr.Senders) != 0 {
		t.Fatal("recorded a sender while detection was off")
	}
	if err := c.Call(ctx, "telegram.detect.start", control.DetectParams{Seconds: 30}, &dr); err != nil || !dr.Armed {
		t.Fatalf("arm: %v", err)
	}
	before := len(h.tg.sent())
	h.tg.push(msg(member, member, "private", "it is me"))
	for i := 0; i < 100 && len(dr.Senders) == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		_ = c.Call(ctx, "telegram.detect.result", nil, &dr)
	}
	if len(dr.Senders) != 1 || dr.Senders[0].ID != member {
		t.Fatalf("detect: %+v", dr)
	}
	for _, x := range h.tg.sent()[before:] {
		if x.Method == "sendMessage" {
			t.Fatal("an unknown sender got a reply during detection")
		}
	}
	// Removing the owner, a duplicate, or an empty list is refused.
	for _, bad := range [][]control.AllowedUserInfo{
		{{ID: member, Role: "owner"}},
		{{ID: owner, Role: "owner"}, {ID: owner, Role: "user"}},
		{},
	} {
		if err := c.Call(ctx, "telegram.allowlist.set", control.AllowlistParams{Users: bad}, nil); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	var got []control.AllowedUserInfo
	if err := c.Call(ctx, "telegram.allowlist.set", control.AllowlistParams{Users: []control.AllowedUserInfo{{ID: owner, Role: "owner", Label: "me"}, {ID: member, Role: "user", Label: "friend"}}}, &got); err != nil || len(got) != 2 {
		t.Fatalf("set: %v %+v", err, got)
	}
	cfg, err := config.Parse(mustRead(t, p))
	if err != nil || len(cfg.Telegram.AllowedUsers) != 2 || cfg.Telegram.Role(member) != config.RoleUser {
		t.Fatalf("config.json not updated: %v %+v", err, cfg)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatal("config.json lost its private mode")
	}
	// The new user works immediately, without a restart.
	h.tg.push(msg(member, member, "private", "/status"))
	waitSentTo(t, h, member, "clawdyx")
	_ = c.Call(ctx, "telegram.detect.result", nil, &dr)
	if dr.Armed || len(dr.Senders) != 0 {
		t.Fatal("detection must be cleared after saving the list")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTokenTestAndReplace(t *testing.T) {
	h := start(t)
	writeConfig(t, h)
	c := withControl(t, h)
	ctx := context.Background()
	var ts control.TokenStatus
	if err := c.Call(ctx, "telegram.token.status", nil, &ts); err != nil || ts.Hint == "" || strings.Contains(ts.Hint, h.cfg.Token[:10]) {
		t.Fatalf("status %v %+v", err, ts)
	}
	if err := c.Call(ctx, "telegram.token.test", control.TokenParams{Token: "nope"}, nil); err == nil {
		t.Fatal("malformed token accepted")
	}
	newTok := "555555555:" + strings.Repeat("Z", 35)
	var tr control.TokenTestResult
	if err := c.Call(ctx, "telegram.token.test", control.TokenParams{Token: newTok}, &tr); err != nil || tr.BotUsername == "" {
		t.Fatalf("test: %v", err)
	}
	if err := c.Call(ctx, "telegram.token.set", control.TokenParams{Token: newTok}, &tr); err != nil {
		t.Fatal(err)
	}
	if got, _ := config.StoredToken(h.app.ConfigPath); got != newTok {
		t.Fatal("token not stored")
	}
	select {
	case <-h.app.RestartRequested:
	case <-time.After(2 * time.Second):
		t.Fatal("no restart requested after the token changed")
	}
	for _, x := range h.tg.sent() {
		if strings.Contains(x.Form["text"], newTok) {
			t.Fatal("token leaked into a message")
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
