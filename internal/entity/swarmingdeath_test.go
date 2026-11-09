package entity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// The Swarming Deaths' kit (the GM Jibrish's petition 1388: "The pet swarming
// death, in all its variants, needs a 300 a tick dot (.5 second cast) it keeps
// up that lasts for 24 seconds. It needs a 1k damage lifetap that heals it. It
// needs a 2k damage hard touch that it can use every 50 seconds. It also needs a
// melee proc for 250 damage, with a 25% proc rate, base."), as
// petfix176_20261109.py writes it: three spells of their own on the workbook
// row, and a template of the Swarming Deaths' own carrying the proc. The
// visuals here are made up; the rest is the script's.

const (
	fxSwarmPlague uint32 = 0x0B100001
	fxSwarmLeech  uint32 = 0x0B100002
	fxSwarmTouch  uint32 = 0x0B100003
)

// swarmKit is the workbook row's abilities: the damage over time kept up from
// 1 s into a fight, cast in half a second; the harm touch every 50 s from 3 s;
// the tap every 30 s from 5 s.
const swarmKit = `[{"kind":"dot","spell_id":905100,"every":24,"first":1,"cast":0.5},` +
	`{"kind":"nuke","spell_id":905102,"every":50,"first":3},` +
	`{"kind":"lifetap","spell_id":905101,"every":30,"first":5}]`

var swarmingDeath = summonSpell(700823, 60, "Swarming Death", "Summon Pet")

// swarmRig is the pet rig with the kit's rows, the summoning's pettemplate row
// naming template (the kit's "pet swarming death", or the control's "pet
// necromancer"), the live 700823's workbook numbers with the kit (swarmKit, or
// another), and a level 60 necromancer to summon it.
func swarmRig(t *testing.T, template string, kit ...string) *petRig {
	t.Helper()
	abilities := swarmKit
	if len(kit) > 0 {
		abilities = kit[0]
	}
	r, _ := sheetRig(t,
		`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast,
			eqprequire, spellname, spelldesc) VALUES
			(905100, 60, 0, 0, 16752, 0, 0, 0, 0, 1, 24, -1, 'Swarming Plague', ''),
			(905101, 60, 0, 0, 16752, 1, 0, 0, 0, 1, 30, -1, 'Swarming Leech', ''),
			(905102, 60, 0, 0, 16752, 0, 0, 0, 0, 1, 50, -1, 'Swarming Harm Touch', '')`,
		fmt.Sprintf(`INSERT INTO spelleffect (spellid, line, fxdict, source) VALUES
			(905100, 'Target, Decrease HP per tic by 300, 24sec', %d, 'test'),
			(905101, 'Target, Drain HP by 1000 give self', %d, 'test'),
			(905102, 'Target, Decrease Hitpoints by 2000', %d, 'test')`, fxSwarmPlague, fxSwarmLeech, fxSwarmTouch),
		`INSERT INTO classtemplates (name, class, abilities, proc_chance, proc_damage, proc_name, proc_resist, note, source)
			VALUES ('pet swarming death', 3, '[]', 25, 250, 'Swarming Sting', 'AR', 'test', 'test')`,
		`UPDATE pettemplate SET template = '`+template+`' WHERE spell = 'swarming death' AND weapon = 0`,
		`INSERT INTO petstats (spellid, sheet_pet_id, name, model, level, type, element, ac, str, sta, agi, dex, wis,
			intelligence, cha, hp, power, hot, pot, ar, cr, lr, fr, pr, dr, str_max, sta_max, agi_max, dex_max, wis_max,
			int_max, cha_max, hp_factor, off_mod, def_mod, wpn1_dmg, wpn1_type, wpn2_dmg, wpn2_type, abilities, source)
			VALUES (700823, '0055', 'Red Swarming Death', 1707298977, 53, 'melee', 'arcane', 1590, 527, 421, 421, 527, 262,
			368, 156, 4254, 5392, 94, 119, 94, 54, 54, 54, 74, 74, 580, 580, 580, 580, 580, 580, 580, 42, 4, 1, 636,
			'slash', 583, 'pierce', '`+abilities+`', 'test')`)
	r.p.Class, r.p.Level = classNecromancer, 60
	return r
}

// swarmFight is a Swarming Death's fight with the rig's crab (a million hit
// points, not fighting back): the pets' and the effects' systems every 100 ms
// from the attack at 9.6 s, the proc's dice under its 25 on every fourth throw
// alone, and what was seen.
type swarmFight struct {
	pet   *NPC
	casts []petSeen // the pet's 0x00B1s
	ticks []int     // when its damage over time ticked on the crab
	most  int       // the most of its damage over time on the crab at once
	heals []int     // its hit points gained, a step each
	dice  int       // the proc's throws: its melee hits that left the crab standing
	lines string    // the log
}

func swarmFightFor(t *testing.T, r *petRig, spell CastSpell, hp int, d time.Duration) *swarmFight {
	t.Helper()
	var lines bytes.Buffer
	was := log.Writer()
	log.SetOutput(&lines)
	defer log.SetOutput(was)
	f := &swarmFight{}
	dice := templateIntn
	templateIntn = func(n int) int { f.dice++; return f.dice % 4 * n / 4 }
	defer func() { templateIntn = dice }()
	r.enemy.HP, r.enemy.MaxHP = 1_000_000, 1_000_000
	pet, _ := r.summon(t, spell, 0)
	f.pet = pet
	ox, oz := pet.X, pet.Z
	pet.X, pet.Y, pet.Z = r.enemy.X-2, r.enemy.Y, r.enemy.Z
	r.w.SpatialIndex.Update(pet.ServerID, ox, oz, pet.X, pet.Z)
	pet.HP = hp
	petID := r.w.objectIDForEntityLocked(r.sess, pet.ServerID)
	r.w.PetCommand(r.sess, petCmdAttack, crabObject, r.at(9600*time.Millisecond))
	r.sent()
	seen := map[*ActiveEffect]time.Time{}
	for ms := 9700; ms <= 9700+int(d/time.Millisecond); ms += 100 {
		now := r.at(time.Duration(ms) * time.Millisecond)
		before := pet.HP
		r.w.mu.Lock()
		r.w.petSystem(now)
		r.w.effectSystem(now)
		r.w.mu.Unlock()
		if pet.HP > before {
			f.heals = append(f.heals, pet.HP-before)
		}
		mine := 0
		for _, e := range r.enemy.Effects {
			if e.pet != pet.ServerID {
				continue
			}
			mine++
			if next, ok := seen[e]; !ok || !next.Equal(e.NextTick) {
				f.ticks = append(f.ticks, ms) // a landing ticks at once, then each tick moves its next
				seen[e] = e.NextTick
			}
		}
		f.most = max(f.most, mine)
		for i, m := range r.sent() {
			if opOfMsg(m) == protocol.OpcodeCastingCombat && binary.LittleEndian.Uint32(m[2:]) == petID {
				f.casts = append(f.casts, petSeen{at: ms, order: i, target: binary.LittleEndian.Uint32(m[6:]),
					ms: binary.LittleEndian.Uint32(m[11:]), fx: binary.LittleEndian.Uint32(m[15:])})
			}
		}
	}
	f.lines = lines.String()
	return f
}

// of is when the pet sent the visual, and with what cast times.
func (f *swarmFight) of(fx uint32) (at []int, ms []uint32) {
	for _, c := range f.casts {
		if c.fx == fx {
			at, ms = append(at, c.at), append(ms, c.ms)
		}
	}
	return at, ms
}

// logged is how many of the log's lines hold s.
func (f *swarmFight) logged(s string) int { return strings.Count(f.lines, s) }

// steps is first, first+every, ... up to last.
func steps(first, every, last int) []int {
	var out []int
	for v := first; v <= last; v += every {
		out = append(out, v)
	}
	return out
}

// checkSwarmKit holds a 110 s fight (the attack at 9.6 s, the clocks from 9.7)
// to the kit: the damage over time cast at 10.7 s in half a second and again
// every 24 s as the one on the crab runs out, landing 1.9 s after its cast, so
// it is never down from its first landing (a tick every 6 s, 300 each) and
// never two; the harm touch at 12.7 s and every 50 s, instant, 2000; the tap at
// 14.7 s and every 30 s, a second's cast, 1000 and the pet healed by it; and
// with the proc, every fourth of its melee hits on the crab standing 250 more,
// seen as a proc is.
func checkSwarmKit(t *testing.T, f *swarmFight, fxPlague, fxLeech, fxTouch uint32, procs bool) {
	t.Helper()
	const last = 9700 + 110000
	for _, c := range []struct {
		name string
		fx   uint32
		ms   uint32
		want []int
	}{
		{"the damage over time", fxPlague, 500, steps(10700, 24000, last)},
		{"the harm touch", fxTouch, 0, steps(12700, 50000, last)},
		{"the tap", fxLeech, 1000, steps(14700, 30000, last)},
	} {
		at, ms := f.of(c.fx)
		if fmt.Sprint(at) != fmt.Sprint(c.want) {
			t.Errorf("%s cast at %v, want %v", c.name, at, c.want)
		}
		for _, m := range ms {
			if m != c.ms {
				t.Errorf("%s's 0x00B1 with a cast of %d ms, want %d", c.name, m, c.ms)
				break
			}
		}
	}
	if want := steps(12600, 6000, last); fmt.Sprint(f.ticks) != fmt.Sprint(want) || f.most != 1 {
		t.Errorf("its damage over time ticked at %v (at most %d at once), want every 6 s from its landing at 12.6 s, one: %v",
			f.ticks, f.most, want)
	}
	for _, c := range []struct {
		line string
		n    int
	}{
		{`"Swarming Plague" ticks on Desert Crab for 300 before`, len(steps(12600, 6000, last))},
		{`lands "Swarming Harm Touch" on Desert Crab for 2000 before`, len(steps(12700, 50000, last))},
		{`lands "Swarming Leech" on Desert Crab for 1000 before`, len(steps(14700, 30000, last))},
	} {
		if n := f.logged(c.line); n != c.n {
			t.Errorf("%d lines %q, want %d", n, c.line, c.n)
		}
	}
	if fmt.Sprint(f.heals) != fmt.Sprint([]int{1000, 1000, 1000, 1000}) {
		t.Errorf("the pet healed %v, want its four taps' 1000 each", f.heals)
	}
	procAt, _ := f.of(fxShieldHit)
	n := f.logged("'s Swarming Sting hits Desert Crab for 250 before")
	switch {
	case procs && (f.dice < 40 || n != f.dice/4 || len(procAt) != n):
		t.Errorf("%d procs (%d seen) of %d melee hits, want a fourth of them", n, len(procAt), f.dice)
	case !procs && (f.dice != 0 || n != 0 || len(procAt) != 0):
		t.Errorf("the control: %d procs (%d seen), %d throws, want none", n, len(procAt), f.dice)
	}
}

// A Swarming Death of the kit fights with all four; the control, its template
// row's "pet necromancer" (no proc), casts the same spells and procs nothing.
// Its damage over time on a clock of 2 s is kept up the same: not cast again
// while one is on its way (1.9 s) or has more than that to run.
func TestASwarmingDeathFightsWithItsKit(t *testing.T) {
	for _, c := range []struct {
		name, template string
		kit            string
		procs          bool
	}{
		{"its kit", "pet swarming death", swarmKit, true},
		{"the control", "pet necromancer", swarmKit, false},
		{"its damage over time every 2 s", "pet swarming death", strings.Replace(swarmKit, `"every":24`, `"every":2`, 1), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := swarmRig(t, c.template, c.kit)
			f := swarmFightFor(t, r, swarmingDeath, 100, 110*time.Second)
			ps := r.w.pets[r.p.ServerID]
			if ps == nil || ps.tmpl == nil || f.pet.sheet == nil || f.pet.Class != 3 || npcSwingKind(f.pet) != 5 {
				t.Fatalf("the pet: template %v, row %v, class %d, swing %d: want its row, a shadow knight swinging a warrior's quad",
					ps != nil && ps.tmpl != nil, f.pet.sheet != nil, f.pet.Class, npcSwingKind(f.pet))
			}
			if want := map[bool]int{true: 25, false: 0}[c.procs]; ps.tmpl.tmpl.ProcChance != want {
				t.Errorf("its template's proc %d%%, want %d", ps.tmpl.tmpl.ProcChance, want)
			}
			checkSwarmKit(t, f, fxSwarmPlague, fxSwarmLeech, fxSwarmTouch, c.procs)
		})
	}
}

// Its damage over time is never two of its owner's on one creature: landed again
// it replaces its first, as one of the owner's pet summoned anew replaces the
// old pet's; another owner's pet's stays beside it. A tick of a pet gone (here
// dismissed) does nothing, and what is left of it runs out.
func TestASwarmingDeathsDoTIsNeverStacked(t *testing.T) {
	r := swarmRig(t, "pet swarming death")
	pet, _ := r.summon(t, swarmingDeath, 0)
	ps := r.w.pets[r.p.ServerID]
	a := &ps.petAbilities()[0]
	if a.Kind != database.AbilityDoT || a.pinned == nil || a.pinned.SpellID != 905100 || a.castTime(a.pinned) != 500*time.Millisecond {
		t.Fatalf("its first ability: %+v", a.TemplateAbility)
	}
	crab := r.enemy
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	other := &ActiveEffect{SpellID: 905100, Name: "Swarming Plague", Caster: 9999, Until: r.at(time.Hour), pet: 8888, HPTick: -300}
	stale := &ActiveEffect{SpellID: 905100, Name: "Swarming Plague", Caster: r.p.ServerID, Until: r.at(time.Hour), pet: 7777, HPTick: -300}
	crab.Effects = []*ActiveEffect{other, stale}
	mine := func() (n int, until time.Time) {
		for _, e := range crab.Effects {
			if e.pet == pet.ServerID {
				n, until = n+1, e.Until
			}
		}
		return n, until
	}
	r.w.mu.Lock()
	r.w.petDoTLocked(ps, crab, a.pinned, a, r.at(20*time.Second))
	r.w.petDoTLocked(ps, crab, a.pinned, a, r.at(30*time.Second))
	r.w.mu.Unlock()
	if n, until := mine(); n != 1 || !until.Equal(r.at(54*time.Second)) || len(crab.Effects) != 2 || crab.Effects[0] != other {
		t.Errorf("after two landings: %d of its own (until %v), %d on the crab, the other owner's kept %v: want one, from the second",
			n, until.Sub(r.t0), len(crab.Effects), crab.Effects[0] == other)
	}
	if lost := 1_000_000 - crab.HP; lost != 600 {
		t.Errorf("the crab lost %d to the two landings' first ticks, want 300 each", lost)
	}
	r.w.mu.Lock()
	r.w.dismissPetLocked(ps, r.sess, "")
	hp := crab.HP
	r.w.effectSystem(r.at(36 * time.Second))
	r.w.mu.Unlock()
	if crab.HP != hp {
		t.Errorf("a tick of a dismissed pet's took %d", hp-crab.HP)
	}
}

// Its damage over time is wanted again only as the one on its target runs out:
// not while a cast of it is on its way there (a 2 s clock is shorter than a
// 1.5 s cast and its 1.4 s landing), not while the one there has more than its
// cast's time and landing to run, and then yes, so the new one lands as the
// old one ends.
func TestASwarmingDeathsDoTIsWantedAsItRunsOut(t *testing.T) {
	r := swarmRig(t, "pet swarming death", strings.Replace(swarmKit, `"every":24,"first":1,"cast":0.5`, `"every":2,"first":1,"cast":1.5`, 1))
	r.summon(t, swarmingDeath, 0)
	ps := r.w.pets[r.p.ServerID]
	a := &ps.petAbilities()[0]
	crab := r.enemy
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	ps.target = crab.ServerID
	at := func(ms int) time.Time { return r.at(time.Duration(ms) * time.Millisecond) }
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	if !r.w.petWantsLocked(ps, a, at(20000)) || !r.w.petCastLocked(ps, 0, crab, at(20000)) {
		t.Fatal("its damage over time not wanted, or not cast, on a crab without it")
	}
	for _, c := range []struct {
		ms   int
		want bool
		why  string
	}{
		{22000, false, "its cast on its way (it lands at 22.9 s)"},
		{22900, false, "landed: 24 s to run"},
		{43999, false, "a hair over 2.9 s to run, its cast's time and landing"},
		{44000, true, "2.9 s to run: the new one lands as it ends"},
	} {
		if c.ms == 22900 {
			r.w.petCastSystemLocked(at(c.ms))
		}
		if got := r.w.petWantsLocked(ps, a, at(c.ms)); got != c.want {
			t.Errorf("at %d ms (%s): wanted %v", c.ms, c.why, got)
		}
	}
}

// Its tap heals it by what it took: the tap's 1000 after the balance's con on
// a red 8 over its owner (45% of it lands) heals 450, where it healed the whole
// figure before; on an even con the whole 1000.
func TestASwarmingDeathsTapHealsWhatLanded(t *testing.T) {
	for _, c := range []struct {
		level, want int
	}{{68, 450}, {60, 1000}} {
		r := swarmRig(t, "pet swarming death")
		r.w.SetBalance(NewBalance([]database.BalanceLevel{{Level: 60, HPPct: 100, DamagePct: 100, ConPct: 100, PetHPPct: 100,
			PetDamagePct: 100}}, []database.BalanceCon{{Diff: 0, DamagePct: 100, TakenPct: 100}, {Diff: 8, DamagePct: 100, TakenPct: 45}}))
		pet, _ := r.summon(t, swarmingDeath, 0)
		ps := r.w.pets[r.p.ServerID]
		crab := r.enemy
		crab.Level, crab.HP, crab.MaxHP = c.level, 1_000_000, 1_000_000
		pet.HP = 100
		a := &ps.petAbilities()[2]
		r.w.mu.Lock()
		r.w.petLandLocked(&petCast{ps: ps, tmpl: ps.tmpl, a: a, spell: a.pinned, target: crab.ServerID}, r.at(20*time.Second))
		r.w.mu.Unlock()
		if lost, healed := 1_000_000-crab.HP, pet.HP-100; lost != c.want || healed != c.want {
			t.Errorf("a level %d crab: lost %d, the pet healed %d; want %d both", c.level, lost, healed, c.want)
		}
	}
}

// Its proc is cut by a creature strong in its kind (arcane), as its spells are:
// a creature of a template resisting AR 50% takes 125 of the 250.
func TestASwarmingDeathsProcIsResistedAsItsSpells(t *testing.T) {
	r := swarmRig(t, "pet swarming death")
	r.summon(t, swarmingDeath, 0)
	ps := r.w.pets[r.p.ServerID]
	crab := r.enemy
	crab.HP, crab.MaxHP = 1_000_000, 1_000_000
	crab.tmpl = &ClassTemplate{ClassTemplate: database.ClassTemplate{Name: "warded", Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100,
		ResistKind: "AR", ResistPct: 50}}
	dice := templateIntn
	templateIntn = func(int) int { return 0 }
	defer func() { templateIntn = dice }()
	r.w.mu.Lock()
	killed := r.w.petTemplateProcLocked(ps, crab, r.at(20*time.Second))
	r.w.mu.Unlock()
	if killed || 1_000_000-crab.HP != 125 {
		t.Errorf("the proc took %d (killed %v), want 125", 1_000_000-crab.HP, killed)
	}
}

// A creature template's ability may cast in its own time too: the necromancer's
// tap (Siphon Life, a 2 s record) cast in half a second, its 0x00B1 saying so
// and its hit landing 1.9 s after its cast began.
func TestATemplateAbilityCastsInItsOwnTime(t *testing.T) {
	r := newTemplateRig(t)
	if _, err := r.db.Exec(`INSERT INTO classtemplates (name, class, abilities, note) VALUES ('quick necromancer', 11,
		'[{"kind":"lifetap","every":5,"first":1,"cast":0.5}]', 'test')`); err != nil {
		t.Fatal(err)
	}
	r.assign("gnoll necro", "quick necromancer")
	necro := r.npc("Gnoll Necro")
	p, sess := r.player("Hero", 101.5, 100)
	r.fight(necro, p.ServerID)
	var castAt, landAt time.Duration = -1, -1
	for d := time.Duration(0); d <= 5*time.Second && landAt < 0; d += 100 * time.Millisecond {
		r.tick(d)
		msgs := drain(sess)
		for _, c := range castsIn(msgs) {
			if c.caster == r.objectID(necro.ServerID) && c.fx == fxSiphon {
				if c.ms != 500 {
					t.Errorf("the tap's 0x00B1 says %d ms, want 500", c.ms)
				}
				if castAt < 0 {
					castAt = d
				}
			}
		}
		for _, l := range linesOf(msgs) {
			if strings.Contains(l, "Siphon Life hits you") && landAt < 0 {
				landAt = d
			}
		}
	}
	if castAt < 0 || landAt-castAt != 1900*time.Millisecond {
		t.Errorf("cast at %v, landed at %v: want 1.9 s after", castAt, landAt)
	}
}
