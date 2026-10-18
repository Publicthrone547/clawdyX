package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Publicthrone547/clawdyX/internal/agent"
)

// Pid files in the state folder: the worker's (`clawdyx run`) and the
// supervisor's (`clawdyx service supervise`, Windows and the Linux
// fallback). `clawdyx service status|stop` and the desktop app read them.
const (
	workerPIDFile     = "worker.pid"
	supervisorPIDFile = "supervisor.pid"
)

// writePIDFile records this process and returns the cleanup.
func writePIDFile(stateDir, name string) func() {
	p := filepath.Join(stateDir, name)
	_ = os.WriteFile(p, []byte(strconv.Itoa(os.Getpid())), 0o600)
	return func() { _ = os.Remove(p) }
}

func readPIDFile(stateDir, name string) int {
	b, err := os.ReadFile(filepath.Join(stateDir, name))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid
}

// writePID records the running worker for `clawdyx service status|stop`.
func writePID(stateDir string) func() { return writePIDFile(stateDir, workerPIDFile) }

func readPID(stateDir string) int { return readPIDFile(stateDir, workerPIDFile) }

// clawdyxAlive reports whether pid (from a pid file) is still a clawdyx
// process; a reused pid of another program does not count.
func clawdyxAlive(pid int) bool {
	return pid > 0 && agent.ProcessStartUnix(pid) != 0 && agent.ProcessComm(pid) == "clawdyx"
}
