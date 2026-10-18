//go:build !windows

package agent

import (
	"syscall"
	"time"
)

// groupAttr starts a child in its own process group.
func groupAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

type stopStep struct {
	sig   syscall.Signal
	grace time.Duration
}

func stopLadder(intGrace, termGrace time.Duration) []stopStep {
	return []stopStep{{syscall.SIGINT, intGrace}, {syscall.SIGTERM, termGrace}, {syscall.SIGKILL, 5 * time.Second}}
}

// signalGroup sends sig to the whole process group led by pid.
func signalGroup(pid int, sig syscall.Signal) error { return syscall.Kill(-pid, sig) }

// KillGroup ends the process group led by pid: SIGTERM, or SIGKILL when force.
func KillGroup(pid int, force bool) error {
	if force {
		return signalGroup(pid, syscall.SIGKILL)
	}
	return signalGroup(pid, syscall.SIGTERM)
}
