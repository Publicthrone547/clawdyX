package config

import (
	"os"
	"path/filepath"
)

// defaultClaudeBinary is where Claude Code's native installer puts it.
func defaultClaudeBinary(home string) string { return filepath.Join(home, ".local", "bin", "claude") }

// DefaultChildPath is the PATH given to claude children: the native
// installer's folder first, then the system folders.
func DefaultChildPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin") + ":/usr/local/bin:/usr/bin:/bin"
}

// xdgDir returns $env when it is an absolute path (the XDG Base Directory
// rules ignore relative values), else home/fallback.
func xdgDir(env, home, fallback string) string {
	if d := os.Getenv(env); d != "" && filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(home, fallback)
}

func configDir(home string) string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", home, ".config"), "clawdyx")
}

// legacyConfigPath: Linux had no earlier release.
func legacyConfigPath(string) string { return "" }

func defaultStateDir(home string) string {
	return filepath.Join(xdgDir("XDG_STATE_HOME", home, filepath.Join(".local", "state")), "clawdyx")
}

func defaultLogDir(home string) string { return filepath.Join(defaultStateDir(home), "logs") }

// pathEqual compares canonical paths; Linux file systems are case-sensitive.
func pathEqual(a, b string) bool { return a == b }

// gitBashUsed: claude.git_bash_path matters only on Windows.
const gitBashUsed = false
