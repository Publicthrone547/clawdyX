package agent

import "golang.org/x/sys/unix"

// passEnv are the variables forwarded to claude children.
var passEnv = []string{"HOME", "USER", "LOGNAME", "TMPDIR"}

// platformEnv is added to every child environment.
var platformEnv = []string{"SHELL=/bin/zsh"}

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
