package processes

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// Sample returns ps stats for the given PIDs (others are ignored). It never
// requests process arguments or environments.
func Sample(ctx context.Context, pids []int) map[int]Stat {
	out := map[int]Stat{}
	if len(pids) == 0 {
		return out
	}
	ids := make([]string, len(pids))
	for i, p := range pids {
		ids[i] = strconv.Itoa(p)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/bin/ps", "-o", "pid=,rss=,pcpu=", "-p", strings.Join(ids, ",")).Output()
	if err != nil && len(b) == 0 {
		return out
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		rss, _ := strconv.ParseInt(f[1], 10, 64)
		cpu, _ := strconv.ParseFloat(f[2], 64)
		out[pid] = Stat{PID: pid, RSSKB: rss, CPU: cpu}
	}
	return out
}

// CountClaude returns the number of processes whose command name is "claude".
func CountClaude(ctx context.Context) int {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/bin/ps", "-axo", "comm=").Output()
	if err != nil {
		return -1
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "claude" || strings.HasSuffix(l, "/claude") {
			n++
		}
	}
	return n
}

// SystemSummary reads load average and memory from sysctl/vm_stat.
func SystemSummary(ctx context.Context) System {
	var s System
	if la, err := unix.SysctlRaw("vm.loadavg"); err == nil && len(la) >= 24 {
		// struct loadavg { fixpt_t ldavg[3]; long fscale; } with 32-bit fixpt_t on darwin.
		get := func(i int) uint32 {
			return uint32(la[i*4]) | uint32(la[i*4+1])<<8 | uint32(la[i*4+2])<<16 | uint32(la[i*4+3])<<24
		}
		scale := float64(uint64(la[16]) | uint64(la[17])<<8 | uint64(la[18])<<16 | uint64(la[19])<<24)
		if scale > 0 {
			s.Load1, s.Load5, s.Load15 = float64(get(0))/scale, float64(get(1))/scale, float64(get(2))/scale
		}
	}
	if total, err := unix.SysctlUint64("hw.memsize"); err == nil {
		s.MemTotal = total
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, "/usr/bin/vm_stat").Output()
	if err == nil {
		page := uint64(16384)
		var free, inactive uint64
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "page size of") {
				for _, f := range strings.Fields(l) {
					if v, err := strconv.ParseUint(f, 10, 64); err == nil {
						page = v
					}
				}
			}
			k, v, ok := strings.Cut(l, ":")
			if !ok {
				continue
			}
			n, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), "."), 10, 64)
			switch strings.TrimSpace(k) {
			case "Pages free":
				free = n
			case "Pages inactive":
				inactive = n
			}
		}
		s.MemFree = (free + inactive) * page
		s.OK, s.HasLoad = s.MemTotal > 0, true
	}
	return s
}

func cpuTimes(ctx context.Context, pids []int) map[int]float64 {
	args := []string{"-o", "pid=,time="}
	for _, p := range pids {
		args = append(args, "-p", strconv.Itoa(p))
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, _ := exec.CommandContext(ctx, "/bin/ps", args...).Output()
	out := map[int]float64{}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if s, ok := ParseCPUTime(f[1]); ok {
			out[pid] = s
		}
	}
	return out
}
