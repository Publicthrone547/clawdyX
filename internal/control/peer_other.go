//go:build !darwin && !linux && !windows

package control

import (
	"errors"
	"net"
)

// The control channel is not available on this platform.
func peerUID(*net.UnixConn) (int, error) { return -1, errors.New("control: unsupported platform") }
