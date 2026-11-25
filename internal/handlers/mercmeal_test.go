package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A grocer's drink and a meal given (petition 3061) reach the merc's kit as what it drinks
// and eats (entity/mercmeal.go): the drink's PoT, the meal's HoT.
func TestMercGivenDrinkAndMealOnItsKit(t *testing.T) {
	r, m, db := newMercFixture(t)
	pot, hot := 30, 40
	putItem(t, db, database.ItemPatternRecord{PatternID: 778001, ItemName: "Elven Wine", ItemDesc: "A fine beverage that aids power regeneration.",
		EquipSlot: -1, ProcAnim: database.ItemTypeUsable, MaxStack: 20, PoT: &pot})
	putItem(t, db, database.ItemPatternRecord{PatternID: 778002, ItemName: "Spiced Jerky", ItemDesc: "A hearty meal that aids health regeneration.",
		EquipSlot: -1, ProcAnim: database.ItemTypeUsable, MaxStack: 20, HoT: &hot})
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1) // a wizard
	r.pack(db, [2]int{778001, 3}, [2]int{778002, 2})
	r.drain()
	m.Command(r.anna, r.pa, "give elven wine")
	m.Command(r.anna, r.pa, "give spiced jerky")
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	k, err := m.kit(row, r.pa.Level)
	if err != nil {
		t.Fatal(err)
	}
	var wine, jerky bool
	for _, c := range k.Carried {
		switch c.Name {
		case "Elven Wine":
			wine = c.Drink && !c.Food && c.PoT == 30 && c.Count == 3
		case "Spiced Jerky":
			jerky = c.Food && !c.Drink && c.HoT == 40 && c.Count == 2
		}
	}
	if !wine || !jerky {
		t.Errorf("its kit carries %+v", k.Carried)
	}
}
