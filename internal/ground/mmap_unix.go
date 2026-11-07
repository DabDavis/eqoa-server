//go:build unix

package ground

import (
	"os"
	"syscall"
)

// mmapFile maps f read-only: every process mapping one file shares its pages.
func mmapFile(f *os.File) ([]byte, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() == 0 {
		return nil, nil
	}
	return syscall.Mmap(int(f.Fd()), 0, int(fi.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
}

func munmapFile(b []byte) {
	if b != nil {
		_ = syscall.Munmap(b)
	}
}
