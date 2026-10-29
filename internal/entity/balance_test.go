package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

func testBalance() *Balance {
	return NewBalance(
		[]database.BalanceLevel{
			{Level: 10, HPPct: 200, DamagePct: 100, PetHPPct: 100, PetDamagePct: 100},
			{Level: 30, HPPct: 400, DamagePct: 150, PetHPPct: 300, PetDamagePct: 120},
		},
		[]database.BalanceCon{
			{Diff: 0, DamagePct: 100, TakenPct: 100},
			{Diff: 2, DamagePct: 140, TakenPct: 80},
			{Diff: 5, DamagePct: 200, TakenPct: 50},
		})
}

// The tables read as their doc says: a row at its key, the line between two
// rows, the end rows past the ends, and nothing at all with no rows.
func TestBalanceTablesInterpolate(t *testing.T) {
	b := testBalance()
	for _, c := range []struct {
		level   int
		hp, dmg float64
	}{{1, 200, 100}, {10, 200, 100}, {20, 300, 125}, {30, 400, 150}, {60, 400, 150}} {
		if r := b.Level(c.level); r.HPPct != c.hp || r.DamagePct != c.dmg {
			t.Errorf("level %d: %+v, want hp %v dmg %v", c.level, r, c.hp, c.dmg)
		}
	}
	for _, c := range []struct {
		diff       int
		dmg, taken float64
	}{{-3, 100, 100}, {1, 120, 90}, {2, 140, 80}, {4, 140 + 2*60/3.0, 80 - 2*30/3.0}, {9, 200, 50}} {
		if r := b.Con(c.diff); r.DamagePct != c.dmg || r.TakenPct != c.taken {
			t.Errorf("diff %d: %+v, want dmg %v taken %v", c.diff, r, c.dmg, c.taken)
		}
	}
	var none *Balance
	if r := none.Level(30); r.HPPct != 100 || r.PetDamagePct != 100 {
		t.Errorf("no balance: %+v", r)
	}
	if r := NewBalance(nil, nil).Con(4); r.DamagePct != 100 || r.TakenPct != 100 {
		t.Errorf("empty tables: %+v", r)
	}
}

// An NPC gets its level's hit points at its spawn, keeps its share of them
// when the table changes, and a training dummy and the hits on it stay as
// measured.
func TestBalanceActsOnNPCsButDummies(t *testing.T) {
	w := NewWorld()
	npc := NPCFromRecord(&database.NPCRecord{ID: 9001, NPCName: "a rat", NPCLevel: 20}, 0, nil)
	dummy := NPCFromRecord(&database.NPCRecord{ID: 9002, NPCName: "Training Dummy", NPCLevel: 20}, 0, nil)
	w.NPCs[npc.ServerID], w.NPCs[dummy.ServerID] = npc, dummy
	w.SetBalance(testBalance())
	if npc.MaxHP != 3600 || npc.HP != 3600 {
		t.Errorf("a level 20 NPC under 300%%: %d/%d HP, want 3600", npc.HP, npc.MaxHP)
	}
	if dummy.MaxHP != NPCMaxHP(20) {
		t.Errorf("the dummy's hit points moved: %d", dummy.MaxHP)
	}
	npc.HP = 900 // a quarter
	w.SetBalance(nil)
	if npc.MaxHP != 1200 || npc.HP != 300 {
		t.Errorf("the table taken away: %d/%d HP, want 300/1200", npc.HP, npc.MaxHP)
	}
	w.SetBalance(testBalance())
	if got := w.npcHitLocked(npc, 18, 100); got != 125*140/100 {
		t.Errorf("a level 20 NPC's 100 on a level 18: %d, want %d", got, 125*140/100)
	}
	if got := w.takenLocked(npc, 15, 100); got != 50 {
		t.Errorf("a level 15's 100 on a level 20 NPC: %d, want 50", got)
	}
	if got := w.takenLocked(npc, 25, 100); got != 100 {
		t.Errorf("a level 25's 100 on a level 20 NPC: %d, want 100", got)
	}
	if w.npcHitLocked(dummy, 1, 100) != 100 || w.takenLocked(dummy, 1, 100) != 100 {
		t.Error("the balance touched a training dummy's fight")
	}
	if got := w.takenLocked(npc, 15, 1); got != 1 {
		t.Errorf("a hit of 1 cut to %d", got)
	}
}
