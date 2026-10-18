package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ScreenSupported reports whether this OS has screen capture.
const ScreenSupported = true

// ErrScreenUnsupported is nil where screens are supported.
var ErrScreenUnsupported error

func plistPath(label string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func launchctl(args ...string) (string, error) {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	return string(out), err
}

// PlistProgram is the program a LaunchAgent plist runs, or "".
func PlistProgram(label string) string {
	b, err := os.ReadFile(plistPath(label))
	if err != nil {
		return ""
	}
	s := string(b)
	i := strings.Index(s, "<key>ProgramArguments</key><array>")
	if i < 0 {
		return ""
	}
	rest := s[i:]
	a := strings.Index(rest, "<string>")
	if a < 0 {
		return ""
	}
	z := strings.Index(rest[a:], "</string>")
	if z < 0 {
		return ""
	}
	return rest[a+8 : a+z]
}

// ServiceStatus reports the LaunchAgent: plist present, known to launchd,
// its pid, and the program it runs.
func ServiceStatus(label, _ string) (installed, loaded bool, pid int, prog string) {
	if _, err := os.Stat(plistPath(label)); err == nil {
		installed = true
	}
	prog = PlistProgram(label)
	if out, err := launchctl("print", "gui/"+strconv.Itoa(os.Getuid())+"/"+label); err == nil {
		loaded = true
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); strings.HasPrefix(l, "pid = ") {
				pid, _ = strconv.Atoi(strings.TrimPrefix(l, "pid = "))
			}
		}
	}
	return installed, loaded, pid, prog
}

// BundleDir returns the .app/Contents directory when running from a bundle.
func BundleDir() string {
	dir := exeDir()
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		return filepath.Dir(dir)
	}
	return ""
}

// AppWorker is the worker inside clawdyX.app (Contents/Helpers/clawdyx),
// or "" when not running from a bundle.
func AppWorker() string {
	if b := BundleDir(); b != "" {
		return filepath.Join(b, "Helpers", "clawdyx")
	}
	return ""
}

// WorkerCandidates are the places a separately installed worker may live.
func WorkerCandidates(home string) []string {
	return []string{filepath.Join(home, "go", "bin", "clawdyx"), "/usr/local/bin/clawdyx", "/opt/homebrew/bin/clawdyx"}
}

// HideWindow does nothing on macOS (console programs open no window).
func HideWindow(*exec.Cmd) {}

// OpenURL opens a URL or a System Settings pane.
func OpenURL(u string) error { return exec.Command("/usr/bin/open", u).Run() }

// OpenFolder shows a folder in Finder.
func OpenFolder(p string) error { return exec.Command("/usr/bin/open", p).Run() }

// TrayAvailable: the macOS menu bar always exists.
func TrayAvailable() bool { return true }
