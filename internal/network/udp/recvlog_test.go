package udp

import (
	"bytes"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// With the packet log off, an address's packets get a line at its first and
// then one every receivedEvery with their count; the line is one the swap
// watcher counts as someone there ("UDP received") and one cmd/movereplay's
// own pattern leaves alone (it pairs only the packet log's lines with their
// raw hex).
func TestReceivedLineEveryThirtySeconds(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	l := &Listener{}
	a := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 40001}
	b := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 40002}
	t0 := time.Unix(1_800_000_000, 0)
	for _, s := range []struct {
		addr *net.UDPAddr
		at   time.Duration
	}{{a, 0}, {a, time.Second}, {b, 2 * time.Second}, {a, 29 * time.Second}, {a, 31 * time.Second}, {b, 20 * time.Second}} {
		l.noteReceived(120, s.addr, t0.Add(s.at))
	}
	var lines []string
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if i := strings.Index(ln, "UDP received"); i >= 0 {
			lines = append(lines, ln[i:])
		}
	}
	want := []string{"10.0.0.7:40001 (1 packets", "10.0.0.8:40002 (1 packets", "10.0.0.7:40001 (3 packets"}
	if len(lines) != len(want) {
		t.Fatalf("%d lines, want %d: %q", len(lines), len(want), lines)
	}
	movereplay := regexp.MustCompile(`^UDP received \d+ bytes from (\S+)$`)
	for i, ln := range lines {
		if !strings.Contains(ln, want[i]) {
			t.Errorf("line %d %q: want %q", i, ln, want[i])
		}
		if movereplay.MatchString(ln) {
			t.Errorf("line %d %q matches cmd/movereplay's packet pattern", i, ln)
		}
	}
}
