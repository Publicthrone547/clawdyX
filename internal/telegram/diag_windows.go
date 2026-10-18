package telegram

import (
	"context"
	"os"

	"github.com/Publicthrone547/clawdyX/internal/control"
	"github.com/Publicthrone547/clawdyX/internal/setup"
)

// platformChecks are the Diagnostics and /doctor checks of this OS.
func (a *App) platformChecks(context.Context) []control.DiagnosticCheck {
	return []control.DiagnosticCheck{gitBashCheck(a.cfg.Claude.GitBashPath)}
}

// gitBashCheck: Claude Code's shell tool needs Git for Windows; without it
// only the observe tier is useful.
func gitBashCheck(configured string) control.DiagnosticCheck {
	c := control.DiagnosticCheck{ID: "git-bash", Label: "Git for Windows (Claude Code's shell)"}
	p := configured
	if p == "" {
		p = setup.DetectGitBash()
	}
	switch {
	case p == "":
		c.Detail = "not found"
		c.Fix = "Install Git for Windows (winget install Git.Git), or set the location of its bash.exe in Settings → Claude Code, then restart the worker."
	case !fileExists(p):
		c.Detail = "set to " + p + ", which does not exist"
		c.Fix = "Fix the location of bash.exe in Settings → Claude Code."
	default:
		c.OK, c.Detail = true, p
	}
	return c
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
