package handlers

import (
	"strconv"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// /aura (gmaura.go): a GM's aura on its target creature's name (npctint) and
// on an item's worn piece (itemaura), each kept, audited and in the world at
// once; bad input and a player's /aura change nothing (the controls).
func TestGMAuraCommand(t *testing.T) {
	c, db, _, gs, gp, ps := campWorld(t)
	targetAs(gp, campWolfNear, 100101, gs, ps)

	c.say(ps, "/aura both FF2020FF")
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM npctint`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("a player's /aura: %d rows, %v", n, err)
	}
	c.say(gs, "/aura sparkly FF2020FF")
	if l := gmReplies(t, gs); !hasLine(l, "Usage") {
		t.Fatalf("a bad kind: %q", l)
	}

	c.say(gs, "/aura both FF2020FF 88EC092D 12")
	if l := gmReplies(t, gs); !hasLine(l, "Every wolf") {
		t.Fatalf("/aura: %q", l)
	}
	tints := c.gw.NPCTints()
	want := database.NPCTint{Name: "wolf", Color: 0xFFFFFFFF, Glow: 0xFF2020FF, AuraKind: database.AuraBoth, AuraSource: 0x88EC092D, AuraNodes: 12}
	if len(tints) != 1 || tints[0] != want {
		t.Fatalf("the world's tints %+v, want %+v", tints, want)
	}
	if auditLines(t, db, "npctint", "insert") != 1 {
		t.Fatal("not audited")
	}
	// particles alone, no glow: kept (they need none)
	c.say(gs, "/aura particles 0")
	if tints := c.gw.NPCTints(); len(tints) != 1 || tints[0].AuraKind != database.AuraParticles || tints[0].Glow != 0 {
		t.Fatalf("particles alone: %+v", tints)
	}
	c.say(gs, "/aura off")
	if tints := c.gw.NPCTints(); len(tints) != 0 || auditLines(t, db, "npctint", "update") != 2 {
		t.Fatalf("/aura off: %+v", tints)
	}

	// an item's worn piece
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 7, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Glowing Bracer', '', 4, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	c.say(gs, "/aura item 999999 shell 3060FFFF")
	if l := gmReplies(t, gs); !hasLine(l, "no item 999999") || c.gw.ItemAuraCount() != 0 {
		t.Fatalf("no such item: %q", l)
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" shell 3060FFFF 0 4")
	if l := gmReplies(t, gs); !hasLine(l, "everyone wearing it shows it") || c.gw.ItemAuraCount() != 1 {
		t.Fatalf("/aura item: %q", l)
	}
	got, err := db.ItemAuras()
	if a := got[int(pid)]; err != nil || a != (database.ItemAura{PatternID: int(pid), Color: 0x3060FFFF, Kind: database.AuraShell, Nodes: 4}) {
		t.Fatalf("the row: %+v, %v", a, err)
	}
	// the trim in black: a dark aura, kept though its colour lights nothing
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" trim 000000FF")
	if l := gmReplies(t, gs); !hasLine(l, "aura trim in 000000FF (dark)") || c.gw.ItemAuraCount() != 1 {
		t.Fatalf("/aura item trim black: %q", l)
	}
	if got, _ := db.ItemAuras(); got[int(pid)] != (database.ItemAura{PatternID: int(pid), Color: 0x000000FF, Kind: database.AuraTrim}) {
		t.Fatalf("the dark trim row: %+v", got[int(pid)])
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" 7 FFC040FF")
	if l := gmReplies(t, gs); !hasLine(l, "Usage") {
		t.Fatalf("kind 7: %q", l)
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" 6 FFC040FF")
	if got, _ := db.ItemAuras(); got[int(pid)].Kind != database.AuraTrimShell {
		t.Fatalf("kind 6 by number: %+v", got[int(pid)])
	}
	c.say(gs, "/aura item "+strconv.Itoa(int(pid))+" off")
	if c.gw.ItemAuraCount() != 0 || auditLines(t, db, "itemaura", "insert") != 1 || auditLines(t, db, "itemaura", "delete") != 1 ||
		auditLines(t, db, "itemaura", "update") != 2 {
		t.Fatal("/aura item off: not deleted, or not audited")
	}
}

// The easy words (parseAuraEasy): colours by name and hex (six digits alpha
// FF, black a dark aura), kinds and places in any order and case, source= and
// nodes=, off; the refusals name the word. The old forms are still read as
// they were (oldAuraForm), and six hex digits after a kind are the easy RRGGBB.
func TestAuraEasyWords(t *testing.T) {
	for _, c := range []struct {
		in   string
		want auraSpec
	}{
		{"blue particles chest", auraSpec{A: database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles}, Where: entity.AuraChest}},
		{"CHEST Particles Blue", auraSpec{A: database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles}, Where: entity.AuraChest}},
		{"yellow shell wholebody", auraSpec{A: database.ItemAura{Color: 0xFFFF30FF, Kind: database.AuraShell}, Where: auraWholeBody}},
		{"black trim legs", auraSpec{A: database.ItemAura{Color: 0x000000FF, Kind: database.AuraTrim}, Where: entity.AuraLegs}},
		{"ff8000 trimshell arms", auraSpec{A: database.ItemAura{Color: 0xFF8000FF, Kind: database.AuraTrimShell}, Where: entity.AuraBracer}},
		{"#20FF2080 both feet nodes=4 source=88EC092D", auraSpec{A: database.ItemAura{Color: 0x20FF2080, Kind: database.AuraBoth, Source: 0x88EC092D, Nodes: 4}, Where: entity.AuraBoots}},
		{"gold head", auraSpec{A: database.ItemAura{Color: 0xFFC040FF, Kind: database.AuraShell}, Where: entity.AuraHelm}},
		{"sparkles hands source=Sword of Virtue", auraSpec{A: database.ItemAura{Kind: database.AuraParticles}, Where: entity.AuraGloves, SourceName: "Sword of Virtue"}},
		{"purple body", auraSpec{A: database.ItemAura{Color: 0xA040FFFF, Kind: database.AuraShell}, Where: auraWholeBody}},
		{"off chest", auraSpec{Off: true, Where: entity.AuraChest}},
		{"off", auraSpec{Off: true, Where: -1}},
		{"source=Heirloom_Sword red particles", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraParticles}, Where: -1, SourceName: "Heirloom Sword"}},
		{"teal robe", auraSpec{A: database.ItemAura{Color: 0x20C0B0FF, Kind: database.AuraShell}, Where: entity.AuraRobe}},
		{"red trim cape", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraTrim}, Where: entity.AuraCape}},
		{"cloak blue particles", auraSpec{A: database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles}, Where: entity.AuraCape}},
		{"epic weapons source=5FD4AE7C", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}, Where: auraBothHands}},
		{"epic mainhand source=Sickle_of_Avhi_Escron", auraSpec{A: database.ItemAura{Kind: database.AuraEpic}, Where: entity.AuraMainHand, SourceName: "Sickle of Avhi Escron"}},
		{"red epic offhand", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraEpic}, Where: entity.AuraOffHand}},
		{"particles secondary", auraSpec{A: database.ItemAura{Kind: database.AuraParticles}, Where: entity.AuraOffHand}},
		{"off weapons", auraSpec{Off: true, Where: auraBothHands}},
		// the named looks (2026-11-14): their words in a row, run together or with underscores; both hands by default
		{"druid staff glow", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45}, Where: auraBothHands, Look: "druid staff glow"}},
		{"Druid_Staff_Glow mainhand", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45}, Where: entity.AuraMainHand, Look: "druid staff glow"}},
		{"offhand druidstaffglow red", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45}, Where: entity.AuraOffHand, Look: "druid staff glow"}},
		// where along the weapon (2026-11-14): along=N (or N%) on a hand, the look's own 45 by default
		{"druid staff glow along=30", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 30}, Where: auraBothHands, Look: "druid staff glow"}},
		{"epic mainhand source=5FD4AE7C along=60%", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C, Nodes: 60}, Where: entity.AuraMainHand}},
		{"monk circlet", auraSpec{A: database.ItemAura{Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: CircletHeightMonk}, Where: entity.AuraHelm, Look: "monk circlet"}},
		{"gold monk_circlet head", auraSpec{A: database.ItemAura{Color: 0xFFC040FF, Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: CircletHeightMonk}, Where: entity.AuraHelm, Look: "monk circlet"}},
		// how high on the head (2026-11-14): height=N on a circlet, the monk circlet's own by default
		// a spell visual's hand emitters, named (2026-11-14): both hands unless one is named
		{"blue freezing offhand", auraSpec{A: database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles, Source: 0x6C151A60}, Where: entity.AuraOffHand, Look: "freezing"}},
		{"freezing", auraSpec{A: database.ItemAura{Kind: database.AuraParticles, Source: 0x6C151A60}, Where: auraBothHands, Look: "freezing"}},
		{"cyan freezing mainhand along=40", auraSpec{A: database.ItemAura{Color: 0x30FFFFFF, Kind: database.AuraParticles, Source: 0x6C151A60, Nodes: 40}, Where: entity.AuraMainHand, Look: "freezing"}},
		// the epics by class (2026-11-14): the first by the class's name, each numbered; primary/secondary
		{"epic necromancer primary", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xFD9C3858}, Where: entity.AuraMainHand, Look: "epic necromancer"}},
		{"epic necromancer2 secondary", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}, Where: entity.AuraOffHand, Look: "epic necromancer2"}},
		{"epic druid2", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A}, Where: auraBothHands, Look: "epic druid2"}},
		{"Epic_Druid1 weapons", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0xD03942DE}, Where: auraBothHands, Look: "epic druid1"}},
		{"red epic paladin mainhand", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraEpic, Source: 0xF841266D}, Where: entity.AuraMainHand, Look: "epic paladin"}},
		{"red energy offhand source=2EC56029", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraEnergy, Source: 0x2EC56029}, Where: entity.AuraOffHand}},
		{"monk circlet height=82", auraSpec{A: database.ItemAura{Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: 82}, Where: entity.AuraHelm, Look: "monk circlet"}},
		{"red circlet head source=1D875ECD height=70%", auraSpec{A: database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: 70}, Where: entity.AuraHelm}},
		{"circlet helm source=1D875ECD", auraSpec{A: database.ItemAura{Kind: database.AuraCirclet, Source: 0x1D875ECD}, Where: entity.AuraHelm}},
		{"sickle taint epic weapons", auraSpec{A: database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}, Where: auraBothHands, Look: "sickle taint"}},
	} {
		got, err := parseAuraEasy(strings.Fields(c.in))
		if err != nil || got != c.want {
			t.Errorf("%q: %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"blue red chest", "blue shell chest legs", "sparkly chest", "shell chest", "off blue chest",
		"blue nodes=40", "blue shell trim", "blue foo=1", "", "druid staff glow shell", "druid staff glow source=88EC092D", "off druid staff glow",
		"druid staff",
		"druid staff glow along=0", "druid staff glow along=101", "druid staff glow chest along=50", "blue shell chest along=50",
		"epic weapons source=5FD4AE7C along=50 nodes=3", "off weapons along=50",
		"epic magician3", "monk circlet height=0", "monk circlet height=101", "blue shell chest height=50", "druid staff glow height=50", "monk circlet height=70 nodes=3"} {
		if _, err := parseAuraEasy(strings.Fields(bad)); err == nil {
			t.Errorf("%q: read, want refused", bad)
		}
	}
	// the old forms
	for _, c := range []struct {
		in   string
		want database.ItemAura
		ok   bool
	}{
		{"both FF2020FF 88EC092D 12", database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraBoth, Source: 0x88EC092D, Nodes: 12}, true},
		{"particles 0", database.ItemAura{Kind: database.AuraParticles}, true},
		{"3 FF0000FF", database.ItemAura{Color: 0xFF0000FF, Kind: database.AuraParticles}, true},
		{"shell ff0000", database.ItemAura{}, false},
		{"shell blue chest", database.ItemAura{}, false},
		{"blue shell", database.ItemAura{}, false},
	} {
		got, ok := oldAuraForm(strings.Fields(c.in))
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("old %q: %+v %v", c.in, got, ok)
		}
	}
}

// The easy words in the game (the owner's rules of 2026-11-13): a slot on a
// targeted player sets THAT character's piece alone (charaura): another
// player in the same item shows nothing; on yourself with no target, yours;
// wholebody on a player is its body (slot 7); no place on a player is
// refused naming both forms; wholebody on a creature its name's npctint; a
// slot on a creature of sets is refused; charaura wins over the item's own
// itemaura on that character, the other wearer keeping the item's; /aura
// alone is the usage; off takes each away, every change audited.
func TestGMAuraEasyCommand(t *testing.T) {
	c, db, _, gs, gp, ps, pp := campWorldWithPlayer(t)
	res, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 5, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Ornate Breastplate', '', 9, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	gp.CharServerID, pp.CharServerID = 9001, 9002
	gp.SetWornPiece(entity.AuraChest, int(pid))
	pp.SetWornPiece(entity.AuraChest, int(pid))

	c.say(gs, "/aura")
	if l := gmReplies(t, gs); !hasLine(l, "/aura blue particles chest") {
		t.Fatalf("/aura alone: %q", l)
	}
	// the targeted player's chest alone
	targetAs(gp, pp.ServerID, 200202, gs)
	c.say(gs, "/aura blue particles chest")
	if l := gmReplies(t, gs); !hasLine(l, "Player's chest: now a blue particles aura") || !hasLine(l, "this character alone") {
		t.Fatalf("/aura blue particles chest: %q", l)
	}
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraChest] != (database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles}) {
		t.Fatalf("the target's chest: %+v", a)
	}
	if a := c.gw.PieceAurasOf(gp); a != nil {
		t.Fatalf("another wearer of the same item glows: %+v", a)
	}
	if c.gw.ItemAuraCount() != 0 || auditLines(t, db, "charaura", "insert") != 1 {
		t.Fatal("an item's aura written, or not audited")
	}
	// its body: under its armour, its own slot
	c.say(gs, "/aura purple shell wholebody")
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraBody] != (database.ItemAura{Color: 0xA040FFFF, Kind: database.AuraShell}) || a[entity.AuraChest].Kind != database.AuraParticles {
		t.Fatalf("the body: %+v", a)
	}
	// its cape (slot 8): kept, nothing worn there yet
	c.say(gs, "/aura red trim cape")
	if l := gmReplies(t, gs); !hasLine(l, "Player's cape: now a red trim aura") || !hasLine(l, "nothing worn there now") {
		t.Fatalf("/aura red trim cape: %q", l)
	}
	pp.SetWornPiece(entity.AuraCape, int(pid))
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraCape] != (database.ItemAura{Color: 0xFF2020FF, Kind: database.AuraTrim}) {
		t.Fatalf("the cape: %+v", a)
	}
	c.say(gs, "/aura off cape")
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraCape] != (database.ItemAura{}) {
		t.Fatalf("the cape's taken away: %+v", a)
	}
	// its weapons (slots 9 and 10): the epic sickle's look on both hands,
	// named by the item; a shell on a hand and epic on a piece refused
	if _, err := db.Exec(`INSERT INTO itempattern (itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 14, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Sickle of Avhi Escron', '', 1607773820, 0)`); err != nil {
		t.Fatal(err)
	}
	c.say(gs, "/aura epic weapons source=Sickle_of_Avhi_Escron")
	if l := gmReplies(t, gs); !hasLine(l, "Player's weapons (both hands): now the epic look of 5FD4AE7C") || !hasLine(l, "whatever that hand holds") {
		t.Fatalf("/aura epic weapons: %q", l)
	}
	taint := database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraMainHand] != taint || a[entity.AuraOffHand] != taint {
		t.Fatalf("the weapons: %+v", a)
	}
	if a := c.gw.PieceAurasOf(gp); a != nil {
		t.Fatalf("another character's weapons: %+v", a)
	}
	inserts := auditLines(t, db, "charaura", "insert")
	c.say(gs, "/aura blue shell mainhand")
	if l := gmReplies(t, gs); !hasLine(l, "a held weapon takes epic, energy or particles") {
		t.Fatalf("a shell on a hand: %q", l)
	}
	c.say(gs, "/aura epic chest source=5FD4AE7C")
	if l := gmReplies(t, gs); !hasLine(l, "epic is a held weapon's look") {
		t.Fatalf("epic on a piece: %q", l)
	}
	if auditLines(t, db, "charaura", "insert") != inserts {
		t.Fatal("a refused aura written")
	}
	// a named look by its name (/aura looks lists them): the druid's staff on both hands
	c.say(gs, "/aura looks")
	if l := gmReplies(t, gs); !hasLine(l, "/aura druid staff glow: the druid's level 49 epic staff") {
		t.Fatalf("/aura looks: %q", l)
	}
	c.say(gs, "/aura druid staff glow")
	if l := gmReplies(t, gs); !hasLine(l, "Player's weapons (both hands): now the epic look of CBF4135A") || !hasLine(l, "(druid staff glow)") {
		t.Fatalf("/aura druid staff glow: %q", l)
	}
	staff := database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45}
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraMainHand] != staff || a[entity.AuraOffHand] != staff {
		t.Fatalf("the staff's glow: %+v", a)
	}
	if l := gmReplies(t, gs); len(l) != 0 {
		t.Fatalf("left over: %q", l)
	}
	c.say(gs, "/aura druid staff glow mainhand along=30")
	if l := gmReplies(t, gs); !hasLine(l, "30% along the weapon") {
		t.Fatalf("along=30: %q", l)
	}
	if a := c.gw.PieceAurasOf(pp); a[entity.AuraMainHand].Nodes != 30 || a[entity.AuraOffHand].Nodes != 45 {
		t.Fatalf("along: %+v", a)
	}
	// the monk circlet (2026-11-14): the helm's slot, whether a helm is worn or not
	c.say(gs, "/aura monk circlet")
	if l := gmReplies(t, gs); !hasLine(l, "Player's helm: now a circlet of 1D875ECD") || !hasLine(l, "(monk circlet)") {
		t.Fatalf("/aura monk circlet: %q", l)
	}
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraHelm] != (database.ItemAura{Kind: database.AuraCirclet, Source: 0x1D875ECD, Nodes: CircletHeightMonk}) {
		t.Fatalf("the circlet, no helm worn: %+v", a)
	}
	c.say(gs, "/aura monk circlet chest")
	if l := gmReplies(t, gs); !hasLine(l, "a circlet goes round the head") {
		t.Fatalf("a circlet on the chest: %q", l)
	}
	c.say(gs, "/aura off helm")
	gmReplies(t, gs)
	// freezing on the off hand: the ice drip of spell visual 6C151A60, blue
	c.say(gs, "/aura blue freezing offhand")
	if l := gmReplies(t, gs); !hasLine(l, "Player's off hand: now a blue particles aura") || !hasLine(l, "(freezing)") {
		t.Fatalf("/aura blue freezing offhand: %q", l)
	}
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraOffHand] != (database.ItemAura{Color: 0x3060FFFF, Kind: database.AuraParticles, Source: 0x6C151A60}) {
		t.Fatalf("freezing: %+v", a)
	}
	c.say(gs, "/aura druid staff glow chest")
	if l := gmReplies(t, gs); !hasLine(l, "epic is a held weapon's look") {
		t.Fatalf("a named epic look on a piece: %q", l)
	}
	c.say(gs, "/aura epic weapons source=Sickle_of_Avhi_Escron")
	gmReplies(t, gs)
	c.say(gs, "/aura off offhand")
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraMainHand] != taint || a[entity.AuraOffHand] != (database.ItemAura{}) {
		t.Fatalf("the off hand's taken away: %+v", a)
	}
	c.say(gs, "/aura off weapons")
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraMainHand] != (database.ItemAura{}) {
		t.Fatalf("the weapons taken away: %+v", a)
	}
	c.say(gs, "/aura red shell")
	if l := gmReplies(t, gs); !hasLine(l, "name where on a player") || !hasLine(l, "wholebody") {
		t.Fatalf("no place on a player: %q", l)
	}
	// the item's own aura: the other wearer shows it, the target its own
	if _, err := db.SetItemAuraAudited(database.ItemAura{PatternID: int(pid), Color: 0xFFC040FF, Kind: database.AuraTrim}, database.GMEdit{}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.gw.LoadItemAuras(db); err != nil {
		t.Fatal(err)
	}
	if a := c.gw.PieceAurasOf(gp); a == nil || a[entity.AuraChest].Kind != database.AuraTrim {
		t.Fatalf("the other wearer: the item's own: %+v", a)
	}
	if a := c.gw.PieceAurasOf(pp); a[entity.AuraChest].Kind != database.AuraParticles {
		t.Fatalf("charaura does not win: %+v", a)
	}
	// yourself, no target: yours
	gp.TargetObjectID = 0
	c.say(gs, "/aura black trim boots")
	if l := gmReplies(t, gs); !hasLine(l, "Your boots: now a black (dark) trim aura") || !hasLine(l, "nothing worn there now") {
		t.Fatalf("your boots: %q", l)
	}
	c.say(gs, "/aura off boots")
	targetAs(gp, pp.ServerID, 200202, gs)
	c.say(gs, "/aura off chest")
	c.say(gs, "/aura off wholebody")
	if a := c.gw.PieceAurasOf(pp); a == nil || a[entity.AuraChest].Kind != database.AuraTrim || a[entity.AuraBody] != (database.ItemAura{}) {
		t.Fatalf("taken away: the item's own back: %+v", a)
	}
	if auditLines(t, db, "charaura", "delete") != 7 {
		t.Fatal("the deletes not audited")
	}

	// a creature: its whole body by name
	targetAs(gp, campWolfNear, 100101, gs, ps)
	c.say(gs, "/aura yellow shell wholebody")
	if tints := c.gw.NPCTints(); len(tints) != 1 || tints[0].Glow != 0xFFFF30FF || tints[0].AuraKind != database.AuraShell {
		t.Fatalf("wholebody: %+v", tints)
	}
	c.say(gs, "/aura black trim")
	if tints := c.gw.NPCTints(); len(tints) != 1 || tints[0].Glow != 0x000000FF || tints[0].AuraKind != database.AuraTrim {
		t.Fatalf("no place on a creature: its whole body: %+v", tints)
	}
	c.say(gs, "/aura blue shell chest")
	if l := gmReplies(t, gs); !hasLine(l, "wears no items of its own") {
		t.Fatalf("a slot on a creature of sets: %q", l)
	}
	c.say(gs, "/aura epic source=5FD4AE7C")
	if l := gmReplies(t, gs); !hasLine(l, "epic is a held weapon's look") {
		t.Fatalf("epic on a creature: %q", l)
	}
	c.say(gs, "/aura monk circlet wholebody")
	if l := gmReplies(t, gs); !hasLine(l, "a circlet is a player's") {
		t.Fatalf("a circlet on a creature: %q", l)
	}
	c.say(gs, "/aura off wholebody")
	if tints := c.gw.NPCTints(); len(tints) != 0 {
		t.Fatalf("/aura off wholebody: %+v", tints)
	}
}

// The epics' named looks: each of the 14 classes with an epic model (not the alchemist) its first by its name,
// each numbered where a class has more than one (35 models: 49 looks), every one
// a held weapon's epic look of a disc model of models 44-78, and /aura looks a
// line each, each one fragment.
func TestEpicLooksByClass(t *testing.T) {
	byName := map[string]auraLook{}
	for _, l := range auraLooks {
		byName[strings.Join(l.Words, " ")] = l
	}
	classes := []string{"shadowknight", "paladin", "warrior", "druid", "cleric", "shaman", "bard", "ranger", "monk", "rogue",
		"enchanter", "wizard", "magician", "necromancer"}
	for _, c := range classes {
		l, ok := byName["epic "+c]
		if !ok || l.A.Kind != database.AuraEpic || !l.Hands || l.A != byName["epic "+c+"1"].A {
			t.Errorf("epic %s: %+v %v", c, l, ok)
		}
	}
	if len(epicLooks) != 35 {
		t.Fatalf("%d epic models", len(epicLooks))
	}
	n := 0
	for name := range byName {
		if strings.HasPrefix(name, "epic ") {
			n++
		}
	}
	if n != 35+len(classes) {
		t.Fatalf("%d epic looks, want %d", n, 35+len(classes))
	}
	if byName["epic druid2"].A.Source != 0xCBF4135A || byName["epic rogue3"].A.Source != 0xBD3C3DC8 {
		t.Fatal("the numbering is not the disc's order")
	}
	for _, line := range auraLooksReply() {
		if len("[GM] "+line)*2+6 > 1156 {
			t.Errorf("a line past one fragment: %q", line)
		}
	}
}
