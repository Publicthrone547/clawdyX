package platform

import "golang.org/x/sys/unix"

// OS facts for build-tagged callers that need a constant.
const (
	IsDarwin  = true
	IsWindows = false
	IsLinux   = false
	ExeSuffix = ""
)

// W is the vocabulary of this OS.
var W = Darwin

func detect(in *Info) {
	in.Session = SessionAqua
	if v, err := unix.Sysctl("kern.osproductversion"); err == nil {
		in.Version = v
	}
}
