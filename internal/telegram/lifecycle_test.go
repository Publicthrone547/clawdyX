package telegram

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
)

var confirmRe = regexp.MustCompile(`/confirm ([A-Z0-9]{6})`)

func confirmToken(t *testing.T, c call) string {
	t.Helper()
	m := confirmRe.FindStringSubmatch(c.Form["text"])
	if m == nil {
		t.Fatalf("no confirmation token in %q", c.Form["text"])
	}
	return m[1]
}

func fakeTranscript(t *testing.T, cfgDir, ws, uuid string) string {
	t.Helper()
	slug, _ := sessions.ProjectSlug(ws)
	dir := filepath.Join(cfgDir, "projects", slug)
	if err := os.MkdirAll(filepath.Join(dir, uuid), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, uuid+".jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewfClearForgetRename(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "/new alpha"))
	h.tg.waitFor(t, "S1</b> “alpha” created")
	h.tg.push(msg(owner, owner, "private", "/newf beta"))
	h.tg.waitFor(t, "S1 forgotten from clawdyX; its Claude Code history is kept")
	h.tg.push(msg(owner, owner, "private", "/switch alpha"))
	h.tg.waitFor(t, "No such session")
	h.tg.push(msg(owner, owner, "private", "/sessions restore S1"))
	h.tg.waitFor(t, "S1 restored")
	h.tg.push(msg(owner, owner, "private", "/rename S2 gamma"))
	h.tg.waitFor(t, "S2 is now called “gamma”")
	h.tg.push(msg(owner, owner, "private", "/mode assisted"))
	h.tg.waitFor(t, "tier set to <b>assisted</b>")
	h.tg.push(msg(owner, owner, "private", "/clear"))
	h.tg.waitFor(t, "Fresh context: <b>S3</b>")
	s3, _ := h.st.GetSession(3)
	if s3.Mode != "assisted" || s3.Name != "gamma-fresh" {
		t.Fatalf("fresh session %+v", s3)
	}
	// /forget asks first and only acts on the exact confirmation.
	h.tg.push(msg(owner, owner, "private", "/forget S2"))
	c := h.tg.waitFor(t, "Forget S2 from clawdyX?")
	if x, _ := h.st.GetSession(2); x.Status != "active" {
		t.Fatal("forgot before confirmation")
	}
	h.tg.push(msg(owner, owner, "private", "/confirm "+confirmToken(t, c)))
	h.tg.waitFor(t, "S2 forgotten from clawdyX. Its Claude Code history is kept")
	if x, _ := h.st.GetSession(2); x.Status != "forgotten" {
		t.Fatal("not forgotten")
	}
}

func TestForgetDeleteRemovesOnlyThatTranscript(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	h := start(t)
	h.tg.push(msg(owner, owner, "private", "/new keep"))
	h.tg.waitFor(t, "S1</b> “keep” created")
	h.tg.push(msg(owner, owner, "private", "/new drop"))
	h.tg.waitFor(t, "S2</b> “drop” created")
	s1, _ := h.st.GetSession(1)
	s2, _ := h.st.GetSession(2)
	keep := fakeTranscript(t, cfgDir, h.ws, s1.ClaudeSessionID)
	drop := fakeTranscript(t, cfgDir, h.ws, s2.ClaudeSessionID)
	h.tg.push(msg(owner, owner, "private", "/forget S2 --delete"))
	c := h.tg.waitFor(t, "permanently delete its Claude Code history")
	if !strings.Contains(c.Form["text"], s2.ClaudeSessionID) {
		t.Fatal("confirmation must name the exact files")
	}
	h.tg.push(msg(owner, owner, "private", "/confirm "+confirmToken(t, c)))
	h.tg.waitFor(t, "history deleted (2 item(s))")
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatal("transcript not deleted")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("another session's transcript was deleted")
	}
	h.tg.push(msg(owner, owner, "private", "/sessions restore S2"))
	h.tg.waitFor(t, "cannot be restored")
	// /newf --delete without a transcript refuses and changes nothing.
	h.tg.push(msg(owner, owner, "private", "/switch S1"))
	h.tg.waitFor(t, "Switched")
	os.Remove(keep)
	h.tg.push(msg(owner, owner, "private", "/newf --delete"))
	h.tg.waitFor(t, "cannot be deleted")
	if a, _ := h.app.sess.Active(); a == nil || a.ID != 1 {
		t.Fatal("refused /newf --delete changed the active session")
	}
}

func TestControlLifecycleMethods(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	h := start(t)
	c := withControl(t, h)
	ctx := context.Background()
	var r control.SessionResult
	if err := c.Call(ctx, "sessions.create", control.SessionCreateParams{Name: "one"}, &r); err != nil {
		t.Fatal(err)
	}
	s1, _ := h.st.GetSession(1)
	fakeTranscript(t, cfgDir, h.ws, s1.ClaudeSessionID)
	var hi control.HistoryInfo
	if err := c.Call(ctx, "sessions.history", control.SessionRefParams{Ref: "S1"}, &hi); err != nil || !hi.Deletable || len(hi.Paths) != 2 {
		t.Fatalf("history: %v %+v", err, hi)
	}
	if err := c.Call(ctx, "sessions.newForget", control.SessionNewForgetParams{Name: "two", DeleteHistory: true}, &r); err != nil {
		t.Fatal(err)
	}
	if r.Previous == nil || r.Previous.Label != "S1" || len(r.Deleted) != 2 || !strings.Contains(r.Message, "history deleted") {
		t.Fatalf("newForget %+v", r)
	}
	if err := c.Call(ctx, "sessions.fresh", nil, &r); err != nil || r.Session.Label != "S3" {
		t.Fatalf("fresh %v %+v", err, r)
	}
	var forgotten []control.SessionInfo
	_ = c.Call(ctx, "sessions.list", control.SessionsListParams{Forgotten: true}, &forgotten)
	if len(forgotten) != 1 || forgotten[0].DeletedAt == 0 {
		t.Fatalf("tombstones %+v", forgotten)
	}
	if err := c.Call(ctx, "sessions.rename", control.SessionRenameParams{Ref: "S2", Name: "bad name"}, nil); err == nil {
		t.Fatal("bad name accepted")
	}
	if err := c.Call(ctx, "sessions.forget", control.SessionForgetParams{Ref: "S2", DeleteHistory: true}, nil); err == nil {
		t.Fatal("delete without transcript must be refused")
	}
	if err := c.Call(ctx, "sessions.forget", control.SessionForgetParams{Ref: "S2"}, &r); err != nil || !strings.Contains(r.Message, "history is kept") {
		t.Fatalf("forget %v %+v", err, r)
	}
}
