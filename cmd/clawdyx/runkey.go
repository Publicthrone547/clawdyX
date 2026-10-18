package main

import (
	"path/filepath"
	"strings"
)

// The Windows login entry (see service_windows.go). Kept platform-neutral
// so the quoting is tested on every build.

// launcherName is the GUI-subsystem launcher next to clawdyx.exe: started
// at login, it starts the supervisor without a console window and exits.
const launcherName = "clawdyxw.exe"

// RunCommand is the Run-key value for the worker exe and its config, every
// argument quoted. With the launcher next to exe it runs the launcher;
// otherwise (a development build) the supervisor directly, which may show
// a console window for a moment at login.
func RunCommand(exe, cfgPath string, launcher bool) string {
	if launcher {
		return quoteArg(filepath.Join(filepath.Dir(exe), launcherName)) + ` -config ` + quoteArg(cfgPath)
	}
	return quoteArg(exe) + ` service supervise -config ` + quoteArg(cfgPath)
}

// quoteArg quotes one command-line argument for Windows. Paths cannot
// contain quotes; a trailing backslash is doubled so it does not escape
// the closing quote.
func quoteArg(s string) string {
	if strings.HasSuffix(s, `\`) {
		s += `\`
	}
	return `"` + s + `"`
}

// runProgram returns the worker executable a Run-key value starts: the
// quoted program, or the clawdyx.exe next to it when that is the launcher.
func runProgram(v string) string {
	if !strings.HasPrefix(v, `"`) {
		return ""
	}
	i := strings.Index(v[1:], `"`)
	if i < 0 {
		return ""
	}
	p := v[1 : 1+i]
	if strings.EqualFold(filepath.Base(p), launcherName) {
		return filepath.Join(filepath.Dir(p), "clawdyx.exe")
	}
	return p
}
