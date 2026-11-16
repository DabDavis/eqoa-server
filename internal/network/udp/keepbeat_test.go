//go:build unix

package udp

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// clientEnds is CLIENT's rule for an established connection on a route not
// terminated (connection.cpp, connection_flush, reason 2): it ends once its
// oldest unacknowledged reliable message is older than the warn time (30 s,
// viclient.cpp 1707) and nothing has arrived for as long.
func clientEnds(oldestUnacked, lastArrival, now time.Duration) bool {
	const warn = 30 * time.Second
	return now-oldestUnacked > warn && now-lastArrival > warn
}

// The restart of 02:17:29 UTC 2026-11-16 (33 sessions) as the log has it: the
// old process's last send at the freeze (1.3 s after the signal, the snapshot
// written), its exec at 3.2 s, the new one's first send when every character
// was built, 31.6 s after the snapshot. A client with a reliable message in
// flight at the freeze (a target, an attack press, a chat line) gave up at
// 31.3 s, before the server spoke again; with a beat every keepBeatEvery it does
// not, though the hold be as long again.
func TestTheRestartOf0218LostEveryClientWithAMessageInFlight(t *testing.T) {
	freeze := 1300 * time.Millisecond
	resumed := freeze + 31613*time.Millisecond
	sent := freeze - 200*time.Millisecond // its message, unacknowledged at the freeze
	ended := false
	for now := freeze; now < resumed; now += 100 * time.Millisecond {
		if clientEnds(sent, freeze, now) {
			ended = true
			break
		}
	}
	if !ended {
		t.Fatal("the model keeps the client of 02:18: it does not reproduce the freeze")
	}
	// the same hold, beaten: the last arrival is never more than a beat ago
	for now := freeze; now < freeze+55*time.Second; now += 100 * time.Millisecond {
		last := freeze + (now-freeze)/keepBeatEvery*keepBeatEvery
		if clientEnds(sent, last, now) {
			t.Fatalf("a beaten client ends %v into the hold", now-freeze)
		}
	}
}

// serverReport is the packet number and the guaranteed ack of a packet the
// server sent (ParsePacket reads a client's): the body's flags (session ack and
// RDP message set), the session ack (the instance), then the number, the flush
// ack and the guaranteed ack.
func serverReport(b []byte, instance uint32) (num, ack uint16, ok bool) {
	in := []byte{byte(instance), byte(instance >> 8), byte(instance >> 16), byte(instance >> 24)}
	for i := 4; i+11 <= len(b); i++ {
		if b[i]&0x60 == 0x60 && b[i]&0x02 != 0 && string(b[i+1:i+5]) == string(in) {
			j := i + 5
			return uint16(b[j]) | uint16(b[j+1])<<8, uint16(b[j+4]) | uint16(b[j+5])<<8, true
		}
	}
	return 0, 0, false
}

// beats collects what a client is sent: each packet's number and when.
type beats struct {
	mu      sync.Mutex
	numbers []uint16
	at      []time.Time
	acks    []uint16
}

func (b *beats) read(t *testing.T, c *net.UDPConn, stop <-chan struct{}) {
	buf := make([]byte, 2048)
	for {
		select {
		case <-stop:
			return
		default:
		}
		c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		num, ack, ok := serverReport(buf[:n], 0x2EC7D061)
		if !ok {
			continue
		}
		b.mu.Lock()
		b.numbers = append(b.numbers, num)
		b.at = append(b.at, time.Now())
		b.acks = append(b.acks, ack)
		b.mu.Unlock()
	}
}

// Through a whole restart that keeps the sessions (the old process held by the
// swap's step, the new one loading the world, then building the characters one
// after another) the client is sent a packet every beat or so, numbered one
// after another with no number twice and none skipped, acknowledging what it
// was told before; and the resumed session's own packets number on from the
// last beat.
func TestTheHoldIsNeverSilent(t *testing.T) {
	old := keepBeatEvery
	keepBeatEvery = 40 * time.Millisecond
	t.Cleanup(func() { keepBeatEvery = old })

	l, sm, client := keepListener(t)
	s := sm.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061)
	ack := s.Connection().LastReceivedMessageSeq()
	stop := make(chan struct{})
	got := &beats{}
	var readers sync.WaitGroup
	readers.Add(1)
	go func() { defer readers.Done(); got.read(t, client, stop) }()
	defer func() { close(stop); readers.Wait() }()

	dir := t.TempDir()
	file := filepath.Join(dir, "keep-sessions.json")
	hold := filepath.Join(dir, "keep.hold")
	os.WriteFile(hold, nil, 0o644)
	go func() { time.Sleep(400 * time.Millisecond); os.Remove(hold) }() // the swap's step
	var env []string
	execed := make(chan struct{})
	o := KeepOptions{File: file, Hold: hold, Saved: filepath.Join(dir, "keep.saved"), HoldMax: 5 * time.Second,
		Exe: "/bin/true", Exec: func(exe string, args, e []string) error {
			env = e
			close(execed)
			select {}
		}}
	go l.KeepRestart(o)
	select {
	case <-execed:
	case <-time.After(5 * time.Second):
		t.Fatal("no exec")
	}
	var fdv, beatv string
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, KeepFDEnv+"="); ok {
			fdv = v
		}
		if v, ok := strings.CutPrefix(e, KeepBeatsEnv+"="); ok {
			beatv = v
		}
	}
	if n, err := strconv.Atoi(beatv); err != nil || n < 5 {
		t.Fatalf("%s=%q: the old process beat a 400 ms hold at 40 ms", KeepBeatsEnv, beatv)
	}

	// the next image: the socket and the beats, the snapshot read before the world loads
	t.Setenv(KeepFDEnv, fdv)
	t.Setenv(KeepBeatsEnv, beatv)
	conn, err := InheritedConn()
	if err != nil || conn == nil {
		t.Fatalf("InheritedConn: %v %v", conn, err)
	}
	defer conn.Close()
	n := InheritedBeats()
	if os.Getenv(KeepBeatsEnv) != "" {
		t.Fatal("the beats' count is left for a later image")
	}
	snap, err := ReadSnapshot(file, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	snap.Beaten(n)
	StartSnapshotBeat(conn, snap)
	time.Sleep(300 * time.Millisecond) // the world's load
	sm2 := session.NewManager()
	l2 := NewListener(config.DefaultConfig(), sm2, nil, entity.NewWorld())
	l2.Adopt(conn)
	testResumeEach = func(*session.Session) { time.Sleep(300 * time.Millisecond) } // a character's building
	t.Cleanup(func() { testResumeEach = nil })
	if resumed, _ := l2.ResumeKept(snap, time.Now()); resumed != 1 {
		t.Fatalf("resumed %d", resumed)
	}
	s2 := sm2.GetSessionByAddr(client.LocalAddr().(*net.UDPAddr), 0x2EC7D061)
	s2.Queue().SetBodyFlags(0x02) // something to say: an ack
	l2.flushSession(s2)
	time.Sleep(100 * time.Millisecond)

	got.mu.Lock()
	defer got.mu.Unlock()
	if len(got.numbers) < 20 {
		t.Fatalf("%d packets in a hold of over a second at a beat of 40 ms", len(got.numbers))
	}
	for i, num := range got.numbers {
		if num != uint16(6+i) {
			t.Fatalf("packet %d is numbered %d, want %d (all: %v)", i, num, 6+i, got.numbers)
		}
		if got.acks[i] != ack {
			t.Fatalf("packet %d acknowledges %d, the client was told %d", i, got.acks[i], ack)
		}
		if i > 0 {
			if gap := got.at[i].Sub(got.at[i-1]); gap > 5*keepBeatEvery {
				t.Fatalf("silent %v between packets %d and %d", gap, num-1, num)
			}
		}
	}
}

// A beat is, byte for byte, the packet PreparePacket makes for a session with
// nothing to send but its acknowledgements (endpoints, header, instance, the
// object id, the acks as they stood, no message); and it leaves the reliable
// messages the session holds as they were, to go next under their own numbers.
func TestABeatIsAnAckOnlyPacket(t *testing.T) {
	a, b := keptNumbersSession(t, 10), keptNumbersSession(t, 10)
	for i := 0; i < 5; i++ {
		a.Connection().LastSentPacketSeq()
		b.Connection().LastSentPacketSeq()
	}
	beat := beatPacket(a)
	b.Queue().SetBodyFlags(0x02) // a guaranteed ack, nothing queued
	if want := NewRDPHandler(nil).PreparePacket(b); string(beat) != string(want) {
		t.Fatalf("the beat\n%x\nPreparePacket's ack-only packet\n%x", beat, want)
	}
	if num, _, ok := serverReport(beat, 0x2EC7D061); !ok || num != 6 {
		t.Fatalf("the beat is packet %d (%v), want 6", num, ok)
	}

	s := keptNumbersSession(t, 0)
	s.Queue().AddWithSequence(numberedLine(7))
	before := len(s.Keep().Pending)
	beatPacket(s)
	if after := len(s.Keep().Pending); after != before || before == 0 {
		t.Fatalf("pending %d before the beat, %d after", before, after)
	}
	if nums := reliableNumbers(t, NewRDPHandler(nil).PreparePacket(s)); len(nums) != 1 || nums[0] != 1 {
		t.Fatalf("the message after the beat goes as %v, want [1]", nums)
	}
}
