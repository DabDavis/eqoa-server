package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// An Heirloom Outfitter, and a genie selling one's stock, lists the buyer's
// class's wares alone (the user, 2026-11-08: a kappa enchanter found no
// heirlooms near its start, as every outfitter now stocks every class); the
// control, any other merchant, lists all of them.
func TestAnOutfittersListIsTheBuyersClasss(t *testing.T) {
	f := newShopFixture(t)
	warrior := f.pattern("Heirloom Helm (warrior)", 0, 1, 100, 1)
	wizard := f.pattern("Heirloom Helm (wizard)", 0, 1, 100, 1)
	every := f.pattern("Staff of Everyone", 0, 1, 100, 14)
	for id, use := range map[int]int{warrior: 1 << 0, wizard: 1 << 13, every: 32767} {
		if _, err := f.db.Exec(`UPDATE itempattern SET classuse = ? WHERE patternid = ?`, use, id); err != nil {
			t.Fatal(err)
		}
	}
	f.catalog(warrior, 1, 100)
	f.catalog(wizard, 1, 100)
	f.catalog(every, 1, 100)
	npc := f.npcs[shopMerchantID]
	names := func(class int, shop string) []string {
		npc.Name = shop
		f.player.Class = class
		var out []string
		for _, w := range f.h.merchantList(f.player, npc).wares {
			out = append(out, w.Pattern.ItemName)
		}
		return out
	}
	for _, c := range []struct {
		shop  string
		class int
		want  []string
	}{
		{"Heirloom Outfitter", 0, []string{"Heirloom Helm (warrior)", "Staff of Everyone"}},
		{"Heirloom Outfitter", 13, []string{"Heirloom Helm (wizard)", "Staff of Everyone"}},
		{entity.GenieName, 13, []string{"Heirloom Helm (wizard)", "Staff of Everyone"}},
		{"Merchant", 0, []string{"Heirloom Helm (warrior)", "Heirloom Helm (wizard)", "Staff of Everyone"}},
	} {
		got := names(c.class, c.shop)
		if len(got) != len(c.want) {
			t.Errorf("%s for class %d: %v, want %v", c.shop, c.class, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s for class %d: %v, want %v", c.shop, c.class, got, c.want)
				break
			}
		}
	}
}
