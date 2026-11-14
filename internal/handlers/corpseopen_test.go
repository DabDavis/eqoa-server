package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lastLootCount is the record count of the last 0x0018 a session was sent, -1 for none.
func lastLootCount(t *testing.T, s *session.Session) int64 {
	t.Helper()
	n := int64(-1)
	for _, m := range s.Queue().GetPending() {
		if d := m.Data(); opOf(d) == protocol.OpcodeLoot {
			n = lootCount(t, d)
		}
	}
	return n
}

// The owner, 2026-11-14: "corpse if not looted for 5 mins should be open to all if
// not a raid". A group's kill (KillCredit.OpenAt five minutes after the death, as the
// World gives one outside a raid): an outsider is refused until then (word 675), the
// killer takes one of its two items, and at five minutes after the death (from the
// death, not the take) the outsider opens what is left. A raid's kill (OpenAt zero)
// stays its kill group's at ten minutes.
func TestACorpseIsAnyonesFiveMinutesAfterItsDeath(t *testing.T) {
	r := newRightsRig(t, 2)
	r.groups[r.player] = 7
	died := r.npc.DeathTime
	r.now = died
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, Group: 7, OpenAt: died.Add(5 * time.Minute),
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})

	r.h.OpenCorpse(r.sess, r.npc) // the killer takes one
	if !opened(r.sess) {
		t.Fatal("the killer could not open its kill")
	}
	ls := r.h.loots()
	ls.mu.Lock()
	c := ls.corpses[r.npc.ServerID]
	c.items = c.items[1:]
	ls.mu.Unlock()
	r.now = died.Add(4 * time.Minute)
	r.h.HandleLootClose(r.sess, nil)
	r.sent()

	r.now = died.Add(5*time.Minute - time.Second)
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider just before five minutes: 0x0085 %d, want %d", got, lootAnotherOwner)
	}
	r.now = died.Add(5 * time.Minute)
	r.h.OpenCorpse(r.other, r.npc)
	if n := lastLootCount(t, r.other); n != 1 {
		t.Fatalf("an outsider at five minutes: %d items shown, want the 1 left", n)
	}
	r.h.HandleLootClose(r.other, nil)
	r.other.Queue().GetPending()

	// a raid's kill keeps its kill group
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, Group: 7,
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	r.now = died.Add(10 * time.Minute)
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner || opened(r.other) {
		t.Fatalf("an outsider on a raid's kill at ten minutes: 0x0085 %d", got)
	}
}

// The open-to-all time comes through a keep restart with the kill group: restored,
// the outsider is refused before it and let in at it.
func TestACorpsesOpenTimeComesThroughARestart(t *testing.T) {
	r := newRightsRig(t, 2)
	died := r.npc.DeathTime
	r.now = died
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, OpenAt: died.Add(5 * time.Minute),
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	prev := keepWindows.Swap(r.h)
	t.Cleanup(func() { keepWindows.Store(prev) })
	b, err := json.Marshal(KeepCorpses())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptCorpse
	if err := json.Unmarshal(b, &kept); err != nil || len(kept) != 1 || kept[0].Right == nil || kept[0].Right.OpenAt == nil {
		t.Fatalf("written %s (%v)", b, err)
	}
	r.h.killRights.Delete(r.npc.ServerID)
	if n := RestoreCorpsesAs(kept, nil); n != 1 {
		t.Fatalf("%d corpses restored", n)
	}
	r.now = died.Add(time.Minute)
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider after the restart, before five minutes: 0x0085 %d", got)
	}
	r.now = died.Add(5 * time.Minute)
	r.h.OpenCorpse(r.other, r.npc)
	if !opened(r.other) {
		t.Fatal("an outsider after the restart, at five minutes, could not open it")
	}
}
