package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// A creature the town guards hit is no one's to loot (entity/guards.go, jib's
// F3 20261030-063534): its corpse answers "That entity is not lootable", by
// 0x0004 and by 0x0015 alike. The control: the same corpse, no guard's,
// opens with its item.
func TestAGuardsKillIsNotLootable(t *testing.T) {
	f := newCorpseFixture(t, 1)
	guards := true
	f.h.guardsKill = func(id entity.EntityID) bool { return guards && id == f.npc.ServerID }

	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	mustOps(t, got, protocol.OpcodeLootOptions)
	if got[0][2] != lootNotLootable {
		t.Fatalf("the guards' corpse answered 0x0085 %d, want not lootable", got[0][2])
	}

	guards = false
	f.h.OpenCorpse(f.sess, f.npc)
	got = f.sent()
	mustOps(t, got, protocol.OpcodeLoot)
	if n := lootCount(t, got[0]); n != 1 {
		t.Fatalf("the control: the player's own corpse sent %d records, want its 1", n)
	}
}
