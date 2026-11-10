package entity

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The magician's level 60 pets as the magician Manifest's petition 1450 asks
// (the owner: "apply as he wrote"), as tools/fix_mage_pets_1450.py writes them:
// the Djinni Vizier 30% larger, its three lightning spells at 1, 3 and 5 s into
// a fight and every 12 s after (1000, 1300, and 1700/1300 round its target);
// the Efreeti Lord's quad of 600 a hit on a creature of its owner's level and
// its 2000 fire nuke every 30 s; the Badi Elder's taunt every 15 s, its crit of
// 2000 every 30 s (its template's timed special, which a workbook pet takes
// since this petition) and its quad near 500 a hit with the magician's epic
// line on it; the Ghilan Noble's cold nukes of 1000 and 1300, 3 s apart every
// 30 s, and its heals of 1500 (at once, every 60 s, below half) and 800 (every
// 30 s, below 60%) on its owner or itself. The visuals here are made up; the
// rest is the tool's and the live rows'.

const (
	fx1450Bolt     uint32 = 0x0B145001
	fx1450Clap     uint32 = 0x0B145002
	fx1450Tempest  uint32 = 0x0B145003
	fx1450Inferno  uint32 = 0x0B145004
	fx1450Chill    uint32 = 0x0B145005
	fx1450Frost    uint32 = 0x0B145006
	fx1450Mending  uint32 = 0x0B145007
	fx1450Hands    uint32 = 0x0B145008
	fx1450Grace    uint32 = 0x0B145009
	fx1450Fire     uint32 = 0x0B14500A
	fx1450Durable  uint32 = 0x0B14500B
	fx1450Blessing uint32 = 0x0B14500C
	fx1450Tide     uint32 = 0x0B14500D
	fx1450Breeding uint32 = 0x0B14500E
)

// The kits as the tool writes them.
const (
	vizierKit1450 = `[{"every":12.0,"first":1.0,"kind":"nuke","spell_id":905450},` +
		`{"every":12.0,"first":3.0,"kind":"nuke","spell_id":905451},{"every":12.0,"first":5.0,"kind":"aoe","spell_id":905452},` +
		`{"every":1800.0,"first":7.0,"kind":"buff","spell_id":902161,"target":"self"}]`
	efreetiKit1450 = `[{"every":30.0,"first":1.0,"kind":"nuke","spell_id":905453},` +
		`{"every":1800.0,"first":5.0,"kind":"buff","spell_id":902164,"target":"self"}]`
	badiKit1450 = `[{"every":15.0,"first":1.0,"kind":"taunt","hits":[[1,0,0]]},` +
		`{"every":1800.0,"first":5.0,"kind":"buff","spell_id":902157,"target":"self"}]`
	ghilanKit1450 = `[{"every":60.0,"first":1.0,"kind":"heal","spell_id":905457,"target":"either","below":50},` +
		`{"every":30.0,"first":1.0,"kind":"heal","spell_id":905456,"target":"either","below":60},` +
		`{"every":1800.0,"first":3.0,"kind":"buff","spell_id":902167,"target":"owner"},` +
		`{"every":30.0,"first":5.0,"kind":"nuke","spell_id":905454},{"every":30.0,"first":8.0,"kind":"nuke","spell_id":905455},` +
		`{"every":30.0,"first":8.0,"kind":"nuke","spell_id":905130}]`
)

var (
	summonDjinniVizier = summonSpell(900761, 60, "Summon Djinni Vizier (60)", "Summons a Djinni Vizier to aid you")
	summonEfreetiLord  = summonSpell(701087, 60, "Summon Efreeti Lord", "Summons an Efreeti Lord to aid you")
	summonBadiElder    = summonSpell(701086, 60, "Summon Badi Elder", "Summons a Badi Elder to aid you")
)

// sacrificialBreeding is the magician's epic line's pet buff (Magi`Kot's
// Sacrificial Breeding, the 60 epic's Mark path), its record and effects as
// the live server keeps them: INT and a 135 proc on its pet's melee hits.
var sacrificialBreeding = SpellOfRow(&database.SpellPatternRecord{SpellID: 701176, AbilityLvl: 60, Range: 16880, CastTime: 3,
	Power: 720, Scope: 3, Recast: 12, EqpRequire: -1, SpellName: "Magi`Kot's Sacrificial Breeding"}, 0,
	"Pet, Increase INT by 100, Decrease Hitpoints by 135, 10min",
	`{"v":1,"scope":"pet","target_type":"Friendly","harmful":false,"duration_s":600,"stack":0,"level":60,"min_level":1,`+
		`"req_equipment":"Any","sheet_type":"Statistic Change","arch_type":"Pet Proc","effects":[{"id":"038F","type":"Statistic `+
		`Change","kind":"stat","stat":"int","amount":100},{"id":"0390","type":"Offensive Proc","kind":"proc","procs":[{"id":"0D41",`+
		`"type":"Dmg/Heal","kind":"hp","amount":-135,"resist":"ar","on":"offense","scope":"target","target_type":"Enemy"}]}]}`,
	fx1450Breeding)

// magePetRig is the pet rig with petition 1450's spells (the tool's records),
// the ones the pets keep, the four summonings' workbook rows as the tool leaves
// them (the live rows' numbers), the Vizier's template row (pet air, as the pet
// import made it), the Badi Elder's template with the tool's crit (crit false:
// with none, as before the tool), and a level 60 magician.
func magePetRig(t *testing.T, crit bool) (*petRig, *database.SQLiteDB) {
	t.Helper()
	r, db := newPetTemplateRig(t, true)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	harm := func(n int, resist string) string {
		return fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,`+
			`"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":%d,"resist":%q}]}`, n, resist)
	}
	heal := func(n int) string {
		return fmt.Sprintf(`{"v":1,"scope":"target","target_type":"Owner/Self","harmful":false,"stack":0,"level":60,`+
			`"min_level":1,"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":%d}]}`, n)
	}
	tempest := `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"radius":15,` +
		`"req_equipment":"Any","effects":[{"type":"Dmg/Heal","kind":"hp","amount":-1700,"resist":"lr"},` +
		`{"type":"Dmg/Heal","kind":"hp2","amount":-1300,"resist":"lr"}]}`
	for _, s := range []struct {
		id, level, cast, recast int
		name, line, effects     string
		fx                      uint32
	}{
		{905450, 60, 0, 12, "Vizier's Bolt", "Target, Decrease Hitpoints by 1000", harm(-1000, "lr"), fx1450Bolt},
		{905451, 60, 0, 12, "Vizier's Thunderclap", "Target, Decrease Hitpoints by 1300", harm(-1300, "lr"), fx1450Clap},
		{905452, 60, 0, 12, "Vizier's Tempest", "Target, Decrease Hitpoints by 1700 aoe Decrease Hitpoints by 1300", tempest,
			fx1450Tempest},
		{905453, 60, 1, 30, "Efreeti's Inferno", "Target, Decrease Hitpoints by 2000", harm(-2000, "fr"), fx1450Inferno},
		{905454, 60, 2, 30, "Ghilan's Chill", "Target, Decrease Hitpoints by 1000", harm(-1000, "cr"), fx1450Chill},
		{905455, 60, 2, 30, "Ghilan's Frost Wave", "Target, Decrease Hitpoints by 1300", harm(-1300, "cr"), fx1450Frost},
		{905456, 60, 3, 30, "Ghilan's Mending", "Target, Increase Hitpoints by 800", heal(800), fx1450Mending},
		{905457, 60, 0, 60, "Ghilan's Lay Hands", "Target, Increase Hitpoints by 1500", heal(1500), fx1450Hands},
		// the spells the kits keep, as the live records' lines
		{902161, 1, 3, 1800, "Djinni's Grace", "Target, Increase AGI by 50, Increase LR by 50, 30min", "", fx1450Grace},
		{902164, 1, 3, 1800, "Efreeti's Fire", "Target, Increase DEX by 50, Increase FR by 50, 30min", "", fx1450Fire},
		{902157, 55, 1, 1800, "Badi's Durability", "Target, Increase STA by 50, Increase PR by 50, Increase DR by 50, 30min", "",
			fx1450Durable},
		{902167, 1, 3, 1800, "Ghilan's Blessing", "Target, Increase WIS by 50, Increase CR by 50, Increase AR by 50, Increase HP per tic by 130, 30min",
			"", fx1450Blessing},
		{905130, 60, 2, 30, "Purifying Tide", "Target undead, Decrease Hitpoints by 1500", "", fx1450Tide},
	} {
		exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, ?, 0, 0, 0, 1, ?, -1, ?, '')`, s.id, s.level, s.cast, s.recast,
			s.name)
		exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, ?, ?, 'test', ?)`, s.id, s.line,
			int64(s.fx), s.effects)
	}
	pets := `INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
		intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max, int_max,
		cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, size, source) VALUES `
	exec(pets+`(900761, '003C', 'Djinni Vizier', 3688505321, 60, 'caster', 'air', 1375, 435, 380, 380, 545, 270, 545, 215, 3440,
		6036, 76, 133, 55, 55, 96, 55, 55, 55, 600, 600, 600, 600, 600, 600, 600, 28, 0, 1, 330, 'blunt', 0, '', ?, 1.3, 'test')`,
		vizierKit1450)
	exec(pets+`(701087, '003D', 'Efreeti Lord', 2856885435, 60, 'melee', 'fire', 1650, 545, 435, 435, 545, 270, 380, 160, 4540,
		5733, 100, 127, 55, 55, 55, 96, 55, 55, 600, 600, 600, 600, 600, 600, 600, 43, 4, 1, 988, 'slash', 988, 'pierce', ?, 1.75,
		'test')`, efreetiKit1450)
	exec(pets+`(701086, '003B', 'Badi Elder', 515091180, 60, 'tank', 'earth', 2750, 490, 545, 435, 325, 380, 215, 215, 7070,
		5885, 156, 130, 59, 59, 59, 59, 79, 79, 600, 600, 600, 600, 600, 600, 600, 79, 2, 3, 881, 'blunt', 881, 'slash', ?, 0,
		'test')`, badiKit1450)
	exec(pets+`(701088, '003E', 'Ghilan Noble', 2430398935, 60, 'priest', 'water', 2200, 435, 490, 380, 270, 600, 270, 545,
		4650, 6338, 103, 140, 67, 100, 67, 67, 67, 67, 600, 600, 600, 600, 600, 600, 600, 40, 0, 4, 385, 'blunt', 0, '', ?, 2.0,
		'test')`, ghilanKit1450)
	exec(`INSERT INTO pettemplate (spell, weapon, template, swing_class, swing, round_ms, note, source)
		VALUES ('summon djinni vizier (60)', 0, 'pet air', -1, -1, 0, 'test', 'test')`)
	if crit {
		exec(`UPDATE classtemplates SET special = 'Critical Hit', special_damage = 33.3333, special_every = 30, special_first = 3
			WHERE name = 'pet badi elder'`)
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.p.Class, r.p.Level = 10, 60
	return r, db
}

// landedBy is how many of the casts begun at (ms, a cast of castMS each) have
// landed by last: castLand after their cast.
func landedBy(at []int, castMS, last int) int {
	n := 0
	for _, a := range at {
		if a+castMS+int(castLand/time.Millisecond) <= last {
			n++
		}
	}
	return n
}

// hitsOn is every figure the pet's hits took off the named creature (the
// pet's "hit" lines: melee, spells and specials alike).
func hitsOn(f *swarmFight, pet, target string) []int {
	re := regexp.MustCompile(regexp.QuoteMeta(fmt.Sprintf("%q hit %s for ", pet, target)) + `(\d+) \(`)
	var out []int
	for _, m := range re.FindAllStringSubmatch(f.lines, -1) {
		n, _ := strconv.Atoi(m[1])
		out = append(out, n)
	}
	return out
}

// checkCasts holds the pet's casts of a visual to the times and cast time
// wanted.
func checkCasts(t *testing.T, f *swarmFight, name string, fx uint32, castMS uint32, want []int) {
	t.Helper()
	at, ms := f.of(fx)
	if fmt.Sprint(at) != fmt.Sprint(want) {
		t.Errorf("%s cast at %v, want %v", name, at, want)
	}
	for _, m := range ms {
		if m != castMS {
			t.Errorf("%s's 0x00B1 with a cast of %d ms, want %d", name, m, castMS)
			break
		}
	}
}

const fight1450 = 112 * time.Second // the attack at 9.6 s, the clocks from 9.7

// lastOf1450 is the fight's last turn, in ms of the rig's clock.
const lastOf1450 = 9700 + int(fight1450/time.Millisecond)

// checkLines holds the counts of the fight's log lines holding each text.
func checkLines(t *testing.T, f *swarmFight, want map[string]int) {
	t.Helper()
	for line, n := range want {
		if got := f.logged(line); got != n {
			t.Errorf("%d lines %q, want %d", got, line, n)
		}
	}
}

// checkVizier1450 holds a Djinni Vizier's fight with the crab to its kit: its
// size 1.3; its bolt, thunderclap and tempest at 1, 3 and 5 s into the fight
// and every 12 s after, instant, landing 1000, 1300 and 3000 (the tempest's
// 1700 and its area's 1300 both on its target). It gives back when the
// tempest was cast.
func checkVizier1450(t *testing.T, f *swarmFight, bolt, clap, tempest uint32) []int {
	t.Helper()
	if f.pet.Name != "Djinni Vizier" || f.pet.Size < 1.2999 || f.pet.Size > 1.3001 {
		t.Errorf("the pet %q at %.3f: want the Djinni Vizier at 1.3", f.pet.Name, f.pet.Size)
	}
	bolts, claps, storms := steps(10700, 12000, lastOf1450), steps(12700, 12000, lastOf1450), steps(14700, 12000, lastOf1450)
	checkCasts(t, f, "the bolt", bolt, 0, bolts)
	checkCasts(t, f, "the thunderclap", clap, 0, claps)
	checkCasts(t, f, "the tempest", tempest, 0, storms)
	checkLines(t, f, map[string]int{
		`lands "Vizier's Bolt" on Desert Crab for 1000 before`:        landedBy(bolts, 0, lastOf1450),
		`lands "Vizier's Thunderclap" on Desert Crab for 1300 before`: landedBy(claps, 0, lastOf1450),
		`lands "Vizier's Tempest" on Desert Crab for 3000 before`:     landedBy(storms, 0, lastOf1450),
	})
	return storms
}

// checkEfreeti1450 holds an Efreeti Lord's fight with a level 60 crab to its
// kit: a quad (a warrior's Attack4) of 600 every hit, its inferno (a 1 s cast)
// at 1 s into the fight and every 30 s after, landing 2000; nothing else hits.
func checkEfreeti1450(t *testing.T, f *swarmFight, inferno uint32) {
	t.Helper()
	if f.pet.Name != "Efreeti Lord" || npcSwingKind(f.pet) != 5 {
		t.Fatalf("the pet %q, swing %d: want the Efreeti Lord's quad (5)", f.pet.Name, npcSwingKind(f.pet))
	}
	infernos := steps(10700, 30000, lastOf1450)
	checkCasts(t, f, "the inferno", inferno, 1000, infernos)
	checkLines(t, f, map[string]int{`lands "Efreeti's Inferno" on Desert Crab for 2000 before`: landedBy(infernos, 1000, lastOf1450)})
	melee, other := 0, map[int]int{}
	for _, n := range hitsOn(f, "Efreeti Lord", "Desert Crab") {
		switch n {
		case 600:
			melee++
		case 2000:
		default:
			other[n]++
		}
	}
	if melee < 60 || len(other) != 0 {
		t.Errorf("%d melee hits of 600 in %v, others %v: want every one 600", melee, fight1450, other)
	}
}

// checkBadi1450 holds a Badi Elder's fight with the crab (its own buff on it)
// to its kit: a quad; its taunt at 1 s into the fight and every 15 s; with its
// template's crit, a Critical Hit of 2000 at 3 s and every 30 s of its time at
// its foe, seen as a creature's special (none without).
func checkBadi1450(t *testing.T, r *petRig, f *swarmFight, crit bool) {
	t.Helper()
	ps := r.w.pets[r.p.ServerID]
	if f.pet.Name != "Badi Elder" || npcSwingKind(f.pet) != 5 || ps == nil || ps.tmpl == nil {
		t.Fatalf("the pet %q, swing %d: want the Badi Elder's quad (5) on its template", f.pet.Name, npcSwingKind(f.pet))
	}
	if tm := ps.tmpl.tmpl; crit != (tm.Special == "Critical Hit" && tm.SpecialEvery == 30 && tm.SpecialDamage > 33.33 &&
		tm.SpecialFirst == 3) {
		t.Errorf("its template %q: special %q every %.1f s, %.4f a level, first %.1f", tm.Name, tm.Special, tm.SpecialEvery,
			tm.SpecialDamage, tm.SpecialFirst)
	}
	checkCasts(t, f, "the taunt", petTauntFX, 0, steps(10700, 15000, lastOf1450))
	var crits []int
	if crit {
		crits = steps(12700, 30000, lastOf1450)
	}
	checkCasts(t, f, "the crit", templateSpecialFX, 0, crits)
	checkLines(t, f, map[string]int{`lands its Critical Hit on Desert Crab for 2000 before the balance`: len(crits)})
}

// checkGhilan1450 holds a Ghilan Noble's fight with the crab (neither it nor its
// owner hurt) to its kit: six abilities; its chill and its frost wave cast at
// those times, 2 s casts, landing 1000 and 1300.
func checkGhilan1450(t *testing.T, r *petRig, f *swarmFight, chill, frost uint32, chills, frosts []int) {
	t.Helper()
	if ps := r.w.pets[r.p.ServerID]; f.pet.Name != "Ghilan Noble" || ps == nil || len(ps.petAbilities()) != 6 {
		t.Fatalf("the pet %q: want the Ghilan Noble with six abilities", f.pet.Name)
	}
	checkCasts(t, f, "the chill", chill, 2000, chills)
	checkCasts(t, f, "the frost wave", frost, 2000, frosts)
	checkLines(t, f, map[string]int{
		`lands "Ghilan's Chill" on Desert Crab for 1000 before`:      landedBy(chills, 2000, lastOf1450),
		`lands "Ghilan's Frost Wave" on Desert Crab for 1300 before`: landedBy(frosts, 2000, lastOf1450),
	})
}

// The Djinni Vizier at 1.3, 30% over its body's 1: its three spells at 1, 3 and
// 5 s into the fight (10.7, 12.7, 14.7) and every 12 s after, instant, its buff
// once between them; the bolt lands 1000, the thunderclap 1300, the tempest 3000
// on its target (its 1700 and its area's 1300) and 1300 on a creature 14 from it
// (16 from the pet, which stands 2 from its target on the far side), nothing on
// one 16 from its target and 14 from the pet (the area is round its target), nor
// on one 25 away.
func TestTheDjinniViziersKitIsPetition1450s(t *testing.T) {
	r, _ := magePetRig(t, true)
	add := func(id EntityID, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: fmt.Sprintf("Sand Crab %d", id), Level: 34, HP: 1_000_000, MaxHP: 1_000_000, NPCType: 0x0100,
			X: r.enemy.X + dx, Y: r.enemy.Y, Z: r.enemy.Z, World: r.enemy.World}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	near, behind, far := add(3301, 14), add(3303, -16), add(3302, 25)
	f := swarmFightFor(t, r, summonDjinniVizier, 3440, fight1450)
	if d := distXZ(f.pet.X, f.pet.Z, behind.X, behind.Z); d > 15 {
		t.Fatalf("the pet %.1f from the creature behind it, want within the area's 15 of the pet", d)
	}
	if f.pet.sheet == nil || f.pet.Name != "Djinni Vizier" || f.pet.Size < 1.2999 || f.pet.Size > 1.3001 {
		t.Fatalf("the pet: row %v, name %q, size %.3f: want the Djinni Vizier's row at 1.3", f.pet.sheet != nil, f.pet.Name, f.pet.Size)
	}
	last := lastOf1450
	storms := checkVizier1450(t, f, fx1450Bolt, fx1450Clap, fx1450Tempest)
	checkCasts(t, f, "Djinni's Grace", fx1450Grace, 3000, []int{16700})
	for _, c := range []struct {
		line string
		n    int
	}{
		{`splashes "Vizier's Tempest" on ` + near.Name + ` for 1300 before`, landedBy(storms, 0, last)},
		{`splashes "Vizier's Tempest" on ` + behind.Name, 0},
		{`splashes "Vizier's Tempest" on ` + far.Name, 0},
	} {
		if n := f.logged(c.line); n != c.n {
			t.Errorf("%d lines %q, want %d", n, c.line, c.n)
		}
	}
	if lost := 1_000_000 - near.HP; lost != 1300*landedBy(storms, 0, last) || behind.HP != 1_000_000 || far.HP != 1_000_000 {
		t.Errorf("the creature 14 from its target lost %d, the one behind the pet %d, the one 25 away %d", lost,
			1_000_000-behind.HP, 1_000_000-far.HP)
	}
}

// The Efreeti Lord quads (a warrior's Attack4 at 60) for 600 every hit on a
// creature of its owner's level, both hands, and casts its inferno (a 1 s cast)
// at 1 s into the fight and every 30 s after, 2000 each; nothing else hits.
func TestTheEfreetiLordQuadsFor600AndBurnsFor2000(t *testing.T) {
	r, _ := magePetRig(t, true)
	r.enemy.Level = 60
	f := swarmFightFor(t, r, summonEfreetiLord, 4540, fight1450)
	if f.pet.Size != 1.75 {
		t.Errorf("its size %.2f, want its 1.75 (petition 1447)", f.pet.Size)
	}
	checkEfreeti1450(t, f, fx1450Inferno)
}

// The Badi Elder (its own buff on it, as it keeps it up) taunts at 1 s into the
// fight and every 15 s, of hate alone, and crits at 3 s and every 30 s of its
// time at its foe: its template's Critical Hit, 2000 at its level 60, seen as a
// creature's special; it quads. The control: its template without the crit (as
// before the tool) taunts the same and crits never.
func TestTheBadiElderTauntsEvery15AndCritsEvery30(t *testing.T) {
	for _, crit := range []bool{true, false} {
		t.Run(fmt.Sprintf("crit %v", crit), func(t *testing.T) {
			r, _ := magePetRig(t, crit)
			r.enemy.Level = 60
			f := swarmFightFor(t, r, summonBadiElder, 7070, fight1450, func(pet *NPC) {
				pet.Effects = append(pet.Effects, &ActiveEffect{SpellID: 902157, Name: "Badi's Durability", Until: r.at(time.Hour)})
			})
			checkBadi1450(t, r, f, crit)
			if at, _ := f.of(fx1450Durable); len(at) != 0 {
				t.Errorf("its buff cast at %v, on it already", at)
			}
		})
	}
}

// The Badi Elder's crit lands through the balance as its other hits do: whole
// on a creature of its owner's level, 45% of it on one 8 over (900).
func TestTheBadiElderCritIsBalanced(t *testing.T) {
	for _, c := range []struct{ level, want int }{{60, 2000}, {68, 900}} {
		r, _ := magePetRig(t, true)
		r.w.SetBalance(NewBalance([]database.BalanceLevel{{Level: 60, HPPct: 100, DamagePct: 100, ConPct: 100, PetHPPct: 100,
			PetDamagePct: 100}}, []database.BalanceCon{{Diff: 0, DamagePct: 100, TakenPct: 100}, {Diff: 8, DamagePct: 100, TakenPct: 45}}))
		r.summon(t, summonBadiElder, 0)
		ps := r.w.pets[r.p.ServerID]
		crab := r.enemy
		crab.Level, crab.HP, crab.MaxHP = c.level, 1_000_000, 1_000_000
		r.w.mu.Lock()
		for ms := 20000; ms <= 23000; ms += 100 { // its first 3 s at its foe
			r.w.petTemplateSpecialLocked(ps, crab, r.at(time.Duration(ms)*time.Millisecond))
		}
		r.w.mu.Unlock()
		if lost := 1_000_000 - crab.HP; lost != c.want {
			t.Errorf("a level %d crab lost %d to its crit, want %d", c.level, lost, c.want)
		}
	}
}

// The Badi Elder's quad on a creature of its owner's level: its hits 276-693
// (484.5 on average) with nothing on it, and about 500 a hit with the
// magician's epic line on it (Magi`Kot's Sacrificial Breeding, cast by its
// owner: its 135 proc on 12% of the hits, a pet's INT being 3 a level). Its
// melee dice sweep the roll evenly, the procs' are seeded.
func TestTheBadiElderQuadsNear500WithTheEpicLine(t *testing.T) {
	r, _ := magePetRig(t, true)
	pet, _ := r.summon(t, summonBadiElder, 0)
	ps := r.w.pets[r.p.ServerID]
	crab := r.enemy
	crab.Level = 60
	zero, top := func(int) int { return 0 }, func(n int) int { return n - 1 }
	if lo, _ := sheetMeleeHit(pet, crab, zero); lo != 276 {
		t.Errorf("its least hit %d, want 276", lo)
	}
	if hi, _ := sheetMeleeHit(pet, crab, top); hi != 693 {
		t.Errorf("its most hit %d, want 693", hi)
	}
	average := func() float64 {
		t.Helper()
		melee, procs := meleeIntn, procIntn
		defer func() { meleeIntn, procIntn = melee, procs }()
		k := 0
		meleeIntn = func(n int) int { k++; return k % n }
		procIntn = rand.New(rand.NewSource(1450)).Intn
		was := log.Writer()
		log.SetOutput(io.Discard)
		defer log.SetOutput(was)
		crab.HP, crab.MaxHP = 1_000_000_000, 1_000_000_000
		const hits = 418 * 12 // the roll's 418 figures, twelve times each
		r.w.mu.Lock()
		for i := 0; i < hits; i++ { // well within the buff's 10 minutes
			pet.RoundHits = i%4 + 1
			r.w.petHitLocked(ps, crab, r.at(30*time.Second+time.Duration(i)*time.Millisecond))
		}
		r.w.mu.Unlock()
		return float64(1_000_000_000-crab.HP) / hits
	}
	bare := average()
	r.castAt(t, sacrificialBreeding, r.w.objectIDForEntityLocked(r.sess, pet.ServerID), 20*time.Second)
	var proc *ActiveEffect
	for _, e := range pet.Effects {
		if e.SpellID == 701176 {
			proc = e
		}
	}
	if proc == nil || proc.Proc != 135 {
		t.Fatalf("Magi`Kot's Sacrificial Breeding on the pet: %+v, want its 135 proc", proc)
	}
	buffed := average()
	if bare < 484 || bare > 485 || buffed < 497 || buffed > 504 {
		t.Errorf("its hits average %.1f bare and %.1f with the epic line's buff: want 484.5 and about 500", bare, buffed)
	}
	t.Logf("the Badi Elder's quad on a level 60: %.1f a hit bare, %.1f with Magi`Kot's Sacrificial Breeding", bare, buffed)
}

// The Ghilan Noble (its owner in its blessing, neither hurt): its chill at 5 s
// into the fight and every 30 s, its frost wave 3 s after each, 2 s casts, 1000
// and 1300; no heal, nor its tide at the living.
func TestTheGhilanNoblesColdNukesAre3sApart(t *testing.T) {
	r, _ := magePetRig(t, true)
	r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 902167, Name: "Ghilan's Blessing", Until: r.at(time.Hour)})
	f := swarmFightFor(t, r, summonGhilanNoble, 4650, fight1450)
	last := lastOf1450
	checkGhilan1450(t, r, f, fx1450Chill, fx1450Frost, steps(14700, 30000, last), steps(17700, 30000, last))
	for _, line := range []string{`Purifying Tide`, `heals`} {
		if n := f.logged(line); n != 0 {
			t.Errorf("%d lines %q, want none", n, line)
		}
	}
	for _, fx := range []uint32{fx1450Hands, fx1450Mending, fx1450Blessing, fx1450Tide} {
		if at, _ := f.of(fx); len(at) != 0 {
			t.Errorf("visual %#x cast at %v, want never", fx, at)
		}
	}
}

// The Ghilan Noble's heals on its owner or itself, the lower share of its hit
// points: its lay on hands (1500, at once) below half, its mending (800, a 3 s
// cast) below 60%; one landing never more than the hit points missing. The
// control: neither below 60%, neither wanted.
func TestTheGhilanNoblesHealsAre1500And800(t *testing.T) {
	for _, c := range []struct {
		name          string
		owner, petPct int // the owner's hit points of 4000, the pet's percent
		hands, mend   bool
		healed        string // who the lay on hands, else the mending, lands on
		gain          int
	}{
		{"the owner at 45%", 1800, 100, true, true, "owner", 1500},
		{"the owner at 55%", 2200, 100, false, true, "owner", 800},
		{"itself at 40%", 4000, 40, true, true, "pet", 1500},
		{"the owner at 45%, itself at 30%", 1800, 30, true, true, "pet", 1500},
		{"the control, both over 60%", 2800, 80, false, false, "", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, _ := magePetRig(t, true)
			pet, _ := r.summon(t, summonGhilanNoble, 0)
			ps := r.w.pets[r.p.ServerID]
			r.p.MaxHP, r.p.HP = 4000, c.owner
			pet.HP = pet.MaxHP * c.petPct / 100
			ab := ps.petAbilities()
			hands, mend := &ab[0], &ab[1]
			now := r.at(30 * time.Second)
			r.w.mu.Lock()
			defer r.w.mu.Unlock()
			if got := [2]bool{r.w.petWantsLocked(ps, hands, now), r.w.petWantsLocked(ps, mend, now)}; got != [2]bool{c.hands, c.mend} {
				t.Fatalf("wanted: the lay on hands %v, the mending %v; want %v, %v", got[0], got[1], c.hands, c.mend)
			}
			i, a := 0, hands
			if !c.hands {
				i, a = 1, mend
			}
			if c.healed == "" {
				return
			}
			ownerWas, petWas := r.p.HP, pet.HP
			if !r.w.petCastLocked(ps, i, nil, now) {
				t.Fatal("not cast")
			}
			r.w.petCastSystemLocked(now.Add(a.castTime(a.pinned) + castLand))
			got := map[string]int{"owner": r.p.HP - ownerWas, "pet": pet.HP - petWas}
			if got[c.healed] != c.gain || got["owner"]+got["pet"] != c.gain {
				t.Errorf("healed: its owner %d, itself %d; want %d on its %s", got["owner"], got["pet"], c.gain, c.healed)
			}
		})
	}
}

// The spells' elements: each pet's figure less what a creature strong in its
// element (a template's 50%) resists, whole on one strong in another.
func TestPetition1450sSpellsAreOfTheirElements(t *testing.T) {
	for _, c := range []struct {
		summon     CastSpell
		index      int
		of, other  string
		full, half int
	}{
		{summonDjinniVizier, 0, "LR", "CR", 1000, 500},
		{summonEfreetiLord, 0, "FR", "CR", 2000, 1000},
		{summonGhilanNoble, 3, "CR", "FR", 1000, 500},
		{summonGhilanNoble, 4, "CR", "LR", 1300, 650},
	} {
		for kind, want := range map[string]int{c.of: c.half, c.other: c.full} {
			r, _ := magePetRig(t, true)
			r.summon(t, c.summon, 0)
			ps := r.w.pets[r.p.ServerID]
			crab := r.enemy
			crab.HP, crab.MaxHP = 1_000_000, 1_000_000
			crab.tmpl = &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "warded", Class: -1, HPPct: 100, TakenPct: 100,
				MeleePct: 100, ResistKind: kind, ResistPct: 50}}
			a := &ps.petAbilities()[c.index]
			r.w.mu.Lock()
			r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: a.pinned, target: crab.ServerID}, r.at(20*time.Second))
			r.w.mu.Unlock()
			if lost := 1_000_000 - crab.HP; lost != want {
				t.Errorf("%s's %s on a crab strong in %s: %d, want %d", c.summon.Name, a.pinned.Name, kind, lost, want)
			}
		}
	}
}
