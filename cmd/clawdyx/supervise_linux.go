package main

import "syscall"

// workerAttr starts the supervised worker in its own process group.
func workerAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
