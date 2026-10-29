package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// npcloot's "*": every creature's corpse rolls its tables after its own;
// a training dummy's does not.
func TestEveryCreaturesLootRow(t *testing.T) {
	setup := func(t *testing.T) *corpseFixture {
		f := newCorpseFixture(t, 0)
		f.npc.Level = 12
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('own', ?, 1, 1, 60)`, []any{f.sword}},
			{`INSERT INTO loottables (tablename, patternid, weight, minlevel, maxlevel) VALUES ('modifiers', ?, 1, 1, 60)`, []any{f.bait}},
			{`INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('beetle', 'own', 100, 1)`, nil},
			{`INSERT INTO npcloot (npc_name, tablename, chance, rolls) VALUES ('*', 'modifiers', 100, 1)`, nil},
		} {
			if _, err := f.db.Exec(q.sql, q.args...); err != nil {
				t.Fatal(err)
			}
		}
		f.h.lootIntn = func(int) int { return 0 }
		return f
	}
	items := func(f *corpseFixture) []int {
		f.h.OpenCorpse(f.sess, f.npc)
		f.sent()
		var out []int
		for _, it := range f.h.loots().corpses[f.npc.ServerID].items {
			out = append(out, it.Pattern.PatternID)
		}
		return out
	}
	f := setup(t)
	if got := items(f); len(got) != 2 || got[0] != f.sword || got[1] != f.bait {
		t.Fatalf("a creature's corpse: %v, want its own table's (%d) then every creature's (%d)", got, f.sword, f.bait)
	}
	// another creature, no table of its own: "*" alone
	g := setup(t)
	g.npc.Name = "wasp"
	if got := items(g); len(got) != 1 || got[0] != g.bait {
		t.Fatalf("a creature of no table: %v, want %d", got, g.bait)
	}
	// the control: a dummy's (were one to die) rolls its own alone
	d := setup(t)
	d.npc.Dummy = entity.TrainingDummy
	if got := items(d); len(got) != 1 || got[0] != d.sword {
		t.Fatalf("a dummy's corpse: %v, want its own table's alone", got)
	}
	// and lootNames says the same of a pet and a charmed creature
	if n := lootNames(&entity.NPC{Name: "pet", Owner: 7}); len(n) != 1 {
		t.Fatalf("a pet rolls %v", n)
	}
	if n := lootNames(&entity.NPC{Name: "creature"}); len(n) != 2 || n[1] != "*" {
		t.Fatalf("a creature rolls %v", n)
	}
}
