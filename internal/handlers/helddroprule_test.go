package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// A held piece drops its look's plain record near the creature's level, and its rare
// one 1 in 500 (the owner, 2026-11-21: "mobs are dropping rare items because they are
// holding those items, the chances of them holding rare items should be like 1 in
// 500", "and let it be around that mobs level"): database/helddrops.go's doc.

const (
	ruleAxeLook   = 0xC0000101 // a plain and a rare record at 30
	ruleStaffLook = 0xC0000102 // rare records alone
	ruleClubLook  = 0xC0000103 // plain records at 10, 27, 28, 32
	ruleBladeLook = 0xC0000104 // plain by its columns, a proc by its description
)

// heldRuleDB is a database with the rule looks' records (and Grak's npcheld row for the
// axe's look), its HeldDrops turned on at the defaults.
func heldRuleDB(t *testing.T) (*database.InventoryStore, *database.HeldDrops) {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "heldrule.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, r := range []struct {
		id          int
		name, desc  string
		level, str  int
		trade, lore int
		model       uint32
	}{
		{9501, "iron axe", "", 30, 0, 0, 0, ruleAxeLook},
		{9502, "axe of might", "", 30, 5, 0, 0, ruleAxeLook},  // a stat
		{9503, "bound staff", "", 30, 0, 1, 0, ruleStaffLook}, // No Trade
		{9504, "lore staff", "", 20, 0, 0, 1, ruleStaffLook},  // Lore
		{9505, "worn club", "", 10, 0, 0, 0, ruleClubLook},
		{9506, "oak club", "", 27, 0, 0, 0, ruleClubLook},
		{9507, "ash club", "", 28, 0, 0, 0, ruleClubLook},
		{9508, "elm club", "", 32, 0, 0, 0, ruleClubLook},
		{9509, "ember blade", "Procs 10-20 fire damage, a 10% chance each hit.", 30, 0, 0, 0, ruleBladeLook},
		{9510, "Grak's Cleaver", "", 30, 0, 0, 1, 0x0D0D0D0D}, // Grak's own (npcheld), Lore (No Trade would go to shares)
	} {
		if _, err := db.Exec(`INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4,
			attacktype, weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft,
			itemname, itemdesc, model, color, str) VALUES (?, 10, 0, 1, 0, 14, 0, ?, 1, 0, 1, 5, 0, ?, 1, 100, 0, 32767, 1023, 0, ?, 0, 0, ?, ?, ?, 0, ?)`,
			r.id, r.trade, r.level, r.lore, r.name, r.desc, int64(int32(r.model)), r.str); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO npcheld (npc_name, hand, patternid) VALUES ('grak', 0, 9510)`); err != nil {
		t.Fatal(err)
	}
	inv := database.NewInventoryStore(db)
	d, err := inv.HeldDrops()
	if err != nil {
		t.Fatal(err)
	}
	return inv, d
}

func TestHeldDropsPlainNormallyRareOneIn500(t *testing.T) {
	_, d := heldRuleDB(t)
	if d.RareOneIn != 500 || d.LevelBelow != 5 || d.LevelAbove != 2 {
		t.Errorf("defaults: 1 in %d, window -%d..+%d, want 500, 5, 2", d.RareOneIn, d.LevelBelow, d.LevelAbove)
	}
	pick := func(model uint32, level int, rare bool) string {
		e, ok := d.Pick(model, false, level, "orc pawn", rare)
		if !ok {
			return ""
		}
		return e.Pattern.ItemName
	}
	// a look with a plain and a rare record: the plain one, the rare one on the roll
	if got := pick(ruleAxeLook, 30, false); got != "iron axe" {
		t.Errorf("the axe's plain draw: %q", got)
	}
	if got := pick(ruleAxeLook, 30, true); got != "axe of might" {
		t.Errorf("the axe's rare draw: %q", got)
	}
	// a look with rare records alone: nothing, the rare one on the roll (the nearest)
	if got := pick(ruleStaffLook, 30, false); got != "" {
		t.Errorf("the staff's plain draw: %q, want nothing (never a rare one in its place)", got)
	}
	if got := pick(ruleStaffLook, 30, true); got != "bound staff" {
		t.Errorf("the staff's rare draw: %q", got)
	}
	if got := pick(ruleStaffLook, 22, true); got != "lore staff" {
		t.Errorf("the staff's rare draw at 22: %q, want the level 20 one", got)
	}
	// a rare draw on a look with plain records alone: the plain one
	if got := pick(ruleClubLook, 28, true); got != "ash club" {
		t.Errorf("the club's rare draw: %q, want its plain one", got)
	}
	// the window: levelreq level-5..level+2, the nearest, of two as near the lower
	for _, c := range []struct {
		level int
		want  string
	}{
		{30, "ash club"},  // 28 (2 under) and 32 (2 over): the lower
		{31, "elm club"},  // 32 one over
		{28, "ash club"},  // its own level
		{36, "elm club"},  // 32, 4 under
		{37, "elm club"},  // 32, 5 under: in
		{38, ""},          // 32 is 6 under: nothing (the worn club far below never)
		{12, "worn club"}, // 10, 2 under
		{8, "worn club"},  // 10, 2 over: in
		{7, ""},           // 10, 3 over: out
		{20, ""},          // 10 is 10 under, 27 is 7 over
	} {
		if got := pick(ruleClubLook, c.level, false); got != c.want {
			t.Errorf("the club at %d: %q, want %q", c.level, got, c.want)
		}
	}
	// a proc by its description alone makes a rare record (SetHeldDrops marks it)
	if d.Rare(9509) {
		t.Error("the ember blade is rare before its proc is read")
	}
	SetHeldDrops(d)
	defer SetHeldDrops(nil)
	if !d.Rare(9509) || pick(ruleBladeLook, 30, false) != "" || pick(ruleBladeLook, 30, true) != "ember blade" {
		t.Errorf("the ember blade (a proc): rare %v, plain %q", d.Rare(9509), pick(ruleBladeLook, 30, false))
	}
	// the report: the axe's and club's looks have a plain record, the staff's and blade's rare ones alone
	rep := HeldDropsReport(d, [][2]uint32{{ruleAxeLook, 0}, {ruleStaffLook, 0}, {ruleClubLook, 0}, {ruleBladeLook, 0}})
	if rep.Mapped != 4 || rep.Plain != 2 || rep.RareOnly != 2 {
		t.Errorf("report %+v", rep)
	}
	// the odds: under 1/500 draws rare; 0 never; 1 always
	for _, c := range []struct {
		oneIn int
		roll  float64
		want  bool
	}{{500, 0, true}, {500, 0.00199, true}, {500, 0.002, false}, {500, 0.9, false}, {0, 0, false}, {1, 0.999, true}} {
		d.RareOneIn = c.oneIn
		if got := d.RollsRare(c.roll); got != c.want {
			t.Errorf("1 in %d, roll %v: rare %v, want %v", c.oneIn, c.roll, got, c.want)
		}
	}
}

func TestACorpsesHeldPieceRollsRareByItsDice(t *testing.T) {
	inv, d := heldRuleDB(t)
	SetHeldDrops(d)
	defer SetHeldDrops(nil)
	h := NewNPCWindowHandler(nil, inv)
	h.lootFloat = func() float64 { return 0.999999 }
	h.lootIntn = func(n int) int { return n - 1 }
	h.coinRoll = func(int) int { return 0 }
	orc := func(name string, look int) *entity.NPC {
		return &entity.NPC{ServerID: 77101, NPCID: 77101, Name: name, Level: 30, PrimaryWeapon: look}
	}
	held := func(npc *entity.NPC) []string {
		var out []string
		for _, it := range h.rollCorpse(npc, time.Now(), 30).items {
			out = append(out, it.Pattern.ItemName)
		}
		return out
	}
	one := func(got []string, want string) bool {
		if want == "" {
			return len(got) == 0
		}
		return len(got) == 1 && got[0] == want
	}
	for _, c := range []struct {
		roll        float64
		look        int
		name, want  string
		description string
	}{
		{0.5, int(uint32(ruleAxeLook)), "orc pawn", "iron axe", "a plain draw"},
		{0.001, int(uint32(ruleAxeLook)), "orc pawn", "axe of might", "the 1 in 500"},
		{0.5, int(uint32(ruleStaffLook)), "orc pawn", "", "rare records alone, a plain draw"},
		{0.001, int(uint32(ruleStaffLook)), "orc pawn", "bound staff", "rare records alone, the 1 in 500"},
		{0.5, 0x0D0D0D0D, "Grak", "Grak's Cleaver", "a name's own record (npcheld), a plain draw"},
		{0.001, 0x0D0D0D0D, "Grak", "Grak's Cleaver", "a name's own record, a rare draw"},
	} {
		roll := c.roll
		h.heldFloat = func() float64 { return roll }
		if got := held(orc(c.name, c.look)); !one(got, c.want) {
			t.Errorf("%s: %v, want %q", c.description, got, c.want)
		}
	}
	// 0: never rare
	d.RareOneIn = 0
	h.heldFloat = func() float64 { return 0 }
	if got := held(orc("orc pawn", int(uint32(ruleAxeLook)))); !one(got, "iron axe") {
		t.Errorf("held_rare_one_in 0, roll 0: %v, want the plain axe", got)
	}
	// the spawn's dice: the same corpse draws the same whoever opens it; over many spawns
	// about 1 in RareOneIn draws rare
	d.RareOneIn = 10
	h.heldFloat = nil
	seed := uint64(1)
	h.lootSeed = func(*entity.NPC) uint64 { return seed }
	rare := 0
	const spawns = 4000
	for seed = 1; seed <= spawns; seed++ {
		a, b := held(orc("orc pawn", int(uint32(ruleAxeLook)))), held(orc("orc pawn", int(uint32(ruleAxeLook))))
		if len(a) != 1 || len(b) != 1 || a[0] != b[0] {
			t.Fatalf("spawn %d: %v then %v", seed, a, b)
		}
		if a[0] == "axe of might" {
			rare++
		}
	}
	if rare < spawns/10/2 || rare > spawns/10*2 {
		t.Errorf("1 in 10 over %d spawns: %d rare", spawns, rare)
	}
}
