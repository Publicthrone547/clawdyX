// Package processes provides read-only process and system visibility:
// per-PID resource sampling, a system summary, and a cached view of Claude
// Code sessions from `claude agents --json`.
package processes

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Publicthrone547/clawdyX/internal/agent"
)

// Stat is a sample of one process.
type Stat struct {
	PID   int
	RSSKB int64
	CPU   float64 // average CPU % (ps %cpu on macOS, lifetime average on Linux and Windows)
}

// RSSKB returns one process's resident size in KiB, or 0.
func RSSKB(pid int) int64 { return Sample(context.Background(), []int{pid})[pid].RSSKB }

// System is a cheap machine summary.
type System struct {
	Load1, Load5, Load15 float64
	HasLoad              bool // the OS has a load average (Windows has none)
	MemTotal             uint64
	MemFree              uint64 // free or reclaimable memory
	OK                   bool
}

// AgentsCache keeps the last `claude agents --json` result, refreshed in the
// background because one call takes 10-15 s.
type AgentsCache struct {
	mu      sync.Mutex
	cli     agent.CLI
	entries []agent.AgentEntry
	at      time.Time
	err     error
	busy    bool
}

// NewAgentsCache returns a cache.
func NewAgentsCache(cli agent.CLI) *AgentsCache { return &AgentsCache{cli: cli} }

// Get returns the cached entries, their age, and the last error.
func (a *AgentsCache) Get() ([]agent.AgentEntry, time.Time, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]agent.AgentEntry(nil), a.entries...), a.at, a.err
}

// Refresh runs the CLI once (skips if a refresh is already running).
func (a *AgentsCache) Refresh(ctx context.Context) {
	a.mu.Lock()
	if a.busy {
		a.mu.Unlock()
		return
	}
	a.busy = true
	a.mu.Unlock()
	e, err := a.cli.Agents(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.busy = false
	if err != nil {
		a.err = err
		return
	}
	a.entries, a.at, a.err = e, time.Now(), nil
}

// Run refreshes periodically until ctx ends. skip reports whether to skip a
// round (e.g. when the job slots are all busy).
func (a *AgentsCache) Run(ctx context.Context, every time.Duration, skip func() bool) {
	a.Refresh(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if skip == nil || !skip() {
				a.Refresh(ctx)
			}
		}
	}
}

// CPUPercent measures each pid's CPU use over interval from two samples of
// its accumulated CPU time (ps's %cpu is a decaying minute-long average and
// is useless for short windows). 100 means one full core.
func CPUPercent(ctx context.Context, pids []int, interval time.Duration) map[int]float64 {
	out := map[int]float64{}
	if len(pids) == 0 {
		return out
	}
	a := cpuTimes(ctx, pids)
	t0 := time.Now()
	select {
	case <-ctx.Done():
		return out
	case <-time.After(interval):
	}
	b := cpuTimes(ctx, pids)
	wall := time.Since(t0).Seconds()
	for pid, v1 := range b {
		if v0, ok := a[pid]; ok && wall > 0 && v1 >= v0 {
			out[pid] = (v1 - v0) / wall * 100
		}
	}
	return out
}

// ParseCPUTime parses ps time output ([[dd-]hh:]mm:ss[.ss]) into seconds.
func ParseCPUTime(s string) (float64, bool) {
	days := 0.0
	if d, rest, ok := strings.Cut(s, "-"); ok {
		v, err := strconv.ParseFloat(d, 64)
		if err != nil {
			return 0, false
		}
		days, s = v, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 0.0
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		total = total*60 + v
	}
	return total + days*86400, true
}
