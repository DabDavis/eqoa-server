package entity

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The pets' class templates (pettemplate.go) over a real database: the seed
// templates and pettemplate rows the database is made with, the game's own
// lines they pick from (as this server keeps them), in the pet rig (a level
// 45 magician beside a beach crab); only the pets' system run, so the crab
// never fights back unless a test lets it.

// petFixture is the pets' database, made once for the tests that only read
// it (TestMain removes it): opening a database takes seconds here.
var petFixture struct {
	once sync.Once
	dir  string
	db   *database.SQLiteDB
	err  error
}

// newPetTemplateRig is the pet rig with the lines' spells and the summonings
// whose descriptions name a weapon in a database (the shared one, or a new
// one of its own to change), its templates read into the world as the server
// reads them.
func newPetTemplateRig(t *testing.T, own ...bool) (*petRig, *database.SQLiteDB) {
	t.Helper()
	r := newPetRig(t)
	var db *database.SQLiteDB
	if len(own) > 0 && own[0] {
		var err error
		if db, err = newPetFixtureDB(t.TempDir() + "/pets.db"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
	} else {
		petFixture.once.Do(func() {
			if petFixture.dir, petFixture.err = os.MkdirTemp("", "pettemplates"); petFixture.err == nil {
				petFixture.db, petFixture.err = newPetFixtureDB(petFixture.dir + "/pets.db")
			}
		})
		if petFixture.err != nil {
			t.Fatal(petFixture.err)
		}
		db = petFixture.db
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	return r, db
}

// newPetFixtureDB makes the pets' database: the lines' spells and the
// summonings whose descriptions name a weapon, the seed templates the
// database is made with.
func newPetFixtureDB(path string) (*database.SQLiteDB, error) {
	db, err := database.NewSQLite(path)
	if err != nil {
		return nil, err
	}
	var failed error
	exec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil && failed == nil {
			failed = fmt.Errorf("%s: %v", q, err)
		}
	}
	for _, s := range []struct {
		id, level, cast, recast, scope int
		name, line, desc               string
		fx                             uint32
		class                          int
	}{
		// the cleric's heal and offensive spells (the water pet)
		{1100, 10, 2, 3, 1, "Healing", "Target, Increase Hitpoints by 125", "", 0x0B000001, 9},
		{235, 1, 2, 3, 1, "Strike", "Target, Decrease Hitpoints by 10", "", 0x0B000002, 9},
		{700190, 20, 2, 3, 1, "Holy Bolt", "Target, Decrease Hitpoints by 142", "", 0x0B000003, 9},
		// the druid's AC, the ranger's AGI, the shaman's STR and DEX lines
		{342, 1, 4, 3, 1, "Woodskin", "Target, Increase AC by 12, 30min", "", 0x0B000004, 7},
		{700267, 24, 4, 3, 1, "Steelskin", "Target, Increase AC by 310, 30min", "", 0x0B000005, 7},
		{700061, 8, 3, 2, 1, "Feet Like Cat", "Target, Increase AGI by 10, 30min", "", 0x0B000006, 1},
		{700965, 37, 4, 1, 1, "Cat's Ease", "Target, Increase AGI by 40, 30min", "", 0x0B000007, 1},
		{700124, 13, 3, 1, 1, "Tribal Brawn", "Target, Increase STR by 25, Increase STA by 25, 30min", "", 0x0B000008, 8},
		{700141, 16, 3, 1, 1, "Deftness", "Target, Increase DEX by 25, Increase AGI by 25, 30min", "", 0x0B000009, 8},
		{700675, 40, 4, 1800, 1, "Avatar", "Target, Increase DEX by 150, 5min", "", 0x0B00000A, 8}, // its 1800 s recast: left out
		// the magician's fire and cold, the wizard's nukes
		{700198, 20, 2, 3, 1, "Lava Strike", "Target, Decrease Hitpoints by 232", "", 0x0B00000B, 10},
		{300676, 24, 2, 3, 1, "Glacial Stream", "Target, Decrease Hitpoints by 216", "", 0x0B00000C, 10},
		{700214, 20, 2, 3, 1, "Shocking Strike", "Target, Decrease Hitpoints by 232", "", 0x0B00000D, 13},
		{700138, 16, 2, 3, 1, "Burning Gaze", "Target, Decrease Hitpoints by 134", "", 0x0B00000E, 13},
		{237, 20, 5, 3, 1, "Courage", "Target, Increase Maximum HP by 40, Increase AC by 40, 1800sec", "", 0x0B000011, 9}, // a buff, no heal: over Healing's level, the control
		{700257, 24, 2, 3, 1, "Freezing Strike", "Target, Decrease Hitpoints by 346", "", 0x0B00000F, 13},
		{700182, 20, 2, 180, 1, "Detonate", "Target, Decrease Hitpoints by 534", "", 0x0B000010, 13}, // 180 s: left out
		// the necromancer's summonings, a weapon version's description saying so
		{914, 24, 6, 30, 0, "Decaying Flesh", "This spell summons pet", "Summons a zombie to aid you.", 0, 11},
		{918, 25, 6, 30, 0, "Decaying Flesh", "This spell summons pet", "Summons a zombie with weapons to aid you.", 0, 11},
		{955, 44, 10, 60, 0, "Howling Death", "Summon Pet", "Summons a spectre to aid you.", 0, 11},
		{956, 44, 10, 60, 0, "Howling Death", "Summon Pet", "Summons a spectre with a weapon to aid you.", 0, 11},
	} {
		exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES (?, ?, 0, 0, 16880, ?, 10, 0, 0, ?, ?, 0, ?, ?)`, s.id, s.level, s.cast, s.scope,
			s.recast, s.name, s.desc)
		exec(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES (?, ?, ?, 'test')`, s.id, s.line, int64(s.fx))
		exec(`INSERT INTO classspells (class, spellid, source) VALUES (?, ?, 'dump')`, s.class, s.id)
	}
	if failed != nil {
		db.Close()
		return nil, failed
	}
	return db, nil
}

// summonSpell is a summoning as the server casts it: its id, level and
// name, the line this server keeps.
func summonSpell(id, level int, name, line string) CastSpell {
	return CastSpell{SpellID: id, DisplayIndex: 40, Level: level, Name: name, CastTime: 8 * time.Second, PowerCost: 100,
		Def: ParseSpellText(line)}
}

var (
	earthElemental   = summonSpell(693, 34, "Earth Elemental", "Summons an Earth Elemental to aid you")
	summonBadi       = summonSpell(701035, 49, "Summon Badi", "Summon Badi the Earth Pet to aid you")
	waterElemental   = summonSpell(709, 39, "Water Elemental", "Summons a Water Elemental to aid you")
	airElemental     = summonSpell(700519, 45, "Air Elemental", "Summons a Air Elemental to aid you")
	pendrils         = summonSpell(498, 1, "Pendril's Animation", "Summons a pet")
	stormOfBlades    = summonSpell(700819, 60, "Storm of Blades", "Summons pet")
	summonGrandAnim  = summonSpell(701263, 60, "Summon Grand Animation", "Summons a pet, level 54")
	decayingBase     = summonSpell(914, 24, "Decaying Flesh", "This spell summons pet")
	decayingWeapons  = summonSpell(918, 25, "Decaying Flesh", "This spell summons pet")
	howlingBase      = summonSpell(955, 44, "Howling Death", "Summon Pet")
	howlingWithBlade = summonSpell(956, 44, "Howling Death", "Summon Pet")
)

// tfight is a templated pet's fight in the rig: the pet beside the crab and
// sent at it, the pets' system run every 100 ms for d (the crab's own
// systems with fightBack), and what it sent and did.
type tfight struct {
	r       *petRig
	pet     *NPC
	ps      *petState
	petID   uint32
	casts   []petSeen // the pet's 0x00B1s
	crabHit []int     // the crab's hit point losses, a tick each
}

func petTemplateFight(t *testing.T, r *petRig, spell CastSpell, class int, d time.Duration, fightBack bool) *tfight {
	t.Helper()
	r.p.Class = class
	r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
	pet, _ := r.summon(t, spell, 0)
	ox, oz := pet.X, pet.Z
	pet.X, pet.Y, pet.Z = r.enemy.X-2, r.enemy.Y, r.enemy.Z
	r.w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
	f := &tfight{r: r, pet: pet, ps: r.w.pets[r.p.ServerID], petID: r.w.objectIDForEntityLocked(r.sess, pet.ServerID)}
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(9600*time.Millisecond))
	r.sent()
	for ms := 9700; ms <= 9700+int(d/time.Millisecond); ms += 100 {
		hp := r.enemy.HP
		if fightBack {
			r.tick(time.Duration(ms) * time.Millisecond)
		} else {
			r.w.mu.Lock()
			r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
			r.w.mu.Unlock()
		}
		if loss := hp - r.enemy.HP; loss > 0 {
			f.crabHit = append(f.crabHit, loss)
		}
		for i, m := range r.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat && binary.LittleEndian.Uint32(m[2:]) == f.petID {
				f.casts = append(f.casts, petSeen{at: ms, order: i, target: binary.LittleEndian.Uint32(m[6:]),
					ms: binary.LittleEndian.Uint32(m[11:]), fx: binary.LittleEndian.Uint32(m[15:])})
			}
		}
	}
	return f
}

// cast is whether the pet sent that visual (a spell's 0x00B1).
func (f *tfight) cast(fx uint32) int {
	n := 0
	for _, c := range f.casts {
		if c.fx == fx {
			n++
		}
	}
	return n
}

// The seeds: every pettemplate row names a template classtemplates is given
// with it, every one of those takes CheckClassTemplate, and the database's
// loader reads them all with no problem but the lines this small database
// lacks (no spell of a kind, which the server's has).
func TestPetTemplateSeeds(t *testing.T) {
	names := map[string]bool{}
	for _, ct := range database.SeedPetClassTemplates {
		if err := database.CheckClassTemplate(ct); err != nil {
			t.Errorf("template %q: %v", ct.Name, err)
		}
		names[ct.Name] = true
	}
	for _, p := range database.SeedPetTemplates {
		if err := database.CheckPetTemplate(p); err != nil || !names[p.Template] {
			t.Errorf("row %q (weapon %v): %v, template %q there: %v", p.Spell, p.Weapon, err, p.Template, names[p.Template])
		}
	}
	r, _ := newPetTemplateRig(t)
	if n := len(r.w.templates.pets); n != len(database.SeedPetTemplates) {
		t.Errorf("%d pet rows read, want %d", n, len(database.SeedPetTemplates))
	}
	if !r.w.templates.weapon[918] || !r.w.templates.weapon[956] || r.w.templates.weapon[914] || r.w.templates.weapon[955] {
		t.Errorf("weapon versions %v: want 918 and 956, not 914 or 955", r.w.templates.weapon)
	}
}

// A necromancer's pet is a shadow knight (its class) that swings a warrior's
// round: two hits from 20 (Attack2), four from 35 (Attack4); the Death
// Sacrament versions swing Sandstorm's recorded rounds, the zombie Attack2,
// the spectre Attack3 every 5.2 s, their hits scaled to the base pet's damage
// a second (x1 and x1.73; the 20% more of their first day taken off by the
// balance's pet fit). The control: with the pettemplate
// rows gone they fight as before (petrotation.go's recorded rounds, a
// warrior's class).
func TestNecromancerPetsAreShadowKnightsThatQuad(t *testing.T) {
	for _, c := range []struct {
		name         string
		spell        CastSpell
		kind         int
		round        time.Duration
		scale, melee float64
	}{
		{"zombie", decayingBase, 4, 0, 1, 100},
		{"zombie with weapons", decayingWeapons, 4, 0, 1, 100},
		{"spectre", howlingBase, 5, 0, 1, 100},
		{"spectre with a weapon", howlingWithBlade, 7, 5200 * time.Millisecond, (4.0 / 6) / (2 / 5.2), 100},
	} {
		t.Run(c.name, func(t *testing.T) {
			r, _ := newPetTemplateRig(t)
			r.p.Class, r.p.Level = classNecromancer, 51
			pet, _ := r.summon(t, c.spell, 0)
			ps := r.w.pets[r.p.ServerID]
			if pet.Class != 3 || npcSwingKind(pet) != c.kind || pet.SwingRound != c.round || ps.tmpl == nil {
				t.Fatalf("class %d, swing %d, round %v, template %v: want class 3, swing %d, round %v", pet.Class,
					npcSwingKind(pet), pet.SwingRound, ps.tmpl != nil, c.kind, c.round)
			}
			if math.Abs(ps.hitScale-c.scale) > 1e-6 || float64(ps.tmpl.tmpl.MeleePct) != c.melee {
				t.Errorf("hits x%.4f, melee %d%%: want x%.4f, %g%%", ps.hitScale, ps.tmpl.tmpl.MeleePct, c.scale, c.melee)
			}
		})
	}
	r, db := newPetTemplateRig(t, true)
	if _, err := db.Exec(`DELETE FROM pettemplate`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	r.p.Class, r.p.Level = classNecromancer, 51
	pet, _ := r.summon(t, howlingBase, 0)
	if pet.Class != classWarrior || npcSwingKind(pet) != 3 || pet.SwingRound != spectreRound || r.w.pets[r.p.ServerID].tmpl != nil {
		t.Errorf("no rows: class %d, swing %d, round %v; want as before (warrior, Attack3 every 5.2 s)", pet.Class, npcSwingKind(pet), pet.SwingRound)
	}
}

// The water pet: one hit a round (a cleric's swing), its owner healed below
// 60% with the cleric's instant heal of its level (Healing, 125 on the owner,
// from the pet; never Courage, a lasting buff that raises hit points), itself
// below half, and a cleric's offensive spell (Holy Bolt at 30%: 43 off the
// crab) on what it fights. The controls: an owner over 60% gets no heal, a
// pet over half heals none of itself.
func TestTheWaterPetHealsItsOwnerThenItselfAndNukes(t *testing.T) {
	r, _ := newPetTemplateRig(t)
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	r.p.HP = r.p.MaxHPNow() * 70 / 100 // the control: over 60%
	f := petTemplateFight(t, r, waterElemental, 10, 10*time.Second, false)
	if f.pet.Class != 9 || npcSwingKind(f.pet) != 0 {
		t.Fatalf("class %d, swing %d: want a cleric's one hit (Attack1)", f.pet.Class, npcSwingKind(f.pet))
	}
	if n := f.cast(0x0B000001); n != 0 {
		t.Errorf("an owner at 70%% healed %d times", n)
	}
	if n := f.cast(0x0B000003); n == 0 {
		t.Error("no Holy Bolt at the crab")
	}
	nuke := false
	for _, l := range f.crabHit {
		nuke = nuke || l == 43
	}
	if !nuke {
		t.Errorf("crab losses %v: want Holy Bolt's 142 at 30%%, 43", f.crabHit)
	}
	// the owner down to half: healed from the pet, 125
	low := r.p.MaxHPNow() / 2
	r.p.HP = low
	r.sent()
	ownerHealed, petHealed := false, false
	for ms := 20000; ms < 44000; ms += 100 {
		r.w.mu.Lock()
		r.w.petSystem(r.at(time.Duration(ms) * time.Millisecond))
		r.w.mu.Unlock()
		for _, di := range damageIndications(r.sent()) {
			if di[0] == int64(f.petID) && di[1] == 125 && di[2] == int64(r.me) {
				ownerHealed = true
			}
		}
		if ownerHealed && !petHealed {
			f.pet.HP = f.pet.MaxHP * 40 / 100 // now itself
			petHealed = true
		}
	}
	if !ownerHealed || r.p.HP != low+125 {
		t.Errorf("the owner: healed %v, %d HP from %d; want 125 from the pet", ownerHealed, r.p.HP, low)
	}
	if f.pet.HP <= f.pet.MaxHP*40/100 {
		t.Errorf("the pet below half never healed itself: %d of %d", f.pet.HP, f.pet.MaxHP)
	}
}

// The earth pet: a warrior's round (two hits at 34, four from 35: Summon
// Badi's at 45), 30% more hit points, 20% less of a creature's melee hit
// landing (Summon Badi, a mastery pet, the earth pet's template but for its
// own 81% hit points: the balance's pet fit of 2026-10-30); a taunt that
// takes the top of the crab's hate and a melee hit;
// and its owner kept in the druid's AC line and the ranger's AGI line of its
// level, not cast again while on. The control: a fire pet's hits on the same
// dice land 125/90 of the earth pet's, and a crab's hit on an air pet 115%.
func TestTheEarthPetTauntsTanksAndBuffsItsOwner(t *testing.T) {
	r, _ := newPetTemplateRig(t)
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	r.enemy.Threats = map[EntityID]int64{r.p.ServerID: 5000}
	f := petTemplateFight(t, r, earthElemental, 10, 40*time.Second, false)
	if f.pet.Class != 0 || npcSwingKind(f.pet) != 4 {
		t.Fatalf("class %d, swing %d: want a warrior's Attack2 at 34", f.pet.Class, npcSwingKind(f.pet))
	}
	if top := r.enemy.Threats[f.pet.ServerID]; top <= 5000 || r.enemy.CurrentEnemy != f.pet.ServerID {
		t.Errorf("the taunt: the pet's hate %d over the owner's 5000, the crab on %d: want the pet on top", top, r.enemy.CurrentEnemy)
	}
	var ac, agi int
	for _, e := range r.p.Effects {
		switch e.SpellID {
		case 700267:
			ac++
		case 700061:
			agi++
		}
	}
	if ac != 1 || agi != 1 || f.cast(0x0B000005) != 1 || f.cast(0x0B000006) != 1 {
		t.Errorf("the owner's buffs: Steelskin %d (cast %d), Feet Like Cat %d (cast %d); want each once and on", ac,
			f.cast(0x0B000005), agi, f.cast(0x0B000006))
	}

	badi, _ := newPetTemplateRig(t)
	b := petTemplateFight(t, badi, summonBadi, 10, time.Second, false)
	if npcSwingKind(b.pet) != 5 {
		t.Errorf("Summon Badi's at %d: swing %d, want a warrior's four hits (Attack4)", b.pet.Level, npcSwingKind(b.pet))
	}
	if want := int(math.Round(float64(b.pet.MaxHP) / 0.81)); math.Abs(float64(want-badi.w.balance.PetMaxHP(b.pet.Level))) > 2 {
		t.Errorf("hit points %d: want 81%% of the balance's %d (its own template)", b.pet.MaxHP, badi.w.balance.PetMaxHP(b.pet.Level))
	}
	er, _ := newPetTemplateRig(t)
	if e := petTemplateFight(t, er, earthElemental, 10, time.Second, false); e.pet.MaxHP != int(math.Round(1.3*float64(er.w.balance.PetMaxHP(e.pet.Level)))) {
		t.Errorf("the Earth Elemental's hit points %d: want 130%% of the balance's %d (its kind's template)", e.pet.MaxHP,
			er.w.balance.PetMaxHP(e.pet.Level))
	}
	badi.w.mu.Lock()
	taken := badi.w.petTakenLocked(b.pet, 100)
	badi.w.mu.Unlock()
	if taken != 80 {
		t.Errorf("a creature's 100 on the earth pet lands %d, want 80", taken)
	}

	fr, _ := newPetTemplateRig(t)
	fire := petTemplateFight(t, fr, fireElemental(), 10, time.Second, false)
	ar, _ := newPetTemplateRig(t)
	air := petTemplateFight(t, ar, airElemental, 10, time.Second, false)
	if fire.ps.meleeOut(100) != 125 || f.ps.meleeOut(100) != 90 {
		t.Errorf("a melee hit of 100: fire %d, earth %d; want 125 and 90", fire.ps.meleeOut(100), f.ps.meleeOut(100))
	}
	ar.w.mu.Lock()
	onAir := ar.w.petTakenLocked(air.pet, 100)
	ar.w.mu.Unlock()
	if onAir != 115 {
		t.Errorf("a creature's 100 on the air pet lands %d, want 115", onAir)
	}
}

// The fire pet casts the magician's fire spell of its level at what it
// fights (Lava Strike at 25%: 58; never the cold Glacial Stream), and keeps
// its owner in the shaman's STR and DEX lines (Tribal Brawn, Deftness; never
// Avatar, of an 1800 s recast).
func TestTheFirePetBurnsAndBuffsStrAndDex(t *testing.T) {
	r, _ := newPetTemplateRig(t)
	f := petTemplateFight(t, r, fireElemental(), 10, 40*time.Second, false)
	if f.cast(0x0B00000B) == 0 || f.cast(0x0B00000C) != 0 {
		t.Errorf("Lava Strike %d, Glacial Stream %d: want the fire one alone", f.cast(0x0B00000B), f.cast(0x0B00000C))
	}
	lava := false
	for _, l := range f.crabHit {
		lava = lava || l == 58
	}
	if !lava {
		t.Errorf("crab losses %v: want Lava Strike's 232 at 25%%, 58", f.crabHit)
	}
	got := map[int]bool{}
	for _, e := range r.p.Effects {
		got[e.SpellID] = true
	}
	if !got[700124] || !got[700141] || got[700675] {
		t.Errorf("the owner's effects %v: want Tribal Brawn and Deftness, never Avatar", got)
	}
}

// The air pet: a wizard's one hit a round, 20% fewer hit points than the
// balance gives (the fire pet: its own), and three nukes on clocks of their
// own: lightning (Shocking Strike), fire (Burning Gaze), cold (Freezing
// Strike); never Detonate (180 s).
func TestTheAirPetCastsThreeNukes(t *testing.T) {
	r, _ := newPetTemplateRig(t)
	f := petTemplateFight(t, r, airElemental, 10, 40*time.Second, false)
	if f.pet.Class != 13 || npcSwingKind(f.pet) != 0 {
		t.Fatalf("class %d, swing %d: want a wizard's one hit", f.pet.Class, npcSwingKind(f.pet))
	}
	for fx, name := range map[uint32]string{0x0B00000D: "Shocking Strike", 0x0B00000E: "Burning Gaze", 0x0B00000F: "Freezing Strike"} {
		if f.cast(fx) == 0 {
			t.Errorf("no %s", name)
		}
	}
	if f.cast(0x0B000010) != 0 {
		t.Error("Detonate, of a 180 s recast, was cast")
	}
	if want := r.w.balance.PetMaxHP(f.pet.Level) * 80 / 100; math.Abs(float64(f.pet.MaxHP-want)) > 1 {
		t.Errorf("hit points %d, want 80%% of the balance's: %d", f.pet.MaxHP, want)
	}
}

// An animation's taunt is 84-88 at every level (the user: Storm of Blades',
// "the enc pet taunt doesnt scale"), landing as it is: Pendril's at 1 and
// Storm of Blades at 60 alike, under a balance that scales a pet's damage
// fivefold. The control: an earth pet's taunt, a melee hit, goes through the
// balance.
func TestTheAnimationsTauntIs84To88AtEveryLevel(t *testing.T) {
	balance := NewBalance([]database.BalanceLevel{{Level: 1, HPPct: 100, DamagePct: 100, ConPct: 100, PetHPPct: 100,
		PetDamagePct: 500, RegenPct: 15, PetSpecialHits: 3}}, nil)
	for _, c := range []struct {
		name  string
		spell CastSpell
		owner int
	}{{"Pendril's at 1", pendrils, 1}, {"Storm of Blades at 60", stormOfBlades, 60}} {
		t.Run(c.name, func(t *testing.T) {
			for _, dice := range []func(int) int{diceLow, diceHigh} {
				r, _ := newPetTemplateRig(t)
				old := templateIntn
				templateIntn = dice
				r.w.balance = balance
				r.p.Level = c.owner
				f := petTemplateFight(t, r, c.spell, classEnchanter, 1500*time.Millisecond, false)
				templateIntn = old
				if f.pet.Class != classEnchanter || npcSwingKind(f.pet) != 0 || f.pet.Level != c.spell.Level {
					t.Fatalf("level %d, class %d, swing %d: want an enchanter's one hit at %d", f.pet.Level, f.pet.Class,
						npcSwingKind(f.pet), c.spell.Level)
				}
				want := 84
				if dice(5) != 0 {
					want = 88
				}
				n := 0
				for _, l := range f.crabHit {
					if l >= 84 && l <= 88 {
						n++
						if l != want {
							t.Errorf("the taunt took %d, want %d", l, want)
						}
					}
				}
				if n != 1 {
					t.Errorf("crab losses %v: want one taunt of %d (84-88 as it is) among the melee hits", f.crabHit, want)
				}
			}
		})
	}
	r, _ := newPetTemplateRig(t)
	r.w.balance = balance
	old := meleeIntn
	meleeIntn = func(n int) int { return n / 2 }
	t.Cleanup(func() { meleeIntn = old })
	f := petTemplateFight(t, r, earthElemental, 10, 1500*time.Millisecond, false)
	r.w.mu.Lock()
	hit := r.w.petDealtLocked(f.pet, r.enemy, f.ps.meleeOut(petMeleeHit(f.pet, r.enemy, meleeIntn)))
	r.w.mu.Unlock()
	if len(f.crabHit) == 0 || f.crabHit[0] != hit {
		t.Errorf("the earth pet's taunt took %v, want a melee hit through the balance: %d", f.crabHit, hit)
	}
}

// An animation heals itself by its four shares, 45%, 20%, 10% and 5% of its
// most hit points, the biggest due that it is missing (the user's rule: no
// overheal), each on its own clock (38, 24, 8 and 5 s): missing half, all
// ready: the 45%; missing 30%: the 20%, not the 45%; missing 30% with the 20%
// down: the 10%; missing 3%: none; a heal down is never cast. Each lands its
// share exactly, never the rest of the way to full.
func TestTheAnimationsHealsByTheirShare(t *testing.T) {
	r, _ := newPetTemplateRig(t)
	r.p.Class = classEnchanter
	pet, _ := r.summon(t, pendrils, 0)
	ps := r.w.pets[r.p.ServerID]
	pet.MaxHP, pet.HP = 1000, 1000
	now := r.at(30 * time.Second)
	f := ps.petAbilityClocks(0, now)
	share := func(i int) int { return ps.petAbilities()[i].HealPct }
	ready := func() {
		for i := range ps.petAbilities() {
			f.next[i] = now.Add(-time.Second)
		}
	}
	heal := func(hp int) (int, int) { // the heal cast, and what it restored
		pet.HP = hp
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		i := ps.petSharePickLocked(f, now)
		if i < 0 {
			return 0, 0
		}
		f.next[i] = now.Add(ps.petAbilities()[i].every)
		before := pet.HP
		r.w.petShareHealLocked(ps, &ps.petAbilities()[i], now)
		return share(i), pet.HP - before
	}
	ready()
	if pct, got := heal(500); pct != 45 || got != 450 {
		t.Errorf("missing half, all ready: the %d%% for %d, want the 45%% for 450", pct, got)
	}
	ready()
	if pct, got := heal(700); pct != 20 || got != 200 {
		t.Errorf("missing 30%%: the %d%% for %d, want the 20%% for 200", pct, got)
	}
	if pct, got := heal(700); pct != 10 || got != 100 {
		t.Errorf("missing 30%%, the 20%% down: the %d%% for %d, want the 10%% for 100", pct, got)
	}
	ready()
	if pct, got := heal(970); pct != 0 || got != 0 {
		t.Errorf("missing 3%%: the %d%% for %d, want none", pct, got)
	}
	for i := range ps.petAbilities() {
		f.next[i] = now.Add(time.Minute)
	}
	if pct, _ := heal(100); pct != 0 {
		t.Errorf("every heal down: the %d%% was cast", pct)
	}
	var clocks []float64
	for _, a := range ps.petAbilities() {
		if petShareHeal(&a) {
			clocks = append(clocks, a.Every)
		}
	}
	if len(clocks) != 4 || clocks[0] != 38 || clocks[1] != 24 || clocks[2] != 8 || clocks[3] != 5 {
		t.Errorf("the heals' clocks %v, want 38, 24, 8, 5 s", clocks)
	}
}

// Pets of no pettemplate row keep ours: a shaman's spirit, a wizard's
// familiar, the Grand Animation (dual hits, its 500 taunt).
func TestPetsWithNoTemplateRowKeepOurs(t *testing.T) {
	for _, c := range []struct {
		spell CastSpell
		class int
	}{
		{summonSpell(700302, 29, "Companion Spirit", "Summons a companion spirit to aid you"), 8},
		{summonSpell(700306, 29, "Familiar", "Summons a Familar to aid you"), 13},
		{summonGrandAnim, classEnchanter},
	} {
		r, _ := newPetTemplateRig(t)
		r.p.Class, r.p.Level = c.class, 60
		pet, _ := r.summon(t, c.spell, 0)
		if ps := r.w.pets[r.p.ServerID]; ps.tmpl != nil || pet.Class != classWarrior {
			t.Errorf("%q: template %v, class %d; want none, a warrior's", c.spell.Name, ps.tmpl != nil, pet.Class)
		}
	}
}
