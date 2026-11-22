package database

import "testing"

// Work board 880: merchantprice is made at the start and read by the
// merchant's name key; a negative price is left out.
func TestMerchantPrices(t *testing.T) {
	db, err := NewSQLite(t.TempDir() + "/prices.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv := NewInventoryStore(db)
	if got, err := inv.MerchantPrices("Tailor Nissa"); got != nil || err != nil {
		t.Fatalf("no rows: %v %v", got, err)
	}
	if _, err := db.Exec(`INSERT INTO merchantprice (merchant, patternid, price) VALUES
		('tailor nissa', 8316, 40), ('tailor nissa', 7, 0), ('tailor nissa', 8, -5), ('merchant efreya', 8316, 50)`); err != nil {
		t.Fatal(err)
	}
	got, err := inv.MerchantPrices("Tailor Nissa ")
	if err != nil || len(got) != 2 || got[8316] != 40 || got[7] != 0 {
		t.Fatalf("Tailor Nissa's prices %v (%v)", got, err)
	}
}
