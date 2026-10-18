package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func runWithStdin(ctx context.Context, c CLI, stdin string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	cmd.Dir, cmd.Env = c.Dir, c.Env
	cmd.Stdin = strings.NewReader(stdin)
	cmd.SysProcAttr = groupAttr()
	cmd.Cancel = func() error { return KillGroup(cmd.Process.Pid, true) }
	cmd.WaitDelay = 2 * time.Second
	out, errb := &ringBuffer{n: 4 << 20}, &ringBuffer{n: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	err := cmd.Run()
	code := exitCode(cmd, err)
	if ctx.Err() != nil {
		return out.String(), errb.String(), code, fmt.Errorf("timed out")
	}
	if _, ok := err.(*exec.ExitError); ok {
		err = nil
	}
	return out.String(), errb.String(), code, err
}
