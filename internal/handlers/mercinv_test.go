package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// What a merc holds (mercinv.go): given from the pack, worn by the player's
// gates and slots (its AC and hands from what it wears), taken back; what it
// held back in the pack at a dismissal (the keeping when the pack is full,
// never lost), kept through a death, and kept apart from a new hire of its
// slot; a Cocktail alchemist's aoe or single kept; the alchemists' stock an
// owner's, a new hire's too, taken into the pack.

func putItem(t *testing.T, db *database.SQLiteDB, r database.ItemPatternRecord) {
	t.Helper()
	if r.ClassUse == 0 {
		r.ClassUse = 32767
	}
	if r.RaceUse == 0 {
		r.RaceUse = 32767
	}
	if r.MaxStack == 0 {
		r.MaxStack = 1
	}
	r.Rent = 1
	if err := database.NewItemPatternStore(db).UpsertItemPattern(&r); err != nil {
		t.Fatal(err)
	}
}

// pack gives Anna the items (pattern, stack) and her client's list of them.
func (r *portalRig) pack(db *database.SQLiteDB, items ...[2]int) {
	r.t.Helper()
	inv := database.NewInventoryStore(db)
	for _, it := range items {
		if _, err := inv.AddCharItem(r.pa.CharServerID, it[0], it[1], 100, 0, database.LocPack); err != nil {
			r.t.Fatal(err)
		}
	}
	r.relist(db)
}

// relist reads Anna's client's list again from her pack, as a dump would.
func (r *portalRig) relist(db *database.SQLiteDB) {
	r.t.Helper()
	rows, err := database.NewInventoryStore(db).GetCharacterItems(r.pa.CharServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	var pack []database.CharInventoryEntry
	for _, x := range rows {
		if x.Location == database.LocPack {
			pack = append(pack, x)
		}
	}
	r.anna.SetInventoryList(newInvList(pack))
}

func packHas(t *testing.T, db *database.SQLiteDB, char uint32, pattern int) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, char, pattern).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mercHas(t *testing.T, db *database.SQLiteDB, char uint32, slot, pattern int) (n, equip int) {
	t.Helper()
	equip = -2
	rows, err := db.Query(`SELECT stack, equiploc FROM charmercitem WHERE serverid = ? AND slot = ? AND patternid = ?`, char, slot, pattern)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s, e int
		rows.Scan(&s, &e)
		n, equip = n+s, e
	}
	return n, equip
}

func (r *portalRig) mercInfo(slot int) entity.MercInfo {
	r.t.Helper()
	for _, i := range r.gw.MercInfosOf(r.anna) {
		if i.Slot == slot {
			return i
		}
	}
	r.t.Fatalf("no merc %d out", slot)
	return entity.MercInfo{}
}

const (
	itBreastplate = 910001
	itTooHigh     = 910002
	itWizardsOnly = 910003
	itGreatsword  = 910004
	itBuckler     = 910005
	itStone       = 910006
	itPotion      = 910007
)

func invItems(t *testing.T, db *database.SQLiteDB) {
	n := func(v int) *int { return &v }
	putItem(t, db, database.ItemPatternRecord{PatternID: itBreastplate, ItemName: "Mithril Breastplate", EquipSlot: 5, LevelReq: 20, AC: n(900), Model: 9})
	putItem(t, db, database.ItemPatternRecord{PatternID: itTooHigh, ItemName: "Crown of Ages", EquipSlot: 1, LevelReq: 55, AC: n(100)})
	putItem(t, db, database.ItemPatternRecord{PatternID: itWizardsOnly, ItemName: "Robe of the Magi", EquipSlot: 2, LevelReq: 1, ClassUse: 1 << 13})
	putItem(t, db, database.ItemPatternRecord{PatternID: itGreatsword, ItemName: "Greatsword", EquipSlot: 15, LevelReq: 1, WeaponDamage: 300,
		AttackType: 1, Model: 0x11112222})
	putItem(t, db, database.ItemPatternRecord{PatternID: itBuckler, ItemName: "Buckler", EquipSlot: 13, LevelReq: 1, AC: n(20), Model: 0x33334444})
	putItem(t, db, database.ItemPatternRecord{PatternID: itStone, ItemName: "Lucky Stone", EquipSlot: -1, LevelReq: 1, Lore: 1, Trade: 1})
	putItem(t, db, database.ItemPatternRecord{PatternID: itPotion, ItemName: "Volatile Potion", EquipSlot: -1, LevelReq: 1, MaxStack: 100})
	// NO RENT, so no heirloom takes its figures from them (heirloom.go's heirloomUnfit)
	if _, err := db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid BETWEEN 910001 AND 910007`); err != nil {
		t.Fatal(err)
	}
}

func TestMercGiveEquipTakeBack(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1) // a wizard: a tank's AC has its level's floor (entity/mercroles.go)
	before := r.mercInfo(1)
	r.pack(db, [2]int{itBreastplate, 1}, [2]int{itStone, 1})
	r.drain()
	m.Command(r.anna, r.pa, "give mithril")
	if packHas(t, db, r.pa.CharServerID, itBreastplate) != 0 {
		t.Fatal("the breastplate is still in the pack")
	}
	if n, e := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); n != 1 || e != -1 {
		t.Fatalf("the merc holds %d (equip %d)", n, e)
	}
	if l := sessionInvList(r.anna); l.held(itBreastplate) != 0 {
		t.Error("the client's list still holds it")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:You give Mithril Breastplate to "+before.Name+".") {
		t.Errorf("sent %q", got)
	}
	m.Command(r.anna, r.pa, "equip mithril")
	worn := r.mercInfo(1)
	if _, e := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); e != 5 {
		t.Fatalf("worn in %d, want the chest (5)", e)
	}
	if worn.AC <= before.AC {
		t.Errorf("AC %d wearing the breastplate, %d before", worn.AC, before.AC)
	}
	m.Command(r.anna, r.pa, "unequip mithril")
	if off := r.mercInfo(1); off.AC != before.AC {
		t.Errorf("AC %d after it came off, %d before", off.AC, before.AC)
	}
	// a NO TRADE LORE item to its own merc and back
	m.Command(r.anna, r.pa, "give lucky stone")
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itStone); n != 1 {
		t.Fatal("a no-trade lore item did not go to its owner's merc")
	}
	m.Command(r.anna, r.pa, "take lucky")
	m.Command(r.anna, r.pa, "take mithril")
	if packHas(t, db, r.pa.CharServerID, itBreastplate) != 1 || packHas(t, db, r.pa.CharServerID, itStone) != 1 {
		t.Fatal("not taken back")
	}
	if l := sessionInvList(r.anna); l.held(itBreastplate) != 1 || l.held(itStone) != 1 {
		t.Error("the client's list was not told")
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); n != 0 {
		t.Error("the merc still holds it")
	}
	if after := r.mercInfo(1); after.AC != before.AC {
		t.Errorf("AC %d after it was taken back, %d before", after.AC, before.AC)
	}
}

// The player's gates and slots: a level, a class mask; a two-hander takes the
// main hand and the shield off its heirlooms, a shield then takes the
// two-hander off.
func TestMercEquipGatesAndSlots(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.pack(db, [2]int{itTooHigh, 1}, [2]int{itWizardsOnly, 1}, [2]int{itGreatsword, 1}, [2]int{itBuckler, 1})
	for _, w := range []string{"crown", "robe of the magi", "greatsword", "buckler"} {
		m.Command(r.anna, r.pa, "give "+w)
	}
	r.drain()
	m.Command(r.anna, r.pa, "equip crown")
	if _, e := mercHas(t, db, r.pa.CharServerID, 1, itTooHigh); e != -1 {
		t.Error("a level 30 wears a level 55 crown")
	}
	m.Command(r.anna, r.pa, "equip robe of the magi")
	if _, e := mercHas(t, db, r.pa.CharServerID, 1, itWizardsOnly); e != -1 {
		t.Error("a warrior wears a wizard's robe")
	}
	if got := r.msgs(r.anna); !strings.Contains(strings.Join(got, "\n"), "cannot wear") {
		t.Errorf("no refusal said: %q", got)
	}
	if w := r.mercInfo(1).Worn; !w[14] || !w[17] {
		t.Fatalf("its heirloom hands: %v", w[14:19])
	}
	m.Command(r.anna, r.pa, "equip greatsword")
	if w := r.mercInfo(1).Worn; !w[16] || w[14] || w[17] {
		t.Fatalf("the two-hander: hands %v, want 16 alone", w[14:19])
	}
	m.Command(r.anna, r.pa, "equip buckler")
	if _, e := mercHas(t, db, r.pa.CharServerID, 1, itGreatsword); e != -1 {
		t.Errorf("the two-hander stays worn (%d) under a shield", e)
	}
	if w := r.mercInfo(1).Worn; w[16] || !w[17] {
		t.Errorf("after the shield: hands %v", w[14:19])
	}
}

// A dismissal gives back what it held: into the pack, else the keeping (the
// pack full), from which /merc claim takes it once there is room. Never lost.
// A death keeps it with the merc; a new hire of a slot whose row went another
// way starts with nothing of the old one's.
func TestMercItemsComeBack(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.pack(db, [2]int{itBreastplate, 1}, [2]int{itBuckler, 1})
	m.Command(r.anna, r.pa, "give mithril")
	m.Command(r.anna, r.pa, "give buckler")
	m.Command(r.anna, r.pa, "equip mithril")
	// a death: it keeps them
	m.note(entity.MercNote{Kind: entity.MercNoteDied, Owner: r.pa, Slot: 1, Until: m.now()})
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); n != 1 {
		t.Fatal("a death lost what it held")
	}
	// the pack full: a dismissal puts them in the keeping
	r.pack(db, [2]int{itStone, 1})
	l := sessionInvList(r.anna)
	l.cap = 1
	m.Command(r.anna, r.pa, "dismiss")
	if n, _ := mercHas(t, db, r.pa.CharServerID, database.MercKeeping, itBreastplate); n != 1 {
		t.Fatalf("the breastplate is not in the keeping (pack %d)", packHas(t, db, r.pa.CharServerID, itBreastplate))
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, database.MercKeeping, itBuckler); n != 1 {
		t.Fatal("the buckler is not in the keeping")
	}
	l.cap = 40
	m.Command(r.anna, r.pa, "claim")
	if packHas(t, db, r.pa.CharServerID, itBreastplate) != 1 || packHas(t, db, r.pa.CharServerID, itBuckler) != 1 {
		t.Fatal("claim did not bring them")
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, database.MercKeeping, itBreastplate); n != 0 {
		t.Fatal("the keeping still holds it: a copy")
	}
	// with room, a dismissal brings them at once
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.relist(db)
	m.Command(r.anna, r.pa, "give buckler")
	m.Command(r.anna, r.pa, "dismiss")
	if packHas(t, db, r.pa.CharServerID, itBuckler) != 1 {
		t.Fatal("a dismissal with room did not give it back")
	}
	// a slot's row gone another way: a new hire there starts empty, the old
	// one's things in the keeping
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.relist(db)
	m.Command(r.anna, r.pa, "give buckler")
	if _, err := db.Exec(`DELETE FROM charmerc WHERE serverid = ?`, r.pa.CharServerID); err != nil {
		t.Fatal(err)
	}
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1) // into the slot its row left
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBuckler); n != 0 {
		t.Error("a new hire took another's buckler")
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, database.MercKeeping, itBuckler); n != 1 {
		t.Error("the old hire's buckler is lost")
	}
}

// /merc aoe and single: a Cocktail alchemist's, kept in its row and its kit;
// any other refused.
func TestMercCocktailToggleKept(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hireWith(r.anna, r.pa, 14, 0, 0, 1) // Cocktail
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0) // Knack
	r.drain()
	m.Command(r.anna, r.pa, "1 aoe")
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if !row.AoE || !r.mercInfo(1).AoE {
		t.Fatalf("aoe not kept: row %v, out %v", row.AoE, r.mercInfo(1).AoE)
	}
	if k, _ := m.kit(row, 30); !k.AoE || !k.Cocktail {
		t.Error("a kit from its row (a login, a keep restart) lost it")
	}
	m.Command(r.anna, r.pa, "2 aoe")
	if row2, _, _ := db.Merc(r.pa.CharServerID, 2); row2.AoE {
		t.Error("a Knack alchemist took aoe")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "err:"+mercThrowsNot) {
		t.Logf("sent %q", got)
	}
	m.Command(r.anna, r.pa, "1 single")
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); row.AoE || r.mercInfo(1).AoE {
		t.Error("single not kept")
	}
}

// A potion given to an alchemist is in its kit to throw; its throw takes one
// off its record. The alchemists' stock is the owner's: what one made stays
// past its dismissal for a new hire, and comes into the pack a stack at a time.
func TestMercAlchemistPotionsAndStock(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0)
	r.pack(db, [2]int{itPotion, 5})
	m.Command(r.anna, r.pa, "give volatile")
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	k, _ := m.kit(row, 30)
	if len(k.Carried) != 1 || k.Carried[0].PatternID != itPotion || k.Carried[0].Count != 5 {
		t.Fatalf("its kit carries %+v", k.Carried)
	}
	m.note(entity.MercNote{Kind: entity.MercNoteUsed, Owner: r.pa, Slot: 1, ItemID: k.Carried[0].ID, PatternID: itPotion, Count: 1})
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itPotion); n != 4 {
		t.Errorf("after a throw it holds %d, want 4", n)
	}
	m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: itPotion, Count: 1})
	m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: itPotion, Count: 1})
	m.Command(r.anna, r.pa, "take volatile")
	m.Command(r.anna, r.pa, "dismiss")
	m.hireWith(r.anna, r.pa, 14, 0, 0, 1)
	m.loadStock(r.pa)
	if st := r.gw.MercStockOf(r.pa.CharServerID); st[itPotion] != 2 {
		t.Fatalf("a new hire's owner's stock %v, want the 2 made before", st)
	}
	before := packHas(t, db, r.pa.CharServerID, itPotion)
	m.takeStock(r.anna, r.pa, m.stockRows(r.pa)[0])
	if got := packHas(t, db, r.pa.CharServerID, itPotion); got != before+2 {
		t.Errorf("pack %d after taking the stock, %d before", got, before)
	}
	if st, _ := db.MercStock(r.pa.CharServerID); st[itPotion] != 0 {
		t.Errorf("the stock still holds %v", st)
	}
}

// The menu's Items: Give an item lists the pack, a choice gives it;
// Inventory lists what it holds, a record's box Equips it. A menu of more
// than a box's six (a shaman's: its mode and its pet) shows five and More,
// the rest after; a Cocktail alchemist's shows Throws.
func TestMercMenuItems(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1)
	r.gw.AskMercGroup(r.anna)
	r.pack(db, [2]int{itBreastplate, 1})
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	r.drain()
	r.talk(r.anna, merc)
	got := r.answer(r.answer(r.msgs(r.anna), "Items"), "Give an item")
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Mithril Breastplate") {
		t.Fatalf("the give box: %q", got)
	}
	r.answer(got, "Mithril Breastplate")
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); n != 1 {
		t.Fatal("not given")
	}
	r.talk(r.anna, merc)
	got = r.answer(r.answer(r.msgs(r.anna), "Items"), "Inventory (1)")
	got = r.answer(got, "Mithril Breastplate")
	if !strings.HasSuffix(got[0], "|Equip|Take back|Back") {
		t.Fatalf("the record's box: %q", got)
	}
	r.answer(got, "Equip")
	if _, e := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); e != 5 {
		t.Errorf("not worn: %d", e)
	}

	r2, m2, _ := newMercFixture(t)
	m2.hireWith(r2.anna, r2.pa, 8, 0, 0, -1) // a shaman: healer and pet
	m2.hireWith(r2.anna, r2.pa, 14, 0, 0, 1) // a Cocktail alchemist
	r2.drain()
	m2.Command(r2.anna, r2.pa, "1 menu")
	got = r2.msgs(r2.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|More") || strings.Count(got[0], "|") != 6 {
		t.Fatalf("the shaman's menu: %q", got)
	}
	if got = r2.answer(got, "More"); !strings.HasSuffix(got[0], "|Items|Dismiss|Back") {
		t.Fatalf("its second box: %q", got)
	}
	m2.Command(r2.anna, r2.pa, "2 menu")
	got = r2.msgs(r2.anna)
	if !strings.Contains(got[0], "|Throws: single|") {
		t.Fatalf("the alchemist's menu: %q", got)
	}
	r2.answer(got, "Throws: single")
	if !r2.mercInfo(2).AoE {
		t.Error("its Throws answer did not turn aoe on")
	}
}

// The alchemists' stock is the owner's rows: a brewed batch and a throw are
// kept, a dismissal keeps it, and a hire after a restart that carried no merc
// of the owner's (no login read it) reads it, so the new merc brews on it and
// throws from it. A ceiling (world.merc_brew_cap) holds a batch to it.
func TestMercAlchemistStockOverRestartAndDismissal(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	char := r.pa.CharServerID
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0)
	if !r.gw.MercStockKnown(char) {
		t.Fatal("the hire gave the World no stock")
	}
	m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: itPotion, Count: 20})
	m.note(entity.MercNote{Kind: entity.MercNoteStockUsed, Owner: r.pa, Slot: 1, PatternID: itPotion, Count: 1})
	if st, _ := db.MercStock(char); st[itPotion] != 19 {
		t.Fatalf("kept %v, want 19 (a batch of 20, one thrown)", st)
	}
	m.Command(r.anna, r.pa, "dismiss")
	if rows, _ := db.Mercs(char); len(rows) != 0 {
		t.Fatalf("the hire not ended: %+v", rows)
	}
	if st, _ := db.MercStock(char); st[itPotion] != 19 {
		t.Fatalf("its dismissal took the stock: %v", st)
	}
	// a restart into a new World, no merc of hers kept, no login read
	r2 := newPortalRig(t)
	r2.gw.SetRaceModels(map[[2]int]uint32{{0, 0}: 1893243078, {0, 1}: 223572789, {2, 0}: 2128125354, {2, 1}: 2358987577})
	heir := NewHeirlooms(db)
	heir.Start(nil, nil)
	m2 := NewMercs(r2.gw, db, heir)
	r2.ih.mercs, r2.h.mercs = m2, m2
	r2.pa.Level, r2.pa.Race, r2.pa.Class = 30, 0, 0
	if r2.pa.CharServerID != char {
		t.Fatalf("the rig's character %d, want %d", r2.pa.CharServerID, char)
	}
	if r2.gw.MercStockKnown(char) {
		t.Fatal("a new World knew the stock before a hire")
	}
	m2.hireWith(r2.anna, r2.pa, 14, 0, 0, 0)
	if st := r2.gw.MercStockOf(char); st[itPotion] != 19 {
		t.Fatalf("a hire after the restart: the World's stock %v, want the 19 kept", st)
	}
	// a ceiling: a batch past it is kept to it, and the World's copy with it
	r2.gw.SetMercBrew(0, 25, -1)
	r2.gw.SetMercStock(char, map[int]int{itPotion: 39}) // the World's batch, ahead of the rows
	m2.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r2.pa, Slot: 1, PatternID: itPotion, Count: 20})
	if st, _ := db.MercStock(char); st[itPotion] != 25 {
		t.Errorf("kept %v past the ceiling of 25", st)
	}
	if st := r2.gw.MercStockOf(char); st[itPotion] != 25 {
		t.Errorf("the World's copy %v, want the rows' 25", st)
	}
	// a keep restart carrying the merc: its Restore reads the stock
	row, ok, _ := db.Merc(char, 1)
	if !ok {
		t.Fatal("no hire kept")
	}
	r3 := newPortalRig(t)
	r3.gw.SetRaceModels(map[[2]int]uint32{{0, 0}: 1893243078, {0, 1}: 223572789, {2, 0}: 2128125354, {2, 1}: 2358987577})
	m3 := NewMercs(r3.gw, db, heir)
	r3.ih.mercs, r3.h.mercs = m3, m3
	r3.pa.Level, r3.pa.Race, r3.pa.Class = 30, 0, 0
	m3.Restore(r3.anna, r3.pa, entity.KeptMerc{Slot: 1, Name: row.Name, HPPct: 1, PowerPct: 1}, time.Now())
	if _, ok := r3.gw.MercInfoOf(r3.anna); !ok {
		t.Fatal("the kept merc not restored")
	}
	if st := r3.gw.MercStockOf(char); st[itPotion] != 25 {
		t.Errorf("after a keep restart the World's stock %v, want the 25 kept", st)
	}
}

// Petition 2965: /merc helm off hides the helm's look of the merc (it still wears it), at once on its
// NPC (the channel every client reads), kept in its row so a kit made again (a login, a keep restart's
// Restore) hides it too; /merc helm alone turns it over, on shows it. A tell's body "helm off" is /merc's.
func TestMercHelmHidden(t *testing.T) {
	r, m, db := newMercFixture(t)
	n := func(v int) *int { return &v }
	const itHelm = 910010
	putItem(t, db, database.ItemPatternRecord{PatternID: itHelm, ItemName: "Iron Helm", EquipSlot: 1, LevelReq: 1, AC: n(5), Model: 7})
	if _, err := db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid = ?`, itHelm); err != nil {
		t.Fatal(err)
	}
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.pack(db, [2]int{itHelm, 1})
	m.Command(r.anna, r.pa, "give iron helm")
	m.Command(r.anna, r.pa, "equip iron helm")
	merc := func() *entity.NPC {
		for _, x := range r.gw.NPCs {
			if x.IsMerc() {
				return x
			}
		}
		t.Fatal("no merc out")
		return nil
	}
	if merc().Head != 7 {
		t.Fatalf("the helm is not shown: head %d", merc().Head)
	}
	ac := r.mercInfo(1).AC
	r.drain()
	m.Command(r.anna, r.pa, "helm off")
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if merc().Head != 0 || merc().HiddenParts&entity.HideHelm == 0 || !row.HideHelm {
		t.Fatalf("helm off: head %d, hidden %x, row %v", merc().Head, merc().HiddenParts, row.HideHelm)
	}
	if r.mercInfo(1).AC != ac {
		t.Error("hiding the helm took its armour off")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+row.Name+"'s helm is hidden. It still wears it.") {
		t.Errorf("sent %q", got)
	}
	// a kit made from the row (a login, Restore) hides it too
	if k, err := m.kit(row, r.pa.Level); err != nil || k.Look.Hidden&entity.HideHelm == 0 || k.Look.Helm != 7 {
		t.Fatalf("the row's kit: %+v %v", k.Look, err)
	}
	m.Command(r.anna, r.pa, "helm") // turned over: shown
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); row.HideHelm || merc().Head != 7 || merc().HiddenParts != 0 {
		t.Fatalf("helm toggled: row %v head %d", row.HideHelm, merc().Head)
	}
	m.Command(r.anna, r.pa, "helm off")
	m.Command(r.anna, r.pa, "helm on")
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); row.HideHelm || merc().Head != 7 {
		t.Fatal("helm on did not show it")
	}
	if !mercWords("helm off") || !mercWords("1 helm") {
		t.Error("a tell's helm is not /merc's")
	}
}

// Petition 2965's follow-up: /merc look off hides every armour piece's look (the chest's body and tint,
// the helm's too) at once on the NPC, its stats kept and its weapon still shown; kept in its row (a kit
// made again hides it), on shows it, a helm /merc helm hid stays hidden. A tell's "look off" is /merc's.
func TestMercLookHidden(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	n := func(v int) *int { return &v }
	const itHelm = 910011
	putItem(t, db, database.ItemPatternRecord{PatternID: itHelm, ItemName: "Iron Helm", EquipSlot: 1, LevelReq: 1, AC: n(5), Model: 7})
	if _, err := db.Exec(`UPDATE itempattern SET rent = 0 WHERE patternid = ?`, itHelm); err != nil {
		t.Fatal(err)
	}
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1)
	r.pack(db, [2]int{itHelm, 1}, [2]int{itBreastplate, 1})
	for _, c := range []string{"give iron helm", "equip iron helm", "give mithril", "equip mithril"} {
		m.Command(r.anna, r.pa, c)
	}
	merc := func() *entity.NPC {
		for _, x := range r.gw.NPCs {
			if x.IsMerc() {
				return x
			}
		}
		t.Fatal("no merc out")
		return nil
	}
	if merc().Head != 7 || merc().Torso != 9 {
		t.Fatalf("worn look not shown: head %d torso %d", merc().Head, merc().Torso)
	}
	ac, weapon := r.mercInfo(1).AC, merc().PrimaryWeapon
	r.drain()
	m.Command(r.anna, r.pa, "look off")
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	x := merc()
	if !row.HideLook || x.Head != 0 || x.Torso != 0 || x.Forearm != 0 || x.NPCGloves != 0 || x.NPCLegs != 0 || x.Feet != 0 ||
		x.RobeStyle != "" || x.Tint != 0 || x.HiddenParts != entity.MercArmorLook {
		t.Fatalf("look off: row %v, %+v", row.HideLook, x)
	}
	if r.mercInfo(1).AC != ac || x.PrimaryWeapon != weapon {
		t.Error("hiding the look took armour or the weapon off")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+row.Name+"'s armour is hidden. It still wears it.") {
		t.Errorf("sent %q", got)
	}
	if k, err := m.kit(row, r.pa.Level); err != nil || k.Look.Hidden != entity.MercArmorLook || k.Look.Chest != 9 {
		t.Fatalf("the row's kit: %+v %v", k.Look, err)
	}
	m.Command(r.anna, r.pa, "helm off")
	m.Command(r.anna, r.pa, "look on")
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); row.HideLook || merc().Torso != 9 || merc().Head != 0 {
		t.Fatalf("look on, helm off: row %v torso %d head %d", row.HideLook, merc().Torso, merc().Head)
	}
	m.Command(r.anna, r.pa, "helm on")
	m.Command(r.anna, r.pa, "look") // turned over: hidden
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); !row.HideLook || merc().Head != 0 {
		t.Fatal("look toggled did not hide it")
	}
	if !mercWords("look off") || !mercWords("2 look") {
		t.Error("a tell's look is not /merc's")
	}
}
