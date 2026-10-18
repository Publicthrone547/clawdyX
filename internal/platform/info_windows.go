package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// OS facts for build-tagged callers that need a constant.
const (
	IsDarwin  = false
	IsWindows = true
	IsLinux   = false
	ExeSuffix = ".exe"
)

// W is the vocabulary of this OS.
var W = Windows

func detect(in *Info) {
	in.Session = SessionWindows
	v := windows.RtlGetVersion()
	in.Version = fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	in.Pretty = WindowsPretty(v.MajorVersion, v.BuildNumber, displayVersion())
}

// displayVersion is the feature update name ("23H2"), or "".
func displayVersion() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	if s, _, err := k.GetStringValue("DisplayVersion"); err == nil {
		return s
	}
	s, _, _ := k.GetStringValue("ReleaseId")
	return s
}
