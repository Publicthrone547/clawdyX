// Package platform is the one place that knows which operating system
// clawdyX runs on: facts about the machine (Info) and the words the bot, the
// CLI and the desktop app use for it (Words). Behaviour that differs per OS
// lives in build-tagged files of the packages that own it; they never test
// runtime.GOOS themselves.
package platform

import (
	"runtime"
	"strings"
	"sync"
)

// Session is the kind of graphical session the process can reach.
type Session string

// Sessions.
const (
	SessionAqua     Session = "aqua"     // macOS
	SessionWindows  Session = "windows"  // an interactive Windows desktop
	SessionX11      Session = "x11"      // Linux with an X server (DISPLAY)
	SessionWayland  Session = "wayland"  // Linux with a Wayland compositor
	SessionHeadless Session = "headless" // Linux without a graphical session
)

// Info describes the running machine. Every field is best effort and never
// makes Detect fail.
type Info struct {
	OS       string  `json:"os"`       // runtime.GOOS
	Arch     string  `json:"arch"`     // runtime.GOARCH
	OSName   string  `json:"osName"`   // "macOS" | "Windows" | "Linux"
	Version  string  `json:"version"`  // "14.6", "10.0.22631", "24.04"
	Pretty   string  `json:"pretty"`   // "macOS 14.6", "Windows 11 23H2", "Ubuntu 24.04.1 LTS"
	Session  Session `json:"session"`  // see Session
	Desktop  string  `json:"desktop"`  // Linux: XDG_CURRENT_DESKTOP (GNOME, KDE, …), else ""
	Headless bool    `json:"headless"` // Session == SessionHeadless
	DistroID string  `json:"distroId"` // Linux: os-release ID and ID_LIKE ("ubuntu debian"), else ""
}

var (
	once    sync.Once
	current Info
)

// Current returns the machine description, read once per process.
func Current() Info {
	once.Do(func() { current = Detect() })
	return current
}

// Detect re-reads the environment (the Linux session can change while a
// worker runs, for example when a user logs in to the desktop).
func Detect() Info {
	in := Info{OS: runtime.GOOS, Arch: runtime.GOARCH, OSName: W.OSName}
	detect(&in)
	in.Headless = in.Session == SessionHeadless
	if in.Pretty == "" {
		in.Pretty = strings.TrimSpace(in.OSName + " " + in.Version)
	}
	return in
}

// DetectSession classifies a Linux environment: Wayland when
// XDG_SESSION_TYPE says so or WAYLAND_DISPLAY is set, X11 when DISPLAY is
// set (XDG_SESSION_TYPE=x11 without DISPLAY is not reachable), else
// headless. getenv is os.Getenv in production.
func DetectSession(getenv func(string) string) Session {
	t := strings.ToLower(strings.TrimSpace(getenv("XDG_SESSION_TYPE")))
	switch {
	case t == "wayland" || getenv("WAYLAND_DISPLAY") != "":
		return SessionWayland
	case getenv("DISPLAY") != "":
		return SessionX11
	}
	return SessionHeadless
}

// ParseOSRelease reads the PRETTY_NAME, VERSION_ID, ID and ID_LIKE fields
// of an /etc/os-release document.
func ParseOSRelease(doc string) (pretty, version, ids string) {
	vals := map[string]string{}
	for _, l := range strings.Split(doc, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		vals[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	ids = strings.TrimSpace(vals["ID"] + " " + vals["ID_LIKE"])
	return vals["PRETTY_NAME"], vals["VERSION_ID"], ids
}
