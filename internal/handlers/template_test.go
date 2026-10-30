package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// #template's choices (template.go) against made-up items, and the whole
// command against a temporary database. Item names are made up.

// item is a made-up wearable pattern: its id, wear slot, AC, STR and DMG.
func item(id, wear, ac, str, dmg int) database.ItemPatternRecord {
	return database.ItemPatternRecord{PatternID: id, ItemName: "item", EquipSlot: wear, AC: &ac, Str: &str, WeaponDamage: dmg, LevelReq: 50, MaxStack: 1}
}

// worn is the picks by slot (-1: the bag), as pattern ids.
func wornBySlot(picks []templatePick) map[int][]int {
	out := map[int][]int{}
	for _, p := range picks {
		out[p.slot] = append(out[p.slot], p.pattern.PatternID)
	}
	return out
}

// Each slot takes its best by templateScore, ties to the higher level and then
// the lower id; two of a kind for the ears and rings (a LORE one once, then the
// next); a fishing pole (wear 20) and a row with no wear slot the client takes
// (0) are never worn.
func TestTheTemplateWearsTheBestOfEachSlot(t *testing.T) {
	head1, head2, head3 := item(1, 1, 50, 0, 0), item(2, 1, 40, 10, 0), item(3, 1, 60, 0, 0)
	head2.LevelReq, head3.LevelReq = 55, 40 // 2 and 3 both score 60: the higher level
	ear, lore, ear2 := item(10, 3, 5, 0, 0), item(11, 8, 9, 0, 0), item(12, 8, 7, 0, 0)
	lore.Lore = 1
	got := wornBySlot(templateGear([]database.ItemPatternRecord{
		head1, head2, head3, ear, lore, ear2,
		item(20, 20, 999, 999, 999), // a fishing pole
		item(21, 0, 999, 999, 999),  // no wear slot
	}, int(ClassWarrior)))
	want := map[int][]int{0: {2}, 2: {10}, 3: {10}, 9: {11}, 10: {12}}
	for slot, ids := range want {
		if !slices.Equal(got[slot], ids) {
			t.Errorf("slot %d: %v, want %v", slot, got[slot], ids)
		}
	}
	if len(got) != len(want) {
		t.Errorf("picks %v, want only %v", got, want)
	}
}

// The forearms take a pair of bracers or two bracelets, whichever adds more;
// a tie goes to the bracers.
func TestTheTemplateTakesBracersOrTwoBracelets(t *testing.T) {
	bracelet := item(1, 6, 30, 0, 0) // 30 twice
	for _, c := range []struct {
		bracers int
		want    map[int][]int
	}{
		{59, map[int][]int{6: {1}, 7: {1}}},
		{60, map[int][]int{8: {2}}},
		{61, map[int][]int{8: {2}}},
	} {
		got := wornBySlot(templateGear([]database.ItemPatternRecord{bracelet, item(2, 7, c.bracers, 0, 0)}, int(ClassWarrior)))
		if len(got) != len(c.want) || !slices.Equal(got[6], c.want[6]) || !slices.Equal(got[7], c.want[7]) || !slices.Equal(got[8], c.want[8]) {
			t.Errorf("bracers of %d: %v, want %v", c.bracers, got, c.want)
		}
	}
}

// The hands go by the DMG the melee rolls on: a two-hander of more DMG beats
// a one-hander and a shield; at the same DMG the one-hander with the best
// thing for the other hand (a shield, a second weapon, a held item) wins when
// it adds more, the two-hander on a tie. A bow is carried, never worn.
func TestTheTemplateArmsTheHandsByTheirDamage(t *testing.T) {
	sword := item(1, 14, 0, 10, 500) // either hand
	shield := item(2, 13, 100, 0, 0)
	held := item(3, 18, 0, 70, 0) // 140
	bow := item(4, 16, 0, 0, 900)
	for _, c := range []struct {
		name  string
		items []database.ItemPatternRecord
		want  map[int][]int
	}{
		{"a two-hander of more DMG", []database.ItemPatternRecord{sword, shield, item(5, 15, 0, 0, 600)}, map[int][]int{16: {5}}},
		{"a two-hander of less DMG", []database.ItemPatternRecord{sword, shield, item(5, 15, 0, 500, 400)}, map[int][]int{14: {1}, 17: {2}}},
		{"same DMG, the held item adds more", []database.ItemPatternRecord{sword, shield, held, item(5, 15, 0, 60, 500)}, map[int][]int{14: {1}, 20: {3}}},
		{"same DMG, the two-hander adds more", []database.ItemPatternRecord{sword, shield, item(5, 15, 0, 200, 500)}, map[int][]int{16: {5}}},
		{"same DMG and score: the two-hander", []database.ItemPatternRecord{sword, item(6, 13, 30, 0, 0), item(5, 15, 30, 10, 500)}, map[int][]int{16: {5}}},
		{"a second sword, the shield worth less", []database.ItemPatternRecord{sword, item(6, 13, 10, 0, 0)}, map[int][]int{14: {1}, 15: {1}}},
		{"a bow in the bag", []database.ItemPatternRecord{sword, shield, bow}, map[int][]int{14: {1}, 17: {2}, -1: {4}}},
	} {
		got := wornBySlot(templateGear(c.items, int(ClassWarrior)))
		ok := len(got) == len(c.want)
		for s, ids := range c.want {
			ok = ok && slices.Equal(got[s], ids)
		}
		if !ok {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// a LORE sword is held once: the shield goes beside it
	lore := sword
	lore.Lore = 1
	if got := wornBySlot(templateGear([]database.ItemPatternRecord{lore, item(6, 13, 10, 0, 0)}, int(ClassWarrior))); !slices.Equal(got[14], []int{1}) || !slices.Equal(got[17], []int{6}) || got[15] != nil {
		t.Errorf("a LORE sword: %v", got)
	}
}

// Only what CLIENT's CanUse allows is a candidate: the class's bit, the race's
// bit, levelreq at most 60, of the wear slots asked for.
func TestTheTemplateTakesOnlyWhatTheCharacterMayWear(t *testing.T) {
	f := newGearFixture(t)
	add := func(name string, wear, class, race, level int) {
		if _, err := f.db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 0, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, 0, 0, ?, ?, 0, 0, 0, 0, ?, '', 0, 0)`,
			wear, level, class, race, name); err != nil {
			t.Fatal(err)
		}
	}
	add("for all", 5, 32767, 1023, 60)
	add("enchanters", 5, 1<<12, 1023, 1)
	add("warriors", 5, 1<<0, 1023, 1)
	add("gnomes", 5, 32767, 1<<3, 1)
	add("ogres", 5, 32767, 1<<9, 1)
	add("at 61", 5, 32767, 1023, 61)
	add("no class", 5, 0, 1023, 1)
	add("a ring", 8, 32767, 1023, 1)
	got, err := f.inv.WearablePatterns(12, 3, 60, []int{5})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.ItemName)
	}
	if want := []string{"for all", "enchanters", "gnomes"}; !slices.Equal(names, want) {
		t.Fatalf("a gnome enchanter may wear %v, want %v", names, want)
	}
}

// The plan: a picked item already held is worn rather than given again; what
// was worn in a slot the template fills, or one a pick takes off, comes off;
// what is worn where the template has nothing stays; past the client's 40
// records nothing more is given, and it is named.
func TestTheTemplatePlansOnTheInventoryHeld(t *testing.T) {
	row := func(id, pattern, wear, slot int) database.CharInventoryEntry {
		return database.CharInventoryEntry{ItemID: id, EquipLoc: slot, Pattern: database.ItemPatternRecord{PatternID: pattern, EquipSlot: wear}}
	}
	items := []database.CharInventoryEntry{
		row(100, 7, 5, 5),    // an old chest, worn
		row(101, 1, 14, -1),  // the picked sword, carried
		row(102, 9, 18, 19),  // a held item: the main hand takes it off
		row(103, 8, 11, 13),  // boots, where the template has none
		row(104, 50, 13, -1), // in the bank
	}
	items[4].Location = 1
	picks := []templatePick{{5, item(3, 5, 99, 0, 0)}, {14, item(1, 14, 0, 0, 500)}, {15, item(1, 14, 0, 0, 500)}}
	plan := planTemplate(items, picks)
	var reused, fresh []int
	for _, r := range plan.rows {
		if r.itemID != 0 {
			reused = append(reused, r.itemID)
		} else {
			fresh = append(fresh, r.pattern.PatternID)
		}
	}
	if !slices.Equal(reused, []int{101}) || !slices.Equal(fresh, []int{3, 1}) {
		t.Errorf("reused %v, new %v; want [101], [3 1]", reused, fresh)
	}
	if !slices.Equal(plan.takeOff, []int{100, 102}) {
		t.Errorf("taken off %v, want [100 102]", plan.takeOff)
	}
	if len(plan.noRoom) != 0 {
		t.Errorf("no room for %v", plan.noRoom)
	}

	// 39 records held: one more fits
	var full []database.CharInventoryEntry
	for i := range 39 {
		full = append(full, row(200+i, 60+i, 10, -1))
	}
	plan = planTemplate(full, picks)
	if len(plan.rows) != 1 || len(plan.noRoom) != 2 {
		t.Errorf("with 39 held: %d given, %d without room; want 1, 2", len(plan.rows), len(plan.noRoom))
	}
}

// The command: a GM's #template on a level 1 gnome enchanter writes level 60
// (with its experience), 1,000,000 tunar, its class's spells and the best of
// what it may wear, worn, to the database, sets the game's copy (full hit
// points at 60) and sends one MemoryDump, at the character's own place, that
// carries it all. A player's #template is refused, as every GM command is.
func TestTemplateMakesTheCharacterASixty(t *testing.T) {
	f := newGearFixture(t)
	for _, q := range []string{
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		 VALUES (10, 4, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Mending', ''), (11, 1, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Spark', ''),
		        (13, 9, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, 'Sword Art', '')`,
		`INSERT INTO classspells (class, spellid, source) VALUES (12, 10, 'test'), (12, 11, 'test'), (0, 13, 'test')`,
	} {
		if _, err := f.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	wear := func(name string, slot, class, ac int) {
		if _, err := f.db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color, AC)
			VALUES (0, 0, 0, 0, ?, 0, 0, 1, 0, 0, 0, 0, 50, 1, 100, 0, ?, 1023, 0, 0, 0, 0, ?, '', 0, 0, ?)`,
			slot, class, name, ac); err != nil {
			t.Fatal(err)
		}
	}
	wear("Made-up Hat", 1, 1<<12, 20)
	wear("Made-up Coat", 5, 1<<12, 40)
	wear("Warrior's Coat", 5, 1<<0, 400) // not an enchanter's

	c, gs, gp, ps, _ := gmWorld(t)
	store := database.NewSQLiteCharacterStore(f.db)
	// the GM's row is character 1 (gmWorld's join), the fixture's "Gear"
	gp.CharServerID, gp.Class, gp.Race = f.sid, 12, 3
	h := &CharacterHandler{store: store, sessionManager: c.sm, gameWorld: c.gw, inventoryStore: f.inv}
	c.h.SetStores(store, f.inv)
	c.h.SetTemplate(h.ApplyTemplate)
	kept := &keptMastery{}
	c.h.SetMasteryStore(kept)
	gp.Mastery = entity.Mastery{Points: 40, Spent: 60}
	gs.SetPlayerObjectID(500000001)
	gs.Queue().GetPending()

	c.say(ps, "#template")
	if got := heard(t, ps); len(got) != 1 || got[0].text != `Unrecognized command "#template".` {
		t.Fatalf("a player's #template: %+v", got)
	}

	c.say(gs, "/template")
	// the mastery points to the most, spent and unspent: 60 spent keeps 1440
	if gp.Mastery.Points != 1440 || gp.Mastery.Spent != 60 || kept.rows[gp.CharServerID].Points != 1440 {
		t.Errorf("the mastery points: %+v, kept %+v; want 1440 unspent beside 60 spent", gp.Mastery, kept.rows[gp.CharServerID])
	}
	if u, sp, ok := masteryPoints(gs); !ok || u != 1440 || sp != 60 {
		t.Errorf("the client was told %d/%d (%v), want 1440/60", u, sp, ok)
	}
	row, err := store.GetByServerID(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if row.Level != 60 || row.TotalXP != entity.XPBase(60) || row.Tunar != templateTunar {
		t.Errorf("the row: level %d, %d xp, %d tunar", row.Level, row.TotalXP, row.Tunar)
	}
	if b := book(t, f.db, f.sid); !slices.Equal(b, []int{11, 10}) {
		t.Errorf("the book %v, want Spark then Mending", b)
	}
	items, _ := f.inv.GetCharacterItems(f.sid)
	worn := map[string]int{}
	for _, e := range items {
		worn[e.Pattern.ItemName] = e.EquipLoc
	}
	if len(worn) != 2 || worn["Made-up Hat"] != 0 || worn["Made-up Coat"] != 5 {
		t.Errorf("the inventory %v", worn)
	}
	if gp.Level != 60 || gp.Tunar != templateTunar || gp.HP != gp.MaxHPNow() || gp.Gear.AC != 60 {
		t.Errorf("the game's copy: level %d, %d tunar, %d/%d HP, gear AC %d", gp.Level, gp.Tunar, gp.HP, gp.MaxHPNow(), gp.Gear.AC)
	}
	var dumps [][]byte
	for _, m := range gs.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && opcode(d) == protocol.OpcodeMemoryDump {
			dumps = append(dumps, d)
		}
	}
	if len(dumps) != 1 {
		t.Fatalf("%d MemoryDumps, want 1", len(dumps))
	}
	if _, world, pos, name := dumpPlace(t, dumps[0]); world != 0 || pos != [3]float32{gp.X, gp.Y, gp.Z} || name != gp.Name {
		t.Errorf("the dump: world %d at %v (%s), want the GM where it stands", world, pos, name)
	}
	var said []string
	for _, l := range heard(t, gs) {
		if l.text != "" {
			said = append(said, l.text)
		}
	}
	if len(said) != 4 || said[0] != "[GM] Template: level 60, 1000000 tunar, 2 spells scribed, 2 items worn" ||
		!strings.HasPrefix(said[3], "[GM]   Mastery points: 1400 more, 1440 unspent, 60 spent") {
		t.Errorf("the answer %q", said)
	}
}

// book is the character's spells in book order, as spell ids.
func book(t *testing.T, db *database.SQLiteDB, sid uint32) []int {
	t.Helper()
	rows, err := db.Query(`SELECT spellid FROM spells WHERE serverid = ? ORDER BY addedorder`, sid)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}
