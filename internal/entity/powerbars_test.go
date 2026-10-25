package entity

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Others' power (powerbars.go) on the assist world (assist_test.go), whose two
// sessions number the same entities differently: a set must name each player
// as the receiving session knows it.

// powerSets takes the OpcodePowerBarsData bodies queued for s.
func powerSets(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodePowerBarsData {
			out = append(out, d[2:])
		}
	}
	return out
}

// bars is a set's body: a u8 count, then each (u32 id, u8 power).
func bars(b ...uint32) []byte {
	out := []byte{byte(len(b) / 2)}
	for i := 0; i < len(b); i += 2 {
		out = binary.LittleEndian.AppendUint32(out, b[i])
		out = append(out, byte(b[i+1]))
	}
	return out
}

// newPowerWorld is the assist world with power: the asker 50 of 100 (127 of
// 255), the leader 30 of 120 (63).
func newPowerWorld(t *testing.T) *assistWorld {
	aw := newAssistWorld(t)
	aw.a.Power, aw.a.MaxPower = 50, 100
	aw.b.Power, aw.b.MaxPower = 30, 120
	aw.b.TargetObjectID = NoAssist
	aw.a.TargetObjectID = NoAssist
	return aw
}

// pass runs one power pass (every fifth tick).
func (aw *assistWorld) pass() {
	aw.w.TickCount += 5
	aw.w.powerSyncSystem()
}

// Nothing goes to a client that never asked; one that asked is sent its set
// at once, again only when it changes, and nothing once it stopped.
func TestPowerBarsOnlyWhenAsked(t *testing.T) {
	aw := newPowerWorld(t)
	aw.group()
	aw.pass()
	if got := append(powerSets(aw.asker), powerSets(aw.leader)...); len(got) != 0 {
		t.Fatalf("sent to clients that never asked: %x", got)
	}
	if !aw.w.SetPowerBars(aw.asker, true) {
		t.Fatal("the asker has a player")
	}
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 63); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("asked: %x, want one %x (the leader as the asker knows it)", got, want)
	}
	if got := powerSets(aw.leader); len(got) != 0 {
		t.Fatalf("the leader never asked and was sent %x", got)
	}
	aw.pass()
	if got := powerSets(aw.asker); len(got) != 0 {
		t.Fatalf("unchanged, sent again: %x", got)
	}
	aw.b.Power = 60
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 127); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader's power changed: %x, want %x", got, want)
	}
	aw.w.SetPowerBars(aw.asker, false)
	aw.b.Power = 90
	aw.pass()
	if got := powerSets(aw.asker); len(got) != 0 {
		t.Fatalf("stopped, still sent %x", got)
	}
	// asked again: the set at once though it is the one sent before
	aw.b.Power = 60
	aw.w.SetPowerBars(aw.asker, true)
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 127); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("asked again: %x, want %x", got, want)
	}
}

// The target and its target, by the asker's ids: the leader's target (its
// own 100001, the asker) is the asker's own object id. The control: the
// leader's 100001 read in the asker's table is the leader himself, so a set
// built off the raw id lacks the asker. An NPC has no power and no bar, but
// its enemy does; nothing at all is an empty set.
func TestPowerBarsTargetAndItsTarget(t *testing.T) {
	aw := newPowerWorld(t)
	aw.w.SetPowerBars(aw.asker, true)
	aw.pass()
	if got, want := powerSets(aw.asker), bars(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("no group, no target: %x, want one empty set", got)
	}

	aw.a.TargetObjectID, aw.b.TargetObjectID = aLeader, bAsker
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 63, askerSelf, 127); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader, who targets the asker: %x, want %x", got, want)
	}
	if e := aw.w.entitySeenByLocked(aw.asker, bAsker); e != aw.b {
		t.Fatalf("control: the leader's own %d read in the asker's table should be the leader, is %v", bAsker, e)
	}

	aw.b.TargetObjectID = bOrc
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 63); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader, who targets the orc: %x, want %x (a creature has no bar)", got, want)
	}

	// the same set again (nothing goes out), reached the other way
	aw.a.TargetObjectID, aw.orc.CurrentEnemy = aOrc, aw.b.ServerID
	aw.pass()
	if got := powerSets(aw.asker); len(got) != 0 {
		t.Fatalf("the orc, fighting the leader: the same set, sent again: %x", got)
	}
	if got, want := encodePowerBars(aw.w.powerBarsLocked(aw.asker, aw.a)), bars(aLeader, 63); !bytes.Equal(got, want) {
		t.Fatalf("the orc, fighting the leader: %x, want %x", got, want)
	}

	aw.a.TargetObjectID = askerSelf
	aw.pass()
	if got, want := powerSets(aw.asker), bars(askerSelf, 127); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the asker itself: %x, want %x", got, want)
	}

	aw.a.TargetObjectID = NoAssist
	aw.pass()
	if got, want := powerSets(aw.asker), bars(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("no target any more: %x, want the empty set", got)
	}
}

// A member out of sight is named by the roster's stand-in; a member is named
// once when it is the target too; a player with no power has no bar.
func TestPowerBarsGroup(t *testing.T) {
	aw := newPowerWorld(t)
	aw.group()
	aw.w.SetPowerBars(aw.asker, true)
	aw.a.TargetObjectID = aLeader
	aw.pass()
	if got, want := powerSets(aw.asker), bars(aLeader, 63); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader, a member and the target: %x, want %x", got, want)
	}

	vs := GetOrCreateVisibility(aw.asker)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = 0, 0
	aw.pass()
	if got, want := powerSets(aw.asker), bars(groupFarID(aw.b.ServerID), 63); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader out of sight: %x, want %x (the roster's stand-in)", got, want)
	}

	aw.b.MaxPower = 0
	aw.pass()
	if got, want := powerSets(aw.asker), bars(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("a member with no power: %x, want the empty set", got)
	}
}

// The body: a u8 count, then each u32 id and u8 power.
func TestEncodePowerBars(t *testing.T) {
	if got := encodePowerBars(nil); !bytes.Equal(got, []byte{0}) {
		t.Fatalf("none: %x", got)
	}
	if got, want := encodePowerBars([]powerBar{{0x01020304, 9}, {0x7F0003EA, 255}}), []byte{2, 4, 3, 2, 1, 9, 0xEA, 3, 0, 0x7F, 255}; !bytes.Equal(got, want) {
		t.Fatalf("two: %x, want %x", got, want)
	}
	if got, ok := powerOf255(&Player{Power: 200, MaxPower: 100}); !ok || got != 255 {
		t.Fatalf("over the most: %d %v, want 255", got, ok)
	}
	if _, ok := powerOf255(&Player{Power: 5}); ok {
		t.Fatal("no power at all: a bar")
	}
}
