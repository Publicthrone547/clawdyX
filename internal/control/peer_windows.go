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

// Go reports AF_UNIX reparse points as ModeSocket, so a regular file at the
// socket path is never mistaken for a stale socket and removed.
func isSocket(st os.FileInfo) bool { return st.Mode()&os.ModeSocket != 0 }

// File modes do not apply on Windows (see peerUID).
func chmodSocket(string) error { return nil }
