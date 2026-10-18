//go:build !windows

package agent

import (
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// passEnv are the variables forwarded to claude children.
var passEnv = []string{"HOME", "USER", "LOGNAME", "TMPDIR"}

// platformEnv is added to every child environment.
var platformEnv = []string{"SHELL=/bin/zsh"}

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

// ProcessStartUnix returns a process's start time (seconds) via sysctl, or 0.
func ProcessStartUnix(pid int) int64 {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil || kp.Proc.P_pid != int32(pid) {
		return 0
	}
	return int64(kp.Proc.P_starttime.Sec)
}

// ProcessComm returns the short command name of a pid, or "".
func ProcessComm(pid int) string {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp == nil || kp.Proc.P_pid != int32(pid) {
		return ""
	}
	b := kp.Proc.P_comm[:]
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return string(b[:n])
}
