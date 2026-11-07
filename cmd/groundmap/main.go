// Command groundmap writes the maps of a server's collision files
// (internal/ground/mapfile.go): each <world>.ground and <world>.walls gets a
// <file>.map beside it, which the game server maps read-only instead of decoding
// zones onto its heap, so every process on a machine shares one copy. Run it after
// a bake; a server finds a map out of date by its source's CRC and decodes as
// before. It needs no game disc, and one zone's memory at a time. A running server
// goes on with the file it has mapped until its next start.
//
//	groundmap worlds/ground
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/eqoa/server/internal/ground"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: groundmap <the ground folder>")
		os.Exit(2)
	}
	start := time.Now()
	var total int64
	err := ground.WriteMaps(os.Args[1], func(file string, n int64) {
		total += n
		fmt.Printf("%s: %.1f MB\n", file, float64(n)/1e6)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%.1f MB in %v\n", float64(total)/1e6, time.Since(start).Round(time.Millisecond))
}
