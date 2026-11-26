package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// Petition 3285: a meal or a grocer's drink given to a merc is Used from its
// inventory (vanilla's window's ask 8, /merc use, the Inventory box's Use): it
// eats or drinks it now, the effect on it as its own turn puts one, one of the
// record used. The window's state marks such a record kind 4 to a client that
// reads layout 2, carried (0) to one that reads 1. The controls: a worn or
// wearable record has no Use, a drink is refused a merc with no power.
func TestMercUseFoodAndDrink(t *testing.T) {
	r, m, db := newMercFixture(t)
	pot, hot := 30, 40
	putItem(t, db, database.ItemPatternRecord{PatternID: 778001, ItemName: "Elven Wine", ItemDesc: "A fine beverage that aids power regeneration.",
		EquipSlot: -1, ProcAnim: database.ItemTypeUsable, MaxStack: 20, PoT: &pot})
	putItem(t, db, database.ItemPatternRecord{PatternID: 778002, ItemName: "Spiced Jerky", ItemDesc: "A hearty meal that aids health regeneration.",
		EquipSlot: -1, ProcAnim: database.ItemTypeUsable, MaxStack: 20, HoT: &hot})
	invItems(t, db)
	r.gw.OnMercNote(m.note) // as Start hears the World
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1) // a wizard: power to fill
	r.pack(db, [2]int{778001, 3}, [2]int{778002, 2}, [2]int{itBuckler, 1})
	r.drain()
	for _, c := range []string{"give elven wine", "give spiced jerky", "give buckler"} {
		m.Command(r.anna, r.pa, c)
	}
	npc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	if npc == nil {
		t.Fatal("no merc out")
	}
	npc.HP, npc.Power = npc.MaxHP/2, npc.MaxPower/2 // over its own turns' marks: only a Use feeds it
	idOf := func(pattern int) int {
		t.Helper()
		items, _ := db.MercItems(r.pa.CharServerID, 1)
		for _, it := range items {
			if it.PatternID == pattern {
				return it.ID
			}
		}
		t.Fatalf("the merc holds no %d", pattern)
		return 0
	}
	effect := func(id int) *entity.ActiveEffect {
		for _, e := range npc.Effects {
			if e.SpellID == id {
				return e
			}
		}
		return nil
	}

	// the window: layout 2 marks the meal and the drink, not the buckler; layout 1 none
	h := NewMercWindowHandler(r.gw, m)
	ask := func(op byte, id int) {
		t.Helper()
		b := []byte{1, op, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24)}
		if err := h.HandleAsk(r.anna, incoming(protocol.OpcodeMercWindowAsk, b)); err != nil {
			t.Fatal(err)
		}
	}
	names := map[int64]string{778001: "Elven Wine", 778002: "Spiced Jerky", itBuckler: "Buckler"}
	kinds := func() map[string]byte {
		t.Helper()
		s, ok := lastMercWinState(t, r.anna)
		if !ok {
			t.Fatal("no state")
		}
		out := map[string]byte{}
		for _, rec := range s.recs {
			out[names[rec.item.pattern]] = rec.kind
		}
		return out
	}
	r.anna.SetMercWindow(1)
	ask(mercAskOpen, 0)
	if k := kinds(); k["Elven Wine"] != mercRecCarried || k["Spiced Jerky"] != mercRecCarried || k["Buckler"] != mercRecCarried {
		t.Errorf("layout 1's kinds %v, want every one carried", k)
	}
	r.anna.SetMercWindow(mercWindowLayoutUse)
	ask(mercAskOpen, 0)
	if k := kinds(); k["Elven Wine"] != mercRecUse || k["Spiced Jerky"] != mercRecUse || k["Buckler"] != mercRecCarried {
		t.Errorf("layout 2's kinds %v, want the wine and the jerky Use (4), the buckler carried", k)
	}

	// Use the jerky from the window: eaten, one used, the state again
	hp := npc.HP
	r.drain()
	ask(mercAskUse, idOf(778002))
	if got := lastMercResult(t, r.anna); got != [3]byte{0x73, mercAskUse, 1} {
		t.Fatalf("use: % x", got)
	}
	if e := effect(entity.FoodEffectID); e == nil || e.Name != "Spiced Jerky" || e.Mods.HoT != 40 {
		t.Fatalf("its meal %+v", e)
	}
	if npc.HP != hp+40 {
		t.Errorf("HP %d, want %d (the first tick at once)", npc.HP, hp+40)
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, 778002); n != 1 {
		t.Errorf("it holds %d jerky, want 1", n)
	}
	if s, ok := lastMercWinState(t, r.anna); !ok || len(s.recs) == 0 {
		t.Error("no state after the use")
	}
	if l := strings.Join(lines(t, r.anna), "|"); !strings.Contains(l, "eats Spiced Jerky") {
		t.Errorf("lines %q", l)
	}

	// /merc use elven wine: drunk
	pw := npc.Power
	m.Command(r.anna, r.pa, "use elven wine")
	if e := effect(entity.DrinkEffectID); e == nil || e.Name != "Elven Wine" || e.Mods.PoT != 30 {
		t.Fatalf("its drink %+v", e)
	}
	if npc.Power != min(pw+30, npc.MaxPower) {
		t.Errorf("power %d, want %d", npc.Power, min(pw+30, npc.MaxPower))
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, 778001); n != 2 {
		t.Errorf("it holds %d wine, want 2", n)
	}
	if l := strings.Join(lines(t, r.anna), "|"); !strings.Contains(l, "drinks Elven Wine") {
		t.Errorf("lines %q", l)
	}

	// a second meal takes the first's place
	ask(mercAskUse, idOf(778002))
	meals := 0
	for _, e := range npc.Effects {
		if e.SpellID == entity.FoodEffectID {
			meals++
		}
	}
	if meals != 1 {
		t.Errorf("%d meals on it, want 1", meals)
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, 778002); n != 0 {
		t.Errorf("it holds %d jerky, want none", n)
	}

	// the controls: the buckler has no Use; a drink for a merc with no power
	r.drain()
	ask(mercAskUse, idOf(itBuckler))
	if got := lastMercResult(t, r.anna); got[2] != 0 {
		t.Error("the buckler was used")
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBuckler); n != 1 {
		t.Errorf("the buckler %d", n)
	}
	npc.MaxPower = 0
	ask(mercAskUse, idOf(778001))
	if got := lastMercResult(t, r.anna); got[2] != 0 {
		t.Error("a drink went to a merc with no power")
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, 778001); n != 2 {
		t.Errorf("it holds %d wine, want 2 (none drunk)", n)
	}
}
