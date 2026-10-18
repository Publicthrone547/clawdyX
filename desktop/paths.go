package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Publicthrone547/clawdyX/desktop/host"
	"github.com/Publicthrone547/clawdyX/internal/config"
)

func isNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func loadConfigFile(path string) (*config.Config, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !config.Private(st) {
		return nil, fmt.Errorf("%s must not be readable by other users (chmod 600)", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return config.Parse(b)
}

// bundleDir returns the .app/Contents directory when running from a bundle.
func bundleDir() string { return host.BundleDir() }

// workerBinary finds the clawdyx worker: the one shipped with the app (see
// appWorker), else CLAWDYX_WORKER, PATH, or ~/go/bin.
func workerBinary() (string, error) {
	if p := appWorker(); p != "" && isExec(p) {
		return p, nil
	}
	if p := os.Getenv("CLAWDYX_WORKER"); p != "" && isExec(p) {
		return p, nil
	}
	if p, err := exec.LookPath("clawdyx"); err == nil {
		return filepath.Abs(p)
	}
	home, _ := os.UserHomeDir()
	for _, p := range host.WorkerCandidates(home) {
		if isExec(p) {
			return p, nil
		}
	}
	return "", errors.New("the clawdyx worker binary was not found")
}

func isExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && config.Executable(st)
}

// homeRel shortens paths under $HOME to ~/… for display.
func homeRel(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
