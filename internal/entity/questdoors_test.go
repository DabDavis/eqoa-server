package entity

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Estate's door (petition 2212) as table questdoors has it
// (tools/ct_guardian_door_20261116.py): the Guardian's post, the porch up to
// his middle, the way in past him, where one turned back is put.
const (
	gdID           EntityID = 118859
	gdX, gdY, gdZ           = float32(25706.209), float32(54.126), float32(3230.661)
	gdOutX, gdOutZ          = float32(25700.0), float32(3231.0)
	gdOutY                  = float32(54.3)
)

var gdDoor = database.QuestDoor{
	NPCID:   int(gdID),
	Inner:   []database.WallBox{{X0: 25707.5, Y0: 44, Z0: 3227, X1: 25718, Y1: 58, Z1: 3235}},
	Outer:   []database.WallBox{{X0: 25690, Y0: 51, Z0: 3215, X1: 25706.2, Y1: 60, Z1: 3250}},
	Outside: [3]float32{gdOutX, gdOutY, gdOutZ},
	Text:    "The Guardian bars your way.",
}

// doorRig is a character at the Estate's door, its session, a clock.
type doorRig struct {
	t    *testing.T
	w    *World
	sess *session.Session
	p    *Player
	g    *NPC
	now  time.Time
}

// newDoorRig logs a character in at (x, y, z) (its saved place) with the
// Guardian at his post, in movecheck's mode.
func newDoorRig(t *testing.T, mode MoveCheckMode, x, y, z float32) *doorRig {
	t.Helper()
	was := MoveCheck()
	SetMoveCheck(mode)
	t.Cleanup(func() { SetMoveCheck(was) })
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7000}, 1, 1, 1, 2, true)
	p := w.CreatePlayer(nil, &database.CharacterRecord{CharName: "Hoofbite", Level: 60, X: x, Y: y, Z: z})
	if p == nil {
		t.Fatal("no player")
	}
	w.UpdatePosition(p.ServerID, x, y, z, 0)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	g := &NPC{ServerID: gdID, Name: "The Guardian", Level: 62, HP: 500, MaxHP: 500, Size: 1.45,
		X: gdX, Y: gdY, Z: gdZ, SpawnX: gdX, SpawnY: gdY, SpawnZ: gdZ}
	w.NPCs[gdID] = g
	w.SpatialIndex.Insert(gdID, gdX, gdZ)
	w.SetQuestDoors([]database.QuestDoor{gdDoor})
	return &doorRig{t: t, w: w, sess: sess, p: p, g: g, now: time.Date(2026, 11, 16, 1, 15, 11, 0, time.UTC)}
}

// report is a 0x40 from (x, y, z) 100 ms after the last.
func (r *doorRig) report(x, y, z float32) {
	r.now = r.now.Add(100 * time.Millisecond)
	r.w.ApplyClientUpdate(r.p.ServerID, ClientUpdate{X: x, Y: y, Z: z, World: r.p.World, HasWorld: true, At: r.now})
}

// walk reports a walk to (x, y, z) a unit a report; it says whether every
// report was taken.
func (r *doorRig) walk(x, y, z float32) bool {
	fx, fy, fz := r.p.X, r.p.Y, r.p.Z
	n := int(max(abs32(x-fx), abs32(z-fz))) + 1
	for i := 1; i <= n; i++ {
		f := float32(i) / float32(n)
		px, py, pz := fx+(x-fx)*f, fy+(y-fy)*f, fz+(z-fz)*f
		r.report(px, py, pz)
		if r.p.X != px || r.p.Z != pz {
			return false
		}
	}
	return true
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// sent is the opcodes queued for the client since the last look.
func (r *doorRig) sent() []protocol.GameOpcode {
	var out []protocol.GameOpcode
	for _, m := range r.sess.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, protocol.GameOpcode(binary.LittleEndian.Uint16(d)))
		}
	}
	return out
}

func has(ops []protocol.GameOpcode, op protocol.GameOpcode) int {
	n := 0
	for _, o := range ops {
		if o == op {
			n++
		}
	}
	return n
}

// atOutside is whether the server holds the character at the door's outside place.
func (r *doorRig) atOutside() bool {
	return r.p.X == gdOutX && r.p.Y == gdOutY && r.p.Z == gdOutZ
}

// A player's relog: logged out in the Guardian's body (25705.8, 55.0,
// 3231.9), logged in there, walked on in to Kithgrin. Turned back to the
// porch, told, and every report from inside before the client heard of it
// refused; a second try is turned back again. In both movecheck modes a
// server runs.
func TestADoorTurnsBackARelogInTheGuardiansBody(t *testing.T) {
	for _, mode := range []MoveCheckMode{MoveCheckLog, MoveCheckEnforce, MoveCheckOff} {
		t.Run(mode.String(), func(t *testing.T) {
			r := newDoorRig(t, mode, 25705.8, 55.0, 3231.9)
			r.sent()
			r.report(25705.8, 55.0, 3231.9) // the client there, as it logs in
			if r.walk(25714.5, 48.9, 3228.7) {
				t.Fatal("walked on in past the Guardian")
			}
			if !r.atOutside() {
				t.Fatalf("turned back to (%.1f, %.1f, %.1f), want the porch's (%.1f, %.1f, %.1f)", r.p.X, r.p.Y, r.p.Z, gdOutX, gdOutY, gdOutZ)
			}
			ops := r.sent()
			if has(ops, protocol.OpcodeTeleport) != 1 || has(ops, protocol.OpcodeClientMessage) != 1 {
				t.Fatalf("sent %v, want one teleport and one line", ops)
			}
			// the client, not yet sent back, goes on in: none of it is taken
			for i := 0; i < 5; i++ {
				r.report(25709+float32(i), 54.1, 3231)
				if !r.atOutside() {
					t.Fatalf("a report from inside after the turn back was taken: (%.1f, %.1f, %.1f)", r.p.X, r.p.Y, r.p.Z)
				}
			}
			r.now = r.now.Add(doorResend)
			r.report(25712, 54.1, 3231)
			if !r.atOutside() || has(r.sent(), protocol.OpcodeTeleport) != 1 {
				t.Fatal("the turn back was not sent again after doorResend")
			}
			// it arrives, and tries again
			r.report(gdOutX, gdOutY, gdOutZ)
			if r.p.door.back {
				t.Fatal("the client's arrival did not end the turn back")
			}
			if r.walk(25712, 54.4, 3232) {
				t.Fatal("the second try walked in")
			}
			if !r.atOutside() {
				t.Fatal("the second try was not put on the porch")
			}
			// on the porch it walks about freely
			r.report(gdOutX, gdOutY, gdOutZ)
			if !r.walk(25695, 54.1, 3240) || !r.walk(25705.5, 54.1, 3231) {
				t.Fatal("a walk on the porch was not taken")
			}
		})
	}
}

// The client's first reports after a login already through the door (it ran
// forward as it logged in): turned back as well.
func TestADoorTurnsBackALoginsFirstReportInside(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25705.8, 54.1, 3231.9)
	r.report(25708.0, 54.1, 3231.5) // its first report, past him
	if !r.atOutside() {
		t.Fatalf("a login's first report past the Guardian taken: (%.1f, %.1f, %.1f)", r.p.X, r.p.Y, r.p.Z)
	}
}

// A login inside (one let in logged out there, or the talisman's landing) is
// inside: not turned back, and walks the house and back up the ramp.
func TestADoorLeavesALoginInsideAlone(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25714.5, 48.9, 3228.7)
	r.report(25714.5, 48.9, 3228.7)
	if !r.walk(25712, 54.4, 3232) || !r.walk(25708, 54.1, 3231) || !r.walk(25716, 49, 3233) {
		t.Fatal("a character logged in inside was turned back")
	}
	if has(r.sent(), protocol.OpcodeTeleport) != 0 {
		t.Fatal("a teleport was sent to a character inside")
	}
}

// With the Guardian out of the world (Cazic's Scent's ten minutes), dead or
// off his post, the door is open; one let in stays in when he is back and
// may walk out, and back in only while he is away again.
func TestADoorOpenWhileItsKeeperIsAway(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25700, 54.3, 3231)
	r.report(25700, 54.3, 3231)
	r.g.HiddenUntil = r.now.Add(10 * time.Minute)
	if !r.walk(25714.5, 48.9, 3228.7) {
		t.Fatal("turned back while the Guardian was out of the world")
	}
	r.g.HiddenUntil = time.Time{}
	// back at his post: the one inside goes about the house, into the
	// doorway behind him and back in
	if !r.walk(25707.0, 54.1, 3231) || !r.walk(25712, 54.4, 3232) || !r.walk(25716, 49, 3233) || !r.walk(25710, 54.1, 3231) {
		t.Fatal("one let in was turned back once the Guardian was back")
	}
	// out on to the porch (the walk out through him is CLIENT's to stop;
	// the server lets it go), and in again: turned back
	if !r.walk(25700, 54.3, 3231) {
		t.Fatal("the walk out was not taken")
	}
	if r.walk(25712, 54.4, 3232) {
		t.Fatal("walked back in past the Guardian")
	}
	// dead
	r.report(gdOutX, gdOutY, gdOutZ)
	r.g.Dead = true
	if !r.walk(25712, 54.4, 3232) {
		t.Fatal("turned back by a dead Guardian")
	}
	r.g.Dead = false
	r.walk(25700, 54.3, 3231)
	// chasing someone 20 off his post
	r.g.X = gdX - 20
	if !r.walk(25712, 54.4, 3232) {
		t.Fatal("turned back by a Guardian off his post")
	}
}

// A GM, and one the quest engine says passes the door (its quest's step, its
// item), walk through.
func TestADoorLetsThroughAGMAndAPasser(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25700, 54.3, 3231)
	r.report(25700, 54.3, 3231)
	r.p.GM = true
	if !r.walk(25712, 54.4, 3232) {
		t.Fatal("a GM was turned back")
	}
	r.p.GM = false
	r.walk(25700, 54.3, 3231)
	r.w.SetQuestState(r.p.CharServerID, map[EntityID]bool{gdID: true}, nil)
	if !r.walk(25712, 54.4, 3232) {
		t.Fatal("a passer was turned back")
	}
	r.w.ForgetQuestState(r.p.CharServerID)
	r.walk(25700, 54.3, 3231)
	if r.walk(25712, 54.4, 3232) {
		t.Fatal("forgotten, it still passed")
	}
}

// A teleport from the porch past the door (the talisman's landing) is the
// server's: the porch's last reports, from before the client heard of it,
// do not make the landing a way in.
func TestADoorTakesATeleportPastIt(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25700, 54.3, 3231)
	r.report(25700, 54.3, 3231)
	r.w.UpdatePosition(r.p.ServerID, 25712, 54.4, 3232, 0) // the talisman's landing
	r.report(25700.5, 54.3, 3231)                          // the porch, before the client heard
	r.report(25701, 54.3, 3231)
	r.report(25712, 54.4, 3232) // there
	if r.p.X != 25712 || r.p.door.back {
		t.Fatalf("the teleport's landing was turned back: (%.1f, %.1f, %.1f)", r.p.X, r.p.Y, r.p.Z)
	}
	if !r.walk(25716, 49, 3233) {
		t.Fatal("the walk on from the landing was turned back")
	}
}

// A door's item is a watched item, and one near the door is near a holding
// rule (the quest engine looks at what it holds there).
func TestADoorsItemIsWatched(t *testing.T) {
	r := newDoorRig(t, MoveCheckLog, 25700, 54.3, 3231)
	d := gdDoor
	d.Item = 1231
	r.w.SetQuestDoors([]database.QuestDoor{d})
	if got := r.w.WatchedItems(); len(got) != 1 || got[0] != 1231 {
		t.Fatalf("watched items %v", got)
	}
	if !r.w.NearHoldingRule(r.p) {
		t.Fatal("6 off the door is not near a holding rule")
	}
	r.w.UpdatePosition(r.p.ServerID, 25000, 54, 3000, 0)
	if r.w.NearHoldingRule(r.p) {
		t.Fatal("far off the door is near a holding rule")
	}
}
