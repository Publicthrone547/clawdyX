package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/config"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

func photos(h *harness) []call {
	var out []call
	for _, c := range h.tg.sent() {
		if c.Method == "sendPhoto" {
			out = append(out, c)
		}
	}
	return out
}

func waitPhotos(t *testing.T, h *harness, n int) []call {
	t.Helper()
	for i := 0; i < 200; i++ {
		if p := photos(h); len(p) >= n {
			return p
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("wanted %d photos, got %d", n, len(photos(h)))
	return nil
}

func screenOn(c *config.Config) { c.Desktop.ScreenControl = true }

func TestScreenCommandsDisabledInConfig(t *testing.T) {
	h := start(t)
	for _, cmd := range []string{"/screenshot", "/screen 2", "/look", "/screens"} {
		h.tg.push(msg(owner, owner, "private", cmd))
	}
	h.tg.waitFor(t, "Screen features are off")
	time.Sleep(300 * time.Millisecond)
	if len(photos(h)) != 0 {
		t.Fatal("captured while disabled in config")
	}
}

func TestScreensListCaptureAndSwitch(t *testing.T) {
	before := fakeCaptures.Load()
	h := startFull(t, screenOn, nil, fakeDesk{})
	time.Sleep(300 * time.Millisecond)
	if fakeCaptures.Load() != before {
		t.Fatal("something was captured at startup")
	}
	h.tg.push(msg(42, 42, "private", "/screen all")) // stranger
	h.tg.push(msg(owner, owner, "private", "/screens"))
	h.tg.waitFor(t, "2 · Side · 300×200")
	if fakeCaptures.Load() != before {
		t.Fatal("/screens must not capture")
	}
	h.tg.push(msg(owner, owner, "private", "/screen 2"))
	p := waitPhotos(t, h, 1)
	if !strings.Contains(p[0].Form["caption"], "Display 2 · Side") || p[0].Form["chat_id"] != fmt.Sprint(owner) {
		t.Fatalf("photo %v", p[0].Form)
	}
	if platform.W.ScreenNote != "" {
		h.tg.waitFor(t, Esc(platform.W.ScreenNote)[:40])
	}
	h.tg.push(msg(owner, owner, "private", "/screen all"))
	waitPhotos(t, h, 3)
	h.tg.push(msg(owner, owner, "private", "/screen 9"))
	h.tg.waitFor(t, "no such display")

	h.tg.push(msg(owner, owner, "private", "/screen off"))
	h.tg.waitFor(t, "switched off")
	n := fakeCaptures.Load()
	h.tg.push(msg(owner, owner, "private", "/screen on"))
	h.tg.waitFor(t, "only be switched on from")
	h.tg.push(msg(owner, owner, "private", "/look 1 what is this"))
	h.tg.waitFor(t, "screens.enabled.set")
	if fakeCaptures.Load() != n || h.app.broker.Held() != 0 {
		t.Fatal("captured while switched off")
	}
	// The local app switches it back on.
	c := withControl(t, h)
	var st control.ScreenStatus
	if err := c.Call(context.Background(), "screens.enabled.set", control.EnabledParams{Enabled: true}, &st); err != nil || !st.Enabled {
		t.Fatalf("enable: %v %+v", err, st)
	}
	var info control.ScreensInfo
	if err := c.Call(context.Background(), "screens.list", nil, &info); err != nil || len(info.Displays) != 2 || !info.Status.Enabled {
		t.Fatalf("list: %v %+v", err, info)
	}
}

func TestLookSendsImageToClaude(t *testing.T) {
	h := startFull(t, screenOn, nil, fakeDesk{})
	h.tg.push(msg(owner, owner, "private", "/look 2 what is on it?"))
	h.tg.waitFor(t, "Looking at display 2")
	h.tg.waitFor(t, "— j-1")
	b, _ := os.ReadFile(filepath.Join(h.ws, ".fakeclaude-args"))
	if !strings.Contains(string(b), `"--input-format","stream-json"`) {
		t.Fatalf("look job args %s", b)
	}
	j, _ := h.st.GetJob(1)
	if j.Kind != "look" {
		t.Fatalf("job kind %q", j.Kind)
	}
	if h.app.broker.Held() != 0 {
		t.Fatal("capture kept after /look")
	}
}

func TestLookAllSendsEveryDisplay(t *testing.T) {
	h := startFull(t, screenOn, nil, fakeDesk{})
	h.tg.push(msg(owner, owner, "private", "/look all which screen shows an error?"))
	h.tg.waitFor(t, "Looking at all 2 displays")
	h.tg.waitFor(t, "— j-1")
	b, _ := os.ReadFile(filepath.Join(h.ws, ".fakeclaude-stdin"))
	in := string(b)
	if n := strings.Count(in, `"type":"image"`); n != 2 {
		t.Fatalf("%d images sent, want 2", n)
	}
	for _, want := range []string{"screenshots of all displays", "1. display 1", "2. display 2 (Side, 300×200)", "which screen shows an error?"} {
		if !strings.Contains(in, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if h.app.broker.Held() != 0 {
		t.Fatal("captures kept after /look all")
	}
}

func TestAppCaptureUseIsQuietOnTelegram(t *testing.T) {
	h := startFull(t, screenOn, nil, fakeDesk{})
	c := withControl(t, h)
	ctx := context.Background()
	var res control.CaptureResult
	if err := c.Call(ctx, "screens.capture", control.CaptureParams{Display: "all"}, &res); err != nil || len(res.Captures) != 2 || len(res.Captures[0].Preview) == 0 {
		t.Fatalf("capture: %v %+v", err, len(res.Captures))
	}
	var full control.FullImage
	if err := c.Call(ctx, "screens.full", control.CaptureRefParams{ID: res.Captures[1].ID}, &full); err != nil || full.Width != 300 {
		t.Fatalf("full: %v %d", err, full.Width)
	}
	if err := c.Subscribe(ctx); err != nil {
		t.Fatal(err)
	}
	before := len(h.tg.sent())
	var sub control.SubmitResult
	if err := c.Call(ctx, "screens.use", control.ScreensUseParams{ID: res.Captures[0].ID, Text: "describe"}, &sub); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(10 * time.Second)
	for done := false; !done; {
		select {
		case ev := <-c.Events():
			if ev.Type == "job.finished" && strings.Contains(string(ev.Data), "Hello world") {
				done = true
			}
		case <-deadline:
			t.Fatal("no job.finished event with the result")
		}
	}
	for _, x := range h.tg.sent()[before:] {
		if x.Method == "sendMessage" || x.Method == "editMessageText" || x.Method == "sendMessageDraft" {
			t.Fatalf("app job leaked to Telegram: %s %v", x.Method, x.Form["text"])
		}
	}
	if err := c.Call(ctx, "screens.use", control.ScreensUseParams{ID: "nope"}, nil); err == nil {
		t.Fatal("unknown capture accepted")
	}
	if err := c.Call(ctx, "screens.discard", control.CaptureRefParams{ID: "all"}, nil); err != nil || h.app.broker.Held() != 0 {
		t.Fatal("discard all failed")
	}
}
