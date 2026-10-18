// Command clawdyxw is the Windows login launcher of the clawdyX worker. It
// is built as a GUI program (-H windowsgui), so starting it at login opens
// no console window. It starts `clawdyx.exe service supervise` from its own
// folder, detached and without a console, passes its arguments on (the
// Run-key value gives -config PATH) and exits.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	worker := filepath.Join(filepath.Dir(exe), "clawdyx.exe")
	cmd := exec.Command(worker, append([]string{"service", "supervise"}, os.Args[1:]...)...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
	if err := cmd.Start(); err != nil {
		os.Exit(1)
	}
	_ = cmd.Process.Release()
}
