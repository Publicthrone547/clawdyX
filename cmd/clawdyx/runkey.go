package main

import "strings"

// The Windows login entry (see service_windows.go). Kept platform-neutral
// so the quoting is tested on every build.

// RunCommand is the Run-key value: the worker and its config, quoted.
func RunCommand(exe, cfgPath string) string {
	return `"` + exe + `" run -config "` + cfgPath + `"`
}

// runProgram returns the executable a Run-key value starts.
func runProgram(v string) string {
	if strings.HasPrefix(v, `"`) {
		if i := strings.Index(v[1:], `"`); i >= 0 {
			return v[1 : 1+i]
		}
	}
	return ""
}
