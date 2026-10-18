package agent

import (
	"os"
	"strconv"
	"strings"
)

// passEnv are the variables forwarded to claude children.
var passEnv = []string{"HOME", "USER", "LOGNAME", "TMPDIR", "XDG_RUNTIME_DIR", "LANG", "LC_ALL"}

// platformEnv is added to every child environment.
var platformEnv = []string{"SHELL=/bin/bash"}

// procRoot is /proc (tests point it at fixtures).
var procRoot = "/proc"

func procStat(pid int) (ProcStat, bool) {
	b, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ProcStat{}, false
	}
	return ParseProcStat(string(b))
}

// ProcessStartUnix returns a process's start time (seconds), or 0: boot
// time plus the start ticks of /proc/<pid>/stat.
func ProcessStartUnix(pid int) int64 {
	st, ok := procStat(pid)
	if !ok {
		return 0
	}
	b, err := os.ReadFile(procRoot + "/stat")
	if err != nil {
		return 0
	}
	boot, ok := ParseBootTime(string(b))
	if !ok {
		return 0
	}
	return boot + int64(st.StartTicks/ClockTicks)
}

// ProcessComm returns the short command name of a pid (at most 15 bytes,
// so "claude" fits), or "".
func ProcessComm(pid int) string {
	b, err := os.ReadFile(procRoot + "/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
