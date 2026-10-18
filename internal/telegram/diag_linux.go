package telegram

import (
	"context"
	"os/exec"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

// platformChecks are the Diagnostics and /doctor checks of this OS.
func (a *App) platformChecks(context.Context) []control.DiagnosticCheck {
	if !a.cfg.Desktop.Camera {
		return nil
	}
	return []control.DiagnosticCheck{ffmpegCheck()}
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
