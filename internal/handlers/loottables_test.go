package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// A creature's loot tables (loottables, npcloot): bound by its name as
// npcclass keys it, each a table's items for its level, rolled at its death
// into the corpse beside its npcitem rows.

// bindLoot writes a table of the fixture's two items for levels 10-20 and
// binds the name to it at that chance and rolls.
func bindLoot(t *testing.T, f *corpseFixture, name string, chance, rolls int) {
	t.Helper()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('plains', ?, 3, 10, 20)`, []any{f.sword}},
		{`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('plains', ?, 1, 10, 20)`, []any{f.bait}},
		{`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('plains', ?, 5, 30, 40)`, []any{f.bait}},
		{`INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES (?, 'plains', ?, ?)`, []any{name, chance, rolls}},
	} {
		if _, err := f.db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
}

// The tables bound to a name, the item rows for the level only, each with
// its item's record; another name, or a level no row covers, gets nothing.
func TestNPCLootRollsByNameAndLevel(t *testing.T) {
	f := newCorpseFixture(t, 0)
	bindLoot(t, f, "beetle", 40, 2)

	rolls, err := f.inv.NPCLootRolls("Beetle ", 15) // the key is npcclass's: any case, trimmed
	if err != nil {
		t.Fatal(err)
	}
	if len(rolls) != 1 || rolls[0].Table != "plains" || rolls[0].Chance != 40 || rolls[0].Rolls != 2 {
		t.Fatalf("rolls %+v", rolls)
	}
	ch := rolls[0].Choices
	if len(ch) != 2 || ch[0].Item.Pattern.PatternID != f.sword || ch[0].Weight != 3 ||
		ch[1].Item.Pattern.PatternID != f.bait || ch[1].Weight != 1 {
		t.Fatalf("level 15's choices %+v", ch)
	}
	if ch[0].Item.Pattern.ItemName != "Rusty Sword" || ch[0].Item.StackLeft != 1 || ch[0].Item.RemainHP != 100 {
		t.Errorf("the record %+v", ch[0].Item)
	}
	if rolls, _ := f.inv.NPCLootRolls("beetle", 25); len(rolls) != 1 || len(rolls[0].Choices) != 0 {
		t.Errorf("level 25 (no row covers it): %+v", rolls)
	}
	if rolls, _ := f.inv.NPCLootRolls("wasp", 15); len(rolls) != 0 {
		t.Errorf("an unbound name: %+v", rolls)
	}
}

// A roll lands below its chance and takes an item by weight; one at or above
// it takes nothing.
func TestRollLootByChanceAndWeight(t *testing.T) {
	sword := database.CharInventoryEntry{Pattern: database.ItemPatternRecord{PatternID: 1}}
	bait := database.CharInventoryEntry{Pattern: database.ItemPatternRecord{PatternID: 2}}
	rolls := []database.LootRoll{{Table: "plains", Chance: 30, Rolls: 3,
		Choices: []database.LootChoice{{Item: sword, Weight: 3}, {Item: bait, Weight: 1}}}}
	// three rolls: the chance dice 29 (lands), 30 (misses), 0 (lands); the
	// pick dice 3 (past the sword's 3: the bait) and 2 (the sword)
	dice := []int{29, 3, 30, 0, 2}
	intn := func(n int) int {
		v := dice[0]
		dice = dice[1:]
		if v >= n {
			t.Fatalf("die %d for intn(%d)", v, n)
		}
		return v
	}
	got := rollLoot(rolls, intn)
	if len(got) != 2 || got[0].Pattern.PatternID != 2 || got[1].Pattern.PatternID != 1 || len(dice) != 0 {
		t.Fatalf("got %+v, dice left %v", got, dice)
	}
	// a table with nothing for the level rolls no dice
	if got := rollLoot([]database.LootRoll{{Table: "empty", Chance: 100, Rolls: 5}}, func(int) int {
		t.Fatal("a die rolled for an empty table")
		return 0
	}); len(got) != 0 {
		t.Fatalf("an empty table gave %+v", got)
	}
}

// The corpse holds its npcitem rows and what its tables rolled at its death,
// once a death: a second opening shows the same, and a creature whose rolls
// all miss holds its rows alone.
func TestACorpseHoldsWhatItsTablesRolled(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.npc.Level = 12
	bindLoot(t, f, "beetle", 50, 2)
	calls := 0
	f.h.lootIntn = func(n int) int {
		calls++
		return 0 // every roll lands, each takes the first item (the sword)
	}
	f.h.OpenCorpse(f.sess, f.npc)
	// its coins first (corpsecoins.go), then the items
	got := f.sent()
	mustOps(t, got, protocol.OpcodePlayerTunar, protocol.OpcodeClientMessage, protocol.OpcodeLoot)
	if n := lootCount(t, got[2]); n != 3 {
		t.Fatalf("%d records, want the npcitem row and two rolled", n)
	}
	before := calls
	f.h.OpenCorpse(f.sess, f.npc)
	if n := lootCount(t, f.sent()[0]); n != 3 || calls != before {
		t.Fatalf("reopened: %d records, %d more dice; the death's loot is rolled once", n, calls-before)
	}

	// the negative control: every roll misses, the npcitem row alone
	g := newCorpseFixture(t, 1)
	g.npc.Level = 12
	bindLoot(t, g, "beetle", 50, 2)
	g.h.lootIntn = func(n int) int { return n - 1 }
	g.h.OpenCorpse(g.sess, g.npc)
	got = g.sent()
	if n := lootCount(t, got[len(got)-1]); n != 1 {
		t.Fatalf("every roll missed: %d records, want 1", n)
	}
}
