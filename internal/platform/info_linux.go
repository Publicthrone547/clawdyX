package platform

import "os"

// OS facts for build-tagged callers that need a constant.
const (
	IsDarwin  = false
	IsWindows = false
	IsLinux   = true
	ExeSuffix = ""
)

// W is the vocabulary of this OS.
var W = Linux

func detect(in *Info) {
	in.Session = DetectSession(os.Getenv)
	if in.Session != SessionHeadless {
		in.Desktop = os.Getenv("XDG_CURRENT_DESKTOP")
	}
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if b, err := os.ReadFile(p); err == nil {
			in.Pretty, in.Version, in.DistroID = ParseOSRelease(string(b))
			break
		}
	}
}
