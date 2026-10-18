//go:build !windows

package control

import "os"

func isSocket(st os.FileInfo) bool { return st.Mode()&os.ModeSocket != 0 }

func chmodSocket(path string) error { return os.Chmod(path, 0o600) }
