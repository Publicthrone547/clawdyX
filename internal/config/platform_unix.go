//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

func defaultClaudeBinary(string) string { return "/opt/homebrew/bin/claude" }

// DefaultChildPath is the PATH given to claude children.
func DefaultChildPath() string {
	return "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
}

func defaultStateDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "clawdyx")
}

func defaultLogDir(home string) string { return filepath.Join(home, "Library", "Logs", "clawdyx") }

// Private reports whether only the owner can read the file (mode 0600).
func Private(st os.FileInfo) bool { return st.Mode().Perm()&0o077 == 0 }

// Executable reports whether st is a regular file with an execute bit.
func Executable(st os.FileInfo) bool { return st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 }
