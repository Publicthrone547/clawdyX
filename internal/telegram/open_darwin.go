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
