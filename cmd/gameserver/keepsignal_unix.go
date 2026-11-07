//go:build unix

package main

import (
	"os"
	"syscall"
)

// keepSignal restarts the server keeping everyone in the world (udp/keep.go).
var keepSignal os.Signal = syscall.SIGUSR2

// keepWarnSignal tells everyone in the world that such a restart is a minute off.
var keepWarnSignal os.Signal = syscall.SIGUSR1
