package telegram

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/observability"
	"github.com/Publicthrone547/clawdyX/internal/platform"
)

// browserName: Linux uses the default browser (desktop.browser_app is a
// macOS application name).
func browserName(string) string { return "the default browser" }

// openEnv are the variables xdg-open needs to reach the desktop session.
var openEnv = []string{"HOME", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "XDG_CURRENT_DESKTOP", "XDG_DATA_DIRS"}

// openURL hands the (already validated http/https) URL to xdg-open, which
// starts the default browser; no shell is involved.
func openURL(ctx context.Context, _, u string) error {
	if platform.Detect().Headless {
		return errors.New("there is no graphical session on this computer")
	}
	bin, err := exec.LookPath("xdg-open")
	if err != nil {
		return errors.New("xdg-open is not installed (package xdg-utils)")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, u)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin"}
	for _, k := range openEnv {
		if v, ok := os.LookupEnv(k); ok {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
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
