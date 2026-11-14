package handlers

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
)

// Petition 5035 (the GM Jibrish), the con's lines: before six minutes an outsider is told when
// the corpse unlocks and its owner how long it is theirs (a group mate too); from six minutes,
// anyone is told it is anyone's and when it decays. A kill a raid made is locked the same, then
// open. A pet's corpse says nothing.
func TestAConOfACorpseSaysItsLock(t *testing.T) {
	r := newRightsRig(t, 1)
	r.groups[r.player] = 7
	r.groups[r.groupMate] = 7
	died := r.npc.DeathTime
	decays := died.Add(20 * time.Minute)
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, Group: 7, OpenAt: died.Add(6 * time.Minute),
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	r.now = died.Add(5 * time.Minute)
	for _, c := range []struct {
		who  string
		got  string
		want string
	}{
		{"an outsider", r.h.corpseConLine(r.other, r.npc, decays), "This corpse unlocks for everyone in 1:00."},
		{"the killer", r.h.corpseConLine(r.sess, r.npc, decays), "Yours alone for 1:00 more."},
		{"a group mate", r.h.corpseConLine(r.mate, r.npc, decays), "Yours alone for 1:00 more."},
	} {
		if c.got != c.want {
			t.Errorf("%s at 5:00: %q, want %q", c.who, c.got, c.want)
		}
	}
	r.now = died.Add(6*time.Minute + 30*time.Second)
	if got := r.h.corpseConLine(r.other, r.npc, decays); got != "Anyone may loot this corpse; it decays in 13:30." {
		t.Errorf("an outsider at 6:30: %q", got)
	}
	r.npc.Owner = 1
	if got := r.h.corpseConLine(r.other, r.npc, decays); got != "" {
		t.Errorf("a pet's corpse: %q", got)
	}
	r.npc.Owner = 0

	// a raid's kill: locked to its raid (its group) until six minutes, then open
	r.now = died
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, Group: 7, OpenAt: died.Add(6 * time.Minute),
		Raid:    &entity.RaidKill{Group: 7, Rule: entity.LootFreeForAll},
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider on a raid's kill at once: 0x0085 %d, want %d", got, lootAnotherOwner)
	}
	r.now = died.Add(6 * time.Minute)
	r.h.OpenCorpse(r.other, r.npc)
	if !opened(r.other) {
		t.Fatal("an outsider could not open a raid's kill at six minutes")
	}
}

// A corpse left as a copy (entity/corpselinger.go) takes its loot along: rolled items, kill
// group and open time, the windows open on it closed, and the World told it moved.
func TestALeftCorpseTakesItsLootAlong(t *testing.T) {
	r := newRightsRig(t, 2)
	died := r.npc.DeathTime
	r.h.heardKillRight(entity.KillCredit{NPCID: r.npc.ServerID, Died: died, OpenAt: died.Add(6 * time.Minute),
		Players: []entity.KillPlayer{{ServerID: r.player.CharServerID, Name: r.player.Name}}})
	r.h.OpenCorpse(r.sess, r.npc)
	if !opened(r.sess) {
		t.Fatal("the killer could not open its kill")
	}
	var moved []entity.EntityID
	r.h.corpseMoved = func(id entity.EntityID) { moved = append(moved, id) }
	from, to := r.npc.ServerID, r.npc.ServerID+1000
	r.h.moveCorpse([]entity.CorpseLeft{{From: from, To: to, Died: died}})
	ls := r.h.loots()
	if ls.corpses[from] != nil || ls.corpses[to] == nil || len(ls.corpses[to].items) != 2 || len(ls.open) != 0 {
		t.Fatalf("after the move: from %v, to %v, open %d", ls.corpses[from] != nil, ls.corpses[to], len(ls.open))
	}
	if _, ok := r.h.killRights.Load(from); ok {
		t.Fatal("the creature keeps the kill group")
	}
	if v, ok := r.h.killRights.Load(to); !ok || !v.(lootRight).openAt.Equal(died.Add(6*time.Minute)) {
		t.Fatal("the copy has no kill group or open time")
	}
	if len(moved) != 1 || moved[0] != to {
		t.Fatalf("the World told %v", moved)
	}
	// the copy opens as the corpse did: the outsider refused before six minutes
	r.npc.ServerID = to
	r.sent()
	r.h.OpenCorpse(r.other, r.npc)
	if got := why(r.other); got != lootAnotherOwner {
		t.Fatalf("an outsider on the left corpse: 0x0085 %d", got)
	}
}
