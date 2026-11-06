//go:build unix

package udp

import (
	"net"
	"os"
	"syscall"
)

// inheritable is a copy of c's socket that survives an exec (its
// close-on-exec flag cleared) and its descriptor's number: the next image of
// the process takes it by that number (InheritedConn). The copy must stay open
// until the exec.
func inheritable(c *net.UDPConn) (*os.File, int, error) {
	f, err := c.File()
	if err != nil {
		return nil, 0, err
	}
	rc, err := f.SyscallConn()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	fd, ferr := -1, error(nil)
	if err := rc.Control(func(p uintptr) {
		fd = int(p)
		if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, p, syscall.F_SETFD, 0); e != 0 {
			ferr = e
		}
	}); err != nil {
		f.Close()
		return nil, 0, err
	}
	if ferr != nil {
		f.Close()
		return nil, 0, ferr
	}
	return f, fd, nil
}
