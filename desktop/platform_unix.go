//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const exeSuffix = ""

const screenSupported = true

var errScreenUnsupported error

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", serviceLabel+".plist")
}

func launchctl(args ...string) (string, error) {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	return string(out), err
}

func plistProgram() string {
	b, err := os.ReadFile(plistPath())
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

// serviceStatus reports the LaunchAgent: plist present, known to launchd,
// its pid, and the program it runs.
func serviceStatus() (installed, loaded bool, pid int, prog string) {
	if _, err := os.Stat(plistPath()); err == nil {
		installed = true
	}
	prog = plistProgram()
	if out, err := launchctl("print", "gui/"+strconv.Itoa(os.Getuid())+"/"+serviceLabel); err == nil {
		loaded = true
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); strings.HasPrefix(l, "pid = ") {
				pid, _ = strconv.Atoi(strings.TrimPrefix(l, "pid = "))
			}
		}
	}
	return installed, loaded, pid, prog
}

// appWorker is the worker inside clawdyX.app (Contents/Helpers/clawdyx),
// or "" when not running from a bundle.
func appWorker() string {
	if b := bundleDir(); b != "" {
		return filepath.Join(b, "Helpers", "clawdyx")
	}
	return ""
}

func hideWindow(*exec.Cmd) {}

func openURL(u string) error { return exec.Command("/usr/bin/open", u).Run() }

func openFolder(p string) error { return exec.Command("/usr/bin/open", p).Run() }

func setTrayIcon(t *application.SystemTray) { t.SetTemplateIcon(trayIcon) }
