package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// heirloomFixture is a warrior (class 0, race 0) holding a helm and a blade
// heirloom, among warriors' helms and blades of several levels, a conjured
// helm, a broken one and an outsized weapon.
type heirloomFixture struct {
	t   *testing.T
	db  *database.SQLiteDB
	h   *Heirlooms
	inv *database.InventoryStore
	sid uint32
}

const (
	hlHelm, hlBlade, hlOtherClass = 714001, 714002, 714003
)

func newHeirloomFixture(t *testing.T) *heirloomFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/heirloom.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Heir", AccountID: 1, Level: 10}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	f := &heirloomFixture{t: t, db: db, inv: database.NewInventoryStore(db), sid: rec.ServerID}
	n := func(v int) *int { return &v }
	items := database.NewItemPatternStore(db)
	put := func(r database.ItemPatternRecord) {
		t.Helper()
		if r.ClassUse == 0 {
			r.ClassUse = 1 // a warrior's
		}
		switch r.Rent { // -1: NO RENT (rent 0, what a spell conjures); 0: unset, an ordinary item
		case -1:
			r.Rent = 0
		case 0:
			r.Rent = 1
		}
		r.RaceUse, r.MaxStack = 32767, 1
		if err := items.UpsertItemPattern(&r); err != nil {
			t.Fatal(err)
		}
	}
	// helms: AC by level; the conjured one (NO RENT) and the broken one never count
	put(database.ItemPatternRecord{PatternID: 900001, ItemName: "helm 5", EquipSlot: 1, LevelReq: 5, AC: n(10), Sta: n(2)})
	put(database.ItemPatternRecord{PatternID: 900002, ItemName: "helm 10", EquipSlot: 1, LevelReq: 10, AC: n(20), Str: n(5)})
	put(database.ItemPatternRecord{PatternID: 900003, ItemName: "helm 20", EquipSlot: 1, LevelReq: 20, AC: n(40), HPMAX: n(30)})
	put(database.ItemPatternRecord{PatternID: 900004, ItemName: "conjured helm", EquipSlot: 1, LevelReq: 5, AC: n(500), Rent: -1})
	put(database.ItemPatternRecord{PatternID: 900005, ItemName: "broken helm", EquipSlot: 1, LevelReq: 5, AC: n(30), PoT: n(987)})
	put(database.ItemPatternRecord{PatternID: 900006, ItemName: "a monk's helm", EquipSlot: 1, LevelReq: 5, AC: n(90), ClassUse: 16})
	put(database.ItemPatternRecord{PatternID: 900007, ItemName: "prop helm", EquipSlot: 1, LevelReq: 5, AC: n(534)})
	put(database.ItemPatternRecord{PatternID: 900008, ItemName: "Froglok Test Helm", EquipSlot: 1, LevelReq: 1, AC: n(300)})
	put(database.ItemPatternRecord{PatternID: 900009, ItemName: "a helm of no level", EquipSlot: 1, LevelReq: 0, AC: n(250)})
	// one-handers by damage; the outsized one never counts
	put(database.ItemPatternRecord{PatternID: 900011, ItemName: "blade 8", EquipSlot: 12, LevelReq: 8, WeaponDamage: 100, AttackType: 1})
	put(database.ItemPatternRecord{PatternID: 900012, ItemName: "blade 15", EquipSlot: 14, LevelReq: 15, WeaponDamage: 200, AttackType: 1, Dex: n(7)})
	put(database.ItemPatternRecord{PatternID: 900013, ItemName: "smasher", EquipSlot: 12, LevelReq: 9, WeaponDamage: 1276, AttackType: 3})
	// the heirlooms: their own names, slots and looks, no figures of their own
	put(database.ItemPatternRecord{PatternID: hlHelm, ItemName: "Heirloom Helm", EquipSlot: 1, LevelReq: 1, Model: 4})
	put(database.ItemPatternRecord{PatternID: hlBlade, ItemName: "Heirloom Blade", EquipSlot: 14, LevelReq: 1, AttackType: 1})
	put(database.ItemPatternRecord{PatternID: hlOtherClass, ItemName: "Heirloom Helm", EquipSlot: 1, LevelReq: 1, ClassUse: 16})
	for _, r := range []struct {
		id, class int
		role      string
	}{{hlHelm, 0, "head"}, {hlBlade, 0, "main"}, {hlOtherClass, 4, "head"}} {
		if _, err := db.Exec(`INSERT INTO heirlooms (patternid, class, role) VALUES (?, ?, ?)`, r.id, r.class, r.role); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int{hlHelm, hlBlade, hlOtherClass, 900002} {
		if _, err := f.inv.AddCharItem(rec.ServerID, id, 1, 100, 0, 0); err != nil {
			t.Fatal(err)
		}
	}
	f.h = NewHeirlooms(db)
	f.h.Start(nil, nil)
	t.Cleanup(func() { db.SetPatternView(nil) })
	return f
}

// held is the character's records as read (its own view), by patternid.
func (f *heirloomFixture) held() map[int]database.ItemPatternRecord {
	f.t.Helper()
	items, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[int]database.ItemPatternRecord{}
	for _, e := range items {
		out[e.Pattern.PatternID] = e.Pattern
	}
	return out
}

func val(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

// An heirloom's figures are the best of its role its class may wear at its
// wearer's level: a warrior of 10 the helm of 10 and the blade of 8, of 20
// the helm of 20 and the blade of 15 (not the conjured helm, the broken one,
// the prop, the test helm, the helm of no known level, the monk's or the
// outsized smasher); its own name, slot and look kept; the
// same record read through GetCharItem the same; any other record as it is.
// The control: with no view, the records' own (no) figures.
func TestAnHeirloomIsItsLevelsBest(t *testing.T) {
	f := newHeirloomFixture(t)
	at := func(level int) map[int]database.ItemPatternRecord {
		f.h.Note(f.sid, 0, 0, level)
		return f.held()
	}
	got := at(10)
	// (STA 2: the class's stats at 10, heirloomStats; STR 5 the helm of 10's, higher)
	if h := got[hlHelm]; val(h.AC) != 20 || val(h.Str) != 5 || val(h.Sta) != 2 || h.ItemName != "Heirloom Helm" || h.EquipSlot != 1 || h.Model != 4 {
		t.Errorf("level 10's helm: AC %d STR %d STA %d, %q slot %d model %d", val(h.AC), val(h.Str), val(h.Sta), h.ItemName, h.EquipSlot, h.Model)
	}
	if b := got[hlBlade]; b.WeaponDamage != 100 || b.EquipSlot != 14 || b.AttackType != 1 {
		t.Errorf("level 10's blade: damage %d slot %d type %d", b.WeaponDamage, b.EquipSlot, b.AttackType)
	}
	if o := got[900002]; val(o.AC) != 20 || o.ItemName != "helm 10" {
		t.Errorf("a plain helm changed: %+v", o)
	}
	got = at(20)
	if h := got[hlHelm]; val(h.AC) != 40 || val(h.HPMAX) != 30 || val(h.Str) != 4 { // STR the class's 4 at 20
		t.Errorf("level 20's helm: AC %d HP %d STR %d", val(h.AC), val(h.HPMAX), val(h.Str))
	}
	if b := got[hlBlade]; b.WeaponDamage != 200 || val(b.Dex) != 7 { // DEX the blade of 15's 7, over the class's 4
		t.Errorf("level 20's blade: damage %d DEX %d", b.WeaponDamage, val(b.Dex))
	}
	// another class's heirloom takes its own class's best (the monk's helm)
	if o := got[hlOtherClass]; val(o.AC) != 90 {
		t.Errorf("the monk's heirloom on a warrior: AC %d, want the monk's helm's 90", val(o.AC))
	}
	items, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range items {
		if e.Pattern.PatternID == hlHelm {
			one, err := f.inv.GetCharItem(f.sid, e.ItemID)
			if err != nil || val(one.Pattern.AC) != 40 {
				t.Errorf("GetCharItem's helm at 20: AC %d (%v)", val(one.Pattern.AC), err)
			}
		}
	}
	f.db.SetPatternView(nil)
	if h := f.held()[hlHelm]; val(h.AC) != 0 {
		t.Errorf("the control: with no view the heirloom has AC %d", val(h.AC))
	}
}

// Off the World's word a character's level is its row's (an offline
// character); the World's level up is heard and the gear refreshed with the
// player whose level rose.
func TestAnHeirloomFollowsALevelUp(t *testing.T) {
	f := newHeirloomFixture(t)
	if h := f.held()[hlHelm]; val(h.AC) != 20 {
		t.Fatalf("the row's level 10: AC %d", val(h.AC))
	}
	gw := entity.NewWorld()
	refreshed := make(chan *entity.Player, 1)
	f.h.Start(gw, func(p *entity.Player) { refreshed <- p })
	p := &entity.Player{ServerID: 77, CharServerID: f.sid, Name: "Heir", Level: 10}
	gw.Players[p.ServerID] = p
	if !gw.SetPlayerLevel(f.sid, 20) {
		t.Fatal("no player to set")
	}
	if got := <-refreshed; got != p {
		t.Fatalf("refreshed %v", got)
	}
	if h := f.held()[hlHelm]; val(h.AC) != 40 {
		t.Errorf("after the level up to 20: AC %d, want 40", val(h.AC))
	}
}

// The chest piece heals at each hit as the Breastplate of the Anointed.
func TestTheHeirloomChestguardHealsNoMore(t *testing.T) {
	// the user, 2026-11-06: "lets turn off the healing effect on hit from
	// heirloom"; the control, the Anointed's own, still heals
	if c, ok := entity.WornShieldOf(714001, "Heirloom Chestguard"); ok {
		t.Errorf("the heirloom chestguard still answers a hit: %+v", c)
	}
	if a, ok := entity.WornShieldOf(0, "Breastplate of the Anointed"); !ok || a.Heal == 0 {
		t.Errorf("the control: the Anointed's %+v (%v)", a, ok)
	}
}

// An epic heirloom (heirlooms.epic; the user, 2026-11-06: "the heirloom
// weapons for every class lets make their lvl 60 epics") is the best of its
// role below its epic's level and the epic itself from it: the epic's damage
// and stats, and its proc (read by the epic's name: Selo's Jeweled Rapier's
// 140), its own name, slot and class kept. The controls: the plain heirloom
// blade at the same level is still its level's best, and the epic's own
// record (a bard's) is never a warrior's best.
func TestAnEpicHeirloomIsItsEpicFromItsLevel(t *testing.T) {
	f := newHeirloomFixture(t)
	n := func(v int) *int { return &v }
	items := database.NewItemPatternStore(f.db)
	for _, r := range []database.ItemPatternRecord{
		{PatternID: 900020, ItemName: "Selo's Jeweled Rapier", EquipSlot: 14, LevelReq: 49, WeaponDamage: 561, AttackType: 5,
			Dex: n(10), Agi: n(5), ClassUse: 32, Rent: 1, RaceUse: 32767, MaxStack: 1},
		{PatternID: 714004, ItemName: "Heirloom Selo's Jeweled Rapier", EquipSlot: 14, LevelReq: 1, AttackType: 5,
			ClassUse: 1, Rent: 1, RaceUse: 32767, MaxStack: 1},
	} {
		if err := items.UpsertItemPattern(&r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.Exec(`INSERT INTO heirlooms (patternid, class, role, epic) VALUES (714004, 0, 'main', 900020)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.inv.AddCharItem(f.sid, 714004, 1, 100, 0, 0); err != nil {
		t.Fatal(err)
	}
	f.h = NewHeirlooms(f.db) // the table read anew
	f.h.Start(nil, nil)
	at := func(level int) map[int]database.ItemPatternRecord {
		f.h.Note(f.sid, 0, 0, level)
		return f.held()
	}
	got := at(20)
	if e := got[714004]; e.WeaponDamage != 200 || val(e.Dex) != 7 || e.ProcName != "" {
		t.Errorf("at 20: damage %d DEX %d proc %q; want the level's best blade's 200, 7 and none", e.WeaponDamage, val(e.Dex), e.ProcName)
	}
	got = at(49)
	e := got[714004]
	// (AGI 10: the class's at 49 over the rapier's 5; DEX the rapier's 10, the class's 10 too)
	if e.WeaponDamage != 561 || val(e.Dex) != 10 || val(e.Agi) != 10 || e.ItemName != "Heirloom Selo's Jeweled Rapier" ||
		e.EquipSlot != 14 || e.ClassUse != 1 {
		t.Errorf("at 49: damage %d DEX %d AGI %d, %q slot %d class %d; want the rapier's 561, 10, and AGI 10 under its own name",
			e.WeaponDamage, val(e.Dex), val(e.Agi), e.ItemName, e.EquipSlot, e.ClassUse)
	}
	name, desc := e.ProcSource()
	if wp, ok := entity.WeaponProcOf(e.PatternID, name, desc); !ok || wp.Kind != entity.ProcHit || wp.Damage != 140 {
		t.Errorf("at 49 its proc: %+v %v; want the rapier's 140 hit", wp, ok)
	}
	if b := got[hlBlade]; b.WeaponDamage != 200 || b.ProcName != "" {
		t.Errorf("the plain heirloom blade at 49: damage %d proc %q; want the level's best, 200, none", b.WeaponDamage, b.ProcName)
	}
	if _, ok := entity.WeaponProcOf(hlBlade, got[hlBlade].ItemName, got[hlBlade].ItemDesc); ok {
		t.Error("the plain heirloom blade procs")
	}
}

// Every heirloom armour and jewellery piece carries its class's stats at its
// wearer's level (the user, 2026-11-06: "all heirlooms should have stat
// increases also not just ac, can have hp, pow, str, sta, and 2 good
// secondary stats. for sk int and agi. for war dex and agi"): a warrior of 10's
// helm the helm of 10's STR 5 kept (its best's, higher) and AC 20, with HP and
// power 13, STA 2 and DEX and AGI 2 added; a level 60's 75, 75 and 12 each; an
// alchemist's INT and WIS where a warrior's are DEX and AGI; the blade, a
// weapon, its stats too.
func TestHeirloomArmourCarriesItsClassesStats(t *testing.T) {
	f := newHeirloomFixture(t)
	f.h.Note(f.sid, 0, 0, 10)
	got := f.held()
	h := got[hlHelm]
	if val(h.AC) != 20 || val(h.Str) != 5 || val(h.HPMAX) != 13 || val(h.POWMAX) != 13 || val(h.Sta) != 2 ||
		val(h.Dex) != 2 || val(h.Agi) != 2 || val(h.Intelligence) != 0 {
		t.Errorf("a warrior of 10's helm: AC %d STR %d HP %d POW %d STA %d DEX %d AGI %d INT %d", val(h.AC), val(h.Str),
			val(h.HPMAX), val(h.POWMAX), val(h.Sta), val(h.Dex), val(h.Agi), val(h.Intelligence))
	}
	// a weapon too (the user: "let heirloom weapon get stats also and shields")
	if b := got[hlBlade]; val(b.HPMAX) != 13 || val(b.Str) != 2 || val(b.Sta) != 2 || val(b.Agi) != 2 {
		t.Errorf("the blade at 10: HP %d STR %d STA %d AGI %d; want 13, 2, 2, 2", val(b.HPMAX), val(b.Str), val(b.Sta), val(b.Agi))
	}
	var p database.ItemPatternRecord
	heirloomStats(&p, 14, 60)
	if val(p.HPMAX) != 75 || val(p.POWMAX) != 75 || val(p.Str) != 12 || val(p.Sta) != 12 || val(p.Intelligence) != 12 ||
		val(p.Wis) != 12 || val(p.Dex) != 0 || val(p.Agi) != 0 {
		t.Errorf("an alchemist of 60's piece: HP %d POW %d STR %d STA %d INT %d WIS %d DEX %d AGI %d", val(p.HPMAX), val(p.POWMAX),
			val(p.Str), val(p.Sta), val(p.Intelligence), val(p.Wis), val(p.Dex), val(p.Agi))
	}
	var one database.ItemPatternRecord
	heirloomStats(&one, 3, 1)
	if val(one.HPMAX) != 1 || val(one.Str) != 1 || val(one.Intelligence) != 1 || val(one.Agi) != 1 {
		t.Errorf("a shadow knight of 1's piece: HP %d STR %d INT %d AGI %d", val(one.HPMAX), val(one.Str), val(one.Intelligence), val(one.Agi))
	}
}
