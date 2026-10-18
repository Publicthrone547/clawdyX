package telegram

import (
	"context"

	"github.com/Publicthrone547/clawdyX/internal/control"
)

// platformChecks are the Diagnostics and /doctor checks of this OS.
func (a *App) platformChecks(context.Context) []control.DiagnosticCheck { return nil }
