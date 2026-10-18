//go:build !windows

package camera

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ErrUnsupported is only returned on platforms without the camera helper.
var ErrUnsupported = errors.New("camera capture is not supported on this platform")

const supported = true

func run(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=" + os.Getenv("HOME")}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
