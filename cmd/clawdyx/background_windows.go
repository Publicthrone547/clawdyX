package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
)

// detachConsole makes `clawdyx run` a quiet background process when nobody
// is watching it: started detached by `clawdyx service start` (no console
// at all), or at login from the Run key (a console of its own, which is
// closed here). In a terminal it shares the console and stays in the
// foreground. In the background, runtime panics go to logDir\stderr.log
// (the log itself goes to bot.log only).
func detachConsole(logDir string) bool {
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		var ids [4]uint32
		n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
		if n != 1 {
			return false
		}
		_, _, _ = procFreeConsole.Call()
	}
	if err := os.MkdirAll(logDir, 0o700); err == nil {
		if f, err := os.OpenFile(filepath.Join(logDir, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
			_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
		}
	}
	return true
}

// relaunch: a background worker that stopped for a restart (exitRestart)
// starts a fresh copy of itself, unless the supervisor started it (the
// supervisor restarts it then). The state lock is already released when
// this runs.
func relaunch(code int, background bool) int {
	if code != exitRestart || !background || os.Getenv(supervisedEnv) != "" {
		return code
	}
	exe, err := os.Executable()
	if err == nil {
		err = spawnDetached(exe, os.Args[1:]...)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "restart failed:", err)
		return exitError
	}
	return exitOK
}

// spawnDetached starts exe without a console window and without inherited
// handles, so callers that read our output (the desktop app) do not wait
// for the worker.
func spawnDetached(exe string, args ...string) error {
	cmd := exec.Command(exe, args...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
