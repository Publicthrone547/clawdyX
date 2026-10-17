package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

func withControl(t *testing.T, h *harness) *control.Client {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "cxa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	srv := control.NewServer(nil)
	h.app.RegisterControl(srv)
	p := filepath.Join(d, "control.sock")
	if err := srv.Listen(p); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Serve(ctx)
	t.Cleanup(func() { cancel(); srv.Close() })
	c, err := control.Dial(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func waitBot(t *testing.T, h *harness, want string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if h.app.BotState().State == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("bot state %q, want %q", h.app.BotState().State, want)
}

func TestControlStatusAndBotLifecycle(t *testing.T) {
	h := start(t)
	c := withControl(t, h)
	ctx := context.Background()
	waitBot(t, h, control.BotPolling)
	var st control.Status
	if err := c.Call(ctx, "status.get", nil, &st); err != nil {
		t.Fatal(err)
	}
	if st.Bot.State != control.BotPolling || st.Version == "" || st.Telegram.TokenHint == "" || st.Claude.DefaultModel != "fable" {
		t.Fatalf("status %+v", st)
	}
	if len(st.Telegram.TokenHint) > 8 {
		t.Fatal("token hint must be a short fingerprint")
	}
	if err := c.Subscribe(ctx); err != nil {
		t.Fatal(err)
	}
	var stop control.BotStopResult
	if err := c.Call(ctx, "bot.stop", control.BotStopParams{}, &stop); err != nil || stop.Bot.State != control.BotStopped {
		t.Fatalf("stop: %v %+v", err, stop)
	}
	if h.st.Setting(keyBotEnabled, "") != "false" {
		t.Fatal("stopped state must be persisted")
	}
	// While stopped, Telegram updates are not processed.
	before := len(h.tg.sent())
	h.tg.push(msg(owner, owner, "private", "/status"))
	time.Sleep(700 * time.Millisecond)
	for _, x := range h.tg.sent()[before:] {
		if x.Method == "sendMessage" {
			t.Fatal("stopped bot replied")
		}
	}
	starts := h.app.counters.PollStarts.Load()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var bs control.BotState
			if err := c.Call(ctx, "bot.start", nil, &bs); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := h.app.counters.PollStarts.Load() - starts; got != 1 {
		t.Fatalf("20 rapid starts created %d pollers, want 1", got)
	}
	// The queued /status is answered once polling is back.
	h.tg.waitFor(t, "clawdyx")
	// Events: stopping → stopped → starting → polling arrive in order.
	seen := map[string]bool{}
	deadline := time.After(3 * time.Second)
	for !(seen["stopped"] && seen["polling"]) {
		select {
		case ev := <-c.Events():
			if ev.Type == "bot.state" {
				var bs control.BotState
				_ = jsonUnmarshal(ev.Data, &bs)
				seen[bs.State] = true
			}
		case <-deadline:
			t.Fatalf("missing bot.state events: %v", seen)
		}
	}
}

func TestControlSessionsJobsAndSettings(t *testing.T) {
	h := start(t)
	c := withControl(t, h)
	ctx := context.Background()
	var r control.SessionResult
	if err := c.Call(ctx, "sessions.create", control.SessionCreateParams{Name: "alpha"}, &r); err != nil || r.Session == nil || !r.Session.Active {
		t.Fatalf("create: %v %+v", err, r)
	}
	if err := c.Call(ctx, "sessions.create", control.SessionCreateParams{Workspace: "nope"}, nil); err == nil {
		t.Fatal("unknown workspace accepted")
	}
	var list []control.SessionInfo
	if err := c.Call(ctx, "sessions.list", nil, &list); err != nil || len(list) != 1 || list[0].Name != "alpha" {
		t.Fatalf("list: %v %+v", err, list)
	}
	if err := c.Call(ctx, "sessions.archive", control.SessionRefParams{Ref: "S1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "sessions.switch", control.SessionRefParams{Ref: "S1"}, nil); err == nil {
		t.Fatal("switch to archived accepted")
	}
	if err := c.Call(ctx, "sessions.restore", control.SessionRefParams{Ref: "S1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "sessions.switch", control.SessionRefParams{Ref: "alpha"}, nil); err != nil {
		t.Fatal(err)
	}
	var e *control.Error
	if err := c.Call(ctx, "effort.set", control.SetEffortParams{Effort: "ultra"}, nil); !errors.As(err, &e) || e.Code != control.CodeInvalidParams {
		t.Fatalf("bad effort: %v", err)
	}
	if err := c.Call(ctx, "effort.set", control.SetEffortParams{Effort: "high"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call(ctx, "mode.set", control.SetModeParams{Mode: "automation"}, nil); err == nil {
		t.Fatal("automation must need the config flag")
	}
	if err := c.Call(ctx, "mode.set", control.SetModeParams{Mode: "assisted"}, nil); err != nil {
		t.Fatal(err)
	}
	var sr control.SettingResult
	if err := c.Call(ctx, "model.set", control.SetModelParams{Model: "sonnet"}, &sr); err != nil || sr.Canonical == "" {
		t.Fatalf("model: %v %+v", err, sr)
	}
	if err := c.Call(ctx, "model.set", control.SetModelParams{Model: "rm -rf"}, nil); err == nil {
		t.Fatal("garbage model accepted")
	}
	var st control.Status
	_ = c.Call(ctx, "status.get", nil, &st)
	if st.Active == nil || st.Active.Effort != "high" || st.Active.Mode != "assisted" || st.Active.Model != "sonnet" {
		t.Fatalf("active %+v", st.Active)
	}
	// A prompt through Telegram shows up in jobs.list and jobs.events.
	h.tg.push(msg(owner, owner, "private", "hello"))
	h.tg.waitFor(t, "Hello world")
	var jobs []control.JobInfo
	for i := 0; i < 50; i++ {
		_ = c.Call(ctx, "jobs.list", control.JobsListParams{Limit: 5}, &jobs)
		if len(jobs) == 1 && jobs[0].State == "completed" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(jobs) != 1 || jobs[0].State != "completed" {
		t.Fatalf("jobs %+v", jobs)
	}
	if err := c.Call(ctx, "jobs.cancel", control.JobRefParams{ID: jobs[0].ID}, nil); err == nil {
		t.Fatal("cancelling a finished job must fail")
	}
	var pi control.ProcessesInfo
	if err := c.Call(ctx, "processes.list", nil, &pi); err != nil || pi.WorkerRSSKB == 0 {
		t.Fatalf("processes: %v %+v", err, pi)
	}
	var lines []control.LogLine
	if err := c.Call(ctx, "log.tail", control.LogTailParams{Lines: 5}, &lines); err != nil {
		t.Fatal(err)
	}
	var mi control.ModelsInfo
	if err := c.Call(ctx, "claude.models", nil, &mi); err != nil || len(mi.Builtin) == 0 {
		t.Fatalf("models: %v", err)
	}
}

func TestTailLogRedactsAndFilters(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bot.log")
	content := `{"time":"t1","level":"INFO","msg":"hello","x":"1"}
not json
{"time":"t2","level":"WARN","msg":"token 123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA leaked"}
{"time":"t3","level":"DEBUG","msg":"noise"}
`
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailLog(p, control.LogTailParams{Lines: 10, Level: "info"})
	if err != nil || len(lines) != 2 {
		t.Fatalf("%v %+v", err, lines)
	}
	if lines[1].Msg == "" || containsToken(lines[1].Msg) {
		t.Fatalf("not redacted: %q", lines[1].Msg)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func containsToken(s string) bool { return strings.Contains(s, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA") }
