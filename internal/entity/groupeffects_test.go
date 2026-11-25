package entity

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The group members' effects (groupeffects.go) on the assist world (assist_test.go), whose
// two sessions number the same entities differently: a set must name each member as the
// receiving session knows it.

// fxSets takes the OpcodeGroupEffectsData bodies queued for s.
func fxSets(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeGroupEffectsData {
			out = append(out, d[2:])
		}
	}
	return out
}

// fxMember is one member of a set's body: its id and icons.
type fxMember struct {
	id    uint32
	icons []uint32
}

// fxSet is a set's body: a u8 count, then each member's u32 id, u8 count and u32 icons.
func fxSet(ms ...fxMember) []byte {
	out := []byte{byte(len(ms))}
	for _, m := range ms {
		out = binary.LittleEndian.AppendUint32(out, m.id)
		out = append(out, byte(len(m.icons)))
		for _, i := range m.icons {
			out = binary.LittleEndian.AppendUint32(out, i)
		}
	}
	return out
}

// fxPass runs one pass of the members' effects (every fifth tick) at now.
func (aw *assistWorld) fxPass(now time.Time) {
	aw.w.TickCount += 5
	aw.w.groupEffectsSystem(now)
}

// one wants exactly one set queued for s, equal to want.
func oneFxSet(t *testing.T, s *session.Session, want []byte, what string) {
	t.Helper()
	if got := fxSets(s); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("%s: %x, want one %x", what, got, want)
	}
}

// Nothing goes to a client that never asked; one that asked is sent its set at once, by
// its own ids, again only when an effect goes on or runs out, and nothing once it stopped.
// An unnamed effect and one run out are no icon; the time left counts for nothing.
func TestGroupEffectsOnlyWhenAskedAndChanged(t *testing.T) {
	aw := newAssistWorld(t)
	now := time.Unix(1_800_000_000, 0)
	aw.group()
	aw.fxPass(now)
	if got := append(fxSets(aw.asker), fxSets(aw.leader)...); len(got) != 0 {
		t.Fatalf("sent to clients that never asked: %x", got)
	}
	if !aw.w.SetGroupEffects(aw.asker, true) {
		t.Fatal("the asker has a player")
	}
	aw.fxPass(now)
	oneFxSet(t, aw.asker, fxSet(fxMember{aLeader, nil}), "asked, the leader bare")
	if aLeader == uint32(aw.b.ServerID) {
		t.Fatal("control: the asker's id for the leader must differ from his ServerID")
	}

	courage := &ActiveEffect{SpellID: 11, Name: "Courage", Icon: 0xA1B2C3D4, Until: now.Add(30 * time.Minute)}
	aw.b.Effects = append(aw.b.Effects,
		courage,
		&ActiveEffect{SpellID: 12, Icon: 0x5, Until: now.Add(time.Minute)},                      // no name: no icon
		&ActiveEffect{SpellID: 13, Name: "Gone", Icon: 0x6, Until: now.Add(-time.Second)},       // run out
		&ActiveEffect{SpellID: 14, Name: "Breeze", Icon: 0x0BADF00D, Until: now.Add(time.Hour)}, // a second
	)
	aw.fxPass(now)
	oneFxSet(t, aw.asker, fxSet(fxMember{aLeader, []uint32{0xA1B2C3D4, 0x0BADF00D}}), "two buffs on the leader")
	if got := fxSets(aw.leader); len(got) != 0 {
		t.Fatalf("the leader never asked and was sent %x", got)
	}

	// later, the same buffs with less time left: nothing goes out
	aw.fxPass(now.Add(10 * time.Second))
	if got := fxSets(aw.asker); len(got) != 0 {
		t.Fatalf("unchanged but for the time left, sent again: %x", got)
	}

	// Courage runs out
	aw.fxPass(now.Add(31 * time.Minute))
	oneFxSet(t, aw.asker, fxSet(fxMember{aLeader, []uint32{0x0BADF00D}}), "Courage ran out")

	// stopped: nothing; asked again: the set at once though it is the one sent before
	aw.w.SetGroupEffects(aw.asker, false)
	courage.Until = now.Add(time.Hour)
	aw.fxPass(now)
	if got := fxSets(aw.asker); len(got) != 0 {
		t.Fatalf("stopped, still sent %x", got)
	}
	aw.w.SetGroupEffects(aw.asker, true)
	aw.fxPass(now)
	aw.w.SetGroupEffects(aw.asker, true)
	aw.fxPass(now)
	if got := fxSets(aw.asker); len(got) != 2 {
		t.Fatalf("asked twice: %d sets, want 2 (each ask sends the set whatever it is)", len(got))
	}

	// out of the group: an empty set, once
	aw.a.GroupID, aw.b.GroupID = 0, 0
	aw.fxPass(now)
	oneFxSet(t, aw.asker, fxSet(), "out of the group")
	aw.fxPass(now)
	if got := fxSets(aw.asker); len(got) != 0 {
		t.Fatalf("still out of the group, sent again: %x", got)
	}
}

// Each client is sent the OTHER members, by its own ids: the leader, asking too, gets the
// asker by the leader's id for him; a merc in the group has its icons; at most
// GroupEffectsMost a member.
func TestGroupEffectsOthersByTheirIDsAndMercs(t *testing.T) {
	aw := newAssistWorld(t)
	now := time.Unix(1_800_000_000, 0)
	aw.group()
	merc := &NPC{ServerID: 2101, Name: "Hired Blade", Owner: aw.b.ServerID, merc: &mercState{}, HP: 50, MaxHP: 50}
	aw.w.NPCs[merc.ServerID] = merc
	aw.w.groups[7].Members = append(aw.w.groups[7].Members, merc.ServerID)
	merc.Effects = []*ActiveEffect{{SpellID: 21, Name: "Ward", Icon: 0x77, Until: now.Add(time.Minute)}}
	for i := range GroupEffectsMost + 3 {
		aw.a.Effects = append(aw.a.Effects, &ActiveEffect{SpellID: 100 + i, Name: "Buff", Icon: uint32(1000 + i), Until: now.Add(time.Hour)})
	}
	aw.w.SetGroupEffects(aw.leader, true)
	aw.fxPass(now)
	got := fxSets(aw.leader)
	if len(got) != 1 {
		t.Fatalf("the leader asked: %d sets, want 1", len(got))
	}
	set := got[0]
	if set[0] != 2 {
		t.Fatalf("members: %d, want 2 (the asker and the merc, never the leader himself): %x", set[0], set)
	}
	if id := binary.LittleEndian.Uint32(set[1:]); id != bAsker {
		t.Fatalf("the asker as the leader knows him: %d, want %d", id, bAsker)
	}
	if n := int(set[5]); n != GroupEffectsMost {
		t.Fatalf("the asker's icons: %d, want the most, %d", n, GroupEffectsMost)
	}
	if first := binary.LittleEndian.Uint32(set[6:]); first != 1000 {
		t.Fatalf("the asker's first icon: %d, want the first to go on, 1000", first)
	}
	rest := set[6+4*GroupEffectsMost:]
	if want := fxSet(fxMember{aw.w.groupIDSeenByLocked(aw.leader, merc.ServerID), []uint32{0x77}})[1:]; !bytes.Equal(rest, want) {
		t.Fatalf("the merc: %x, want %x", rest, want)
	}
	// a dead merc is off the group's window, and so off the set
	merc.Dead = true
	aw.fxPass(now)
	if got := fxSets(aw.leader); len(got) != 1 || got[0][0] != 1 {
		t.Fatalf("the merc dead: %x, want one set of the asker alone", got)
	}
}

// fxEffect is one effect of a version 2 set: its icon, ms left and name.
type fxEffect struct {
	icon uint32
	left uint32
	name string
}

// fxSet2 is a version 2 body of one member: its id, then each effect's u32 icon, u32 ms
// left and name (a u8 byte count and UTF-8).
func fxSet2(id uint32, fx ...fxEffect) []byte {
	out := binary.LittleEndian.AppendUint32([]byte{1}, id)
	out = append(out, byte(len(fx)))
	for _, e := range fx {
		out = binary.LittleEndian.AppendUint32(out, e.icon)
		out = binary.LittleEndian.AppendUint32(out, e.left)
		out = append(out, byte(len(e.name)))
		out = append(out, e.name...)
	}
	return out
}

// Version 2 (petition 2962, Graga: "the buff icons of group members should reveal the
// buff when mouse hovering and have timers like the new buff timers"): each icon with its
// time left and name, a slot with no end the server keeps as 0; sent again only when an
// effect goes on, runs out or is cast again, never for its time running down; an answer
// past the newest reads as the newest; and a client that answered 1 is sent icons alone,
// byte for byte as before (the control).
func TestGroupEffectsVersion2NamesAndTimes(t *testing.T) {
	aw := newAssistWorld(t)
	now := time.Unix(1_800_000_000, 0)
	aw.group()
	courage := &ActiveEffect{SpellID: 11, Name: "Courage", Icon: 0xA1B2C3D4, Until: now.Add(30 * time.Minute)}
	aw.b.Effects = append(aw.b.Effects, courage,
		&ActiveEffect{SpellID: 14, Name: "Breeze", Icon: 0x0BADF00D, Until: now.Add(time.Hour)})
	aw.b.Buffs = []BuffSlot{{Icon: 0x99, Name: "Marked"}} // a GM's #effect: no end the server keeps
	if !aw.w.SetGroupEffectsVersion(aw.asker, 2) {
		t.Fatal("the asker has a player")
	}
	aw.fxPass(now)
	oneFxSet(t, aw.asker, fxSet2(aLeader,
		fxEffect{0xA1B2C3D4, 1_800_000, "Courage"}, fxEffect{0x0BADF00D, 3_600_000, "Breeze"}, fxEffect{0x99, 0, "Marked"}),
		"version 2: names and times")

	// later, the same effects with less time left: nothing goes out
	aw.fxPass(now.Add(10 * time.Second))
	if got := fxSets(aw.asker); len(got) != 0 {
		t.Fatalf("unchanged but for the time left, sent again: %x", got)
	}
	// Courage cast again: the new time
	courage.Until = now.Add(40 * time.Minute)
	aw.fxPass(now.Add(10 * time.Second))
	oneFxSet(t, aw.asker, fxSet2(aLeader,
		fxEffect{0xA1B2C3D4, 2_390_000, "Courage"}, fxEffect{0x0BADF00D, 3_590_000, "Breeze"}, fxEffect{0x99, 0, "Marked"}),
		"Courage cast again")

	// an answer past the newest reads as the newest
	aw.w.SetGroupEffectsVersion(aw.asker, 7)
	aw.fxPass(now)
	if got := fxSets(aw.asker); len(got) != 1 || len(got[0]) == len(fxSet(fxMember{aLeader, []uint32{1, 2, 3}})) {
		t.Fatalf("answered 7: %x, want one version 2 set", got)
	}

	// the control: answered 1, the icons alone
	aw.w.SetGroupEffectsVersion(aw.asker, 1)
	aw.fxPass(now)
	oneFxSet(t, aw.asker, fxSet(fxMember{aLeader, []uint32{0xA1B2C3D4, 0x0BADF00D, 0x99}}), "version 1: icons alone")
	if aw.a.GroupEffectsV2 {
		t.Fatal("answered 1, still version 2")
	}
}

// A merc's effects in version 2: each with its time left, a were form's (past a year) with
// none shown (0), as the buff window shows it.
func TestGroupEffectsVersion2Merc(t *testing.T) {
	aw := newAssistWorld(t)
	now := time.Unix(1_800_000_000, 0)
	aw.group()
	merc := &NPC{ServerID: 2101, Name: "Hired Blade", Owner: aw.b.ServerID, merc: &mercState{}, HP: 50, MaxHP: 50}
	aw.w.NPCs[merc.ServerID] = merc
	aw.w.groups[7].Members = append(aw.w.groups[7].Members, merc.ServerID)
	merc.Effects = []*ActiveEffect{
		{SpellID: 21, Name: "Ward", Icon: 0x77, Until: now.Add(time.Minute)},
		{SpellID: 22, Name: "Wolf Form", Icon: 0x78, Until: now.Add(2 * noEndShown)},
	}
	aw.w.SetGroupEffectsVersion(aw.leader, 2)
	aw.fxPass(now)
	got := fxSets(aw.leader)
	if len(got) != 1 || got[0][0] != 2 {
		t.Fatalf("the leader asked: %x, want one set of two members", got)
	}
	id := aw.w.groupIDSeenByLocked(aw.leader, merc.ServerID)
	want := fxSet2(id, fxEffect{0x77, 60_000, "Ward"}, fxEffect{0x78, 0, "Wolf Form"})[1:]
	if !bytes.HasSuffix(got[0], want) {
		t.Fatalf("the merc: %x, want it to end %x", got[0], want)
	}
}
