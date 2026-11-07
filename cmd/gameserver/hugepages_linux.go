package main

import "syscall"

// prSetTHPDisable is PR_SET_THP_DISABLE: no transparent huge pages for this
// process, kept through an exec (a restart that keeps the sessions), and khugepaged
// leaves it alone too.
const prSetTHPDisable = 41

// disableHugePages is server.no_huge_pages.
func disableHugePages() error {
	if _, _, e := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetTHPDisable, 1, 0, 0, 0, 0); e != 0 {
		return e
	}
	return nil
}
