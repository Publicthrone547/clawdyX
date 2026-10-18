package processes

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
)

// procRoot is /proc (tests point it at fixtures).
var procRoot = "/proc"

func readProc(parts ...string) (string, bool) {
	b, err := os.ReadFile(procRoot + "/" + strings.Join(parts, "/"))
	return string(b), err == nil
}

func bootTime() int64 {
	doc, ok := readProc("stat")
	if !ok {
		return 0
	}
	b, _ := agent.ParseBootTime(doc)
	return b
}

// Sample returns resident size and CPU use (average over the process's
// lifetime, like ps on Linux) for the given PIDs from /proc.
func Sample(_ context.Context, pids []int) map[int]Stat {
	out := map[int]Stat{}
	boot, now, page := bootTime(), time.Now().Unix(), int64(os.Getpagesize())
	for _, pid := range pids {
		line, ok := readProc(strconv.Itoa(pid), "stat")
		if !ok {
			continue
		}
		st, ok := agent.ParseProcStat(line)
		if !ok {
			continue
		}
		s := Stat{PID: pid, RSSKB: st.RSSPages * page / 1024}
		if boot > 0 {
			age := time.Duration(now-(boot+int64(st.StartTicks/agent.ClockTicks))) * time.Second
			s.CPU = LifetimeCPU(float64(st.UTime+st.STime)/agent.ClockTicks, age)
		}
		out[pid] = s
	}
	return out
}

// CountClaude returns the number of processes whose command name is "claude".
func CountClaude(context.Context) int {
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return -1
	}
	n := 0
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if c, ok := readProc(e.Name(), "comm"); ok && strings.TrimSpace(c) == "claude" {
			n++
		}
	}
	return n
}

// SystemSummary reads the load average and memory from /proc.
func SystemSummary(context.Context) System {
	var s System
	if doc, ok := readProc("loadavg"); ok {
		s.Load1, s.Load5, s.Load15, s.HasLoad = ParseLoadAvg(doc)
	}
	if doc, ok := readProc("meminfo"); ok {
		s.MemTotal, s.MemFree = ParseMemInfo(doc)
	}
	s.OK = s.MemTotal > 0
	return s
}

func cpuTimes(_ context.Context, pids []int) map[int]float64 {
	out := map[int]float64{}
	for _, pid := range pids {
		if line, ok := readProc(strconv.Itoa(pid), "stat"); ok {
			if st, ok := agent.ParseProcStat(line); ok {
				out[pid] = float64(st.UTime+st.STime) / agent.ClockTicks
			}
		}
	}
	return out
}
