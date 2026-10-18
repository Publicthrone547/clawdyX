package control

import (
	"net"
	"os"
)

// Windows has no peer credentials for AF_UNIX sockets. Access is decided
// by the socket file's ACL instead: connecting needs write access to it,
// and it lives in the user's %LOCALAPPDATA%\clawdyx, which inherits a
// profile ACL that admits only that user, SYSTEM and administrators. So
// every connection that gets this far is accepted as the server's own user.
func peerUID(*net.UnixConn) (int, error) { return os.Getuid(), nil }

// Go may not report ModeSocket for AF_UNIX reparse points; anything but a
// directory at the socket path is treated as a stale socket.
func isSocket(st os.FileInfo) bool { return !st.IsDir() }

// File modes do not apply on Windows (see peerUID).
func chmodSocket(string) error { return nil }
