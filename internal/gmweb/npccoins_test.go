package gmweb

import "testing"

// Work board 658: an npccoins row is keyed by the name's key and holds
// 0 <= coins_min <= coins_max; checkRow refuses anything else.
func TestCheckRowHoldsNPCCoins(t *testing.T) {
	good := []map[string]any{
		{"npc_name": "lil'snapa", "coins_min": 214, "coins_max": 214},
		{"npc_name": "razeal moloch", "coins_min": 24388.0, "coins_max": 46040.0},
		{"coins_max": 9},
	}
	for _, r := range good {
		if err := checkRow("npccoins", r); err != nil {
			t.Errorf("%v refused: %v", r, err)
		}
	}
	bad := []map[string]any{
		{"npc_name": "Lil'snapa", "coins_min": 214, "coins_max": 214},
		{"npc_name": "", "coins_min": 1, "coins_max": 2},
		{"npc_name": "snowgrin", "coins_min": 5, "coins_max": 4},
		{"npc_name": "snowgrin", "coins_min": -1, "coins_max": 4},
	}
	for _, r := range bad {
		if err := checkRow("npccoins", r); err == nil {
			t.Errorf("%v allowed", r)
		}
	}
	if catalog["npccoins"].Label != "Corpse tunar" {
		t.Errorf("no Corpse tunar page: %+v", catalog["npccoins"])
	}
}

// Work board 880: a merchantprice row is keyed by the merchant's name key and
// an item, its price 0 or more.
func TestCheckRowHoldsMerchantPrices(t *testing.T) {
	for _, r := range []map[string]any{
		{"merchant": "tailor nissa", "patternid": 8316, "price": 40},
		{"merchant": "initiate firebringer", "patternid": 700168.0, "price": 0},
	} {
		if err := checkRow("merchantprice", r); err != nil {
			t.Errorf("%v refused: %v", r, err)
		}
	}
	for _, r := range []map[string]any{
		{"merchant": "Tailor Nissa", "patternid": 8316, "price": 40},
		{"merchant": "tailor nissa", "patternid": 0, "price": 40},
		{"merchant": "tailor nissa", "patternid": 8316, "price": -1},
		{"merchant": "tailor nissa", "patternid": 8316, "price": nil},
	} {
		if err := checkRow("merchantprice", r); err == nil {
			t.Errorf("%v allowed", r)
		}
	}
	if catalog["merchantprice"].Label != "Merchant prices" {
		t.Errorf("no Merchant prices page")
	}
}
