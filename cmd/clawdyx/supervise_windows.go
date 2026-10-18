package main

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// workerAttr starts the supervised worker without a console of its own
// (no window ever appears) in its own process group.
func workerAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}
