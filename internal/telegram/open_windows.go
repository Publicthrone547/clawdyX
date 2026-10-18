package telegram

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Platform wording for replies.
const (
	thisComputer    = "this PC"
	startAgainHint  = "Start it again on the PC with: clawdyx service start (or from the clawdyX app)."
	stopServiceHint = "Stop the service on the PC: clawdyx service stop"
	startNeedsHint  = "You will need the PC (clawdyx service start or the clawdyX app) to start it again."
)

// browserName: Windows always uses the default browser (desktop.browser_app
// is a macOS application name).
func browserName(string) string { return "the default browser" }

// openURL hands the (already validated http/https) URL to the default
// browser through the URL protocol handler; no shell is involved.
func openURL(ctx context.Context, _, u string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rundll := filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe")
	cmd := exec.CommandContext(ctx, rundll, "url.dll,FileProtocolHandler", u)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	return cmd.Run()
}
