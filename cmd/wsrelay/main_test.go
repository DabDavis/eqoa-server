package main

import "testing"

func TestLookupBindsOnlyLoopback(t *testing.T) {
	for _, addr := range []string{":0", "0.0.0.0:0", "[::]:0", "203.0.113.1:0"} {
		if ln, err := listenLookup(addr); err == nil {
			ln.Close()
			t.Fatalf("lookup accepted %s", addr)
		}
	}
	ln, err := listenLookup("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()
}
