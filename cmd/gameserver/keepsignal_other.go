//go:build !unix

package main

import "os"

// keepSignal: none off Unix, where a socket cannot be handed across an exec.
var keepSignal os.Signal

// keepWarnSignal: none either; nothing warns of a restart that cannot happen.
var keepWarnSignal os.Signal
