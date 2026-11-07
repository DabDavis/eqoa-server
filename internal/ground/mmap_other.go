//go:build !unix

package ground

import "os"

// mmapFile says there is no mapping here (a nil result and no error): every load
// decodes, as it always did.
func mmapFile(f *os.File) ([]byte, error) { return nil, nil }

func munmapFile(b []byte) {}
