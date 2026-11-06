//go:build unix

package main

import (
	"os"
	"syscall"
)

// keepSignal restarts the server keeping everyone in the world (udp/keep.go).
var keepSignal os.Signal = syscall.SIGUSR2
