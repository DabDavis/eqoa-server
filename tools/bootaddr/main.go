// bootaddr reports, and optionally rewrites, the login address inside a built
// EQOABOOT.ELF.
//
//	go run ./tools/bootaddr players/EQOABOOT.template.ELF
//	go run ./tools/bootaddr players/EQOABOOT.template.ELF 10.0.0.5 out.ELF
//
// The server does this for itself at startup, so this exists for the two cases
// the server cannot cover: make-distro.sh proving a template can be aimed
// BEFORE it ships, and an operator answering "which machine is this file
// actually pointing at" about a loader someone has already been sent.
//
// That second question is the whole reason the address is worth a tool. It is
// two MIPS instructions in the middle of a stripped binary; it is not a string,
// so `strings` finds nothing, and a wrong one costs a tester an evening.
package main

import (
	"fmt"
	"os"

	"github.com/eqoa/server/internal/bootelf"
)

func main() {
	if len(os.Args) != 2 && len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr,
			"usage: bootaddr <EQOABOOT.ELF> [<new-ip> <out.ELF>]")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ip, err := bootelf.Address(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(os.Args) == 2 {
		fmt.Println(ip)
		return
	}
	out, err := bootelf.Retarget(raw, os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(os.Args[3], out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%s: %s -> %s\n", os.Args[3], ip, os.Args[2])
}
