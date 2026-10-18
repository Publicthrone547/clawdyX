package main

import (
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed build/appicon.png
var appIconPNG []byte

const exeSuffix = ".exe"

const screenSupported = false

var errScreenUnsupported = errors.New("screen capture is not supported on Windows yet (macOS only)")

// serviceStatus reports the worker's login entry (HKCU Run value written by
// `clawdyx service start`) and the running worker's pid.
func serviceStatus() (installed, loaded bool, pid int, prog string) {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue(serviceLabel); err == nil {
			installed = true
			if strings.HasPrefix(v, `"`) {
				if i := strings.Index(v[1:], `"`); i >= 0 {
					prog = v[1 : 1+i]
				}
			}
		}
		k.Close()
	}
	if cfg, err := loadConfig(); err == nil {
		if b, err := os.ReadFile(filepath.Join(cfg.StateDir, "worker.pid")); err == nil {
			if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 0 && alive(p) {
				loaded, pid = true, p
			}
		}
	}
	return installed, loaded, pid, prog
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

// appWorker is the worker shipped next to clawdyX.exe (helpers\clawdyx.exe;
// a separate folder because Windows file names ignore case), or "".
func appWorker() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	p := filepath.Join(filepath.Dir(exe), "helpers", "clawdyx.exe")
	if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
		return p
	}
	return ""
}

// hideWindow keeps console programs started by the app from opening a window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
}

func openURL(u string) error {
	return exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe"), "url.dll,FileProtocolHandler", u).Run()
}

func openFolder(p string) error {
	// explorer.exe exits with 1 even on success.
	_ = exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), p).Run()
	return nil
}

func setTrayIcon(t *application.SystemTray) { t.SetIcon(appIconPNG) }
