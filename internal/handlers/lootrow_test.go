package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// An open-world creature rolls its own row's npcloot tables ("#n") as well as
// its name's (work board 1554; 1599: the Tower of Phantoms' two elder spirits
// alone drop the Spirit Maker's Robe): the row bound, its corpse can roll the
// table; another creature of the same name, of another row, cannot.
func TestAnOpenWorldCreatureRollsItsRowsLoot(t *testing.T) {
	// the names: its name, its row, every creature's; a copy rolls its source row's
	for _, c := range []struct {
		n    *entity.NPC
		want []string
	}{
		{&entity.NPC{ServerID: 190044, Name: "elder spirit", World: 2}, []string{"elder spirit", "#190044", "*"}},
		{&entity.NPC{ServerID: 9, Name: "elder spirit", World: 2, Temporary: true, CopyOf: 190044}, []string{"elder spirit", "#190044", "*"}},
		{&entity.NPC{ServerID: 9, Name: "elder spirit", Temporary: true}, []string{"elder spirit", "*"}},
	} {
		got := lootNames(c.n)
		if len(got) != len(c.want) {
			t.Fatalf("%+v rolls %v, want %v", c.n, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%+v rolls %v, want %v", c.n, got, c.want)
			}
		}
	}

	// the corpse of the row bound holds the table's item
	f := newCorpseFixture(t, 0)
	f.npc.Level = 12
	bindLoot(t, f, entity.RowKey(f.npc.ServerID), 100, 1)
	f.h.lootIntn = func(n int) int { return 0 }
	f.h.OpenCorpse(f.sess, f.npc)
	got := f.sent()
	if n := lootCount(t, lastOp(t, got, protocol.OpcodeLoot)); n != 1 {
		t.Fatalf("its row bound: %d records, want the table's one", n)
	}

	// the control: the same name, another row: nothing
	g := newCorpseFixture(t, 0)
	g.npc.Level = 12
	bindLoot(t, g, entity.RowKey(g.npc.ServerID+1), 100, 1)
	g.h.lootIntn = func(n int) int { return 0 }
	g.h.OpenCorpse(g.sess, g.npc)
	got = g.sent()
	for _, m := range got {
		if opOf(m) == protocol.OpcodeLoot && lootCount(t, m) != 0 {
			t.Fatalf("another row bound: %d records, want none", lootCount(t, m))
		}
	}
}

// A fractional npcloot chance (work board 1310 item 2, 1599: 0.1 for one kill
// in a thousand) is read as written, and lands when a float in [0, 100) is
// below it; a whole chance rolls intn(100) as it always did.
func TestAFractionalLootChance(t *testing.T) {
	f := newCorpseFixture(t, 0)
	bindLoot(t, f, "beetle", 1, 1)
	if _, err := f.db.Exec(`UPDATE npcloot SET chance = 0.1 WHERE npc_name = 'beetle'`); err != nil {
		t.Fatal(err)
	}
	rolls, err := f.inv.NPCLootRolls("beetle", 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(rolls) != 1 || rolls[0].Chance != 0.1 {
		t.Fatalf("rolls %+v, want the chance 0.1", rolls)
	}
	one := func(v int, wantN int) func(int) int {
		first := true
		return func(n int) int {
			if first {
				first = false
				if n != wantN {
					t.Fatalf("the chance die is intn(%d), want intn(%d)", n, wantN)
				}
				return v
			}
			return 0
		}
	}
	// 0.1%: 99 thousandths of a percent lands, 100 does not
	if got := rollLoot(rolls, one(99, 100_000)); len(got) != 1 {
		t.Fatalf("a die of 0.099 at 0.1: %d items, want 1", len(got))
	}
	if got := rollLoot(rolls, one(100, 100_000)); len(got) != 0 {
		t.Fatalf("a die of 0.100 at 0.1: %d items, want none", len(got))
	}
	// a whole chance, as before
	whole := []database.LootRoll{{Table: "plains", Chance: 30, Rolls: 1, Choices: rolls[0].Choices}}
	if got := rollLoot(whole, one(29, 100)); len(got) != 1 {
		t.Fatalf("29 at 30: %d items, want 1", len(got))
	}
	if got := rollLoot(whole, one(30, 100)); len(got) != 0 {
		t.Fatalf("30 at 30: %d items, want none", len(got))
	}
	// and over many kills, about one in a thousand
	n := 0
	for i := 0; i < 200_000; i++ {
		n += len(rollLoot(rolls, nil))
	}
	if n < 120 || n > 290 {
		t.Fatalf("200,000 kills at 0.1%%: %d drops, want about 200", n)
	}
}
