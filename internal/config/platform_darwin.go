package config

import (
	"path/filepath"
	"strings"
)

func defaultClaudeBinary(string) string { return "/opt/homebrew/bin/claude" }

// DefaultChildPath is the PATH given to claude children.
func DefaultChildPath() string {
	return "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
}

// configDir is ~/.config/clawdyx on macOS too (not ~/Library), as in 0.x.
func configDir(home string) string { return filepath.Join(home, ".config", "clawdyx") }

// legacyConfigPath: the macOS config never moved.
func legacyConfigPath(string) string { return "" }

func defaultStateDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "clawdyx")
}

func defaultLogDir(home string) string { return filepath.Join(home, "Library", "Logs", "clawdyx") }

// pathEqual compares canonical paths; macOS volumes are case-insensitive
// by default.
func pathEqual(a, b string) bool { return strings.EqualFold(a, b) }

// gitBashUsed: claude.git_bash_path matters only on Windows.
const gitBashUsed = false
