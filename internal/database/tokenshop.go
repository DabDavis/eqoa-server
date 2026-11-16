package database

import (
	"fmt"
	"strings"
	"time"
)

// tokenshop.go — shops paid in an item, not in tunar (the owner, 2026-11-16:
// "add Toy Shoppe close to the tree by Weapon Enchanter. The NPC will sell our
// toys like barrel of monkeys, snowgun, etc. We will make more like Santa's
// Elf illusion the gnome with red/green clothing etc."; handlers/tokenshop.go).
//
//   - tokenshop: what each token shop sells, a row an item: the shop (the NPC's
//     name: every NPC of a name with rows here is that shop), the record sold,
//     its price, the item it is paid in (empty: the season's token, Holly
//     Token), what the shopkeeper says of it before the buyer says yes (empty:
//     the record's own description), and its place in the list (sort, then the
//     price). A row of price 0 or less is not offered. Read at every talk and
//     again at the buyer's Yes, so a GM's row counts at once.
//     tools/toy_shoppe_20261116.py writes the Toy Shoppe's three.
//   - tokenshop_purchases: every purchase, written as it is made: when, who,
//     the shop, the item, its price and what it was paid in. A log: nothing
//     reads it back.
//   - itemoutfit: what an item's illusion dresses its bearer in (entity/
//     outfit.go): while the body an item's use gave is the one shown, the
//     bearer's own armour, robe and their colours are not drawn but these
//     are, a row a dye slot (chardyes' numbers: chest 0 .. helm 5, robe 9, the
//     trims 11-16): look, the armour set (0-8; the robe's style, 255 none)
//     and color, 0xRRGGBBAA (0: none; a PS2 is sent the disc tint nearest
//     it, and no trim). An armour slot with no row is bare, and no robe is
//     worn without one. Santa's Elf's (716912) is a gnome's red and green.

// TokenShopSchema, TokenShopPurchasesSchema and ItemOutfitSchema are the three
// tables (tools/toy_shoppe_20261116.py makes them by the same words where a
// server has not yet: its test holds the two to each other).
const TokenShopSchema = `CREATE TABLE IF NOT EXISTS tokenshop (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	shop      TEXT NOT NULL,
	patternid INTEGER NOT NULL,
	price     INTEGER NOT NULL,
	token     TEXT NOT NULL DEFAULT '',
	says      TEXT NOT NULL DEFAULT '',
	sort      INTEGER NOT NULL DEFAULT 0,
	source    TEXT NOT NULL DEFAULT 'gm',
	UNIQUE (shop, patternid)
)`

const TokenShopPurchasesSchema = `CREATE TABLE IF NOT EXISTS tokenshop_purchases (
	id        INTEGER PRIMARY KEY AUTOINCREMENT,
	at        TEXT NOT NULL,
	serverid  INTEGER NOT NULL,
	charname  TEXT NOT NULL DEFAULT '',
	accountid INTEGER NOT NULL DEFAULT 0,
	shop      TEXT NOT NULL DEFAULT '',
	patternid INTEGER NOT NULL,
	item      TEXT NOT NULL DEFAULT '',
	price     INTEGER NOT NULL,
	token     TEXT NOT NULL DEFAULT ''
)`

const ItemOutfitSchema = `CREATE TABLE IF NOT EXISTS itemoutfit (
	patternid INTEGER NOT NULL,
	slot      INTEGER NOT NULL,
	look      INTEGER NOT NULL DEFAULT 0,
	color     INTEGER NOT NULL DEFAULT 0,
	note      TEXT NOT NULL DEFAULT '',
	source    TEXT NOT NULL DEFAULT 'gm',
	PRIMARY KEY (patternid, slot)
)`

// ensureTokenShop makes the three tables (initSchema).
func (db *SQLiteDB) ensureTokenShop() error {
	for _, q := range []struct{ name, schema string }{
		{"tokenshop", TokenShopSchema}, {"tokenshop_purchases", TokenShopPurchasesSchema}, {"itemoutfit", ItemOutfitSchema}} {
		if _, err := db.Exec(q.schema); err != nil {
			return fmt.Errorf("%s: %w", q.name, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tokenshop_purchases_char ON tokenshop_purchases(serverid)`); err != nil {
		return fmt.Errorf("tokenshop_purchases: %w", err)
	}
	return nil
}

// TokenShopRow is a row of tokenshop.
type TokenShopRow struct {
	ID        int
	Shop      string
	PatternID int
	Price     int
	Token     string // "": the season's token
	Says      string
	Sort      int
}

// TokenShopStock is what the shop of the name sells (its name matched with
// no regard to case or the spaces round it): the rows of a price above 0, by
// sort, then the cheapest first, then as they were written.
func (db *SQLiteDB) TokenShopStock(shop string) ([]TokenShopRow, error) {
	rows, err := db.Query(`SELECT id, shop, patternid, price, token, says, sort FROM tokenshop
		WHERE lower(trim(shop)) = ? AND price > 0 ORDER BY sort, price, id`, strings.ToLower(strings.TrimSpace(shop)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TokenShopRow
	for rows.Next() {
		var r TokenShopRow
		if err := rows.Scan(&r.ID, &r.Shop, &r.PatternID, &r.Price, &r.Token, &r.Says, &r.Sort); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TokenShops is every shop's name (in lower case, trimmed) that has a row
// offered.
func (db *SQLiteDB) TokenShops() (map[string]bool, error) {
	rows, err := db.Query(`SELECT DISTINCT lower(trim(shop)) FROM tokenshop WHERE price > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = true
	}
	return out, rows.Err()
}

// TokenShopPurchase is a purchase as tokenshop_purchases keeps it.
type TokenShopPurchase struct {
	At        time.Time
	ServerID  uint32
	CharName  string
	AccountID int
	Shop      string
	PatternID int
	Item      string
	Price     int
	Token     string
}

// LogTokenShopPurchase keeps a purchase.
func (db *SQLiteDB) LogTokenShopPurchase(p TokenShopPurchase) error {
	_, err := db.Exec(`INSERT INTO tokenshop_purchases (at, serverid, charname, accountid, shop, patternid, item, price, token)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.At.UTC().Format(time.RFC3339), p.ServerID, p.CharName, p.AccountID, p.Shop, p.PatternID, p.Item, p.Price, p.Token)
	return err
}

// ItemOutfitRow is an item's outfit as itemoutfit holds it: each armour
// slot's set (chest .. helm), the robe's style (0xFF none) and a colour per
// dye slot (0 none).
type ItemOutfitRow struct {
	PatternID int
	Sets      [6]byte
	Robe      byte
	Colors    [DyeSlots]uint32
}

// ItemOutfits is every item's outfit by its record. A row of a slot that is
// no dye slot is left out; an armour set past 255 is taken as 0 (bare).
func (db *SQLiteDB) ItemOutfits() (map[int]ItemOutfitRow, error) {
	rows, err := db.Query(`SELECT patternid, slot, look, color FROM itemoutfit ORDER BY patternid, slot`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]ItemOutfitRow{}
	for rows.Next() {
		var pid, slot int
		var look, color int64
		if err := rows.Scan(&pid, &slot, &look, &color); err != nil {
			return nil, err
		}
		if slot < 0 || slot >= DyeSlots {
			continue
		}
		o, ok := out[pid]
		if !ok {
			o = ItemOutfitRow{PatternID: pid, Robe: 0xFF}
		}
		switch {
		case slot <= DyeHelm:
			if look >= 0 && look <= 255 {
				o.Sets[slot] = byte(look)
			}
		case slot == DyeRobe:
			if look >= 0 && look < 255 {
				o.Robe = byte(look)
			}
		}
		o.Colors[slot] = uint32(color)
		out[pid] = o
	}
	return out, rows.Err()
}
