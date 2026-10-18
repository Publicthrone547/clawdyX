//go:build !windows

package config

import "os"

// Private reports whether only the owner can read the file (mode 0600).
func Private(st os.FileInfo) bool { return st.Mode().Perm()&0o077 == 0 }

// Executable reports whether st is a regular file with an execute bit.
func Executable(st os.FileInfo) bool { return st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 }
