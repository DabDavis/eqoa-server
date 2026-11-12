package entity

import (
	"encoding/binary"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

func withWispRace(t *testing.T) {
	withOptionRaces(t,
		database.RaceDefRow{ID: 30, Name: "Jal'Raeth", WireRace: 8, BodyMale: signed(bodyJalRaeth), BodyFemale: signed(bodyJalRaeth)},
		database.RaceDefRow{ID: 31, Name: "Wisp", WireRace: 0, BodyMale: signed(bodyHumanMale), BodyFemale: signed(bodyHumanFemale)})
}

// a geared player: a sword, a shield, plate on every slot, a robe
func geared(race int) *Player {
	p := &Player{Name: "Glim", Race: race, ModelID: signed(bodyHumanMale), Size: 1,
		Primary: 0x11111111, Secondary: 0x22222222, Shield: 0x33333333, PrimaryType: 1, SecondaryType: 2,
		Chest: 3, Bracer: 3, Gloves: 3, Legs: 3, Boots: 3, Helm: 3, RobeWorn: true, Robe: 4, RobeTint: 5}
	for i := range p.ArmorColors {
		p.ArmorColors[i] = 0xFF0000FF
	}
	return p
}

// A Wisp in its own form is sent with nothing worn (held items, armour, tints,
// robe), its attack types kept; a Wisp under an illusion (a glamour) and a human
// (the control) in all of it. The character list's gear goes bare the same way.
func TestAWispInItsOwnFormShowsNothingWorn(t *testing.T) {
	withWispRace(t)
	if !IsWispRace(31) || IsWispRace(30) || IsWispRace(0) {
		t.Fatal("the Wisp is race 31 alone")
	}
	worn := func(b []byte) bool {
		return binary.LittleEndian.Uint32(b[67:71]) != 0 && binary.LittleEndian.Uint32(b[75:79]) != 0 &&
			b[93] != 0 && b[98] != 0 && b[109] != 0xFF
	}
	bare := func(b []byte) bool {
		for i := 67; i < 79; i++ {
			if b[i] != 0 {
				return false
			}
		}
		for i := 93; i < 105; i++ {
			if b[i] != 0 {
				return false
			}
		}
		return b[109] == 0xFF && b[110] == 0
	}

	w := geared(31)
	if !w.WispForm() || w.wispMortal() {
		t.Fatal("a Wisp in its own body is in its own form")
	}
	b := SerializePlayerUpdate(7, w)
	if !bare(b) || b[91] != 1 || b[92] != 2 {
		t.Errorf("a Wisp in its own form: % x (attack types %d %d)", b[67:111], b[91], b[92])
	}
	if string(b[116:120]) != "Glim" || binary.LittleEndian.Uint32(b[28:32]) != bodyHumanMale {
		t.Error("the rest of the channel is as ever")
	}
	if w.Primary == 0 || w.Chest == 0 {
		t.Error("the Wisp's own gear is untouched")
	}

	w.Effects = []*ActiveEffect{{SpellID: 496, Name: "Glamour: Human", Form: bodyHumanMale}}
	if w.WispForm() || !w.wispMortal() {
		t.Fatal("a glamoured Wisp is in a mortal form")
	}
	if b := SerializePlayerUpdate(7, w); !worn(b) {
		t.Errorf("a glamoured Wisp shows its gear: % x", b[67:111])
	}
	w.Effects = nil
	w.GMForm = 0x12345678
	if w.WispForm() {
		t.Error("a GM's Change Sprite is another body")
	}

	if b := SerializePlayerUpdate(7, geared(0)); !worn(b) {
		t.Errorf("the control, a human: % x", b[67:111])
	}

	g := make([]byte, 32)
	for i := range g {
		g[i] = 7
	}
	g[17], g[18] = 1, 2
	HideCharacterGear(g)
	if binary.LittleEndian.Uint32(g[0:4]) != 0xFFFFFFFF || g[4] != 0 || g[5] != 0 || g[16] != 0 || g[17] != 1 || g[18] != 2 ||
		g[20] != 0xFF || g[25] != 0xFF || g[26] != 0 || g[31] != 0 {
		t.Errorf("the character list's gear, bare: % x", g)
	}
}

// The looks leave a mortal-form Wisp's wisp out (a glamour of a human is the
// Wisp's own model, so a client could not tell otherwise); every other Wisp
// keeps it. The tick notes the forms and the looks are made again from the
// characters last read, and only on a change.
func TestAMortalWispLosesItsWispFromTheLooks(t *testing.T) {
	withWispRace(t)
	looks := []database.CharLook{{Part: "wisp", Kind: database.LookTexture, Value: bodyWillOWisp}}
	chars := []database.LookCharacter{
		{Name: "Glim", Race: 31, ModelID: signed(bodyHumanMale), Looks: looks},
		{Name: "Wil", Race: 31, ModelID: signed(bodyHumanMale), Looks: looks},
	}
	all := encodeRaceLooks(chars, nil)
	if string(all) != string(EncodeRaceLooks(chars)) || binary.LittleEndian.Uint16(all[1:3]) != 2 {
		t.Fatalf("both Wisps: % x", all)
	}
	one := encodeRaceLooks(chars, map[string]bool{"Glim": true})
	if binary.LittleEndian.Uint16(one[1:3]) != 1 || string(one[4:7]) != "Wil" {
		t.Errorf("Glim in a mortal form is left out: % x", one)
	}

	w := NewWorld()
	w.noteWispFormsLocked(nil)
	if w.wispLooks.key != "" || w.wispLooks.mortalNames()["Glim"] {
		t.Fatal("no Wisp in a mortal form")
	}
	w.noteWispFormsLocked([]string{"Wil", "Glim"})
	if m := w.wispLooks.mortalNames(); !m["Glim"] || !m["Wil"] || len(m) != 2 {
		t.Errorf("both mortal: %v", m)
	}
	w.noteWispFormsLocked([]string{"Glim", "Wil"})
	if m := w.wispLooks.mortalNames(); len(m) != 2 {
		t.Errorf("the same set, unchanged: %v", m)
	}
	w.noteWispFormsLocked(nil)
	if len(w.wispLooks.mortalNames()) != 0 {
		t.Error("the glamours ended: every wisp back")
	}
}

// The Wisp's rows as tools/wisp_kit_20261111.py writes them parse to their
// figures and are listed for race 31 alone (its bit, 1 << 31), any class; the
// control: the human (race 0), whose mask is 1, and the Jal'Raeth (30).
func TestTheWispsMasteriesAreTheWisps(t *testing.T) {
	withWispRace(t)
	row := func(id, cat int, effects string) database.CMRankRow {
		return database.CMRankRow{ID: id, Category: cat, Classes: 32767, Races: 1 << 31, Effects: effects, Enabled: true}
	}
	will := ParseCMRank(row(2141, 1, "PoT:10"))
	mc := ParseCMRank(row(2143, CMCategoryMaster, "DEXMax:50,INTMax:25,WISMax:25,Pow:500,STRMax:-25,STAMax:-25,AGIMax:-25,AC:-250"))
	if will.Effects[cmSlotPoT] != 10 {
		t.Errorf("Will-of-the-Wisp: %v", will.Effects)
	}
	e := mc.Effects
	if e[10] != 50 || e[12] != 25 || e[11] != 25 || e[cmSlotPower] != 500 || e[7] != -25 || e[8] != -25 || e[9] != -25 || e[cmSlotAC] != -250 {
		t.Errorf("Transcendent: %v", e)
	}
	for class := 0; class < 15; class++ {
		if !will.OfferedTo(class, 31) || !mc.OfferedTo(class, 31) {
			t.Errorf("a Wisp of class %d is not offered them", class)
		}
	}
	if will.OfferedTo(0, 0) || will.OfferedTo(0, 30) || mc.OfferedTo(13, 0) {
		t.Error("the controls: a human or a Jal'Raeth is offered the Wisp's")
	}
}

// The Wisp's rows in a World: a Wisp of level 60 is listed Will-of-the-Wisp,
// Trickster Flame and Transcendent's three ranks, buys the racials (Trickster
// Flame giving the six glamours and Remove Illusion) and Transcendent once 500
// points are spent, its PoT, power and AC then the ranks'. The control: a human
// is listed none of them and refused one.
func TestAWispBuysItsMasteries(t *testing.T) {
	withWispRace(t)
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	wisp, all := 1<<31, 32767
	w.SetCMCatalog([]database.CMRankRow{
		{ID: 1701, Category: 1, Classes: all, Races: 1, Name: "Retreat", Cost: 15, Level: 6, Enabled: true},
		{ID: 2141, Category: 1, Classes: all, Races: wisp, Mastery: "Will-of-the-Wisp", Name: "Will-of-the-Wisp", Cost: 5, Level: 6, Effects: "PoT:10", Enabled: true},
		{ID: 2142, Category: 1, Classes: all, Races: wisp, Mastery: "Trickster Flame", Name: "Trickster Flame", Cost: 10, Level: 6, Effects: "PoT:5",
			Adds:      "Glamour: Human@1; Glamour: Elf@4; Glamour: Erudite@8; Glamour: Halfling@12; Glamour: Dwarf@16; Glamour: Gnome@20; Remove Illusion@1",
			AddSpells: "496,503,507,517,524,531,1793", Enabled: true},
		{ID: 2143, Category: CMCategoryMaster, Classes: all, Races: wisp, Mastery: "Transcendent", Rank: 1, Name: "Transcendent", Cost: 200, Level: 55, Spent: 500,
			Effects: "DEXMax:50,INTMax:25,WISMax:25,Pow:500,STRMax:-25,STAMax:-25,AGIMax:-25,AC:-250", Enabled: true},
		{ID: 2144, Category: CMCategoryMaster, Classes: all, Races: wisp, Mastery: "Transcendent", Rank: 2, Name: "Unbound Will", Parent: 2143, Cost: 75, Level: 60, Effects: "Pow:500,PoT:20", Enabled: true},
		{ID: 2145, Category: CMCategoryMaster, Classes: all, Races: wisp, Mastery: "Transcendent", Rank: 3, Name: "Pale Fire", Parent: 2143, Cost: 75, Level: 60, Effects: "INT:25,WIS:25", Enabled: true},
	})
	in := func(port, id int, p *Player) *session.Session {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: port}, uint32(id), uint32(id), 1, 2, true)
		w.Players[p.ServerID] = p
		s.SetECSEntity(p)
		s.SetCharacterInWorld(true)
		return s
	}
	f := &Player{ServerID: 677, CharServerID: 677, Name: "Flicker", Race: 31, Class: 13, Level: 60, Mastery: Mastery{Points: 1000}}
	h := &Player{ServerID: 678, CharServerID: 678, Name: "Hum", Race: 0, Class: 13, Level: 60, Mastery: Mastery{Points: 1000}}
	fs, hs := in(7201, 677, f), in(7202, 678, h)
	ids := func(l []CMEntry) []int {
		var out []int
		for _, e := range l {
			out = append(out, e.ID)
		}
		return out
	}
	if got := ids(w.CMList(fs, CMKnows{})); !slices.Equal(got, []int{2141, 2142, 2143, 2144, 2145}) {
		t.Errorf("the Wisp's list: %v", got)
	}
	if got := ids(w.CMList(hs, CMKnows{})); !slices.Equal(got, []int{1701}) {
		t.Errorf("the control, a human's list: %v", got)
	}
	if r := w.BuyCM(hs, 2141, CMKnows{}); r.Refusal == "" {
		t.Error("a human bought Will-of-the-Wisp")
	}
	for _, id := range []int{2141, 2142} {
		if r := w.BuyCM(fs, id, CMKnows{}); r.Refusal != "" {
			t.Fatalf("%d: %s", id, r.Refusal)
		}
	}
	if r := w.BuyCM(fs, 2143, CMKnows{}); r.Refusal == "" {
		t.Error("Transcendent with 15 points spent")
	}
	f.Mastery.Spent = 500
	r := w.BuyCM(fs, 2143, CMKnows{})
	if r.Refusal != "" {
		t.Fatal(r.Refusal)
	}
	if gives, _ := w.cms.byID[2142].BookChange(); len(gives) != 7 || gives[0].SpellID != 496 || gives[6].Name != "Remove Illusion" {
		t.Errorf("Trickster Flame gives %v", gives)
	}
	if e := f.CMEffects; e[cmSlotPoT] != 15 || e[cmSlotPower] != 500 || e[cmSlotAC] != -250 || e[10] != 50 || e[7] != -25 {
		t.Errorf("the Wisp's effects: %v", e)
	}
}

// raimentJSON is the Wisp's Raiment's workbook record as
// tools/wisp_raiment_20261111.py writes it.
const raimentJSON = `{"v":1,"scope":"self","target_type":"Self","harmful":false,"duration_s":3600,"stack":0,"level":1,` +
	`"min_level":1,"req_equipment":"Any","sheet_type":"Alter Appearance","effects":[{"id":"7E00","type":"Wisp's Raiment","kind":"raiment"}]}`

// The Wisp's Raiment (Trickster Flame's; the owner, 2026-11-11: "there isn't a
// form that shows the wisp with all the gear as an orb") shows a Wisp in its
// own form in all it wears, its wisp kept in the race looks (no mortal form);
// it ends with Remove Illusion, a glamour cast over it takes its place, it is
// kept over a logout, and it ends at its hour. The control: before it the Wisp
// shows nothing worn, and on a human it changes nothing.
func TestTheWispsRaimentShowsItsGear(t *testing.T) {
	withWispRace(t)
	c := illusionRig(t)
	g := geared(31)
	c.p.Race, c.p.ModelID, c.p.Name = 31, g.ModelID, "Glim"
	c.p.Primary, c.p.Secondary, c.p.Shield, c.p.PrimaryType, c.p.SecondaryType = g.Primary, g.Secondary, g.Shield, 1, 2
	c.p.Chest, c.p.Bracer, c.p.Gloves, c.p.Legs, c.p.Boots, c.p.Helm = 3, 3, 3, 3, 3, 3
	c.p.RobeWorn, c.p.Robe = true, 4
	c.w.SetRaceModels(map[[2]int]uint32{{0, 0}: bodyHumanMale, {0, 1}: bodyHumanFemale})
	shows := func(p *Player) bool {
		b := SerializePlayerUpdate(7, p)
		return binary.LittleEndian.Uint32(b[67:71]) != 0 && b[93] != 0 && b[109] != 0xFF
	}
	if shows(c.p) || !c.p.WispForm() {
		t.Fatal("the control: a Wisp in its own form shows nothing worn")
	}
	raiment := illusionSpell(9100, "Wisp's Raiment", "", "")
	raiment.Def = SpellDefFrom("Wisp's Raiment", "Self, Shows Gear, 60min", "", raimentJSON)
	raiment.Scope = 0
	if raiment.Def.Target != TargetSelf || raiment.Def.Duration != time.Hour || raiment.Def.FormKind != FormNone {
		t.Fatalf("the record reads as %+v", raiment.Def)
	}
	c.land(t, raiment, 0xFFFFFFFF, 0)
	if !shows(c.p) || c.p.WispForm() || c.p.wispMortal() {
		t.Fatalf("under its Raiment: gear %v, own form %v, mortal %v; want its gear, its wisp kept", shows(c.p), c.p.WispForm(), c.p.wispMortal())
	}
	if m, _ := channelBody(c.p); m != bodyHumanMale {
		t.Errorf("its body %#x, want its own", m)
	}

	rows := savedEffects(c.p, c.at(30*time.Second))
	d := illusionRig(t)
	d.p.Race = 31
	if d.w.RestoreEffects(d.sess, rows, d.at(0)) != 1 || !d.p.inRaiment() {
		t.Fatalf("after a logout: %+v", d.p.Effects)
	}

	end := illusionSpell(1793, "Remove Illusion", "", "This ability removes the current illusion on you.")
	end.Scope = 0
	c.land(t, end, 0xFFFFFFFF, 20*time.Second)
	if shows(c.p) || !c.p.WispForm() || len(c.p.Effects) != 0 {
		t.Fatalf("after Remove Illusion: gear %v, effects %+v; want its own form", shows(c.p), c.p.Effects)
	}

	c.land(t, raiment, 0xFFFFFFFF, 40*time.Second)
	glamour := illusionSpell(496, "Glamour: Human", "Target, Faction Change, Increase AC by 50, Gives Form, 60min", "CM: Illusion and faction change.")
	c.land(t, glamour, 0xFFFFFFFF, 60*time.Second)
	if c.p.inRaiment() || !c.p.wispMortal() || !shows(c.p) {
		t.Fatalf("a glamour over the Raiment: raiment %v, mortal %v; want the glamour alone", c.p.inRaiment(), c.p.wispMortal())
	}
	c.land(t, raiment, 0xFFFFFFFF, 80*time.Second)
	if !c.p.inRaiment() || c.p.wispMortal() {
		t.Fatalf("the Raiment over a glamour: raiment %v, mortal %v; want the Raiment alone", c.p.inRaiment(), c.p.wispMortal())
	}
	c.tick(80*time.Second + 5*time.Second + castLand + time.Hour)
	if shows(c.p) || !c.p.WispForm() {
		t.Fatal("after its hour: its own form, nothing worn")
	}

	h := illusionRig(t)
	h.p.Race, h.p.Chest, h.p.Primary = 0, 3, g.Primary
	h.land(t, raiment, 0xFFFFFFFF, 0)
	if !shows(h.p) && SerializePlayerUpdate(7, h.p)[93] == 0 {
		t.Error("the control: a human under it shows as ever")
	}
}

// The Wisp's colour (the owner, 2026-11-12: "also add color customize to wisp
// char sel"): a colour row on the wisp's own part of both bodies, its first step
// the wisp's own; select's Customize re-chooses it (SelectRows, ReselectOptions)
// with the wisp kept, and nothing else of the Wisp's; a mortal-form Wisp's looks
// leave it out with the wisp. The controls: the Jal'Raeth (no row re-chosen at
// select), a game race's skin only with the shades on.
func TestTheWispsColour(t *testing.T) {
	withWispRace(t)
	withSkins(t, false)
	var row *OptionRow
	rows := RaceOptions(31)
	for i := range rows {
		if rows[i].Key == wispColourKey {
			row = &rows[i]
		}
	}
	if row == nil || row.Kind != KindColour || !row.AtSelect || row.Steps(0)[0].Value != 0 || len(row.Steps(1)) < 8 {
		t.Fatalf("the colour row: %+v", row)
	}
	for _, b := range []uint32{bodyHumanMale, bodyHumanFemale} {
		if len(row.PartsOf(b)) != 1 {
			t.Errorf("body %08X: parts %+v", b, row.PartsOf(b))
		}
	}
	if got := SelectRows(31); len(got) != 1 || got[0] != wispColourKey {
		t.Errorf("the Wisp's rows at select: %v", got)
	}
	if got := SelectRows(30); len(got) != 0 {
		t.Errorf("the control, the Jal'Raeth: %v", got)
	}
	if got := SelectRows(8); len(got) != 0 {
		t.Errorf("the control, an erudite without the shades: %v", got)
	}
	withSkins(t, true)
	if got := SelectRows(8); len(got) != 1 || got[0] != "skin" {
		t.Errorf("an erudite with the shades: %v", got)
	}

	wisp := database.CharLook{Part: "wisp", Kind: database.LookTexture, Value: bodyWillOWisp}
	looks, changed, err := ReselectOptions(31, 1, []database.CharLook{wisp}, []OptionChoice{{Key: wispColourKey, Step: 3}})
	if err != nil || len(changed) != 1 || len(looks) != 2 || looks[0] != wisp || looks[1].Part != wispColourKey || looks[1].Value != wispColours[3].Value {
		t.Fatalf("Amber: %+v %v %v", looks, changed, err)
	}
	if back, _, err := ReselectOptions(31, 1, looks, []OptionChoice{{Key: wispColourKey, Step: 0}}); err != nil || len(back) != 1 || back[0] != wisp {
		t.Errorf("Natural again: %+v %v", back, err)
	}
	for name, c := range map[string][]OptionChoice{
		"the wisp": {{Key: "wisp", Step: 0}},
		"the body": {{Key: "body", Step: 0}},
		"too far":  {{Key: wispColourKey, Step: len(wispColours)}},
	} {
		if _, _, err := ReselectOptions(31, 0, []database.CharLook{wisp}, c); err == nil {
			t.Errorf("%s: kept", name)
		}
	}
	if _, _, err := ReselectOptions(30, 0, nil, []OptionChoice{{Key: "body", Step: 1}}); err == nil {
		t.Error("the control: the Jal'Raeth's body re-chosen at select")
	}

	// the looks: the colour on the wisp's part, left out with the wisp in a mortal form
	chars := []database.LookCharacter{{Name: "Glim", Race: 31, Sex: 1, ModelID: signed(bodyHumanFemale), Looks: looks}}
	k, v, p := lookParts(31, bodyHumanFemale, looks)
	if len(p) != 2 || k[1] != KindColour || v[1] != wispColours[3].Value || p[1].Entries[0] != 7 || p[0].Entries[0] != 7 {
		t.Errorf("her look's parts: %v %x %+v", k, v, p)
	}
	if one := encodeRaceLooks(chars, map[string]bool{"Glim": true}); binary.LittleEndian.Uint16(one[1:3]) != 0 {
		t.Errorf("a mortal-form Wisp keeps its colour: % x", one)
	}
}
