package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A creature killed holding a weapon or a shield drops it (helddrop.go): the
// catalog's record of its look for its level (a plain one but 1 in 500: the
// rules are helddroprule_test.go's), of several the lowest patternid,
// any record of the look (No Trade, Lore, No Rent, an heirloom, an Ascended copy,
// an epic, a quest's: the owner, 2026-11-18, ticket 2366) but an item a player
// owns alone and a GM's test record; nothing for a look no record has, for a pet,
// or with held drops off (the control).

const (
	heldSwordLook  = 0xC0000001 // a model past 2^31, written signed in itempattern
	heldShieldLook = 0x0000BEEF
	heldNoLook     = 0x0BADF00D
)

// heldDB is a database whose catalog has the sword look's records at levels 5,
// 20 (two) and the barred ones, the shield look's at 10, and a breastplate of
// the sword's look; its HeldDrops map turned on.
func heldDB(t *testing.T) (*database.InventoryStore, *database.HeldDrops) {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "held.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	look := uint32(heldSwordLook)
	sword := int64(int32(look))
	type rec struct {
		id                             int
		name                           string
		slot, level, trade, rent, lore int
		model                          int64
	}
	for _, r := range []rec{
		{8001, "rusty short sword", 14, 5, 0, 1, 0, sword},
		{8002, "fine short sword", 12, 20, 0, 1, 0, sword},
		{8003, "gleaming short sword", 14, 20, 0, 1, 0, sword},
		{8004, "bound short sword", 14, 25, 1, 1, 0, sword}, // NO TRADE
		{8005, "lore short sword", 14, 25, 0, 1, 1, sword},  // LORE
		{8006, "Ascended fine short sword", 12, 25, 0, 1, 0, sword},
		{8007, "family short sword", 14, 26, 0, 1, 0, sword},     // an heirloom
		{8008, "reward short sword", 14, 27, 0, 1, 0, sword},     // a quest's reward
		{8011, "rented short sword", 14, 28, 0, 0, 0, sword},     // NO RENT
		{8012, "Sword of the Epic Test", 14, 29, 0, 1, 0, sword}, // a GM's test record
		{8013, "owned short sword", 14, 21, 0, 1, 0, sword},      // a player's alone (itemowner)
		{8009, "wooden shield", 13, 10, 0, 1, 0, heldShieldLook},
		{8010, "short sword breastplate", 1, 1, 0, 1, 0, sword}, // worn on the chest: no hand's
	} {
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4,
			attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,
			itemname, itemdesc, model, color) VALUES (?, 10, 0, 1, 0, ?, 0, ?, ?, 0, 1, 5, 0, ?, 1, 100, 0, 32767, 1023, 0, ?, 0, 0, ?, '', ?, 0)`,
			r.id, r.slot, r.trade, r.rent, r.level, r.lore, r.name, r.model); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS heirlooms (patternid INTEGER PRIMARY KEY, class INTEGER NOT NULL, role TEXT NOT NULL, epic INTEGER NOT NULL DEFAULT 0)`,
		`INSERT INTO heirlooms (patternid, class, role) VALUES (8007, 0, 'tank')`,
		`CREATE TABLE IF NOT EXISTS queststeps (questid INTEGER NOT NULL, step INTEGER NOT NULL, items TEXT NOT NULL DEFAULT '',
			rewarditems TEXT NOT NULL DEFAULT '', choices TEXT NOT NULL DEFAULT '', byclass TEXT NOT NULL DEFAULT '', PRIMARY KEY (questid, step))`,
		`INSERT INTO queststeps (questid, step, rewarditems) VALUES (900001, 1, '8008')`,
		`CREATE TABLE IF NOT EXISTS itemowner (patternid INTEGER PRIMARY KEY, serverid INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO itemowner (patternid, serverid) VALUES (8013, 5)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%v\n%s", err, q)
		}
	}
	inv := database.NewInventoryStore(db)
	d, err := inv.HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	return inv, d
}

func TestHeldDropsMapALookToItsLevelsRecord(t *testing.T) {
	_, d := heldDB(t)
	if d.Records != 10 {
		t.Errorf("%d records by look, want the nine swords and the shield (left out: %v)", d.Records, d.Left)
	}
	for _, why := range []string{"test", "owned"} {
		if d.Left[why] != 1 {
			t.Errorf("left out as %q: %d, want 1 (%v)", why, d.Left[why], d.Left)
		}
	}
	if len(d.Left) != 2 {
		t.Errorf("left out %v, want the test record and the owned one alone", d.Left)
	}
	// the window off (the nearest at or below, as before it): the lists' order and bars
	d.LevelBelow, d.LevelAbove = 100, 0
	pick := func(model uint32, shield bool, level int, who string, rare bool) string {
		e, ok := d.Pick(model, shield, level, who, rare)
		if !ok {
			return ""
		}
		return e.Pattern.ItemName
	}
	name := func(model uint32, shield bool, level int, who string) string {
		return pick(model, shield, level, who, false)
	}
	if got := name(heldSwordLook, false, 10, "gnoll scout"); got != "rusty short sword" {
		t.Errorf("level 10: %q, want the level 5 record", got)
	}
	if got := name(heldSwordLook, false, 3, "gnoll scout"); got != "" || !d.Has(heldSwordLook, false) {
		t.Errorf("level 3: %q, want nothing (none at or below), the look known", got)
	}
	// plain: of several of a levelreq, the lowest patternid, whatever the name; the owned
	// level 21 record never; No Trade, Lore and No Rent are rare records: a plain draw never
	// drops one (the "Ascended" name alone makes no copy: no itemascended row)
	for _, c := range []struct {
		level int
		rare  bool
		want  string
	}{{20, false, "fine short sword"}, {21, false, "fine short sword"}, {25, false, "Ascended fine short sword"},
		{26, false, "family short sword"}, {27, false, "reward short sword"}, {60, false, "reward short sword"},
		// a rare draw: the rare record nearest, else the plain one
		{20, true, "fine short sword"}, {25, true, "bound short sword"}, {27, true, "bound short sword"},
		{28, true, "rented short sword"}, {60, true, "rented short sword"}} {
		for _, who := range []string{"gnoll scout", "orc pawn", "Ice goblin warrior"} {
			if got := pick(heldSwordLook, false, c.level, who, c.rare); got != c.want {
				t.Errorf("%s at %d (rare %v): %q, want %q", who, c.level, c.rare, got, c.want)
			}
		}
	}
	if got := name(heldShieldLook, true, 30, "gnoll scout"); got != "wooden shield" {
		t.Errorf("the shield: %q", got)
	}
	if got := name(heldShieldLook, false, 30, "gnoll scout"); got != "" {
		t.Errorf("the shield's look in a hand: %q, want nothing (no hand's record)", got)
	}
	if got := name(heldSwordLook, true, 30, "gnoll scout"); got != "" {
		t.Errorf("the sword's look as a shield: %q", got)
	}
	if d.Has(heldNoLook, false) || name(heldNoLook, false, 60, "x") != "" {
		t.Error("an unknown look maps")
	}
	rep := HeldDropsReport(d, [][2]uint32{{heldSwordLook, 0}, {heldSwordLook, 0}, {heldShieldLook, 1}, {heldNoLook, 0}})
	if rep.Models != 3 || rep.Mapped != 2 || rep.Plain != 2 || rep.RareOnly != 0 || rep.Rows != 4 || rep.RowsMapped != 3 || rep.Unmapped[[2]uint32{heldNoLook, 0}] != 1 {
		t.Errorf("report %+v", rep)
	}
}

func TestACorpseDropsWhatItsCreatureHeld(t *testing.T) {
	inv, d := heldDB(t)
	d.LevelBelow = 25 // the level 10 shield in reach of a level 30 kobold
	h := NewNPCWindowHandler(nil, inv)
	h.lootFloat = func() float64 { return 0.999999 }
	h.lootIntn = func(n int) int { return n - 1 }
	h.coinRoll = func(int) int { return 0 }
	h.heldFloat = func() float64 { return 0.5 } // a plain draw
	names := func(c *corpse) map[string]int {
		out := map[string]int{}
		for _, it := range c.items {
			out[it.Pattern.ItemName]++
		}
		return out
	}
	scout := func() *entity.NPC {
		return &entity.NPC{ServerID: 77001, NPCID: 77001, Name: "kobold runt", Level: 30, World: 0,
			PrimaryWeapon: int(uint32(heldSwordLook)), Shield: heldShieldLook}
	}
	SetHeldDrops(d)
	defer SetHeldDrops(nil)

	got := names(h.rollCorpse(scout(), time.Now(), 30))
	want, _ := d.Pick(heldSwordLook, false, 30, "kobold runt", false)
	if want.Pattern.ItemName != "reward short sword" || got[want.Pattern.ItemName] != 1 || got["wooden shield"] != 1 || len(got) != 2 {
		t.Errorf("a level 30 kobold with a sword and shield left %v, want %s and the wooden shield", got, want.Pattern.ItemName)
	}
	for _, it := range h.rollCorpse(scout(), time.Now(), 30).items {
		if it.StackLeft != 1 || it.RemainHP != it.Pattern.MaxHP || it.Pattern.PatternID == 0 {
			t.Errorf("a held drop's record %+v", it)
		}
	}
	// carrying the shield alone (its spawn's roll), the shield alone
	bare := scout()
	bare.PrimaryWeapon = 0
	if got := names(h.rollCorpse(bare, time.Now(), 30)); len(got) != 1 || got["wooden shield"] != 1 {
		t.Errorf("a kobold with the shield alone left %v", got)
	}
	// judged against level 5: the level 27 sword is past its reach (fitItems), the shield not
	if got := names(h.rollCorpse(scout(), time.Now(), 5)); len(got) != 1 || got["wooden shield"] != 1 {
		t.Errorf("judged at level 5: %v, want the shield alone", got)
	}
	// a look no record has: nothing extra
	odd := scout()
	odd.PrimaryWeapon, odd.Shield = heldNoLook, 0
	if got := names(h.rollCorpse(odd, time.Now(), 30)); len(got) != 0 {
		t.Errorf("an unknown look dropped %v", got)
	}
	// a pet's corpse: nothing
	pet := scout()
	pet.Owner = 5
	if got := names(h.rollCorpse(pet, time.Now(), 30)); got["wooden shield"] != 0 {
		t.Errorf("a pet dropped %v", got)
	}
	// the control: held drops off, nothing extra
	SetHeldDrops(nil)
	if got := names(h.rollCorpse(scout(), time.Now(), 30)); len(got) != 0 {
		t.Errorf("held drops off: %v", got)
	}
}
