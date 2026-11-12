package entity

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A raid's mercs in its roster (raidroster.go; the owner, 2026-11-12: "im not able to
// target mercs in a raid group"): vanilla draws the roster in place of the group's bars,
// so a merc missing from it was in no window and could not be clicked. Each beside its
// control.

// rosterRecord is one 0x7E1A record as a client that answered a reads it.
type rosterRecord struct {
	id               uint32
	name             string
	class, level, hp byte
	group, kind      byte
}

// readMercRoster reads the roster s was last queued (drained), as a client that answered
// answer reads it (a page with any other length fails).
func readMercRoster(t *testing.T, s *session.Session, answer uint32) []rosterRecord {
	t.Helper()
	var out []rosterRecord
	for _, m := range drain(s) {
		if opOfMsg(m) != protocol.OpcodeRaidRoster {
			continue
		}
		first, count := int(binary.LittleEndian.Uint16(m[4:])), int(m[6])
		if first == 0 {
			out = nil
		}
		b := m[7:]
		for k := 0; k < count; k++ {
			n := int(b[4])
			rec := rosterRecord{id: binary.LittleEndian.Uint32(b), name: string(b[5 : 5+n]), class: b[5+n], level: b[6+n], hp: b[7+n]}
			b = b[9+n:]
			if answer >= RaidRosterWithGroups {
				rec.group, b = b[0], b[1:]
			}
			if answer >= RaidRosterWithKinds {
				rec.kind, b = b[0], b[1:]
			}
			out = append(out, rec)
		}
		if len(b) != 0 {
			t.Fatalf("%d bytes past a page's records", len(b))
		}
	}
	return out
}

func rosterPassMerc(r *mercRig) {
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	r.w.TickCount += raidRosterTicks - r.w.TickCount%raidRosterTicks
	r.w.raidRosterSystem()
}

// A raid's merc is in every roster, after its owner, in its owner's group, by the id
// each client knows it by (its channel's to one that sees it: what the client targets
// by, and what the server finds again as the merc); its kind says merc to a client that
// answered 3, and the players' say player (the control). A client that answered 1 or 2
// reads its records with no byte it does not know of.
func TestARaidsMercIsInItsRosterByTheIDItsTargetedBy(t *testing.T) {
	r, ms, g, ss, _ := mercRaid(t, "Other")
	merc := ms.npc
	const chanID = 0x00C0FFEE
	vs := GetOrCreateVisibility(ss[0])
	vs.ChannelEntity = []EntityID{merc.ServerID}
	vs.ChannelObjectID = []uint32{chanID}

	r.w.SetRaidRosterAnswer(ss[0], RaidRosterWithKinds)
	r.w.SetRaidRosterAnswer(r.sess, RaidRosterWithGroups)
	drain(ss[0])
	drain(r.sess)
	rosterPassMerc(r)

	got := readMercRoster(t, ss[0], RaidRosterWithKinds)
	if len(got) != 3 {
		t.Fatalf("Other's roster: %+v, want the owner, its merc and Other (members %v)", got, g.Members)
	}
	if got[0].name != "Owner" || got[1].name != merc.Name || got[2].name != "Other" {
		t.Fatalf("the order: %+v", got)
	}
	m := got[1]
	if m.id != chanID {
		t.Errorf("the merc's id %#x, want its channel's %#x (the id the client targets by)", m.id, chanID)
	}
	if m.kind != raidRosterKindMerc || got[0].kind != 0 || got[2].kind != 0 {
		t.Errorf("kinds %d %d %d: want player, merc, player", got[0].kind, m.kind, got[2].kind)
	}
	if m.group != got[0].group || m.group == 0 {
		t.Errorf("the merc in group %d, its owner in %d", m.group, got[0].group)
	}
	if int(m.class) != merc.Class || int(m.level) != merc.Level || m.hp != 255 {
		t.Errorf("the merc's record %+v: class %d level %d, full health", m, merc.Class, merc.Level)
	}
	r.w.mu.Lock()
	e, ok := r.w.findEntityByObjectIDLocked(ss[0], m.id)
	r.w.mu.Unlock()
	if n, isNPC := e.(*NPC); !ok || !isNPC || n != merc {
		t.Errorf("the roster's id %#x finds %v, not the merc", m.id, e)
	}

	// its owner's client, which answered 2: the merc there too, its group its owner's,
	// by groupFarID's stand-in where it has no channel of it
	own := readMercRoster(t, r.sess, RaidRosterWithGroups)
	if len(own) != 3 || own[1].name != merc.Name || own[1].id != groupFarID(merc.ServerID) || own[1].group != own[0].group {
		t.Errorf("the owner's roster: %+v", own)
	}
}

// A merc dead is out of the roster (the control: alive, it is in it).
func TestARaidsDeadMercIsOutOfItsRoster(t *testing.T) {
	r, ms, _, ss, _ := mercRaid(t, "Other")
	r.w.SetRaidRosterAnswer(ss[0], 1)
	drain(ss[0])
	rosterPassMerc(r)
	if got := readMercRoster(t, ss[0], 1); len(got) != 3 {
		t.Fatalf("alive: %+v", got)
	}
	r.w.mu.Lock()
	ms.npc.Dead = true
	r.w.mu.Unlock()
	rosterPassMerc(r)
	if got := readMercRoster(t, ss[0], 1); len(got) != 2 {
		t.Fatalf("dead: %+v", got)
	}
}
