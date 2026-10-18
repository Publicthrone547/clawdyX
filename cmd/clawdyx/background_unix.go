//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// On macOS launchd owns the background worker, on Linux systemd (or the
// supervisor where no systemd user manager runs): they redirect stdout and
// stderr and restart the worker after exitRestart.

func detachConsole(string) bool { return false }

func relaunch(code int, _ bool) int { return code }

// spawnDetached starts exe in a new session without inherited handles, so
// it outlives the terminal that started it (the Linux supervisor fallback).
func spawnDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
