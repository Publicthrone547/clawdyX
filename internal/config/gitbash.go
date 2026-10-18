package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// checkGitBash validates claude.git_bash_path: an absolute path of an
// existing bash.exe or sh.exe.
func checkGitBash(p string) error {
	if !filepath.IsAbs(p) {
		return errors.New("must be an absolute path")
	}
	switch strings.ToLower(filepath.Base(p)) {
	case "bash.exe", "sh.exe":
	default:
		return errors.New(`must point at bash.exe (for example C:\Program Files\Git\bin\bash.exe)`)
	}
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("is not a file")
	}
	return nil
}
