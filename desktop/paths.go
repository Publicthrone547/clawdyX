package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s must not be readable by other users (chmod 600)", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return config.Parse(b)
}

// bundleDir returns the .app/Contents directory when running from a bundle.
func bundleDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		return filepath.Dir(dir)
	}
	return ""
}

// workerBinary finds the clawdyx worker: inside the bundle
// (Contents/Helpers/clawdyx), else CLAWDYX_WORKER, PATH, or ~/go/bin.
func workerBinary() (string, error) {
	if b := bundleDir(); b != "" {
		p := filepath.Join(b, "Helpers", "clawdyx")
		if isExec(p) {
			return p, nil
		}
	}
	if p := os.Getenv("CLAWDYX_WORKER"); p != "" && isExec(p) {
		return p, nil
	}
	if p, err := exec.LookPath("clawdyx"); err == nil {
		return filepath.Abs(p)
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(home, "go", "bin", "clawdyx"), "/usr/local/bin/clawdyx"} {
		if isExec(p) {
			return p, nil
		}
	}
	return "", errors.New("the clawdyx worker binary was not found")
}

func isExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}

// homeRel shortens paths under $HOME to ~/… for display.
func homeRel(p string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
