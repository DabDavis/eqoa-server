package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The Toy Shoppe's Puppet Strings (the owner, 2026-11-17: "charms the target if
// light blue or lower", "the charm has 10 charges then disappears";
// entity/charmcon.go): a new set holds 10 (itemeffect.charges); each charm spends
// one (0x003E), a refusal none (a dark blue creature: "too strong", red); the
// tenth charm takes the set ("Your Puppet Strings snap."), and Master Lizzlen
// never restores them (No Recharge: lizzlen_test.go).
func TestThePuppetStringsHaveTenCharges(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	f.player.Level = 60
	npc := &entity.NPC{ServerID: 2002, Name: "a fire beetle", Level: 59, HP: 100, MaxHP: 100,
		X: f.player.X + 2, Y: f.player.Y, Z: f.player.Z}
	w.NPCs[npc.ServerID] = npc
	vs := entity.GetOrCreateVisibility(f.sess)
	vs.ChannelEntity[0] = npc.ServerID
	vs.ChannelObjectID[0] = 0x01312E98
	id := f.potion("Puppet Strings", -1, 1, 0, "Target, Charm up to light blue, No Recharge, 10min", "Puppet Strings", "")
	if _, err := f.db.Exec(`UPDATE itemeffect SET charges = 10 WHERE patternid = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE itempattern SET procanim = 7, attacktype = 0 WHERE patternid = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inv.AddCharItem(f.sid, id, 1, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	f.loadList()
	left := func() (records, charges int) {
		t.Helper()
		if err := f.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(remaincharge), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`,
			f.sid, id).Scan(&records, &charges); err != nil {
			t.Fatal(err)
		}
		return
	}
	if r, c := left(); r != 1 || c != 10 {
		t.Fatalf("a new set: %d records, %d charges; want 1, 10", r, c)
	}

	// dark blue (59 to 60): refused, nothing spent
	msgs := f.useAs(0, 0x01312E98, 0)
	if npc.Owner != 0 || len(msgsOf(msgs, protocol.OpcodeInteractItem)) != 0 {
		t.Fatalf("a dark blue creature: owner %d, 0x003E %d", npc.Owner, len(msgsOf(msgs, protocol.OpcodeInteractItem)))
	}
	if r, c := left(); r != 1 || c != 10 {
		t.Fatalf("after a refusal: %d records, %d charges; want 1, 10", r, c)
	}

	npc.Level = 40 // light blue to 60
	for use := 1; use <= 10; use++ {
		msgs := f.useAs(0, 0x01312E98, 0)
		if npc.Owner != f.player.ServerID {
			t.Fatalf("charm %d: not taken (owner %d)", use, npc.Owner)
		}
		if got := msgsOf(msgs, protocol.OpcodeInteractItem); len(got) != 1 {
			t.Fatalf("charm %d: %d 0x003E, want 1", use, len(got))
		}
		snapped := false
		for _, m := range msgs {
			if bytes.Equal(m, clientMessagePayload("Your Puppet Strings snap.")) {
				snapped = true
			}
		}
		r, c := left()
		if use < 10 {
			if r != 1 || c != 10-use || snapped {
				t.Fatalf("charm %d: %d records, %d charges, snapped %v; want 1, %d, false", use, r, c, snapped, 10-use)
			}
		} else if r != 0 || !snapped {
			t.Fatalf("the tenth charm: %d records left, snapped %v; want none, true", r, snapped)
		}
		w.PetCommand(f.sess, 5, 0, time.Now()) // dismissed, for the next
	}
	if l := sessionInvList(f.sess); l != nil {
		if _, ok := l.at(0); ok {
			t.Error("the list still holds the set")
		}
	}
}
