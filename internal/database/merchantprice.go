package database

import "fmt"

// merchantprice.go — a merchant's own price for an item (work board 880): live merchants charged different
// prices for the same item (a leather strip 40 at one, 50 at another; a class merchant's newbie clothing 0), and
// itempattern keeps one value an item. A row names the merchant by its name as npcclass keys it and an item
// record; its price is what one costs at that merchant's window (handlers/buyback.go waresFor), shown and
// charged. Selling is unchanged: the client works out what a merchant pays from the player's own record. Read
// at every window open and buy: a GM's change counts at once.
const merchantPriceSchema = `
	CREATE TABLE IF NOT EXISTS merchantprice (
		merchant  TEXT NOT NULL,
		patternid INTEGER NOT NULL,
		price     INTEGER NOT NULL,
		source    TEXT DEFAULT 'gm',
		note      TEXT DEFAULT '',
		PRIMARY KEY (merchant, patternid)
	);`

// ensureMerchantPrices makes table merchantprice (initSchema).
func (db *SQLiteDB) ensureMerchantPrices() error {
	if _, err := db.Exec(merchantPriceSchema); err != nil {
		return fmt.Errorf("merchantprice: %w", err)
	}
	return nil
}

// MerchantPrices is a merchant's own prices by item record (none: nil), the merchant by its NPC name. A
// negative price is left out (the panel refuses one).
func (s *InventoryStore) MerchantPrices(merchant string) (map[int]int, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT patternid, price FROM merchantprice WHERE merchant = ?`, NPCClassKey(merchant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out map[int]int
	for rows.Next() {
		var id, price int
		if err := rows.Scan(&id, &price); err != nil {
			return nil, err
		}
		if price < 0 {
			continue
		}
		if out == nil {
			out = map[int]int{}
		}
		out[id] = price
	}
	return out, rows.Err()
}
