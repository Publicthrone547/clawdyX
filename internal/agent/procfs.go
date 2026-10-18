package agent

import (
	"strconv"
	"strings"
)

// ProcStat is the part of a Linux /proc/<pid>/stat line clawdyX uses.
type ProcStat struct {
	Comm       string // field 2, without the parentheses (at most 15 bytes)
	UTime      uint64 // field 14, clock ticks in user mode
	STime      uint64 // field 15, clock ticks in kernel mode
	StartTicks uint64 // field 22, clock ticks after boot
	RSSPages   int64  // field 24, resident pages
}

// ParseProcStat parses a /proc/<pid>/stat line. The command name may
// contain spaces and parentheses, so the fields are read after the last ')'.
func ParseProcStat(line string) (ProcStat, bool) {
	open, end := strings.IndexByte(line, '('), strings.LastIndexByte(line, ')')
	if open < 0 || end < open {
		return ProcStat{}, false
	}
	f := strings.Fields(line[end+1:]) // f[0] is field 3 (state)
	if len(f) < 22 {
		return ProcStat{}, false
	}
	num := func(i int) uint64 { v, _ := strconv.ParseUint(f[i-3], 10, 64); return v }
	rss, _ := strconv.ParseInt(f[24-3], 10, 64)
	return ProcStat{Comm: line[open+1 : end], UTime: num(14), STime: num(15), StartTicks: num(22), RSSPages: rss}, true
}

// ParseBootTime reads the "btime" line (seconds since the epoch) of /proc/stat.
func ParseBootTime(doc string) (int64, bool) {
	for _, l := range strings.Split(doc, "\n") {
		if v, ok := strings.CutPrefix(l, "btime "); ok {
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return n, err == nil
		}
	}
	return 0, false
}

// ClockTicks is USER_HZ, the unit of the /proc time fields. It is 100 on
// every mainstream Linux architecture; Go cannot call sysconf without cgo.
const ClockTicks = 100
