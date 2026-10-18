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
