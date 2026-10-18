//go:build !darwin

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// The supervisor (`clawdyx service supervise`) keeps the worker running
// where no service manager does: on Windows, started at login through
// clawdyxw.exe, and on Linux without a systemd user manager. It runs
// `clawdyx run` and starts it again after a crash, like launchd's KeepAlive
// and systemd's Restart=on-failure: a clean exit (0) ends supervision, a
// restart request (exitRestart) starts the worker again at once, any other
// exit after a delay that grows from 10 s to 5 min.

const (
	supervisedEnv  = "CLAWDYX_SUPERVISED" // set for the worker; it then never relaunches itself
	supervisorLock = "supervisor.lock"
	stopFileName   = "supervisor.stop" // written by `clawdyx service stop`
	firstDelay     = 10 * time.Second
	maxDelay       = 5 * time.Minute
	stableUptime   = 10 * time.Minute
)

// nextDelay is the wait before restarting a worker that exited with code
// after running for uptime; prev is the previous wait.
func nextDelay(prev time.Duration, code int, uptime time.Duration) time.Duration {
	switch {
	case code == exitRestart:
		return 0
	case uptime >= stableUptime || prev == 0:
		return firstDelay
	case prev*2 > maxDelay:
		return maxDelay
	}
	return prev * 2
}

// superviseLoop is the restart policy, separated from processes for tests.
type superviseLoop struct {
	start   func() (wait func() int, err error) // starts the worker
	stopped func() bool                         // the stop file exists
	sleep   func(time.Duration) bool            // false when stopped meanwhile
	now     func() time.Time
	logf    func(format string, a ...any)
}

func (s superviseLoop) run() int {
	var delay time.Duration
	for !s.stopped() {
		wait, err := s.start()
		if err != nil {
			delay = nextDelay(delay, exitError, 0)
			s.logf("could not start the worker: %v; trying again in %s", err, delay)
			if !s.sleep(delay) {
				break
			}
			continue
		}
		t0 := s.now()
		code := wait()
		if s.stopped() {
			break
		}
		if code == exitOK {
			s.logf("the worker stopped cleanly; supervision ends")
			return exitOK
		}
		delay = nextDelay(delay, code, s.now().Sub(t0))
		s.logf("the worker exited with code %d; starting it again in %s", code, delay)
		if delay > 0 && !s.sleep(delay) {
			break
		}
	}
	s.logf("stop requested; supervision ends")
	return exitOK
}

// supervise is `clawdyx service supervise`: one supervisor per state
// folder (a lock), a pid file for `service status`, a log of its own.
func supervise(cfgPath string) int {
	abs, _ := filepath.Abs(cfgPath)
	stateDir, logDir := config.Folders(abs)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return exitError
	}
	_ = os.MkdirAll(logDir, 0o700)
	var logw io.Writer = io.Discard
	if f, err := os.OpenFile(filepath.Join(logDir, "supervisor.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer f.Close()
		if st, err := f.Stat(); err == nil && st.Size() > 1<<20 {
			_ = f.Truncate(0)
		}
		logw = f
	}
	logf := func(format string, a ...any) {
		fmt.Fprintf(logw, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, a...))
	}
	lock := flock.New(filepath.Join(stateDir, supervisorLock))
	if ok, err := lock.TryLock(); err != nil || !ok {
		return exitOK // another supervisor already looks after this worker
	}
	defer lock.Unlock()
	defer writePIDFile(stateDir, supervisorPIDFile)()
	stopFile := filepath.Join(stateDir, stopFileName)
	_ = os.Remove(stopFile) // left over from an interrupted stop
	exe, err := os.Executable()
	if err != nil {
		logf("cannot resolve the executable: %v", err)
		return exitError
	}
	logf("supervising %s run -config %s", exe, abs)
	stopped := func() bool { _, err := os.Stat(stopFile); return err == nil }
	code := superviseLoop{
		start: func() (func() int, error) {
			cmd := exec.Command(exe, "run", "-config", abs)
			cmd.Env = append(os.Environ(), supervisedEnv+"=1")
			cmd.Dir = filepath.Dir(exe)
			cmd.SysProcAttr = workerAttr()
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			return func() int {
				err := cmd.Wait()
				var ee *exec.ExitError
				if errors.As(err, &ee) {
					return ee.ExitCode()
				}
				if err != nil {
					return exitError
				}
				return exitOK
			}, nil
		},
		stopped: stopped,
		sleep: func(d time.Duration) bool {
			for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(time.Second) {
				if stopped() {
					return false
				}
			}
			return !stopped()
		},
		now:  time.Now,
		logf: logf,
	}.run()
	_ = os.Remove(stopFile)
	return code
}

// waitSupervisorGone waits up to timeout for the supervisor to exit after
// the stop file was written, and ends a supervisor that does not.
func waitSupervisorGone(stateDir string, timeout time.Duration) {
	for end := time.Now().Add(timeout); time.Now().Before(end); time.Sleep(250 * time.Millisecond) {
		if !clawdyxAlive(readPIDFile(stateDir, supervisorPIDFile)) {
			return
		}
	}
	if pid := readPIDFile(stateDir, supervisorPIDFile); clawdyxAlive(pid) {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}
}

func writeStopFile(stateDir string) error {
	return os.WriteFile(filepath.Join(stateDir, stopFileName), []byte("stop\n"), 0o600)
}

func removeStopFile(stateDir string) { _ = os.Remove(filepath.Join(stateDir, stopFileName)) }

func supervisorAlive(stateDir string) (int, bool) {
	pid := readPIDFile(stateDir, supervisorPIDFile)
	return pid, clawdyxAlive(pid)
}
