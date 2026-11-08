package entity

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The capes in view (capes.go) on the assist world (assist_test.go), whose two
// sessions number the same entities differently: each set must name the
// wearers as the receiving session knows them.

// capeSets takes the OpcodeCapesInView bodies queued for s, a long one put
// back together from its fragments (the queue cuts a message past
// session.MaxFragmentSize into segments, then its last part).
func capeSets(s *session.Session) [][]byte {
	var out [][]byte
	var frags []byte
	for _, m := range s.Queue().GetPending() {
		if m.Type == protocol.MessageTypeSegmentReliable {
			frags = append(frags, m.Data()...)
			continue
		}
		d := append(frags, m.Data()...)
		frags = nil
		if len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeCapesInView {
			out = append(out, d[2:])
		}
	}
	return out
}

// capesBody is a set's body: a u8 count, then each (u32 id, u8 style, u32
// colour), given as triples.
func capesBody(e ...uint32) []byte {
	out := []byte{byte(len(e) / 3)}
	for i := 0; i < len(e); i += 3 {
		out = binary.LittleEndian.AppendUint32(out, e[i])
		out = append(out, byte(e[i+1]))
		out = binary.LittleEndian.AppendUint32(out, e[i+2])
	}
	return out
}

// newCapeWorld is the assist world on a server with capes, the asker's client
// drawing them: the asker wears style 2, the leader style 5 in red.
func newCapeWorld(t *testing.T) *assistWorld {
	t.Helper()
	SetCapes(true)
	t.Cleanup(func() { SetCapes(false) })
	aw := newAssistWorld(t)
	aw.a.CapeStyle = 2
	aw.b.CapeStyle, aw.b.CapeColor = 5, 0xC01010FF
	aw.asker.OfferCapesOnce()
	aw.asker.SetCapes(true)
	return aw
}

// A client that draws capes is sent its own cape (by its own id) and those on
// its entity slots (by its slots' ids), never an NPC's; again only when the set
// changes or it is due (ResendCapes: a MemoryDump, a world change), and the
// whole set each time. The controls: the leader's client never answered and is
// sent nothing, nor is anyone on a server without capes.
func TestCapesInViewOnlyToClientsThatDrawThem(t *testing.T) {
	aw := newCapeWorld(t)
	aw.w.capesSyncSystem()
	want := capesBody(askerSelf, 2, 0, aLeader, 5, 0xC01010FF)
	if got := capeSets(aw.asker); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the asker's first set: %x, want one %x (itself, then the leader as it knows it)", got, want)
	}
	if got := capeSets(aw.leader); len(got) != 0 {
		t.Fatalf("the leader's client never answered and was sent %x", got)
	}
	aw.w.capesSyncSystem()
	if got := capeSets(aw.asker); len(got) != 0 {
		t.Fatalf("unchanged, sent again: %x", got)
	}
	aw.b.CapeStyle, aw.b.CapeColor = 0, 0 // the leader takes its cape off
	aw.w.capesSyncSystem()
	if got, want := capeSets(aw.asker), capesBody(askerSelf, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader's cape off: %x, want %x", got, want)
	}
	GetOrCreateVisibility(aw.asker).ResendCapes()
	aw.w.capesSyncSystem()
	if got, want := capeSets(aw.asker), capesBody(askerSelf, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("due again, unchanged: %x, want %x again", got, want)
	}

	// the other session's numbering: once the leader's client answers, the
	// asker is the leader's bAsker
	aw.b.CapeStyle = 5
	aw.leader.OfferCapesOnce()
	aw.leader.SetCapes(true)
	aw.w.capesSyncSystem()
	if got, want := capeSets(aw.leader), capesBody(leaderSelf, 5, 0, bAsker, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader's set: %x, want %x", got, want)
	}
	capeSets(aw.asker)

	SetCapes(false)
	aw.a.CapeStyle = 7
	aw.w.capesSyncSystem()
	if got := append(capeSets(aw.asker), capeSets(aw.leader)...); len(got) != 0 {
		t.Fatalf("a server without capes sent %x", got)
	}
}

// An invisible player's cape is left out of another's view (its body is
// hidden there) and kept in its own; a player with none is in no set.
func TestAnInvisiblePlayersCapeIsHiddenFromOthers(t *testing.T) {
	aw := newCapeWorld(t)
	aw.leader.OfferCapesOnce()
	aw.leader.SetCapes(true)
	aw.a.GMHidden = true // the asker hidden, as an invisibility effect hides
	aw.w.capesSyncSystem()
	if got, want := capeSets(aw.asker), capesBody(askerSelf, 2, 0, aLeader, 5, 0xC01010FF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the hidden asker's own set: %x, want %x", got, want)
	}
	if got, want := capeSets(aw.leader), capesBody(leaderSelf, 5, 0xC01010FF); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the leader's view of the hidden asker: %x, want %x (no cape where it stands)", got, want)
	}
	aw.a.GMHidden = false
	aw.w.capesSyncSystem()
	if got, want := capeSets(aw.leader), capesBody(leaderSelf, 5, 0xC01010FF, bAsker, 2, 0); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("visible again: %x, want %x", got, want)
	}
}

// A full view's set (every protocol slot a caped player) goes out whole in
// one message past a default message's size, its count and every entry in it.
func TestAFullViewsCapesGoOutWhole(t *testing.T) {
	aw := newCapeWorld(t)
	vs := GetOrCreateVisibility(aw.asker)
	n := 203
	vs.ChannelEntity = make([]EntityID, n)
	vs.ChannelObjectID = make([]uint32, n)
	for i := 0; i < n; i++ {
		p := &Player{ServerID: EntityID(5000 + i), Name: "Caped", CapeStyle: uint8(1 + i%6)}
		aw.w.Players[p.ServerID] = p
		vs.ChannelEntity[i], vs.ChannelObjectID[i] = p.ServerID, uint32(200000+i)
	}
	aw.w.capesSyncSystem()
	got := capeSets(aw.asker)
	if len(got) != 1 || len(got[0]) != 1+9*(n+1) || got[0][0] != byte(n+1) {
		t.Fatalf("a full view: %d sets, the first %d bytes (count %d), want one of %d with %d", len(got), len(got[0]), got[0][0], 1+9*(n+1), n+1)
	}
	last := got[0][1+9*n:]
	if id := binary.LittleEndian.Uint32(last); id != uint32(200000+n-1) || last[4] != uint8(1+(n-1)%6) {
		t.Fatalf("the last entry: id %d style %d", id, last[4])
	}
}
