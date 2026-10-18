package platform

import "strings"

// WindowsPretty names a Windows release: build 22000 and later is Windows
// 11 (the kernel version stays 10.0).
func WindowsPretty(major, build uint32, feature string) string {
	name := "Windows"
	switch {
	case major == 10 && build >= 22000:
		name = "Windows 11"
	case major == 10:
		name = "Windows 10"
	}
	return strings.TrimSpace(name + " " + feature)
}
