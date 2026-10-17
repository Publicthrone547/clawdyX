//go:build !darwin && !linux

package control

import (
	"errors"
	"net"
)

// The control channel is not available on this platform yet (Windows will
// use a named pipe with an explicit DACL).
func peerUID(*net.UnixConn) (int, error) { return -1, errors.New("control: unsupported platform") }
