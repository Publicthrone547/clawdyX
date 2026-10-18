package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	"github.com/Publicthrone547/clawdyX/internal/camera"
	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/store"
)

// The camera follows the screen features' rules: owner only, private chat
// only, nothing captured at startup or on a timer, media in memory only
// (at most camera.HoldTTL), an audit row per capture, and /screen off
// (the shared runtime switch) turns it off. Claude never gets a camera tool.

// CameraOn reports whether camera captures may happen now.
func (a *App) CameraOn() bool { return a.cameraAllowed() == nil }

func (a *App) cameraAllowed() error {
	if !a.cfg.Desktop.Camera {
		return errors.New("the camera is off (clawdyX app → Settings → Features → Camera)")
	}
	if a.st.Setting(keyScreenEnabled, "true") == "false" {
		return camera.ErrOff
	}
	return nil
}

func (a *App) initCamera(p camera.Provider) {
	a.cams = camera.NewBroker(p, a.cameraAllowed, func(r camera.AuditRecord) {
		a.st.AuditCapture(store.Capture{Kind: r.Kind, DisplayKey: "camera:" + r.Camera.ID, Bytes: r.Bytes,
			DurationMS: r.Duration.Milliseconds(), Destination: r.Destination, Requester: r.Requester})
	})
	a.cams.OnChange = func(capturing bool, name string) {
		a.emit("camera.state", map[string]any{"capturing": capturing, "camera": name})
	}
}

func (a *App) cameraOffReply(m *models.Message, err error) {
	switch {
	case !a.cfg.Desktop.Camera:
		a.replyTo(m, "The camera is off. Turn it on in the clawdyX app on "+a.words.TheComputer+": Settings → Features → Camera, then restart the worker.")
	case errors.Is(err, camera.ErrOff):
		a.replyTo(m, "Screen and camera features are switched off (/screen off). Switch them on in the clawdyX app (Monitors) or with: clawdyx ctl screens.enabled.set '{\"enabled\":true}'")
	default:
		a.replyTo(m, "❌ "+Esc(err.Error()))
	}
}

// cmdCameras lists the cameras without capturing anything.
func (a *App) cmdCameras(m *models.Message) {
	if !a.cfg.Desktop.Camera {
		a.cameraOffReply(m, nil)
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	cs, err := a.cams.Cameras(ctx)
	if err != nil {
		a.replyTo(m, "❌ Could not list cameras: "+Esc(err.Error()))
		return
	}
	state := "on"
	if !a.CameraOn() {
		state = "switched off (/screen off); switch on in the app"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>Cameras</b> (%d) · camera %s\n", len(cs), state)
	for _, c := range cs {
		sb.WriteString(Esc(c.Label()) + "\n")
	}
	switch acc, _ := a.cams.Check(ctx); acc {
	case "granted":
		sb.WriteString("Camera permission: granted\n")
	case "denied":
		sb.WriteString("<b>" + Esc(a.words.CameraDeniedStatus) + "</b>\n")
	case "not asked yet":
		sb.WriteString(strings.TrimSpace("Camera permission: not asked yet. "+Esc(a.words.CameraAskNote)) + "\n")
	}
	fmt.Fprintf(&sb, "/camera [N] sends you a photo · /camera video SECONDS [N] a clip of up to %d s without sound · /screen off switches the camera off too", camera.MaxVideoSeconds)
	a.replyTo(m, sb.String())
}

// parseCameraArgs reads "/camera [N] [video SECONDS]" and
// "/camera video SECONDS [N]". secs is 0 for a photo.
func parseCameraArgs(args []string) (sel string, video bool, secs int, err error) {
	for i := 0; i < len(args); i++ {
		switch w := strings.ToLower(args[i]); {
		case w == "video" || w == "clip":
			if video {
				return "", false, 0, errors.New("say video once")
			}
			video = true
			if i+1 >= len(args) {
				return "", false, 0, errors.New("give the clip length in seconds, for example /camera video 10")
			}
			i++
			n, perr := strconv.Atoi(strings.TrimSuffix(strings.ToLower(args[i]), "s"))
			if perr != nil {
				return "", false, 0, fmt.Errorf("%q is not a number of seconds", args[i])
			}
			secs = n
		case isDisplayRef(w) && w != "main" && sel == "":
			sel = w
		default:
			return "", false, 0, fmt.Errorf("unknown argument %q. Use /camera [N] or /camera video SECONDS [N]", args[i])
		}
	}
	if video {
		if err := camera.CheckSeconds(secs); err != nil {
			return "", false, 0, err
		}
	}
	return sel, video, secs, nil
}

// cmdCamera: /camera [N], /camera video SECONDS [N].
func (a *App) cmdCamera(m *models.Message, args []string) {
	sel, video, secs, err := parseCameraArgs(args)
	if err != nil {
		a.replyTo(m, "Not recorded: "+Esc(err.Error())+".")
		return
	}
	if err := a.cameraAllowed(); err != nil {
		a.cameraOffReply(m, err)
		return
	}
	if video {
		a.replyTo(m, fmt.Sprintf("🎥 Recording %d s (no sound). The camera light is on while it records.", secs))
	}
	a.go_(func() {
		ctx, cancel := context.WithTimeout(a.ctx, time.Duration(secs)*time.Second+2*time.Minute)
		defer cancel()
		var md *camera.Media
		var err error
		if video {
			md, err = a.cams.Video(ctx, sel, secs, "telegram", "telegram-owner")
		} else {
			md, err = a.cams.Photo(ctx, sel, "telegram", "telegram-owner")
		}
		if err != nil {
			a.replyTo(m, "❌ Camera failed: "+Esc(err.Error()))
			return
		}
		defer a.cams.Discard(md.ID)
		caption := fmt.Sprintf("Camera %d · %s · %s · sent only to you", md.Camera.Index, md.Camera.Name, md.TakenAt.Format("15:04:05"))
		if video {
			caption = fmt.Sprintf("Camera %d · %s · %d s · %s · sent only to you", md.Camera.Index, md.Camera.Name, secs, md.TakenAt.Format("15:04:05"))
			err = a.send.Video(ctx, m.Chat.ID, md.Data, secs, caption, m.ID)
		} else {
			err = a.send.Photo(ctx, m.Chat.ID, md.Data, caption, m.ID)
		}
		if err != nil {
			a.replyTo(m, "❌ Could not send it to Telegram.")
		}
	})
}

// cameraCheck is the Diagnostics line for the camera. It lists the cameras
// and reads the permission status; it never captures.
func (a *App) cameraCheck(ctx context.Context) control.DiagnosticCheck {
	c := control.DiagnosticCheck{ID: "camera", Label: "Camera"}
	switch {
	case !camera.Supported:
		c.OK, c.Detail = true, "not available on "+a.words.ThisComputer+" in this version"
		return c
	case !a.cfg.Desktop.Camera:
		c.OK, c.Detail = true, "off (turn it on in Settings → Features)"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cs, err := a.cams.Cameras(ctx)
	if err != nil {
		c.Detail = observability.Truncate(observability.Redact(err.Error()), 160)
		if errors.Is(err, camera.ErrNoHelper) {
			c.Fix = "Run the worker from the clawdyX app: Settings → Background worker → Use the app's worker."
		}
		return c
	}
	if len(cs) == 0 {
		c.Detail, c.Fix = "no camera found", a.words.NoCameraFix
		return c
	}
	names := make([]string, 0, len(cs))
	for _, k := range cs {
		names = append(names, k.Name)
	}
	found := fmt.Sprintf("%d found (%s)", len(cs), strings.Join(names, ", "))
	c.FixKey = "camera"
	switch acc, _ := a.cams.Check(ctx); acc {
	case "granted":
		c.OK, c.Detail = true, "permission granted · "+found
	case "denied":
		c.Detail, c.Fix = "permission not granted · "+found, a.words.CameraDeniedFix
	case "not asked yet":
		c.Warn, c.Detail = true, "permission not asked yet · "+found
		c.Fix = a.words.CameraAskNote
	default:
		c.Warn, c.Detail = true, "permission unknown until the first /camera · "+found
	}
	if !a.CameraOn() {
		c.Detail += " · switched off (/screen off)"
	}
	return c
}
