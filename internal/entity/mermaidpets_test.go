package entity

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// The Mermaid's pets (tools/mermaid_pets_20261113.py, Zauz's math, the owner
// 2026-11-13): Aquatic Birthright 1-5, racials of 15 CMs at levels 15 to 55,
// each after the one before and each replacing the one before's summoning in
// the book; and Aquatic Ascension, Siren Royal's fourth rank (50 CMs, level
// 60), which needs every Birthright, the catalog's own require_ranks.

// mermaidPetRanks are the ranks as the tool writes them (its test reads this
// table): id|category|mastery|rank|name|parent|cost|level|adds|add_spells|
// removes|remove_spells|require_ranks. 2174 is the kit's Siren Royal.
const mermaidPetRanks = `2174|4|Siren Royal|1|Siren Royal|0|200|55|||||
2177|1|Aquatic Birthright|1|Aquatic Birthright 1|0|15|15|Water Elementaling@15|760180|||
2178|1|Aquatic Birthright|2|Aquatic Birthright 2|2177|15|25|Water Elementalkin@25|760181|Water Elementaling@15|760180|
2179|1|Aquatic Birthright|3|Aquatic Birthright 3|2178|15|35|Water Elementalkin (29)@35|760182|Water Elementalkin@25|760181|
2180|1|Aquatic Birthright|4|Aquatic Birthright 4|2179|15|45|Water Elemental (34)@45|760183|Water Elementalkin (29)@35|760182|
2181|1|Aquatic Birthright|5|Aquatic Birthright 5|2180|15|55|Summon Ghilan@55|760184|Water Elemental (34)@45|760183|
2182|4|Siren Royal|4|Aquatic Ascension|2174|50|60|Aquatic Ascension@60|760179|Summon Ghilan@55|760184|2177,2178,2179,2180,2181`

func mermaidPetCatalog(t *testing.T) []database.CMRankRow {
	t.Helper()
	var rows []database.CMRankRow
	for _, line := range strings.Split(mermaidPetRanks, "\n") {
		f := strings.Split(line, "|")
		if len(f) != 13 {
			t.Fatalf("rank line %q: %d fields", line, len(f))
		}
		n := func(s string) int {
			v, err := strconv.Atoi(s)
			if err != nil {
				t.Fatalf("rank line %q: %v", line, err)
			}
			return v
		}
		r := database.CMRankRow{ID: n(f[0]), Category: n(f[1]), Classes: 32767, Races: 1 << 32, Mastery: f[2], Rank: n(f[3]),
			Name: f[4], Parent: n(f[5]), Cost: n(f[6]), Level: n(f[7]), Adds: f[8], AddSpells: f[9], Removes: f[10],
			RemoveSpells: f[11], RequireRanks: f[12], Enabled: true}
		if r.ID == 2174 {
			r.Spent = 500 // every Master Class's rank 1
		}
		rows = append(rows, r)
	}
	return rows
}

func abilityIDs(as []CMAbility) []int {
	var out []int
	for _, a := range as {
		out = append(out, a.SpellID)
	}
	return out
}

// Bought in order, by any class of Mermaid and no other race: each rank at
// its level, after the one before; the Ascension after Siren Royal and all
// five; a Birthright not removed while the Ascension needs it; each rank's
// book change its summoning in and the one before's out (a removal the
// reverse).
func TestMermaidPetRanks(t *testing.T) {
	rows := mermaidPetCatalog(t)
	for _, r := range rows {
		rk := ParseCMRank(r)
		for c := 0; c <= 14; c++ {
			if !rk.OfferedTo(c, 32) {
				t.Errorf("rank %d not offered to a Mermaid of class %d", r.ID, c)
			}
		}
		if rk.OfferedTo(0, 31) || rk.OfferedTo(10, 8) || rk.OfferedTo(0, 0) {
			t.Errorf("rank %d offered to another race", r.ID)
		}
	}
	w := NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.SetCMCatalog(rows)
	s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 8), Port: 7113}, 1, 1, 1, 2, true)
	p := &Player{ServerID: 1002, CharServerID: 1002, Name: "Merrow", Race: 32, Class: 0, Level: 20,
		Mastery: Mastery{Points: 1000, Spent: 500}}
	w.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	buy := func(id int, want string) {
		t.Helper()
		if got := w.BuyCM(s, id, CMKnows{}).Refusal; got != want {
			t.Errorf("buying %d at level %d: %q, want %q", id, p.Level, got, want)
		}
	}
	buy(2178, "You must have Aquatic Birthright 1 first.")
	buy(2177, "")
	buy(2178, "You must be level 25 to buy Aquatic Birthright 2.")
	p.Level = 60
	buy(2179, "You must have Aquatic Birthright 2 first.")
	buy(2182, "You must have Siren Royal first.")
	buy(2174, "")
	buy(2182, "You must have Aquatic Birthright 2 first.")
	for _, id := range []int{2178, 2179, 2180} {
		buy(id, "")
	}
	buy(2182, "You must have Aquatic Birthright 5 first.")
	buy(2181, "")
	before := p.Mastery.Points
	buy(2182, "")
	if paid := before - p.Mastery.Points; paid != 50 {
		t.Errorf("Aquatic Ascension cost %d, want 50", paid)
	}
	if r := w.RemoveCM(s, 2181); r.Refusal != "Remove Aquatic Ascension before Aquatic Birthright 5." {
		t.Errorf("removing Birthright 5 under the Ascension: %q", r.Refusal)
	}
	for id, want := range map[int][2]string{2177: {"[760180]", "[]"}, 2178: {"[760181]", "[760180]"},
		2181: {"[760184]", "[760183]"}, 2182: {"[760179]", "[760184]"}} {
		gives, takes := w.cms.byID[id].BookChange()
		if g, k := fmtInts(abilityIDs(gives)), fmtInts(abilityIDs(takes)); g != want[0] || k != want[1] {
			t.Errorf("rank %d gives %s, takes %s; want %s, %s", id, g, k, want[0], want[1])
		}
	}
	r := w.RemoveCM(s, 2182)
	if r.Refusal != "" || fmtInts(abilityIDs(r.GiveBack)) != "[760184]" || fmtInts(abilityIDs(r.TakeOut)) != "[760179]" {
		t.Errorf("removing the Ascension: %q, gives back %v, takes out %v; want Summon Ghilan back, Aquatic Ascension out",
			r.Refusal, abilityIDs(r.GiveBack), abilityIDs(r.TakeOut))
	}
}

func fmtInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = strconv.Itoa(n)
	}
	return "[" + strings.Join(s, " ") + "]"
}

// mermaidPetRig is the pet rig with the tool's copies' workbook rows (their
// numbers the live rows' of the magician's spells copied), their pets' spells
// and the Ascension's template row.
func mermaidPetRig(t *testing.T) *petRig {
	t.Helper()
	r, db := newPetTemplateRig(t, true)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, s := range []struct {
		id, level, cast int
		name, line      string
	}{
		{902087, 4, 3, "Water Heal Level 4", "Target, Increase Hitpoints by 140"},
		{902088, 4, 2, "Water Nuke Level 4", "Target, Decrease Hitpoints by 32"},
		{902139, 43, 3, "Water Heal Level 43", "Target, Increase Hitpoints by 1505"},
		{902140, 43, 2, "Water Nuke Level 43", "Target, Decrease Hitpoints by 344"},
		{902153, 50, 3, "Water Heal Level 50", "Target, Increase Hitpoints by 1750"},
		{902154, 50, 2, "Water Nuke Level 50", "Target, Decrease Hitpoints by 400"},
	} {
		exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, ?, 0, 0, 0, 1, 12, -1, ?, '')`, s.id, s.level, s.cast, s.name)
		exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, 0, 'test')`, s.id, s.line)
	}
	kit := func(heal, nuke int) string {
		return `[{"every":30.0,"first":1.0,"kind":"heal","spell_id":` + strconv.Itoa(heal) + `,"target":"either","below":60},` +
			`{"every":12.0,"first":3.0,"kind":"nuke","spell_id":` + strconv.Itoa(nuke) + `}]`
	}
	pets := `INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
		intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max, int_max,
		cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, abilities, size, source) VALUES `
	exec(pets+`(760180, '0019', 'Water Elementaling', 3876934402, 4, 'priest', 'water', 160, 78, 82, 74, 66, 90, 66, 86, 89,
		165, 2, 4, 29, 32, 29, 29, 29, 29, 90, 90, 90, 90, 90, 90, 90, 8, 0, 1, 28, 'blunt', ?, 0, 'test')`, kit(902087, 902088))
	exec(pets+`(760184, '0033', 'Ghilan', 1689387666, 43, 'priest', 'water', 1720, 351, 394, 308, 222, 480, 222, 437, 3002,
		4207, 67, 93, 58, 90, 58, 58, 58, 58, 480, 480, 480, 480, 480, 480, 480, 34, 0, 3, 301, 'blunt', ?, 2, 'test')`,
		kit(902139, 902140))
	exec(pets+`(760179, '003A', 'Ghilan', 1689387666, 51, 'priest', 'water', 2000, 400, 450, 350, 250, 550, 250, 500, 3945,
		5400, 87, 119, 63, 100, 63, 63, 63, 63, 550, 550, 550, 550, 550, 550, 550, 38, 0, 4, 350, 'blunt', ?, 2, 'test')`,
		kit(902153, 902154))
	exec(`INSERT INTO pettemplate (spell, weapon, template, swing_class, swing, round_ms, note, source)
		VALUES ('aquatic ascension', 0, 'pet water', -1, -1, 0, 'test', 'test')`)
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.p.Race = 32
	return r
}

var (
	mermaidWaterElementaling = summonSpell(760180, 15, "Water Elementaling", "Summons a water elementaling to aid you")
	mermaidSummonGhilan      = summonSpell(760184, 55, "Summon Ghilan", "Summons a Ghilan to aid you")
	mermaidAquaticAscension  = summonSpell(760179, 60, "Aquatic Ascension", "Summons a Ghilan to aid you")
)

// Every class of Mermaid summons each pet, of its row's level and hit points
// and body, fighting as the magician's water pet; a second summoning
// dismisses the first (one pet at a time: it dies away, petition 1398). Water Elementaling's body no NPC
// wears and none has its name: before, only a class with a stand-in (the
// magician's elementaling, a necromancer's skeleton) got a pet of it, a
// warrior none.
func TestMermaidPetsAnyClass(t *testing.T) {
	for class := 0; class <= 14; class++ {
		r := mermaidPetRig(t)
		r.p.Class, r.p.Level = class, 15
		pet, _ := r.summon(t, mermaidWaterElementaling, 0)
		if pet.Level != 4 || pet.MaxHP != 89 || uint32(pet.ModelID) != 3876934402 || pet.Name != "Water Elementaling" {
			t.Errorf("class %d: Water Elementaling is %q level %d, %d HP, model %d; want level 4, 89 HP, model 3876934402",
				class, pet.Name, pet.Level, pet.MaxHP, uint32(pet.ModelID))
		}
		r.p.Level = 60
		ghilan, _ := r.summon(t, mermaidSummonGhilan, 10*time.Second)
		if ghilan.Level != 43 || ghilan.MaxHP != 3002 || !pet.Dead {
			t.Errorf("class %d: Summon Ghilan is level %d, %d HP, the elementaling dismissed %v; want 43, 3002, dismissed",
				class, ghilan.Level, ghilan.MaxHP, pet.Dead)
		}
		asc, _ := r.summon(t, mermaidAquaticAscension, 30*time.Second)
		ps := r.w.pets[r.p.ServerID]
		if asc.Level != 51 || asc.MaxHP != 3945 || uint32(asc.ModelID) != 1689387666 || !ghilan.Dead {
			t.Errorf("class %d: Aquatic Ascension is level %d, %d HP, model %d, the first Ghilan dismissed %v; want 51, 3945, "+
				"the Ghilan's, dismissed", class, asc.Level, asc.MaxHP, uint32(asc.ModelID), ghilan.Dead)
		}
		if ps == nil || ps.tmpl == nil || !strings.HasPrefix(ps.tmpl.tmpl.Name, "pet water") {
			t.Errorf("class %d: Aquatic Ascension's pet fights by %v, want the magician's water pet's template", class, ps)
		}
		owned := 0
		for _, n := range r.w.NPCs {
			if n.Owner == r.p.ServerID && !n.Dead {
				owned++
			}
		}
		if owned != 1 || len(r.w.pets) != 1 {
			t.Errorf("class %d: %d pets alive, %d kept, want one", class, owned, len(r.w.pets))
		}
	}
}
