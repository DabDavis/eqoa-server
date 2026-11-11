package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// The mercenaries' handlers (merc.go): a Mercenary Liaison hires one, a box at
// a time, kept in charmerc; its kit is its class's heirlooms at its owner's
// level; /merc's spellings; a login brings it back unless it rests; off, a
// liaison says nothing and /merc says so.

// mercHeirloom is one heirloom record of the fixture: its role, slot, look,
// attack type and the class it is.
type mercHeirloom struct {
	id, class  int
	role       string
	slot       int
	model      int
	attackType int
}

// The fixture's heirlooms: a warrior's set (two swords, a shield, a robe) and a
// wizard's (a staff, a robe), the seed's ids.
var mercHeirlooms = []mercHeirloom{
	{714000, 0, "head", 1, 4, 0}, {714001, 0, "chest", 5, 4, 0}, {714002, 0, "bracers", 7, 4, 0}, {714003, 0, "hands", 19, 4, 0},
	{714004, 0, "legs", 10, 4, 0}, {714005, 0, "feet", 11, 4, 0}, {714006, 0, "robe", 2, 3, 0}, {714007, 0, "ear", 3, 0, 0},
	{714008, 0, "neck", 4, 0, 0}, {714009, 0, "ring", 8, 0, 0}, {714010, 0, "bracelet", 6, 0, 0}, {714011, 0, "waist", 9, 0, 0},
	{714012, 0, "shield", 13, 1890000000, 0}, {714013, 0, "main", 14, 1205436896, 1}, {714300, 0, "main", 14, 1205436897, 1},
	{714188, 13, "head", 1, 1, 0}, {714189, 13, "chest", 5, 1, 0}, {714194, 13, "robe", 2, 5, 0},
	{714201, 13, "twohand", 15, 495410893, 4},
}

// newMercFixture is a portal rig's world over a database of the fixture's
// heirlooms, a real chest and sword of level 20 for a warrior's figures, and
// a warrior's Taunt and a wizard's bolt in their classes' books.
func newMercFixture(t *testing.T) (*portalRig, *Mercs, *database.SQLiteDB) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/merc.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); SetMercs(false); db.SetPatternView(nil) })
	n := func(v int) *int { return &v }
	items := database.NewItemPatternStore(db)
	put := func(r database.ItemPatternRecord) {
		t.Helper()
		if r.ClassUse == 0 {
			r.ClassUse = 1
		}
		r.Rent, r.RaceUse, r.MaxStack = 1, 32767, 1
		if err := items.UpsertItemPattern(&r); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range mercHeirlooms {
		put(database.ItemPatternRecord{PatternID: h.id, ItemName: "Heirloom " + h.role, EquipSlot: h.slot, LevelReq: 1, Model: int64(h.model),
			AttackType: h.attackType, ClassUse: 1 << uint(h.class)})
		if _, err := db.Exec(`INSERT INTO heirlooms (patternid, class, role) VALUES (?, ?, ?)`, h.id, h.class, h.role); err != nil {
			t.Fatal(err)
		}
	}
	put(database.ItemPatternRecord{PatternID: 900001, ItemName: "a plate tunic", EquipSlot: 5, LevelReq: 20, AC: n(50)})
	put(database.ItemPatternRecord{PatternID: 900002, ItemName: "a broadsword", EquipSlot: 14, LevelReq: 20, WeaponDamage: 120, AttackType: 1})
	for _, s := range []struct {
		id, class, level, scope, cast, recast, power int
		name, line                                   string
	}{
		{2295, 0, 5, 1, 0, 5, 19, "Taunt", "Target, Increase Hate by 100 ; Increase 10% based on STR"},
		{950, 13, 10, 1, 2, 10, 40, "Ice Bolt", "Target, Decrease Hitpoints by 300"},
	} {
		if _, err := db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope,
			recast, eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, ?, ?, 0, 0, ?, ?, 0, ?, '')`,
			s.id, s.level, s.cast, s.power, s.scope, s.recast, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, 'test')`, s.id, s.line); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (?, ?, 'vendor')`, s.class, s.id); err != nil {
			t.Fatal(err)
		}
	}
	r := newPortalRig(t)
	r.gw.SetRaceModels(map[[2]int]uint32{{0, 0}: 1893243078, {0, 1}: 223572789, {2, 0}: 2128125354, {2, 1}: 2358987577})
	heir := NewHeirlooms(db)
	heir.Start(nil, nil)
	m := NewMercs(r.gw, db, heir)
	r.ih.mercs, r.h.mercs = m, m
	r.pa.Level, r.pa.Race, r.pa.Class = 30, 0, 0
	SetMercs(true)
	return r, m, db
}

// liaison is a Mercenary Liaison beside Anna.
func (r *portalRig) liaison() *entity.NPC {
	n := &entity.NPC{ServerID: 7790, Name: MercLiaisonName, X: 5001, Y: 50, Z: 5000, Level: 60, HP: 1000, MaxHP: 1000, NPCType: 0x80}
	r.gw.NPCs[n.ServerID] = n
	return n
}

// answer picks the answer of the box asked last that reads want, and returns
// what was sent after.
func (r *portalRig) answer(asked []string, want string) []string {
	r.t.Helper()
	box := boxID(asked)
	for _, m := range asked {
		if !strings.HasPrefix(m, "ask#") {
			continue
		}
		opts := strings.Split(m[strings.Index(m, "|")+1:], "|")
		for i, o := range opts {
			if o == want {
				r.pick(r.anna, box, byte(i))
				return r.msgs(r.anna)
			}
		}
		r.t.Fatalf("no answer %q in %q", want, m)
	}
	r.t.Fatalf("no box in %q", asked)
	return nil
}

// A liaison asks a calling, a class of it and a race; "Like me" hires one of
// its owner's race and sex at once: in the world at its owner's level, its
// row kept, a line said. A race of the game's asks a man or a woman after.
func TestMercLiaisonHires(t *testing.T) {
	r, _, db := newMercFixture(t)
	lia := r.liaison()
	r.drain()
	r.talk(r.anna, lia)
	got := r.msgs(r.anna)
	if len(got) != 1 || !strings.HasSuffix(got[0], "|A healer|A tank|A melee fighter|A caster") {
		t.Fatalf("the liaison's first box: %q", got)
	}
	got = r.answer(got, "A tank")
	if !strings.HasSuffix(got[0], "|Warrior|Paladin|Shadow Knight") {
		t.Fatalf("the tanks' box: %q", got)
	}
	got = r.answer(got, "Warrior")
	if !strings.HasSuffix(got[0], "|Like me: Human|Human to Dwarf|Troll to Ogre") {
		t.Fatalf("the race box: %q", got)
	}
	got = r.answer(got, "Like me: Human")
	info, ok := r.gw.MercInfoOf(r.anna)
	if !ok || info.Class != 0 || info.Race != 0 || info.Level != 30 || info.Stance != entity.MercAssist {
		t.Fatalf("hired: %+v %v (sent %q)", info, ok, got)
	}
	if !hasMsg(got, "box:Mercenary Liaison: "+info.Name+" the Human Warrior is yours.*") {
		t.Errorf("the hire's line: %q", got)
	}
	row, kept, err := db.Merc(r.pa.CharServerID)
	if err != nil || !kept || row.Name != info.Name || row.Class != 0 || row.Race != 0 {
		t.Errorf("its row: %+v %v %v", row, kept, err)
	}
	// a dark elf woman wizard, by her race's box and the sex's
	r.talk(r.anna, lia)
	got = r.answer(r.msgs(r.anna), "A caster")
	got = r.answer(got, "Wizard")
	got = r.answer(got, "Human to Dwarf")
	if !strings.HasSuffix(got[0], "|Human|Elf|Dark Elf|Gnome|Dwarf") {
		t.Fatalf("the first five races: %q", got)
	}
	got = r.answer(got, "Dark Elf")
	got = r.answer(got, "A woman")
	if info, ok = r.gw.MercInfoOf(r.anna); !ok || info.Class != 13 || info.Race != 2 || info.Sex != 1 {
		t.Fatalf("the wizard: %+v %v (sent %q)", info, ok, got)
	}
}

// With world.open_races, the liaison offers the races creation offers past
// the game's (the froglok, every racedefs row): one of one body is hired with
// no sex asked, in its body, passing as its wire race. The control: off, no
// "Other races".
func TestMercLiaisonOtherRaces(t *testing.T) {
	r, _, _ := newMercFixture(t)
	entity.SetRaceDefs([]database.RaceDefRow{{ID: 13, Name: "Goblin", WireRace: 2, BodyMale: 0x6C0E0D20, BodyFemale: 0x6C0E0D20}})
	t.Cleanup(func() { entity.SetRaceDefs(nil); entity.SetCreationRules(entity.CreationRules{}) })
	lia := r.liaison()
	ask := func() []string {
		r.drain()
		r.talk(r.anna, lia)
		got := r.answer(r.msgs(r.anna), "A melee fighter")
		return r.answer(got, "Rogue")
	}
	if got := ask(); strings.Contains(got[0], "Other races") {
		t.Fatalf("other races offered with world.open_races off: %q", got)
	}
	entity.SetCreationRules(entity.CreationRules{Races: true})
	got := r.answer(ask(), "Other races")
	if !strings.HasSuffix(got[0], "|Froglok|Goblin") {
		t.Fatalf("the other races: %q", got)
	}
	got = r.answer(got, "Goblin")
	info, ok := r.gw.MercInfoOf(r.anna)
	if !ok || info.Race != 13 || info.Class != 6 {
		t.Fatalf("the goblin rogue: %+v %v (sent %q)", info, ok, got)
	}
	for _, n := range r.gw.NPCs {
		if n.Owner == r.pa.ServerID && (uint32(n.ModelID) != 0x6C0E0D20 || n.Race != "2") {
			t.Errorf("the goblin merc wears %#x as race %q", uint32(n.ModelID), n.Race)
		}
	}
}

// A merc's kit is its class's heirlooms at its owner's level: a warrior's
// armour's look and a level's best figures (the plate tunic's AC 50 on the
// heirloom chest at 30), a sword and a shield, no robe; a wizard's staff in
// both hands and its robe; base stats a player's at the level; its book its
// class's to its level.
func TestMercKitIsItsClassesHeirlooms(t *testing.T) {
	_, m, _ := newMercFixture(t)
	war, err := m.kit(database.MercRow{Name: "Ralthas", Class: 0, Race: 0}, 30)
	if err != nil {
		t.Fatal(err)
	}
	l := war.Look
	if l.Chest != 4 || l.Helm != 4 || l.Gloves != 4 || l.Robe != -1 || l.Primary != 1205436896 || l.Shield != 1890000000 || l.PrimaryType != 1 {
		t.Errorf("the warrior's looks: %+v", l)
	}
	if !war.Worn[14] || !war.Worn[17] || war.Worn[15] || war.Worn[16] || war.AttackType != 1 {
		t.Errorf("the warrior's hands: %v type %d", war.Worn[14:19], war.AttackType)
	}
	if war.Gear.AC < 50 || war.Gear.WeaponDamage != 120 {
		t.Errorf("the warrior's gear: AC %d, weapon %d; the level's best tunic's 50 and sword's 120", war.Gear.AC, war.Gear.WeaponDamage)
	}
	if war.Base != entity.SimBaseStats(0, 0, 30) {
		t.Errorf("the warrior's base stats %v, a player's %v", war.Base, entity.SimBaseStats(0, 0, 30))
	}
	if len(war.Spells) != 1 || war.Spells[0].Name != "Taunt" {
		t.Errorf("the warrior's book: %v", war.Spells)
	}
	wiz, err := m.kit(database.MercRow{Name: "Velith", Class: 13, Race: 2, Sex: 1}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !wiz.Worn[16] || wiz.Worn[14] || wiz.Look.Robe != 5 || wiz.Look.Primary != 495410893 || len(wiz.Spells) != 1 {
		t.Errorf("the wizard: hands %v robe %d primary %d book %d", wiz.Worn[14:19], wiz.Look.Robe, wiz.Look.Primary, len(wiz.Spells))
	}
	// below 20 a melee fighter holds one weapon; from 20 two
	low, _ := m.kit(database.MercRow{Name: "Rax", Class: 0, Race: 0}, 10)
	if low.Gear.WeaponDamage == 120 {
		t.Error("a level 10's heirloom sword dealt a level 20 sword's damage")
	}
}

// /merc in every spelling: "/merc", "#merc", a PS2's "/t merc ..."; its stance
// kept in its row; "/t merc" with a tell's words is a tell. Off, /merc says so
// and a liaison says nothing.
func TestMercCommands(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hire(r.anna, r.pa, 0, 0, 0)
	if _, ok := r.gw.MercInfoOf(r.anna); !ok {
		t.Fatal("no merc hired")
	}
	r.drain()
	r.say(r.anna, "/merc stance passive")
	if info, _ := r.gw.MercInfoOf(r.anna); info.Stance != entity.MercPassive {
		t.Fatalf("/merc stance passive: %v", info.Stance)
	}
	if row, _, _ := db.Merc(r.pa.CharServerID); row.Stance != int(entity.MercPassive) {
		t.Errorf("the stance not kept: %d", row.Stance)
	}
	r.say(r.anna, "#merc stance aggressive")
	if info, _ := r.gw.MercInfoOf(r.anna); info.Stance != entity.MercAggressive {
		t.Fatalf("#merc stance aggressive: %v", info.Stance)
	}
	r.say(r.anna, "/t merc stance assist")
	if info, _ := r.gw.MercInfoOf(r.anna); info.Stance != entity.MercAssist {
		t.Fatalf("/t merc stance assist: %v", info.Stance)
	}
	if mercWords("hello there") || !mercWords("") || !mercWords("Dismiss") {
		t.Error("a tell to merc is taken for /merc, or /merc's words are not")
	}
	r.drain()
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Your mercenary *") {
		t.Errorf("/merc: %q", got)
	}
	r.say(r.anna, "/t merc dismiss")
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Error("/t merc dismiss left the merc")
	}
	if _, kept, _ := db.Merc(r.pa.CharServerID); kept {
		t.Error("the dismissal kept the hire")
	}
	SetMercs(false)
	r.drain()
	r.say(r.anna, "/merc")
	if got := r.msgs(r.anna); !hasMsg(got, "red:"+mercOffLine) {
		t.Errorf("/merc with mercenaries off: %q", got)
	}
	if r.ih.mercs.Talk(r.anna, r.pa, r.liaison()) {
		t.Error("a liaison talked with mercenaries off")
	}
}

// A login brings the hire back at its owner's level; one resting after its
// death is not, its time said; /merc after the rest brings it.
func TestMercComesBackAtLogin(t *testing.T) {
	r, m, db := newMercFixture(t)
	now := time.Unix(1_800_000_000, 0)
	m.now = func() time.Time { return now }
	if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Name: "Ralthas", Class: 0, Race: 0, Stance: 2}); err != nil {
		t.Fatal(err)
	}
	m.comeBack(r.anna, r.pa)
	if info, ok := r.gw.MercInfoOf(r.anna); !ok || info.Name != "Ralthas" || info.Level != 30 {
		t.Fatalf("at the login: %+v %v", info, ok)
	}
	// its death: a rest kept; the next login says it rests
	m.note(entity.MercNote{Kind: entity.MercNoteDied, Owner: r.pa, Name: "Ralthas", Until: now.Add(entity.MercRest)})
	r.gw.DismissMerc(r.anna, now)
	r.drain()
	m.comeBack(r.anna, r.pa)
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Fatal("a resting merc came back")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Your mercenary Ralthas rests for 60 more seconds.") {
		t.Errorf("the rest's line: %q", got)
	}
	now = now.Add(entity.MercRest)
	r.say(r.anna, "/merc")
	if _, ok := r.gw.MercInfoOf(r.anna); !ok {
		t.Error("/merc after the rest did not bring it back")
	}
}

// A familiar's item (a row of familiars) calls it by its cast: begun at its
// Use, there once the cast has landed, sent away by "/familiar dismiss"; with
// familiars off the item is quiet and casts nothing (the control).
func TestFamiliarItemCallsIt(t *testing.T) {
	r, _, db := newMercFixture(t)
	t.Cleanup(func() { SetFamiliars(false) })
	if _, err := db.Exec(`INSERT INTO familiars (patternid, name, model, size, hp, regen, cast_ms, heal, heal_below, heal_every_ms)
		VALUES (715060, 'Healbot', 4222348032, 1, 100000, 2000, 2000, 600, 75, 6000)`); err != nil {
		t.Fatal(err)
	}
	item := &database.ItemPatternRecord{PatternID: 715060, ItemName: "Healbot's Cog", EquipSlot: -1}
	f := NewFamiliars(r.gw, db)
	r.h.familiars = f
	if !f.IsItem(715060) || f.IsItem(714000) {
		t.Fatal("the familiar's item is not one, or another item is")
	}
	r.drain()
	f.Use(r.anna, r.pa, item)
	if got := r.msgs(r.anna); !hasMsg(got, "red:Healbot's Cog is quiet here.") || r.pa.Casting != nil {
		t.Fatalf("with familiars off: %q, a cast %v", got, r.pa.Casting != nil)
	}
	SetFamiliars(true)
	start := time.Now()
	f.Use(r.anna, r.pa, item)
	if r.pa.Casting == nil {
		t.Fatal("the item's use began no cast")
	}
	r.gw.Tick(start.Add(2*time.Second + 1500*time.Millisecond))
	if name, _, ok := r.gw.FamiliarOf(r.anna); !ok || name != "Healbot" {
		t.Fatalf("after the cast: %q %v", name, ok)
	}
	r.say(r.anna, "/familiar dismiss")
	if _, _, ok := r.gw.FamiliarOf(r.anna); ok {
		t.Error("/familiar dismiss left Healbot")
	}
}

// Petition 1504 (Lifeispain): a merc back at a login is asked into a group,
// and the no dismisses it, its row dropped as /merc dismiss drops it; the next
// login brings none. The control: the row is kept until the answer.
func TestADeclinedMercsHireEnds(t *testing.T) {
	r, m, db := newMercFixture(t)
	r.gw.OnMercNote(m.note) // as Start hears the World
	if err := db.SetMerc(database.MercRow{ServerID: r.pa.CharServerID, Name: "Ullin", Class: 9, Race: 0, Stance: 2}); err != nil {
		t.Fatal(err)
	}
	m.comeBack(r.anna, r.pa)
	if _, ok := r.gw.MercInfoOf(r.anna); !ok {
		t.Fatal("no merc at the login")
	}
	r.gw.Tick(time.Now())
	if _, kept, _ := db.Merc(r.pa.CharServerID); !kept {
		t.Fatal("the hire was ended before the answer")
	}
	r.drain()
	r.gw.GroupDecline(r.anna, r.anna.PlayerObjectID(), 1)
	r.gw.Tick(time.Now())
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Fatal("the declined merc stayed")
	}
	if _, kept, _ := db.Merc(r.pa.CharServerID); kept {
		t.Error("the declined merc's hire was kept")
	}
	if got := r.msgs(r.anna); !hasMsg(got, "chat:Your mercenary Ullin is dismissed*") {
		t.Errorf("the owner's lines: %q", got)
	}
	m.comeBack(r.anna, r.pa)
	if _, ok := r.gw.MercInfoOf(r.anna); ok {
		t.Error("a login after the no brought the merc back")
	}
}
