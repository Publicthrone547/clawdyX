//go:build darwin

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
		var x *unix.Xucred
		x, gerr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if gerr == nil {
			uid = int(x.Uid)
		}
	})
	if err != nil {
		return -1, err
	}
	return uid, gerr
}
