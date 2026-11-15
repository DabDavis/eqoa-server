package handlers

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// masterFixture is the merc fixture with a catalog of Master Classes and the
// gnome's Invention: Tinker (a gnome's, any class: its first rank, Construct,
// Repair, and Construct's Rat look), the warrior's Soldier and Defender, the
// necromancer's Lich; their abilities' spells.
func masterFixture(t *testing.T) (*portalRig, *Mercs, *database.SQLiteDB) {
	t.Helper()
	r, m, db := newMercFixture(t)
	for _, s := range []struct {
		id, level, scope int
		name, line       string
	}{
		{9900, 6, 0, "Invention", "Summons an Invention to aid you"},
		{9901, 60, 0, "Construct", "Summons a Construct to aid you"},
		{9902, 60, 0, "Construct (Rat)", "Summons a Construct to aid you"},
		{9903, 60, 3, "Repair", "Pet, Increase Hitpoints by 250"},
		{9904, 60, 1, "Soldier's Cry", "Target, Increase Hate by 900"},
		{9905, 60, 0, "Defender's Guard", "Self, Increase AC by 300, 30min"},
		{9906, 60, 0, "Lich", "Self, Gives Form, Decrease HP per tic by 196"},
	} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, 1, 50, 0, 0, ?, 6, 0, ?, '')`, s.id, s.level, s.scope, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, 'test')`, s.id, s.line); err != nil {
			t.Fatal(err)
		}
	}
	rows := []database.CMRankRow{
		{ID: 1501, Category: 1, Classes: 32767, Races: 8, Mastery: "Invention", Rank: 1, Name: "Invention", Cost: 20, Level: 6,
			Adds: "Invention@6", AddSpells: "9900"},
		{ID: 1541, Category: 4, Classes: 32767, Races: 8, Mastery: "Tinker", Rank: 1, Name: "Tinker", Cost: 200, Level: 55,
			Effects: "AGIMax:50,INTMax:50,HP:150,Pow:150,PoT:20"},
		{ID: 1542, Category: 4, Classes: 32767, Races: 8, Mastery: "Tinker", Rank: 2, Name: "Construct", Parent: 1541, Cost: 75, Level: 60,
			Adds: "Construct@60", AddSpells: "9901"},
		{ID: 1543, Category: 4, Classes: 32767, Races: 8, Mastery: "Tinker", Rank: 3, Name: "Repair", Parent: 1541, Cost: 75, Level: 60,
			Effects: "PoT:20", Adds: "Repair@60", AddSpells: "9903"},
		{ID: 900842, Category: 4, Classes: 32767, Races: 8, Mastery: "Tinker", Rank: 4, Name: "Construct (Rat)", Parent: 1542, Cost: 1,
			Level: 60, Requires: "Construct@60", RequireSpells: "9901", Adds: "Construct (Rat)@60", AddSpells: "9902"},
		{ID: 3221, Category: 4, Classes: 1, Races: 2047, Mastery: "Soldier", Rank: 1, Name: "Soldier", Cost: 200, Level: 55,
			MinStats: "STR:200,STA:200", RequireRanks: "2033", Effects: "HP:500"},
		{ID: 3223, Category: 4, Classes: 1, Races: 2047, Mastery: "Soldier", Rank: 3, Name: "Soldier's Cry", Parent: 3221, Cost: 75, Level: 60,
			Adds: "Soldier's Cry@60", AddSpells: "9904"},
		{ID: 3201, Category: 4, Classes: 1, Races: 2047, Mastery: "Defender", Rank: 1, Name: "Defender", Cost: 200, Level: 55},
		{ID: 3202, Category: 4, Classes: 1, Races: 2047, Mastery: "Defender", Rank: 2, Name: "Defender's Guard", Parent: 3201, Cost: 75,
			Level: 60, Adds: "Defender's Guard@60", AddSpells: "9905"},
		{ID: 6501, Category: 4, Classes: 2048, Races: 2047, Mastery: "Lich", Rank: 1, Name: "Lich", Cost: 200, Level: 55},
		{ID: 6502, Category: 4, Classes: 2048, Races: 2047, Mastery: "Lich", Rank: 2, Name: "Lich Form", Parent: 6501, Cost: 75, Level: 60,
			Adds: "Lich@60", AddSpells: "9906"},
	}
	for i := range rows {
		rows[i].Enabled = true
	}
	r.gw.SetCMCatalog(rows)
	return r, m, db
}

// /merc master: the Master Classes a merc's class and race may take, by
// kind; refused below 55 and for one its class may not take; given, its
// ranks' abilities in its book (Tinker's Construct its pet, not the Rat look;
// Repair), shown in /merc and its target window; another in its place takes
// the first one's abilities away; none takes it away; kept in its row and so
// back at a login (Restore, as a keep restart's). Below 55 a gnome merc
// summons its Invention.
func TestMercMasterCommand(t *testing.T) {
	r, m, db := masterFixture(t)
	r.pa.Level = 30
	m.hire(r.anna, r.pa, 0, 3, 0) // a gnome warrior
	r.gw.AskMercGroup(r.anna)
	rows, _ := db.Mercs(r.pa.CharServerID)
	name := rows[0].Name
	r.drain()

	r.say(r.anna, "/merc master")
	got := r.msgs(r.anna)
	if !hasMsg(got, "chat:"+name+" holds no Master Class.") ||
		!hasMsg(got, "chat:A Gnome Warrior may take: Class Defender, Soldier; Racial Tinker. *") {
		t.Fatalf("the list: %q", got)
	}
	r.say(r.anna, "/merc master tinker")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+name+" must be level 55 to hold a Master Class (it is 30).") {
		t.Fatalf("below 55: %q", got)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Master != "" {
		t.Fatalf("kept below 55: %q", row.Master)
	}
	k, _ := m.kit(rows[0], 30)
	if s := entity.MercPetFor(k); s == nil || s.Name != "Invention" || k.Master != "" {
		t.Errorf("a gnome warrior of 30: pet %v, master %q", s, k.Master)
	}

	r.pa.Level = 60
	m.rekit(r.pa.CharServerID, 60)
	r.drain()
	r.say(r.anna, "/merc master lich")
	if got := r.msgs(r.anna); !hasMsg(got, `red:A Gnome Warrior cannot take "lich". *`) {
		t.Fatalf("a necromancer's: %q", got)
	}
	r.say(r.anna, "/merc master tink")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+" is now a Tinker: Construct, Repair.") {
		t.Fatalf("Tinker: %q", got)
	}
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if row.Master != "Tinker" {
		t.Fatalf("its row: %q", row.Master)
	}
	k, _ = m.kit(row, 60)
	ids := func(k entity.MercKit) []int {
		var out []int
		for _, s := range k.Spells {
			out = append(out, s.SpellID)
		}
		return out
	}
	if b := ids(k); !slices.Contains(b, 9901) || !slices.Contains(b, 9903) || slices.Contains(b, 9902) {
		t.Errorf("Tinker's book: %v", b)
	}
	if s := entity.MercPetFor(k); s == nil || s.Name != "Construct" {
		t.Errorf("Tinker's pet: %v", s)
	}
	info, _ := r.gw.MercInfoOf(r.anna)
	if info.Master != "Tinker" {
		t.Errorf("out: %+v", info)
	}
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	if got := r.gw.MercTitle(merc); got != "Tinker" {
		t.Errorf("its title: %q", got)
	}
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, ", Master Class Tinker") }) {
		t.Errorf("/merc: %q", got)
	}

	// another in its place: Tinker's abilities go
	r.say(r.anna, "/merc master soldier")
	r.drain()
	row, _, _ = db.Merc(r.pa.CharServerID, 1)
	k, _ = m.kit(row, 60)
	if b := ids(k); row.Master != "Soldier" || !slices.Contains(b, 9904) || slices.Contains(b, 9901) || slices.Contains(b, 9903) ||
		len(k.MasterPets) != 0 {
		t.Errorf("Soldier: %q, book %v, pets %v", row.Master, b, k.MasterPets)
	}
	spells, _ := r.gw.MercSpells(r.anna, 1)
	if !slices.ContainsFunc(spells, func(s entity.MercSpell) bool { return s.Name == "Soldier's Cry" && s.Use == "taunt" }) {
		t.Errorf("the merc out casts: %+v", spells)
	}

	// back at a login (a keep restart's Restore, made from its row)
	if _, ok := r.gw.DismissMerc(r.anna, 1, time.Now()); !ok {
		t.Fatal("no merc to take away")
	}
	m.Restore(r.anna, r.pa, entity.KeptMerc{Slot: 1, Name: name, HPPct: 1, PowerPct: 1}, time.Now())
	if info, ok := r.gw.MercInfoOf(r.anna); !ok || info.Master != "Soldier" {
		t.Errorf("restored: %+v %v", info, ok)
	}

	// none
	r.say(r.anna, "/merc master none")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:"+name+" no longer holds the Master Class of Soldier.") {
		t.Errorf("none: %q", got)
	}
	spells, _ = r.gw.MercSpells(r.anna, 1)
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Master != "" ||
		slices.ContainsFunc(spells, func(s entity.MercSpell) bool { return s.Name == "Soldier's Cry" }) {
		t.Errorf("after none: %q, %+v", row.Master, spells)
	}
}

// A human warrior is offered no racial Master Class (Tinker is a gnome's);
// its menu's Master (level 55 or more) asks its kinds, then a kind's names,
// and a choice is /merc master's. Below 55 the menu has no Master.
func TestMercMasterMenu(t *testing.T) {
	r, m, db := masterFixture(t)
	r.pa.Level = 30
	m.hire(r.anna, r.pa, 0, 0, 0)
	r.gw.AskMercGroup(r.anna)
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	r.drain()
	r.talk(r.anna, merc)
	if got := r.msgs(r.anna); len(got) != 1 || strings.Contains(got[0], "Master") {
		t.Fatalf("the menu at 30: %q", got)
	}
	r.pa.Level = 60
	m.rekit(r.pa.CharServerID, 60)
	r.drain()
	r.talk(r.anna, merc)
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Advice|Spells|Stance: assist|Master: none|Items|Dismiss") {
		t.Fatalf("the menu at 60: %q", got)
	}
	got = r.answer(got, "Master: none")
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Class (2)") {
		t.Fatalf("the kinds: %q", got)
	}
	got = r.answer(got, "Class (2)")
	if len(got) != 1 || !strings.HasSuffix(got[0], "|Defender|Soldier") {
		t.Fatalf("the class's: %q", got)
	}
	r.answer(got, "Defender")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Master != "Defender" {
		t.Fatalf("picked: %q", row.Master)
	}
	r.talk(r.anna, merc)
	got = r.answer(r.msgs(r.anna), "Master: Defender")
	if !strings.HasSuffix(got[0], "|Class (2)|None") {
		t.Fatalf("the kinds with one held: %q", got)
	}
	got = r.answer(r.answer(got, "Class (2)"), "Defender (now)")
	r.talk(r.anna, merc)
	r.answer(r.answer(r.msgs(r.anna), "Master: Defender"), "None")
	if row, _, _ := db.Merc(r.pa.CharServerID, 1); row.Master != "" {
		t.Errorf("none: %q", row.Master)
	}
}

// A page of a kind's Master Classes holds the client's room less one, More
// between the pages and back to the first after the last.
func TestMercMasterMenuPages(t *testing.T) {
	r, m, _ := masterFixture(t)
	var rows []database.CMRankRow
	for i, n := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf"} {
		rows = append(rows, database.CMRankRow{ID: 7000 + i, Category: 4, Classes: 1, Races: 2047, Mastery: n, Rank: 1, Name: n, Level: 55,
			Enabled: true})
	}
	r.gw.SetCMCatalog(rows)
	r.pa.Level = 60
	m.hire(r.anna, r.pa, 0, 0, 0)
	r.gw.AskMercGroup(r.anna)
	merc := r.gw.NPCByID(r.gw.MercOf(r.anna))
	r.drain()
	r.talk(r.anna, merc)
	got := r.answer(r.answer(r.msgs(r.anna), "Master: none"), "Class (7)")
	if !strings.HasSuffix(got[0], "|Alpha|Bravo|Charlie|Delta|Echo|More") {
		t.Fatalf("page 1: %q", got)
	}
	got = r.answer(got, "More")
	if !strings.HasSuffix(got[0], "|Foxtrot|Golf|More") {
		t.Fatalf("page 2: %q", got)
	}
	got = r.answer(got, "More")
	if !strings.HasSuffix(got[0], "|Alpha|Bravo|Charlie|Delta|Echo|More") {
		t.Fatalf("back to page 1: %q", got)
	}
}
