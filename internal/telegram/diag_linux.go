package telegram

import (
	"context"
	"os/exec"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// platformChecks are the Diagnostics and /doctor checks of this OS.
func (a *App) platformChecks(context.Context) []control.DiagnosticCheck {
	checks := []control.DiagnosticCheck{sessionCheck(platform.Detect())}
	if a.cfg.Desktop.Camera {
		checks = append(checks, ffmpegCheck())
	}
	return checks
}

// sessionCheck names the graphical session the worker sees, which decides
// the screen features (a server has none; that is fine).
func sessionCheck(in platform.Info) control.DiagnosticCheck {
	c := control.DiagnosticCheck{ID: "session", Label: "Graphical session", OK: true}
	switch in.Session {
	case platform.SessionX11:
		c.Detail = "X11: screenshots, mouse and keyboard"
	case platform.SessionWayland:
		c.Detail = "Wayland: screenshots only"
		c.Fix = "Mouse and keyboard tools need an X11 session (choose it on the login screen)."
	default:
		c.Detail = "none (a server): screen features are off, the camera still works"
	}
	if in.Desktop != "" {
		c.Detail += " · " + in.Desktop
	}
	return c
}

// ffmpegCheck: camera clips are recorded by ffmpeg; photos do not need it.
func ffmpegCheck() control.DiagnosticCheck {
	c := control.DiagnosticCheck{ID: "ffmpeg", Label: "ffmpeg (camera clips)"}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		c.OK, c.Detail = true, p
		return c
	}
	c.OK, c.Warn = true, true
	c.Detail = "not installed: photos work, clips do not"
	c.Fix = "Install ffmpeg with your package manager (for example sudo apt install ffmpeg)."
	return c
}
