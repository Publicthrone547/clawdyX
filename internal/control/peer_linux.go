//go:build linux

package control

import (
	"net"

	"golang.org/x/sys/unix"
)

func peerUID(c *net.UnixConn) (int, error) {
	rc, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid, gerr := -1, error(nil)
	err = rc.Control(func(fd uintptr) {
		var u *unix.Ucred
		u, gerr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if gerr == nil {
			uid = int(u.Uid)
		}
	})
	if err != nil {
		return -1, err
	}
	return uid, gerr
}
