package udp

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A session's messages are handled one at a time, each whole, under its item
// lists' hold (session.LockItems): its packets come each in a goroutine of its
// own, and two of its handlers at once put its item lists and its client's apart
// (Live petition 5401, handlers/itemsync.go). Another session's go on beside
// them, and whatever else takes the hold (the World's item hooks) waits for the
// handler.
func TestASessionsMessagesAreHandledOneAtATime(t *testing.T) {
	r := NewHandlerRegistry()
	var in, most atomic.Int32
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	op := protocol.GameOpcode(0x7EF0)
	r.Register(op, func(*session.Session, *protocol.Message) {
		n := in.Add(1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		in.Add(-1)
	})
	a := session.NewSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 1), Port: 1}, 1, 1, 0, 0, true)
	b := session.NewSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 2), Port: 2}, 2, 2, 0, 0, true)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)

	var wg sync.WaitGroup
	for _, s := range []*session.Session{a, a, b} {
		wg.Add(1)
		go func(s *session.Session) {
			defer wg.Done()
			r.Handle(s, msg)
		}(s)
	}
	// a's first and b's are in; a's second waits for a's first
	for k := 0; k < 2; k++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("the two sessions' handlers did not run beside each other")
		}
	}
	select {
	case <-entered:
		t.Fatal("a session's second message was handled while its first was")
	case <-time.After(100 * time.Millisecond):
	}
	// and a hook taking a's hold waits too
	hooked := make(chan struct{})
	go func() {
		a.LockItems()
		close(hooked)
		a.UnlockItems()
	}()
	select {
	case <-hooked:
		t.Fatal("a's item lists were taken while its handler held them")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	<-hooked
	if m := most.Load(); m != 2 {
		t.Fatalf("at most %d handlers at once, want 2 (one a session)", m)
	}
}
