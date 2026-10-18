//go:build !windows

package main

// On macOS launchd owns the background worker: it redirects stdout and
// stderr to files and restarts the worker after exitRestart (KeepAlive).

func detachConsole(string) bool { return false }

func writePID(string) func() { return func() {} }

func relaunch(code int, _ bool) int { return code }
