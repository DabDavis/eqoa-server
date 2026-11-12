package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// rightsRig is the corpse fixture with a second player (Outsider) and a third
// (Mate), each its own session, and a clock for the holds.
type rightsRig struct {
	*corpseFixture
	other, mate         *session.Session
	outsider, groupMate *entity.Player
	groups              map[*entity.Player]uint32
	now                 time.Time
}

func newRightsRig(t *testing.T, items int) *rightsRig {
	r := &rightsRig{corpseFixture: newCorpseFixture(t, items), other: testSession(), mate: testSession(), now: time.Now()}
	r.outsider = &entity.Player{ServerID: 1002, CharServerID: 902, Name: "Outsider"}
	r.groupMate = &entity.Player{ServerID: 1003, CharServerID: 903, Name: "Mate"}
	r.groups = map[*entity.Player]uint32{}
	for _, s := range []*session.Session{r.other, r.mate} {
		s.SetInventoryList(newInvList(nil))
		s.SetBankList(newInvList(nil))
	}
	r.h.playerOf = func(s *session.Session) *entity.Player {
		switch s {
		case r.sess:
			return r.player
		case r.other:
			return r.outsider
		case r.mate:
			return r.groupMate
		}
		return nil
	}
	r.h.groupIDOf = func(p *entity.Player) uint32 { return r.groups[p] }
	r.h.lootNow = func() time.Time { return r.now }
	return r
}

// why is the 0x0085 reason a session was last sent, 0 for none.
func why(s *session.Session) byte {
	var last byte
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if opOf(d) == protocol.OpcodeLootOptions {
			last = d[2]
		}
	}
	return last
}

// opened is whether a session was sent its corpse's window (0x0018).
func opened(s *session.Session) bool {
	for _, m := range s.Queue().GetPending() {
		if opOf(m.Data()) == protocol.OpcodeLoot {
			return true
		}
	}
	return false
}

// Petition 1537: a corpse is its kill group's. The killer opens it; one outside
// the group is told it belongs to another (0x0085 3); one in the killer's group
// when it opens (a member farther off at the death) may. The controls: a corpse
// no one was credited with is anyone's, and a kill credit of another death (the
// creature's earlier one) holds nothing.
func TestOnlyTheKillGroupLootsACorpse(t *testing.T) {
	r := newRightsRig(t, 2)
	r.groups[r.player] = 7
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: r.npc.DeathTime, Group: 7,
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})

	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider's opening: 0x0085 %d, want %d", got, lootAnotherOwner)
	}
	if len(r.h.loots().open) != 0 {
		t.Fatal("the outsider holds the corpse")
	}

	r.h.OpenCorpse(r.sess, r.npc) // the killer
	if !opened(r.sess) {
		t.Fatal("the killer could not open its kill")
	}
	r.h.HandleLootClose(r.sess, nil)
	r.sent()

	r.groups[r.groupMate] = 7 // in the killer's group now
	r.h.OpenCorpse(r.mate, r.npc)
	if !opened(r.mate) {
		t.Fatal("a member of the killer's group could not open it")
	}
	r.h.HandleLootClose(r.mate, nil)
	r.mate.Queue().GetPending()

	// the control: no kill credit, anyone's
	r.h.killRights.Delete(r.npc.ServerID)
	r.h.OpenCorpse(r.other, r.npc)
	if !opened(r.other) {
		t.Fatal("a corpse no one was credited with did not open for an outsider")
	}
	r.h.HandleLootClose(r.other, nil)
	r.other.Queue().GetPending()

	// an earlier death's credit holds nothing
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: r.npc.DeathTime.Add(-time.Hour),
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID}}})
	r.h.OpenCorpse(r.other, r.npc)
	if !opened(r.other) {
		t.Fatal("another death's credit held this corpse")
	}

	// it goes with the corpse
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: r.npc.DeathTime,
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID}}})
	r.h.CorpsesGone([]entity.EntityID{r.npc.ServerID})
	if _, ok := r.h.killRights.Load(r.npc.ServerID); ok {
		t.Fatal("a gone corpse's kill group is remembered")
	}
}

// Petition 1540: one looter at a time. While the killer's window is open, its
// group mate's opening is told the corpse is being looted (0x0085 1); closed,
// the mate opens it. A holder that has done nothing for lootHoldIdle, or walked
// off, or left the world, is let go of (its window closed) and the next opens.
func TestOneLooterAtATime(t *testing.T) {
	r := newRightsRig(t, 2)
	r.h.OpenCorpse(r.sess, r.npc)
	if !opened(r.sess) {
		t.Fatal("the first opening")
	}
	r.h.OpenCorpse(r.mate, r.npc)
	if got := why(r.mate); got != lootBeingLooted {
		t.Fatalf("a second opening while it is open: 0x0085 %d, want %d", got, lootBeingLooted)
	}
	r.h.HandleLootClose(r.sess, nil)
	r.sent()
	r.h.OpenCorpse(r.mate, r.npc)
	if !opened(r.mate) {
		t.Fatal("closed by the first, the corpse did not open for the second")
	}

	// a take keeps the hold; idle past lootHoldIdle it is let go of
	r.now = r.now.Add(lootHoldIdle - time.Second)
	r.h.OpenCorpse(r.sess, r.npc)
	if got := why(r.sess); got != lootBeingLooted {
		t.Fatalf("within the idle time: 0x0085 %d", got)
	}
	r.now = r.now.Add(2 * time.Second)
	r.h.OpenCorpse(r.sess, r.npc)
	if !opened(r.sess) {
		t.Fatal("an idle holder was not let go of")
	}
	closed := false
	for _, m := range r.mate.Queue().GetPending() {
		closed = closed || opOf(m.Data()) == protocol.OpcodeClientCloseLoot
	}
	if !closed {
		t.Fatal("the idle holder's window was not closed")
	}

	// walked off
	r.player.X = 500
	r.h.OpenCorpse(r.mate, r.npc)
	if !opened(r.mate) {
		t.Fatal("a holder that walked off was not let go of")
	}
	r.sent()

	// left the world
	gone := r.groupMate
	r.groupMate = nil
	r.player.X = 0
	r.h.OpenCorpse(r.sess, r.npc)
	r.groupMate = gone
	if !opened(r.sess) {
		t.Fatal("a holder gone from the world was not let go of")
	}
}

// The owner, 2026-11-12 ("the rolling reset is deleting loot on mobs"): a corpse's
// kill group comes through a keep restart, an unopened corpse's too, and a copy's
// corpse made again under another id (entity.RestoreCopies) keeps it there. Written
// and read through JSON, as the file; "the new process" is the same handler with
// its corpses and kill groups forgotten. The control: restored with nothing, the
// corpse is anyone's (what a restart did before).
func TestAKillGroupComesThroughARestart(t *testing.T) {
	r := newRightsRig(t, 2)
	r.groups[r.player] = 7
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: r.npc.DeathTime, Group: 7,
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	prev := keepWindows.Swap(r.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	b, err := json.Marshal(KeepCorpses())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptCorpse
	if err := json.Unmarshal(b, &kept); err != nil || len(kept) != 1 || kept[0].Right == nil || kept[0].Opened ||
		kept[0].Right.Group != 7 || len(kept[0].Right.Chars) != 1 || kept[0].Right.Chars[0] != r.player.CharServerID {
		t.Fatalf("written %s (%v)", b, err)
	}
	forget := func() {
		r.h.killRights.Delete(r.npc.ServerID)
		r.h.killRights.Delete(r.npc.ServerID + 7)
		ls := r.h.loots()
		ls.mu.Lock()
		clear(ls.corpses)
		clear(ls.open)
		ls.mu.Unlock()
	}
	forget()

	// the control: nothing restored, anyone's
	r.h.OpenCorpse(r.other, r.npc)
	if !opened(r.other) {
		t.Fatal("control: a corpse with no kill group did not open for an outsider")
	}
	r.other.Queue().GetPending()
	forget()

	// restored under another id: the outsider refused, the group's member let in
	old := r.npc.ServerID
	r.npc.ServerID = old + 7
	if n := RestoreCorpsesAs(kept, map[entity.EntityID]entity.EntityID{old: old + 7}); n != 1 {
		t.Fatalf("%d corpses restored", n)
	}
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider's opening after the restart: 0x0085 %d, want %d", got, lootAnotherOwner)
	}
	r.groups[r.groupMate] = 7 // RestoreGroups forms the group again under its id
	r.h.OpenCorpse(r.mate, r.npc)
	if !opened(r.mate) {
		t.Fatal("the kill group's member could not open it after the restart")
	}
}
