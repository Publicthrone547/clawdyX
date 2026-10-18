package main

import (
	"os/exec"

	"github.com/Publicthrone547/clawdyX/desktop/host"
)

// The OS-specific behaviour lives in package host; these keep the call
// sites short.

const screenSupported = host.ScreenSupported

var errScreenUnsupported = host.ErrScreenUnsupported

// serviceStatus reports the worker's login entry and the running worker.
func serviceStatus() (installed, loaded bool, pid int, prog string) {
	stateDir := ""
	if cfg, err := loadConfig(); err == nil {
		stateDir = cfg.StateDir
	}
	return host.ServiceStatus(serviceLabel, stateDir)
}

func appWorker() string { return host.AppWorker() }

func hideWindow(cmd *exec.Cmd) { host.HideWindow(cmd) }

func openURL(u string) error { return host.OpenURL(u) }

func openFolder(p string) error { return host.OpenFolder(p) }
