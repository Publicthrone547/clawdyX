package telegram

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/jobs"
	"github.com/Publicthrone547/clawdyX/internal/screen"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

func displayInfo(d screen.Display) control.DisplayInfo {
	return control.DisplayInfo{Index: d.Index, Key: d.Key, Name: d.Name, PixelW: d.PixelW, PixelH: d.PixelH, PointW: d.W, PointH: d.H, Scale: d.Scale, Main: d.Main, Builtin: d.Builtin}
}

func captureInfo(c *screen.Capture) control.CaptureInfo {
	return control.CaptureInfo{ID: c.ID, Display: displayInfo(c.Display), TakenAt: c.TakenAt.UnixMilli(), Preview: c.Preview.JPEG,
		PreviewW: c.Preview.Width, PreviewH: c.Preview.Height, FullW: c.Preview.SrcW, FullH: c.Preview.SrcH, Bytes: len(c.PNG)}
}

func (a *App) registerScreenControl(srv *control.Server) {
	srv.Handle("screens.list", 15*time.Second, typed(func(ctx context.Context, _ none) (any, error) {
		out := control.ScreensInfo{Status: control.ScreenStatus(a.screenStatus()), Displays: []control.DisplayInfo{}, Access: "unknown",
			Held: a.broker.Held(), CapturesToday: a.st.CaptureCount(time.Now().Truncate(24 * time.Hour))}
		ds, err := a.broker.Displays(ctx)
		if err != nil {
			out.Error = err.Error()
		}
		for _, d := range ds {
			out.Displays = append(out.Displays, displayInfo(d))
		}
		out.Access, _ = a.broker.Access()
		return out, nil
	}))
	srv.HandleLarge("screens.capture", 90*time.Second, typed(func(ctx context.Context, p control.CaptureParams) (any, error) {
		caps, err := a.broker.Capture(ctx, p.Display, "preview", "app")
		if err != nil && len(caps) == 0 {
			return nil, control.Errorf("%s", err.Error())
		}
		res := control.CaptureResult{Captures: []control.CaptureInfo{}}
		for _, c := range caps {
			res.Captures = append(res.Captures, captureInfo(c))
		}
		if err != nil {
			res.Error = err.Error()
		}
		return res, nil
	}))
	srv.HandleLarge("screens.full", 30*time.Second, typed(func(_ context.Context, p control.CaptureRefParams) (any, error) {
		c, ok := a.broker.Get(p.ID)
		if !ok {
			return nil, control.Errorf("that capture is no longer held (captures are kept in memory for 5 minutes)")
		}
		img, err := png.Decode(bytes.NewReader(c.PNG))
		if err != nil {
			return nil, control.Errorf("could not decode the capture")
		}
		b := img.Bounds()
		enc, err := screen.EncodeJPEGSize(img, b.Dx(), b.Dy(), 88)
		if err != nil {
			return nil, control.Errorf("could not encode the capture")
		}
		return control.FullImage{JPEG: enc.JPEG, Width: enc.Width, Height: enc.Height}, nil
	}))
	srv.Handle("screens.discard", 5*time.Second, typed(func(_ context.Context, p control.CaptureRefParams) (any, error) {
		if p.ID == "" || p.ID == "all" {
			a.broker.Clear()
		} else {
			a.broker.Discard(p.ID)
		}
		return map[string]bool{"ok": true}, nil
	}))
	srv.Handle("screens.enabled.set", 5*time.Second, typed(func(_ context.Context, p control.EnabledParams) (any, error) {
		if p.Enabled && !a.cfg.Desktop.ScreenControl {
			return nil, control.Errorf("screen features are off in config.json (desktop.screen_control); enable them there first")
		}
		a.SetScreenEnabled(p.Enabled)
		return control.ScreenStatus(a.screenStatus()), nil
	}))
	srv.Handle("screens.authorize", 5*time.Second, typed(func(_ context.Context, p control.AuthorizeParams) (any, error) {
		if err := a.screenAllowed(); err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		a.st.AuditCapture(store.Capture{DisplayKey: "display:" + p.Display, Destination: "mcp", Requester: "claude"})
		a.emit("capture.state", map[string]any{"capturing": false, "display": "Claude screenshot tool", "source": "mcp"})
		return map[string]bool{"allowed": true}, nil
	}))
	srv.Handle("jobs.submit", 15*time.Second, typed(func(_ context.Context, p control.SubmitParams) (any, error) {
		return a.submitFromApp(p)
	}))
	srv.Handle("screens.use", 15*time.Second, typed(func(_ context.Context, p control.ScreensUseParams) (any, error) {
		if p.ID == "" {
			return nil, control.InvalidParams("id is required")
		}
		return a.submitFromApp(control.SubmitParams{Session: p.Session, Text: p.Text, CaptureID: p.ID})
	}))
}

// submitFromApp queues a prompt typed in the desktop app. Its progress and
// result go to the app only.
func (a *App) submitFromApp(p control.SubmitParams) (any, error) {
	text := strings.TrimSpace(p.Text)
	var images []agent.Image
	kind := "prompt"
	if p.CaptureID != "" {
		if err := a.screenAllowed(); err != nil {
			return nil, control.Errorf("%s", err.Error())
		}
		c, ok := a.broker.Get(p.CaptureID)
		if !ok {
			return nil, control.Errorf("that capture is no longer held (captures are kept in memory for 5 minutes); capture again")
		}
		if text == "" {
			text = DefaultLookPrompt
		}
		text = lookPrompt(c.Display, text)
		images = []agent.Image{{MediaType: "image/jpeg", Data: c.Preview.JPEG}}
		kind = "look"
		a.st.AuditCapture(store.Capture{DisplayKey: c.Display.Key, Width: c.Preview.SrcW, Height: c.Preview.SrcH, Bytes: len(c.Preview.JPEG), Destination: "claude", Requester: "app"})
	}
	if text == "" {
		return nil, control.InvalidParams("text is required")
	}
	if len(text) > MaxPromptBytes+1024 {
		return nil, control.InvalidParams("the prompt is longer than 32 KB")
	}
	s, err := a.targetSession(p.Session)
	if err != nil {
		return nil, err
	}
	j, pos, err := a.jobs.Submit(jobs.Request{Session: s, Prompt: text, Images: images, Kind: kind,
		Registered: func(j *store.Job) { a.quiet.add(j.ID) }})
	if err != nil {
		switch {
		case errors.Is(err, jobs.ErrQueueFull):
			return nil, control.Errorf("the queue for %s is full", s.Label())
		case errors.Is(err, jobs.ErrPaused):
			return nil, control.Errorf("the bot is paused; resume it first")
		case errors.Is(err, jobs.ErrShutdown):
			return nil, control.Errorf("the worker is shutting down")
		}
		return nil, control.Errorf("could not queue the prompt: %v", err)
	}
	return control.SubmitResult{Job: jobInfo(j, true), Position: pos}, nil
}
