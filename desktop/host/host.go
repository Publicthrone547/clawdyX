// Package host is the desktop app's OS-specific behaviour that does not
// need the Wails runtime: the background worker's service status, where
// the bundled worker lives, and opening URLs and folders. Keeping it out of
// package main lets it be type-checked for every OS from any machine (the
// app itself needs cgo on macOS and Linux).
package host

import (
	"os"
	"path/filepath"
)

// exeDir is the folder of the running app binary (symlinks resolved), or "".
func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func regular(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
