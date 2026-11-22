package handlers

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Hogarth Bartholomew's gem setting (gemsetter.go), on the portal rig: Anna at (5000, 50, 5000)
// in world 0 with a row of her own, Hogarth 3 off her, and gemsetting as the tool writes it for
// one weapon family and two gems: Obliterator plain, Green and White Obliterator canonical, a
// transfer's copy of the White one (canonical 0), the Green and White gems. The records are
// made up; the figures are not tested here (the tool's test does).

type gemsRig struct {
	*portalRig
	db       *database.SQLiteDB
	inv      *database.InventoryStore
	hogarth  *entity.NPC
	finished bool
	ids      map[string]int
}

func newGemsRig(t *testing.T) *gemsRig {
	t.Helper()
	r := &gemsRig{portalRig: newPortalRig(t), finished: true, ids: map[string]int{}}
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "gems.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r.db, r.inv = db, database.NewInventoryStore(db)
	rec := &database.CharacterRecord{CharName: "Anna", AccountID: 1, Level: 60}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	r.pa.CharServerID, r.pa.Level = rec.ServerID, 60
	r.anna.SetInventoryList(newInvList(nil))
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990101, ?, 0, 5003, 50, 5000, 0, 1893243078, 1.0, 55, 128, '0', 0)`, GemSetterName); err != nil {
		t.Fatal(err)
	}
	for _, it := range []struct {
		name, kind, family, colour string
		slot, lore, canon          int
	}{
		{"Obliterator", "weapon", "obliterator", "", 12, 1, 1},
		{"Green Obliterator", "weapon", "obliterator", "green", 12, 1, 1},
		{"White Obliterator", "weapon", "obliterator", "white", 12, 1, 1},
		{"White Obliterator copy", "weapon", "obliterator", "white", 12, 1, 0},
		{"Green Gem of Khalimet", "gem", "", "green", -1, 1, 1},
		{"White Gem of Khalimet", "gem", "", "white", -1, 1, 1},
		{"Rusty Sword", "", "", "", 12, 0, 0}, // not of the table
	} {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 1, 0, ?, 0, 0, 1, 0, 1, 700, 0, 55, 1, 20000, 80, 32767, 1023, 0, ?, 0, 0, ?, '', 0, 0)`,
			it.slot, it.lore, it.name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		r.ids[it.name] = int(id)
		if it.kind != "" {
			if _, err := db.Exec(`INSERT INTO gemsetting (patternid, kind, family, colour, canonical, source) VALUES (?, ?, ?, ?, ?, 'test')`,
				id, it.kind, it.family, it.colour, it.canon); err != nil {
				t.Fatal(err)
			}
		}
	}
	r.gw.LoadNPCs(database.NewNPCStore(db))
	for _, n := range r.gw.NPCs {
		if n.Name == GemSetterName {
			r.hogarth = n
		}
	}
	if r.hogarth == nil {
		t.Fatal("no Hogarth")
	}
	r.ih.gemSetter = NewGemSetter(r.gw, db)
	r.ih.gemSetter.finished = func(id uint32) map[int]bool {
		if r.finished && id == r.pa.CharServerID {
			return map[int]bool{GemSetterQuest: true}
		}
		return map[int]bool{}
	}
	r.pa.Tunar = 50000
	return r
}

// carry puts one of the record named into Anna's pack, as a give does.
func (r *gemsRig) carry(name string) {
	r.t.Helper()
	pat, err := database.NewItemPatternStore(r.db).GetItemPattern(r.ids[name])
	if err != nil || pat == nil {
		r.t.Fatalf("%s: %v", name, err)
	}
	if n, err := GiveItemNow(r.anna, r.gw, r.inv, *pat, 1); err != nil || n != 1 {
		r.t.Fatalf("carrying %s: %d %v", name, n, err)
	}
}

// held is how many of the record named Anna holds (charinventory, and the session's list agrees).
func (r *gemsRig) held(name string) int {
	r.t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT count(*) FROM charinventory WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, r.ids[name]).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	if l := sessionInvList(r.anna); l.held(r.ids[name]) != n {
		r.t.Errorf("%s: the session's list holds %d, the database %d", name, l.held(r.ids[name]), n)
	}
	return n
}

func (r *gemsRig) visit() ([]string, uint32) {
	r.t.Helper()
	r.drain()
	r.talk(r.anna, r.hogarth)
	got := r.msgs(r.anna)
	return got, heroesBoxOf(got)
}

func (r *gemsRig) choose(box uint32, option byte) ([]string, uint32) {
	r.t.Helper()
	r.pick(r.anna, box, option)
	got := r.msgs(r.anna)
	return got, heroesBoxOf(got)
}

// lastAsk is the last question's text and answers ("question|a|b"), "" none.
func lastAsk(msgs []string) string {
	out := ""
	for _, m := range msgs {
		if strings.HasPrefix(m, "ask#") {
			_, rest, _ := strings.Cut(m, ":")
			out = rest
		}
	}
	return out
}

func saidHas(msgs []string, part string) bool {
	for _, m := range msgs {
		if strings.Contains(m, part) {
			return true
		}
	}
	return false
}

// Setting a gem: the plain weapon and the gem carried, 10,000 tunar; Yes takes both and the price
// and gives the coloured record. Taking it out: the coloured weapon, 2,000; the plain weapon and
// the gem come back. A box is answered once.
func TestHogarthSetsAndTakesOutAGem(t *testing.T) {
	r := newGemsRig(t)
	r.carry("Obliterator")
	r.carry("Green Gem of Khalimet")
	got, box := r.visit()
	if !IsGemSetterBox(box) || !saidHas(got, "box:Hogarth Bartholomew: Khalimet's gems fit the warlords' weapons. I set one for 10,000 tunar and take one out for 2,000 tunar") ||
		!strings.HasSuffix(lastAsk(got), "What shall I do?|Set a gem in a weapon|Take a gem out of a weapon") {
		t.Fatalf("the greeting: %q (box %#x)", got, box)
	}
	got, box = r.choose(box, 0)
	if !strings.HasSuffix(lastAsk(got), "Which weapon?|Obliterator") {
		t.Fatalf("the weapons: %q", got)
	}
	got, box = r.choose(box, 0)
	if !strings.HasSuffix(lastAsk(got), "Which gem shall I set?|Green Gem of Khalimet") {
		t.Fatalf("the gems: %q", got)
	}
	got, box = r.choose(box, 0)
	if !strings.HasSuffix(lastAsk(got), "Set the Green Gem of Khalimet in your Obliterator for 10,000 tunar?|Yes|No") {
		t.Fatalf("the price: %q", got)
	}
	if r.held("Obliterator") != 1 || r.held("Green Gem of Khalimet") != 1 || r.pa.Tunar != 50000 {
		t.Fatal("asking took something")
	}
	confirm := box
	got, _ = r.choose(confirm, 0)
	if !saidHas(got, "Hogarth Bartholomew: There: your Green Obliterator.") || !hasMsg(got, "op:003B") {
		t.Errorf("the setting said %q", got)
	}
	if r.held("Obliterator") != 0 || r.held("Green Gem of Khalimet") != 0 || r.held("Green Obliterator") != 1 || r.pa.Tunar != 40000 {
		t.Fatalf("after the setting: plain %d, gem %d, green %d, purse %d", r.held("Obliterator"), r.held("Green Gem of Khalimet"), r.held("Green Obliterator"), r.pa.Tunar)
	}
	var purse int
	if err := r.db.QueryRow(`SELECT tunar FROM characters WHERE serverid = ?`, r.pa.CharServerID).Scan(&purse); err != nil || purse != 40000 {
		t.Errorf("the purse kept: %d (%v)", purse, err)
	}
	// the old box again: nothing
	if got, _ := r.choose(confirm, 0); len(got) != 0 || r.held("Green Obliterator") != 1 || r.pa.Tunar != 40000 {
		t.Errorf("the old box answered again: %q", got)
	}

	// out again
	_, box = r.visit()
	got, box = r.choose(box, 1)
	if !strings.HasSuffix(lastAsk(got), "Which weapon's gem shall I take out?|Green Obliterator") {
		t.Fatalf("the coloured weapons: %q", got)
	}
	got, box = r.choose(box, 0)
	if !strings.HasSuffix(lastAsk(got), "Take the gem out of your Green Obliterator for 2,000 tunar?|Yes|No") {
		t.Fatalf("the price: %q", got)
	}
	got, _ = r.choose(box, 0)
	if !saidHas(got, "There: the gem is out, whole. Your Obliterator and your Green Gem of Khalimet.") {
		t.Errorf("the taking out said %q", got)
	}
	if r.held("Obliterator") != 1 || r.held("Green Gem of Khalimet") != 1 || r.held("Green Obliterator") != 0 || r.pa.Tunar != 38000 {
		t.Fatalf("after the taking out: plain %d, gem %d, green %d, purse %d", r.held("Obliterator"), r.held("Green Gem of Khalimet"), r.held("Green Obliterator"), r.pa.Tunar)
	}
}

// A transfer's copy of a coloured weapon (canonical 0) is taken in as its record: the gem out of
// it gives the canonical plain weapon and gem.
func TestHogarthTakesACopyAsItsRecord(t *testing.T) {
	r := newGemsRig(t)
	r.carry("White Obliterator copy")
	_, box := r.visit()
	_, box = r.choose(box, 1)
	_, box = r.choose(box, 0)
	r.choose(box, 0)
	if r.held("White Obliterator copy") != 0 || r.held("Obliterator") != 1 || r.held("White Gem of Khalimet") != 1 || r.pa.Tunar != 48000 {
		t.Fatalf("copy %d, plain %d, white gem %d, purse %d", r.held("White Obliterator copy"), r.held("Obliterator"), r.held("White Gem of Khalimet"), r.pa.Tunar)
	}
}

// The negative controls: each leaves the pack and the purse as they were.
func TestHogarthRefuses(t *testing.T) {
	t.Run("before Gnadriin he is the quest's Hogarth", func(t *testing.T) {
		r := newGemsRig(t)
		r.finished = false
		r.carry("Obliterator")
		r.carry("Green Gem of Khalimet")
		r.drain()
		if r.ih.gemSetter.Talk(r.anna, r.pa, r.hogarth) {
			t.Error("he set gems for a character that has not finished Gnadriin")
		}
	})
	t.Run("another NPC is no setter", func(t *testing.T) {
		r := newGemsRig(t)
		if r.ih.gemSetter.Talk(r.anna, r.pa, &entity.NPC{ServerID: 3301, Name: "Hogarth's Apprentice", NPCType: 0x80}) {
			t.Error("another NPC set gems")
		}
	})
	t.Run("an empty table: no setter", func(t *testing.T) {
		r := newGemsRig(t)
		if _, err := r.db.Exec(`DELETE FROM gemsetting`); err != nil {
			t.Fatal(err)
		}
		if r.ih.gemSetter.Talk(r.anna, r.pa, r.hogarth) {
			t.Error("he set gems with no table")
		}
	})
	for _, tc := range []struct {
		name   string
		carry  []string
		purse  int
		remove bool
		setup  func(r *gemsRig)
		want   string
	}{
		{"no plain weapon", []string{"Green Gem of Khalimet", "Rusty Sword"}, 50000, false, nil, "You carry no plain Khal weapon."},
		{"no gem", []string{"Obliterator"}, 50000, false, nil, "You carry no Gem of Khalimet."},
		{"nothing to take out", []string{"Obliterator", "Green Gem of Khalimet"}, 50000, true, nil, "You carry no Khal weapon with a gem in it."},
		{"a worn weapon is not offered", []string{"Green Gem of Khalimet", "Obliterator"}, 50000, false, func(r *gemsRig) {
			if _, err := r.db.Exec(`UPDATE charinventory SET equiploc = 12 WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, r.ids["Obliterator"]); err != nil {
				r.t.Fatal(err)
			}
		}, "You carry no plain Khal weapon."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newGemsRig(t)
			for _, n := range tc.carry {
				r.carry(n)
			}
			if tc.setup != nil {
				tc.setup(r)
			}
			r.pa.Tunar = tc.purse
			_, box := r.visit()
			opt := byte(0)
			if tc.remove {
				opt = 1
			}
			got, next := r.choose(box, opt)
			if !saidHas(got, tc.want) || next != 0 {
				t.Errorf("said %q (a box %#x), want %q", got, next, tc.want)
			}
			if r.pa.Tunar != tc.purse {
				t.Errorf("the purse %d, want %d", r.pa.Tunar, tc.purse)
			}
		})
	}
	// at the price: short, a Lore result held, the gem gone since the question, an aura on the copy,
	// too far away; nothing taken
	for _, tc := range []struct {
		name  string
		purse int
		setup func(r *gemsRig)
		want  string
	}{
		{"a purse short", 9999, nil, "My price is 10,000 tunar, and you carry 9,999 tunar."},
		{"the coloured record held already", 50000, func(r *gemsRig) { r.carry("Green Obliterator") }, "You can hold only one Green Obliterator"},
		{"the gem gone since", 50000, func(r *gemsRig) {
			rows, err := r.inv.GetCharacterItems(r.pa.CharServerID)
			if err != nil {
				r.t.Fatal(err)
			}
			for _, row := range rows {
				if row.Pattern.PatternID == r.ids["Green Gem of Khalimet"] {
					takeStacks(r.inv, r.anna, r.pa, "test", []database.CharInventoryEntry{row}, 1)
				}
			}
		}, "You no longer carry the Green Gem of Khalimet in your pack."},
		{"an aura on the weapon", 50000, func(r *gemsRig) {
			var item int
			if err := r.db.QueryRow(`SELECT itemid FROM charinventory WHERE serverid = ? AND patternid = ?`, r.pa.CharServerID, r.ids["Obliterator"]).Scan(&item); err != nil {
				r.t.Fatal(err)
			}
			r.t.Cleanup(func() { database.SetInstanceAuraCache(nil) }) // the cache is the process's
			if err := r.db.SetInstanceAuraAudited(database.InstanceAura{ItemID: item, Tag: "Virtuous",
				First: database.ItemAura{Kind: database.AuraEpic, Source: 0x88EC092D}, Source: "test"}, database.GMEdit{Username: "test"}, "test"); err != nil {
				r.t.Fatal(err)
			}
		}, "carries an enchanter's aura"},
		{"too far away", 50000, func(r *gemsRig) { r.pa.X = 5200 }, "Come closer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newGemsRig(t)
			r.carry("Obliterator")
			r.carry("Green Gem of Khalimet")
			r.pa.Tunar = tc.purse
			_, box := r.visit()
			_, box = r.choose(box, 0)
			_, box = r.choose(box, 0)
			_, box = r.choose(box, 0) // the price asked
			if tc.setup != nil {
				tc.setup(r)
			}
			before := map[string]int{}
			for _, n := range []string{"Obliterator", "Green Gem of Khalimet", "Green Obliterator"} {
				before[n] = r.held(n)
			}
			got, _ := r.choose(box, 0)
			if !saidHas(got, tc.want) {
				t.Errorf("said %q, want %q", got, tc.want)
			}
			for n, k := range before {
				if r.held(n) != k {
					t.Errorf("%s: %d held, was %d", n, r.held(n), k)
				}
			}
			if r.pa.Tunar != tc.purse {
				t.Errorf("the purse %d, want %d", r.pa.Tunar, tc.purse)
			}
		})
	}
	t.Run("No changes nothing", func(t *testing.T) {
		r := newGemsRig(t)
		r.carry("Obliterator")
		r.carry("Green Gem of Khalimet")
		_, box := r.visit()
		_, box = r.choose(box, 0)
		_, box = r.choose(box, 0)
		_, box = r.choose(box, 0)
		got, _ := r.choose(box, 1)
		if !saidHas(got, "As you like.") || r.held("Obliterator") != 1 || r.held("Green Obliterator") != 0 || r.pa.Tunar != 50000 {
			t.Errorf("No: %q", got)
		}
	})
	t.Run("a give that fails puts everything back", func(t *testing.T) {
		r := newGemsRig(t)
		r.carry("Obliterator")
		r.carry("Green Gem of Khalimet")
		g := r.ih.gemSetter
		real := g.give
		g.give = func(sess *session.Session, pat database.ItemPatternRecord) (int, error) {
			if pat.PatternID == r.ids["Green Obliterator"] {
				return 0, fmt.Errorf("a test's failure")
			}
			return real(sess, pat)
		}
		_, box := r.visit()
		_, box = r.choose(box, 0)
		_, box = r.choose(box, 0)
		_, box = r.choose(box, 0)
		got, _ := r.choose(box, 0)
		if !saidHas(got, "Something slipped. Here is all you gave me") {
			t.Errorf("said %q", got)
		}
		if r.held("Obliterator") != 1 || r.held("Green Gem of Khalimet") != 1 || r.held("Green Obliterator") != 0 || r.pa.Tunar != 50000 {
			t.Errorf("after a failed give: plain %d, gem %d, green %d, purse %d", r.held("Obliterator"), r.held("Green Gem of Khalimet"), r.held("Green Obliterator"), r.pa.Tunar)
		}
	})
}
