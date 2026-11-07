//go:build !linux

package main

import "errors"

// disableHugePages is server.no_huge_pages: Linux's alone.
func disableHugePages() error {
	return errors.New("not on this system")
}
