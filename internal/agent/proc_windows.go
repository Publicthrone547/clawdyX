package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// passEnv are the variables forwarded to claude children. Windows programs
// (Node, Git Bash, which Claude Code uses for its shell tool) need the
// profile and system folders; secrets are still never forwarded.
var passEnv = []string{
	"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "USERNAME", "USERDOMAIN",
	"APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "SystemRoot", "SystemDrive", "windir", "ComSpec", "PATHEXT",
	"ProgramData", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "CommonProgramFiles",
	"NUMBER_OF_PROCESSORS", "PROCESSOR_ARCHITECTURE", "OS", "CLAUDE_CODE_GIT_BASH_PATH",
}

var platformEnv []string

// groupAttr starts a child in its own process group without a console
// window (the background worker has no console of its own).
func groupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW, HideWindow: true}
}

type stopStep struct {
	sig   syscall.Signal
	grace time.Duration
}

// stopLadder has a single step on Windows: console programs cannot be sent
// SIGINT or SIGTERM from a process without their console, so the tree is
// ended at once. The interrupted turn may be missing from the transcript.
func stopLadder(time.Duration, time.Duration) []stopStep {
	return []stopStep{{syscall.SIGKILL, 5 * time.Second}}
}

func signalGroup(pid int, _ syscall.Signal) error { return KillGroup(pid, true) }

// KillGroup ends pid and every process it started (taskkill /T /F).
func KillGroup(pid int, _ bool) error {
	tk := filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe")
	cmd := exec.Command(tk, "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	if err := cmd.Run(); err != nil {
		if ProcessStartUnix(pid) == 0 {
			return syscall.ESRCH
		}
		return err
	}
	return nil
}

func openQuery(pid int) (windows.Handle, error) {
	return windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
}

// ProcessStartUnix returns a process's creation time (seconds), or 0.
func ProcessStartUnix(pid int) int64 {
	h, err := openQuery(pid)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0
	}
	var code uint32
	if windows.GetExitCodeProcess(h, &code) == nil && code != 259 { // STILL_ACTIVE
		return 0
	}
	return created.Nanoseconds() / 1e9
}

// ProcessComm returns the executable's base name without ".exe", or "".
func ProcessComm(pid int) string {
	h, err := openQuery(pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	base := filepath.Base(windows.UTF16ToString(buf[:n]))
	if strings.EqualFold(filepath.Ext(base), ".exe") {
		base = base[:len(base)-4]
	}
	return base
}
