package host

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/Publicthrone547/clawdyX/internal/config"
)

// ScreenSupported: the worker picks X11 or Wayland per session and
// explains a session without a screen itself.
const ScreenSupported = true

// ErrScreenUnsupported is nil where screens are supported.
var ErrScreenUnsupported error

func xdgConfigHome() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" && filepath.IsAbs(d) {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// ServiceShow parses `systemctl show` output (KEY=value lines).
func ServiceShow(out string) map[string]string {
	m := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			m[k] = v
		}
	}
	return m
}

// ServiceStatus reports the systemd user unit (or the XDG autostart
// fallback with the supervisor): installed, running, the worker's pid and
// the program it runs.
func ServiceStatus(label, stateDir string) (installed, loaded bool, pid int, prog string) {
	unit := config.SystemdUnit(label)
	unitFile := filepath.Join(xdgConfigHome(), "systemd", "user", unit+".service")
	if b, err := os.ReadFile(unitFile); err == nil {
		installed, prog = true, config.UnitProgram(string(b))
	}
	if out, err := exec.Command("systemctl", "--user", "show", unit+".service", "-p", "ActiveState,MainPID").Output(); err == nil {
		st := ServiceShow(string(out))
		if n, _ := strconv.Atoi(st["MainPID"]); st["ActiveState"] == "active" && n > 0 {
			return installed, true, n, prog
		}
	}
	if _, err := os.Stat(filepath.Join(xdgConfigHome(), "autostart", unit+".desktop")); err == nil {
		installed = true
	}
	if stateDir != "" {
		if b, err := os.ReadFile(filepath.Join(stateDir, "worker.pid")); err == nil {
			if p, _ := strconv.Atoi(strings.TrimSpace(string(b))); p > 0 {
				if comm, err := os.ReadFile("/proc/" + strconv.Itoa(p) + "/comm"); err == nil && strings.TrimSpace(string(comm)) == "clawdyx" {
					loaded, pid = true, p
				}
			}
		}
	}
	return installed, loaded, pid, prog
}

// BundleDir is the macOS .app bundle; there is none on Linux.
func BundleDir() string { return "" }

// AppWorker is the worker next to the app binary (the tarball, the
// AppImage and the packages install both into one folder), or "".
func AppWorker() string {
	if d := exeDir(); d != "" {
		if p := filepath.Join(d, "clawdyx"); regular(p) {
			return p
		}
	}
	return ""
}

// WorkerCandidates are the places a separately installed worker may live.
func WorkerCandidates(home string) []string {
	return []string{filepath.Join(home, ".local", "bin", "clawdyx"), "/usr/local/bin/clawdyx", "/usr/bin/clawdyx", filepath.Join(home, "go", "bin", "clawdyx")}
}

// HideWindow does nothing on Linux (console programs open no window).
func HideWindow(*exec.Cmd) {}

// OpenURL opens a URL in the default browser (xdg-open).
func OpenURL(u string) error { return xdgOpen(u) }

// OpenFolder shows a folder in the file manager (xdg-open).
func OpenFolder(p string) error { return xdgOpen(p) }

func xdgOpen(arg string) error {
	bin, err := exec.LookPath("xdg-open")
	if err != nil {
		return errors.New("xdg-open is not installed (package xdg-utils)")
	}
	cmd := exec.Command(bin, arg)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// TrayAvailable reports whether a StatusNotifier host (KDE, or GNOME with
// the AppIndicator extension) can show the app's tray icon.
func TrayAvailable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}

// InstalledBrowsers: /open uses the default browser here; there is no choice.
func InstalledBrowsers() []string { return []string{} }

// SettingsLinks: desktops differ too much for links; Fix texts name the
// commands instead.
var SettingsLinks = map[string]string{}
