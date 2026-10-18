package host

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ScreenSupported reports whether this OS has screen capture.
const ScreenSupported = false

// ErrScreenUnsupported explains the missing screen capture.
var ErrScreenUnsupported = errors.New("screen capture is not available on this PC yet")

// ServiceStatus reports the worker's login entry (HKCU Run value written
// by `clawdyx service start`) and the running worker's pid from the state
// folder. The entry runs the login launcher clawdyxw.exe; the worker is the
// clawdyx.exe next to it.
func ServiceStatus(label, stateDir string) (installed, loaded bool, pid int, prog string) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue(label); err == nil {
			installed = true
			prog = RunProgram(v)
		}
		k.Close()
	}
	if stateDir != "" {
		if b, err := os.ReadFile(filepath.Join(stateDir, "worker.pid")); err == nil {
			if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 0 && alive(p) {
				loaded, pid = true, p
			}
		}
	}
	return installed, loaded, pid, prog
}

// RunProgram returns the worker a Run-key value starts: the quoted
// program, or the clawdyx.exe next to it when that is clawdyxw.exe.
func RunProgram(v string) string {
	if !strings.HasPrefix(v, `"`) {
		return ""
	}
	i := strings.Index(v[1:], `"`)
	if i < 0 {
		return ""
	}
	p := v[1 : 1+i]
	if strings.EqualFold(filepath.Base(p), "clawdyxw.exe") {
		p = filepath.Join(filepath.Dir(p), "clawdyx.exe")
	}
	return p
}

func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

// BundleDir is the macOS .app bundle; there is none on Windows.
func BundleDir() string { return "" }

// AppWorker is the worker shipped next to clawdyX.exe (helpers\clawdyx.exe;
// a separate folder because Windows file names ignore case), or "".
func AppWorker() string {
	if d := exeDir(); d != "" {
		if p := filepath.Join(d, "helpers", "clawdyx.exe"); regular(p) {
			return p
		}
	}
	return ""
}

// WorkerCandidates are the places a separately installed worker may live.
func WorkerCandidates(home string) []string {
	return []string{filepath.Join(home, "go", "bin", "clawdyx.exe")}
}

// HideWindow keeps console programs started by the app from opening a window.
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
}

// OpenURL hands a URL (https: or ms-settings:) to its protocol handler.
func OpenURL(u string) error {
	return exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", u).Run()
}

// OpenFolder shows a folder in File Explorer.
func OpenFolder(p string) error {
	// explorer.exe exits with 1 even on success.
	_ = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), p).Run()
	return nil
}

// TrayAvailable: the Windows notification area always exists.
func TrayAvailable() bool { return true }

// InstalledBrowsers: /open uses the default browser here; there is no choice.
func InstalledBrowsers() []string { return []string{} }
