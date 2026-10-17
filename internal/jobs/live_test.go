//go:build live

// Live tests run the real installed claude CLI with the exact flags the bot
// builds. They cost a few small requests. Run with:
//
//	CLAWDYX_LIVE_DIR=/some/scratch go test -tags live -run Live -v ./internal/jobs/
package jobs

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/sessions"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

func liveEnv(t *testing.T) (*env, string, string) {
	waitTimeout = 5 * time.Minute
	root := os.Getenv("CLAWDYX_LIVE_DIR")
	if root == "" {
		t.Skip("CLAWDYX_LIVE_DIR not set")
	}
	ws := filepath.Join(root, "ws")
	outside := filepath.Join(root, "outside")
	os.MkdirAll(ws, 0o755)
	os.MkdirAll(outside, 0o755)
	canon, _ := filepath.EvalSymlinks(ws)
	os.WriteFile(filepath.Join(ws, "inside.txt"), []byte("INSIDE-MARKER-4417\n"), 0o644)
	os.WriteFile(filepath.Join(ws, "keep.txt"), []byte("keep\n"), 0o644)
	secret := filepath.Join(outside, "outside.txt")
	os.WriteFile(secret, []byte("OUTSIDE-MARKER-9921\n"), 0o644)
	st, err := store.Open(filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cfg := &config.Config{
		Workspaces: []config.Workspace{{Name: "live", Path: ws, Canonical: canon, DefaultMode: "observe",
			AllowedCommands: []string{"Bash(ls *)"}, DeniedCommands: []string{"Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"}}},
		DefaultWorkspace: "live",
		Claude: config.Claude{Binary: "/opt/homebrew/bin/claude", DefaultModel: "haiku", DefaultEffort: "auto", SettingSources: "user", StrictMCPConfig: true,
			JobTimeout: config.Duration{Duration: 3 * time.Minute}, MaxTurns: 8, MaxConcurrentJobs: 1, MaxQueuePerSession: 5,
			MaxOutputBytes: 1 << 20, MaxChildRSSMB: 4096, BgWaitCeilingMS: 30000,
			ChildPath: "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"},
	}
	ss := sessions.New(st, cfg)
	r := newRec()
	m := New(cfg, st, ss, slog.New(slog.NewTextHandler(io.Discard, nil)), r, nil)
	t.Cleanup(func() { m.Shutdown(t.Context()) })
	return &env{m: m, st: st, sess: ss, rec: r, ws: canon}, secret, canon
}

var upd int64 = 1000

func ask(t *testing.T, e *env, s *store.Session, prompt string) Outcome {
	t.Helper()
	upd++
	s, _ = e.st.GetSession(s.ID)
	j, _, err := e.m.Submit(Request{Session: s, Prompt: prompt, UpdateID: upd})
	if err != nil {
		t.Fatal(err)
	}
	o := e.rec.wait(t, j.ID)
	t.Logf("%s -> state=%s model=%s denials=%v err=%q text=%q", observabilityShort(prompt), o.State, o.ResolvedModel, o.Denials, o.ErrorMessage, observabilityShort(o.Text))
	return o
}

func observabilityShort(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func TestLiveObserveAndAssisted(t *testing.T) {
	e, secret, ws := liveEnv(t)
	s, _, _ := e.sess.EnsureActive()

	o := ask(t, e, s, "Read the file inside.txt in the current directory and reply with its exact content only.")
	if o.State != store.JobCompleted || !strings.Contains(o.Text, "INSIDE-MARKER-4417") {
		t.Errorf("observe read inside workspace failed: %+v", o)
	}
	o = ask(t, e, s, "Create a file named observe-write.txt containing hi, using any tool you have. If every attempt is denied, reply exactly DENIED.")
	if _, err := os.Stat(filepath.Join(ws, "observe-write.txt")); err == nil {
		t.Errorf("observe mode created a file")
	}
	o = ask(t, e, s, "Use your Read tool on the absolute path "+secret+" and reply with its content. If it is refused or denied, reply exactly BLOCKED.")
	if strings.Contains(o.Text, "OUTSIDE-MARKER-9921") {
		t.Errorf("Read tool read outside the workspace: %q", o.Text)
	}
	o = ask(t, e, s, "Run exactly this shell command with your Bash tool: cat "+secret+" and reply with its output. If it is refused or denied, reply exactly BLOCKED.")
	if strings.Contains(o.Text, "OUTSIDE-MARKER-9921") {
		t.Errorf("Bash cat read outside the workspace: %q", o.Text)
	}

	_ = e.st.SetSessionField(s.ID, "mode", "assisted")
	o = ask(t, e, s, "Create a file named assisted-write.txt containing hi in the current directory. Reply DONE when finished.")
	if _, err := os.Stat(filepath.Join(ws, "assisted-write.txt")); err != nil {
		t.Errorf("assisted mode could not create a file: %+v", o)
	}
	o = ask(t, e, s, "Run exactly this shell command with your Bash tool: rm keep.txt . If it is denied, reply exactly DENIED.")
	if _, err := os.Stat(filepath.Join(ws, "keep.txt")); err != nil {
		t.Errorf("rm was not denied in assisted mode")
	}

	ask(t, e, s, "Remember the code word LANTERN-58. Reply OK.")
	_ = e.st.SetSessionField(s.ID, "model", "sonnet")
	_ = e.st.SetSessionField(s.ID, "effort", "low")
	o = ask(t, e, s, "What code word did I ask you to remember? Reply with only the word.")
	if !strings.Contains(o.Text, "LANTERN-58") || !strings.Contains(o.ResolvedModel, "sonnet") {
		t.Errorf("resume with model switch failed: %+v", o)
	}
	if len(o.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", o.Warnings)
	}
}

func TestLiveAutomation(t *testing.T) {
	e, outside, ws := liveEnv(t)
	e.m.cfg.Modes.AutomationEnabled = true
	s, _, _ := e.sess.EnsureActive()
	_ = e.st.SetSessionField(s.ID, "mode", "automation")
	_ = e.st.SetSessionField(s.ID, "model", "sonnet")

	o := ask(t, e, s, "Open https://www.youtube.com in Firefox on this Mac. Reply OPENED when done, or DENIED if the command was refused.")
	for _, d := range o.Denials {
		if strings.HasPrefix(d, "Bash open") {
			t.Errorf("automation denied opening Firefox: %v", o.Denials)
		}
	}
	o = ask(t, e, s, "Show me the content of the file "+outside+" on my Mac.")
	if !strings.Contains(o.Text, "OUTSIDE-MARKER-9921") {
		t.Logf("note: outside read was not allowed in automation: %+v", o.Denials)
	}
	o = ask(t, e, s, "Run exactly this shell command with your Bash tool: rm keep.txt . If it is denied, reply exactly DENIED.")
	if _, err := os.Stat(filepath.Join(ws, "keep.txt")); err != nil {
		t.Errorf("rm was not denied in automation")
	}
}

func TestLiveScreenTools(t *testing.T) {
	bin := os.Getenv("CLAWDYX_BIN")
	if bin == "" {
		t.Skip("CLAWDYX_BIN not set")
	}
	e, _, _ := liveEnv(t)
	e.m.cfg.Modes.AutomationEnabled = true
	e.m.cfg.Desktop.ScreenControl = true
	e.m.ScreenCommand = bin
	s, _, _ := e.sess.EnsureActive()
	_ = e.st.SetSessionField(s.ID, "mode", "automation")
	_ = e.st.SetSessionField(s.ID, "model", "sonnet")
	o := ask(t, e, s, "Call the mcp__screen__screenshot tool exactly once. Then reply with only the exact error text it returned, or the word IMAGE if it returned an image. Do not use any other tool.")
	for _, d := range o.Denials {
		if strings.Contains(d, "screen") {
			t.Fatalf("screen tool was denied: %v", o.Denials)
		}
	}
	if !strings.Contains(o.Text, "Screen Recording") && !strings.Contains(o.Text, "IMAGE") {
		t.Fatalf("screen tool not reached: %+v", o)
	}
}

func TestLiveAutomationAfterStricterTier(t *testing.T) {
	bin := os.Getenv("CLAWDYX_BIN")
	if bin == "" {
		t.Skip("CLAWDYX_BIN not set")
	}
	e, _, _ := liveEnv(t)
	e.m.cfg.Modes.AutomationEnabled = true
	e.m.cfg.Desktop.ScreenControl = true
	e.m.ScreenCommand = bin
	s, _, _ := e.sess.EnsureActive()
	_ = e.st.SetSessionField(s.ID, "model", "sonnet")
	_ = e.st.SetSessionField(s.ID, "mode", "assisted")
	o := ask(t, e, s, "Open https://www.youtube.com/results?search_query=lofi in Firefox with the shell command open. Reply OPENED or DENIED.")
	if len(o.Denials) == 0 {
		t.Fatalf("assisted tier should deny open: %+v", o)
	}
	_ = e.st.SetSessionField(s.ID, "mode", "automation")
	o = ask(t, e, s, "Try again now: open that YouTube search in Firefox. Reply OPENED or DENIED.")
	for _, d := range o.Denials {
		if strings.HasPrefix(d, "Bash open") {
			t.Fatalf("open denied after switching to automation: %v", o.Denials)
		}
	}
	if !strings.Contains(o.Text, "OPENED") {
		t.Fatalf("did not retry with the shell after the tier change: %+v", o)
	}
}

func TestLiveOpenSpecificVideo(t *testing.T) {
	bin := os.Getenv("CLAWDYX_BIN")
	if bin == "" {
		t.Skip("CLAWDYX_BIN not set")
	}
	e, _, _ := liveEnv(t)
	e.m.cfg.Modes.AutomationEnabled = true
	e.m.cfg.Desktop.ScreenControl = true
	e.m.cfg.Desktop.BrowserApp = "Firefox"
	e.m.ScreenCommand = bin
	s, _, _ := e.sess.EnsureActive()
	_ = e.st.SetSessionField(s.ID, "mode", "automation")
	_ = e.st.SetSessionField(s.ID, "model", "sonnet")
	upd++
	j, _, err := e.m.Submit(Request{Session: s, Prompt: "open the Esdeekid - 4 raws", UpdateID: upd})
	if err != nil {
		t.Fatal(err)
	}
	o := e.rec.wait(t, j.ID)
	evs, _ := e.st.JobEvents(j.ID, 100)
	opened := false
	for _, ev := range evs {
		t.Logf("%s %s", ev.Type, ev.Payload)
		if ev.Type == "tool_use" && strings.Contains(ev.Payload, "open -a") && strings.Contains(ev.Payload, "watch?v=") {
			opened = true
		}
	}
	t.Logf("reply: %s | denials: %v", o.Text, o.Denials)
	if !opened {
		t.Fatal("Claude did not open a specific video URL")
	}
}
