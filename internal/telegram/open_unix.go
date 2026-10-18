//go:build !windows

package telegram

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/observability"
)

// Platform wording for replies.
const (
	thisComputer    = "the Mac"
	startAgainHint  = "launchd will not restart a clean exit; start it again with: launchctl kickstart gui/$(id -u)/local.clawdyx"
	stopServiceHint = "Stop the service from macOS: launchctl bootout gui/$(id -u)/local.clawdyx"
	startNeedsHint  = "You will need macOS (launchctl) to start it again."
)

func browserName(app string) string { return app }

// openURL launches the URL with /usr/bin/open (argument array, no shell).
func openURL(ctx context.Context, app, u string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/open", "-a", app, "--", u)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(observability.Truncate(msg, 200))
	}
	return nil
}
