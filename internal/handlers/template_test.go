package handlers

import (
	"slices"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// #template (template.go) against a temporary database. Item names are made up.

// The command: a GM's #template on a level 1 gnome enchanter writes level 60
// (with its experience), 1,000,000 tunar, its class's spells and its class's
// heirlooms, worn, to the database, sets the game's copy (full hit points at
// 60) and sends one MemoryDump, at the character's own place, that carries it
// all. A player's #template is refused, as every GM command is.
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
	heirloom(t, f.db, "Made-up Hat", 1, 12, "head", 20)
	heirloom(t, f.db, "Made-up Coat", 5, 12, "chest", 40)
	heirloom(t, f.db, "Warrior's Coat", 5, 0, "chest", 400) // not an enchanter's
	templateItem(t, f.db, "Best Hat Of All", 1, 500)        // no heirloom: the template gives none of the rest

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
	// the mastery points to the most, spent and unspent: 60 spent keeps 1940
	if gp.Mastery.Points != 1940 || gp.Mastery.Spent != 60 || kept.rows[gp.CharServerID].Points != 1940 {
		t.Errorf("the mastery points: %+v, kept %+v; want 1940 unspent beside 60 spent", gp.Mastery, kept.rows[gp.CharServerID])
	}
	if u, sp, ok := masteryPoints(gs); !ok || u != 1940 || sp != 60 {
		t.Errorf("the client was told %d/%d (%v), want 1940/60", u, sp, ok)
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
	if len(said) != 4 || said[0] != "[GM] Template: level 60, 1000000 tunar, 2 spells scribed, 2 heirlooms given, 2 of them worn" ||
		!strings.HasPrefix(said[3], "[GM]   Mastery points: 1900 more, 1940 unspent, 60 spent") {
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

// A GM's #template <player> is that player's (the user, 2026-11-07), who is told: in any case of the name, with the
// GM reading what it came to. No name is the GM's own, no such player is said, and a player can't do it to anyone.
func TestAGMTemplatesAPlayer(t *testing.T) {
	c, gs, _, ps, _ := gmWorld(t)
	chatText := func(s *session.Session) string {
		var all []string
		for _, h := range heard(t, s) {
			all = append(all, h.text)
		}
		return strings.Join(all, "|")
	}
	var got []*session.Session
	c.h.SetTemplate(func(s *session.Session) ([]string, error) {
		got = append(got, s)
		return []string{"Template: the line"}, nil
	})
	c.say(gs, "#template player")
	if len(got) != 1 || got[0] != ps {
		t.Fatalf("#template player: %d calls, on the player's session %v", len(got), len(got) == 1 && got[0] == ps)
	}
	if text := chatText(ps); !strings.Contains(text, "A GM gave your character the template") {
		t.Errorf("the player's told: %q", text)
	}
	if text := chatText(gs); !strings.Contains(text, "Template: the line") {
		t.Errorf("the GM's told: %q", text)
	}
	c.say(gs, "#template")
	if len(got) != 2 || got[1] != gs {
		t.Fatalf("#template alone: %d calls", len(got))
	}
	c.say(gs, "#template Nobody")
	if len(got) != 2 {
		t.Errorf("#template Nobody ran: %d calls", len(got))
	}
	c.say(ps, "#template Gm")
	if len(got) != 2 {
		t.Errorf("a player's #template ran: %d calls", len(got))
	}
}

// The template's book holds all of a class's spells: a mastery's, an epic's and a master class's too, where a new
// character's holds none of them; a were's stays out.
func TestTemplateGrantsEverySpell(t *testing.T) {
	f := newGearFixture(t)
	spell := func(id int, name, desc, source string) {
		t.Helper()
		if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
			VALUES (?, 4, 0, 0, 16880, 2, 15, 1, 2, 1, 1, -1, ?, ?)`, id, name, desc); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (12, ?, ?)`, id, source); err != nil {
			t.Fatal(err)
		}
	}
	spell(10, "Plain", "", "eqoa-data")
	spell(11, "Of A Mastery", "", "eqoa-data cm")
	spell(12, "Of An Epic", "", "eqoa-data epic")
	spell(13, "Of A Master", "", "eqoa-data master Faithful")
	store := database.NewSQLiteCharacterStore(f.db)
	n, err := store.GrantClassSpells(f.sid, 12)
	if err != nil || n != 1 {
		t.Fatalf("a class grant: %d, %v; want the plain one", n, err)
	}
	if n, err = store.GrantEveryClassSpell(f.sid, 12); err != nil || n != 3 {
		t.Fatalf("every spell: %d more, %v; want 3", n, err)
	}
	if n, err = store.GrantEveryClassSpell(f.sid, 12); err != nil || n != 0 {
		t.Fatalf("again: %d more, %v; want none", n, err)
	}
}

// templateItem is a made-up item any class may wear: its pattern id.
func templateItem(t *testing.T, db *database.SQLiteDB, name string, wear, ac int) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color, AC)
		VALUES (0, 0, 0, 0, ?, 0, 0, 1, 0, 0, 0, 0, 1, 1, 100, 0, 32767, 1023, 0, 0, 0, 0, ?, '', 0, 0, ?)`, wear, name, ac)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return int(id)
}

// heirloom is a made-up item as a class's heirloom of the role: its pattern id.
func heirloom(t *testing.T, db *database.SQLiteDB, name string, wear, class int, role string, ac int) int {
	t.Helper()
	id := templateItem(t, db, name, wear, ac)
	if _, err := db.Exec(`INSERT INTO heirlooms (patternid, class, role) VALUES (?, ?, ?)`, id, class, role); err != nil {
		t.Fatal(err)
	}
	return id
}

// The template gives a class's heirlooms a character lacks and nothing else (the user, 2026-11-07: "only heirloom if they
// dont have"): the lowest of a role, none of a role it holds in its pack or its bank, another class's never; a new one
// worn only in a free slot with nothing taken off, what it wears left on and what it had kept. The hands: a main hand and
// its shield worn and the two-hander carried; a class with no main hand wears the two-hander and carries the shield.
func TestTheTemplateGivesOnlyTheHeirloomsACharacterLacks(t *testing.T) {
	type got struct {
		loc, n int
	}
	inv := func(t *testing.T, f *gearFixture) map[string]got {
		t.Helper()
		items, err := f.inv.GetCharacterItems(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]got{}
		for _, e := range items {
			g := out[e.Pattern.ItemName]
			out[e.Pattern.ItemName] = got{e.EquipLoc, g.n + 1}
		}
		return out
	}
	t.Run("a main hand", func(t *testing.T) {
		f := newGearFixture(t)
		h := &CharacterHandler{inventoryStore: f.inv}
		heirloom(t, f.db, "H Helm", 1, 3, "head", 1)
		heirloom(t, f.db, "H Coat", 5, 3, "chest", 1)
		heirloom(t, f.db, "H Boots", 11, 3, "feet", 1)
		heirloom(t, f.db, "H Ring", 8, 3, "ring", 1)
		heirloom(t, f.db, "H Sword", 12, 3, "main", 1)
		heirloom(t, f.db, "H Later Sword", 12, 3, "main", 1) // the same role, a higher id
		heirloom(t, f.db, "H Shield", 13, 3, "shield", 1)
		heirloom(t, f.db, "H Greatsword", 15, 3, "twohand", 1)
		heirloom(t, f.db, "Other Class's Boots", 11, 0, "feet", 1)
		// a hat of its own, worn; its heirloom coat in the bank, its boots in the pack
		own := templateItem(t, f.db, "Own Hat", 1, 9)
		id, err := f.inv.AddCharItem(f.sid, own, 1, 100, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.inv.SetEquipLoc(f.sid, id, 0); err != nil {
			t.Fatal(err)
		}
		var coat, boots int
		if err := f.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = 'H Coat'`).Scan(&coat); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = 'H Boots'`).Scan(&boots); err != nil {
			t.Fatal(err)
		}
		cid, err := f.inv.AddCharItem(f.sid, coat, 1, 100, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`UPDATE charinventory SET location = 1 WHERE itemid = ?`, cid); err != nil {
			t.Fatal(err)
		}
		if _, err := f.inv.AddCharItem(f.sid, boots, 1, 100, 0, 0); err != nil {
			t.Fatal(err)
		}

		_, given, worn, err := h.templateHeirlooms(f.sid, 3, invListCap)
		if err != nil {
			t.Fatal(err)
		}
		g := inv(t, f)
		want := map[string]got{
			"Own Hat":      {0, 1},  // left on
			"H Helm":       {-1, 1}, // its slot is taken: carried
			"H Coat":       {-1, 1}, // held in the bank: not given again
			"H Boots":      {-1, 1}, // held in the pack: not given again, nor worn
			"H Ring":       {9, 1},
			"H Sword":      {14, 1},
			"H Shield":     {17, 1},
			"H Greatsword": {-1, 1}, // it would take the hands off
		}
		for name, w := range want {
			if g[name] != w {
				t.Errorf("%s: %+v, want %+v (all %+v)", name, g[name], w, g)
			}
		}
		if len(g) != len(want) {
			t.Errorf("%d kinds, want %d: %+v", len(g), len(want), g)
		}
		if given != 5 || worn != 3 {
			t.Errorf("%d given, %d worn; want 5 and 3", given, worn)
		}

		// again: nothing more
		if _, given, _, err := h.templateHeirlooms(f.sid, 3, invListCap); err != nil || given != 0 {
			t.Errorf("a second time: %d given, %v", given, err)
		}
	})
	t.Run("no main hand", func(t *testing.T) {
		f := newGearFixture(t)
		h := &CharacterHandler{inventoryStore: f.inv}
		heirloom(t, f.db, "H Shield", 13, 13, "shield", 1)
		heirloom(t, f.db, "H Staff", 15, 13, "twohand", 1)
		if _, _, _, err := h.templateHeirlooms(f.sid, 13, invListCap); err != nil {
			t.Fatal(err)
		}
		g := inv(t, f)
		if g["H Staff"] != (got{16, 1}) || g["H Shield"] != (got{-1, 1}) {
			t.Errorf("%+v, want the staff worn and the shield carried", g)
		}
	})
	t.Run("no room", func(t *testing.T) {
		f := newGearFixture(t)
		h := &CharacterHandler{inventoryStore: f.inv}
		heirloom(t, f.db, "H Helm", 1, 3, "head", 1)
		junk := templateItem(t, f.db, "Junk", 1, 1)
		for range invListCap {
			if _, err := f.inv.AddCharItem(f.sid, junk, 1, 100, 0, 0); err != nil {
				t.Fatal(err)
			}
		}
		lines, given, _, err := h.templateHeirlooms(f.sid, 3, invListCap)
		if err != nil || given != 0 || len(lines) != 1 || !strings.Contains(lines[0], "No room for H Helm") {
			t.Errorf("a full pack: %q, %d given, %v", lines, given, err)
		}
	})
}

// The template never takes from a purse or a level: one past them keeps what it has.
func TestTheTemplateKeepsAFullerPurseAndAHigherLevel(t *testing.T) {
	f := newGearFixture(t)
	c, gs, gp, _, _ := gmWorld(t)
	store := database.NewSQLiteCharacterStore(f.db)
	gp.CharServerID, gp.Class, gp.Race = f.sid, 12, 3
	gp.Level, gp.Tunar = 100, 5_000_000
	h := &CharacterHandler{store: store, sessionManager: c.sm, gameWorld: c.gw, inventoryStore: f.inv}
	lines, err := h.ApplyTemplate(gs)
	if err != nil {
		t.Fatal(err)
	}
	if gp.Level != 100 || gp.Tunar != 5_000_000 {
		t.Errorf("level %d, %d tunar: want them kept", gp.Level, gp.Tunar)
	}
	row, err := store.GetByServerID(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if row.Tunar != 5_000_000 {
		t.Errorf("the row's purse %d", row.Tunar)
	}
	if !strings.HasPrefix(lines[0], "Template: level 100, 5000000 tunar") {
		t.Errorf("the answer %q", lines[0])
	}
}
