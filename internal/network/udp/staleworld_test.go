package udp

import (
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// report is a 0x40 of the client's own world and place (the listener's
// layout: byte 0 the world, 1-9 x, y, z as 24-bit fractions).
func report(seq uint16, world byte, x, y, z float32) *protocol.Message {
	d := make([]byte, 40)
	d[0] = world
	put := func(at int, v, span, off float64) {
		r := uint32((v + off) / span * float64(0xFFFFFF))
		d[at], d[at+1], d[at+2] = byte(r>>16), byte(r>>8), byte(r)
	}
	put(1, float64(x), 36000, 4000)
	put(4, float64(y), 2000, 1000)
	put(7, float64(z), 36000, 4000)
	m := protocol.NewMessage(protocol.MessageType(0x40), 0)
	m.SetData(d)
	m.Sequence = seq
	return m
}

// Petition 1748 (the GM Jibrish: "extremely slow zoning load time"): his #goto
// from Odus to Freeport, taken just as a restart's session came back, met a
// report from Odus the client had sent before it had the teleport. That report
// was taken as the arrival: the hold on his state channels lifted, Freeport's
// scene sent into the load, and he stood at Odus's spot in Tunaria; then the
// restart's dump went a second later, in the middle of the load. Now a report
// of the world left, while the hold lasts, neither ends the load nor moves him,
// and the resume dump waits for the load (WorldLoadUnderway). The report of
// the new world does both, as before.
func TestAReportFromTheWorldLeftIsNotTheArrival(t *testing.T) {
	w := entity.NewWorld()
	l := &Listener{gameWorld: w}
	sess := session.NewSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9}, 1, 1, 1, 2, false)
	sess.SetCharacterInWorld(true)
	p := w.CreatePlayer(sess, &database.CharacterRecord{CharName: "Jibrish", ServerID: 44, Level: 60, World: 0, X: 25274, Y: 54, Z: 15719})
	if p == nil {
		t.Fatal("no player")
	}
	// the teleport: world 2 to world 0, its hold
	sess.HoldStateChannels(15 * time.Second)
	now := time.Now()
	if !sess.WorldLoadUnderway(now, time.Minute) {
		t.Fatal("no load under way after the hold")
	}
	if !l.processClientUpdate(sess, report(10, 2, 10854, 107, 6981)) {
		t.Fatal("the stale report did not decode")
	}
	if !sess.StateChannelsHeld() || !sess.WorldLoadUnderway(now, time.Minute) {
		t.Error("a report of the world left ended the load")
	}
	if p.X < 25000 || p.World != 0 {
		t.Errorf("a report of the world left moved him to (%.0f, %.0f) world %d", p.X, p.Z, p.World)
	}
	// the arrival: its own world
	if !l.processClientUpdate(sess, report(11, 0, 25280, 54, 15720)) {
		t.Fatal("the arrival did not decode")
	}
	if sess.StateChannelsHeld() || sess.WorldLoadUnderway(now, time.Minute) {
		t.Error("the arrival did not end the load")
	}
	if p.X < 25270 || p.X > 25290 {
		t.Errorf("the arrival left him at x %.0f", p.X)
	}
	// a load under way longer than most counts no more (a client that never
	// arrives still gets its dump)
	sess.HoldStateChannels(15 * time.Second)
	if sess.WorldLoadUnderway(time.Now().Add(2*time.Minute), time.Minute) {
		t.Error("a load two minutes old under way for a one-minute most")
	}
	// the hold over (0x0049 or its deadline), a report of another world is
	// taken as ever
	sess.ReleaseStateChannels()
	l.processClientUpdate(sess, report(12, 2, 10854, 107, 6981))
	if sess.WorldLoadUnderway(time.Now(), time.Minute) {
		t.Error("with no hold, a report did not end the load as before")
	}
}
