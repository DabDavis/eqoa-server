//go:build !unix

package udp

import (
	"errors"
	"net"
	"os"
)

// inheritable: a socket handed across an exec is Unix's (keep_unix.go).
func inheritable(c *net.UDPConn) (*os.File, int, error) {
	return nil, 0, errors.New("a restart that keeps the sessions needs a Unix")
}
