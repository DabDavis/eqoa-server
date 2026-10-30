package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The balance's seed rows (database/balanceseed.go) held to the user's
// targets of 2026-10-29 ("yellows (1-2 levels higher) should always be a tough
// fight. Reds (3+ levels) are possible, but definitely should not be
// trivial"), on fights the World's own systems fight (balancesim.go) with the
// dice seeded. With no database the players are synthetic: a tank (a warrior,
// sword and shield) and a melee class (a rogue, two weapons) of each level,
// their training points spent, in the middling gear of their last ten levels
// as eqoaclassic's items give it (AC about 20 and 14 a level, a weapon of 10
// DMG a level, read off the kits of balancekit.go), fighting hand to hand and
// nothing else. The same fights on a server's own kits, spells and pets, class
// by class, are balancesim_real_test.go's (EQOA_REAL_DB).

// simKit is a synthetic player of the archetype at the level, with two
// abilities standing in for its class's book (what the real books give a
// warrior or a rogue by the level, roughly): a combat art (cast at once, every
// eight seconds, a quarter of the level squared and a fifth of its STR: the
// real arts climb from a Kick's 16 to a Boot's 522 every six seconds at 51)
// and a lasting buff (AC 7 and hit points 10 a level).
func simKit(class, level int) SimPlayer {
	pl := SimPlayer{Class: class, Race: 0, Level: level, Base: SimBaseStats(class, 0, level),
		Play: SimPlay{Melee: true, Buffs: true, Spells: true}}
	art := ParseSpellText(fmt.Sprintf("Target, Decrease Hitpoints by %d ; Increase 20%% based on STR", max(level*level/4, 5)))
	buff := ParseSpellText(fmt.Sprintf("Self, Increase AC by %d, Increase Maximum HP by %d, 30min", 7*level, 10*level))
	pl.Spells = []CastSpell{
		{SpellID: 9101, DisplayIndex: 1, Level: level, Scope: 1, Name: "an art", Recast: 8 * time.Second, PowerCost: level, Range: 10, Def: art},
		{SpellID: 9102, DisplayIndex: 2, Level: level, Scope: 0, Name: "a buff", CastTime: time.Second, PowerCost: level, Def: buff},
	}
	// AC as the middling kits wear it (a warrior's 63 at 10, 524 at 30, 1182
	// at 60; a rogue's 45, 356 and 880), and their stats from about 10 (by 60
	// STR 170, STA 100, AGI 180-350, DEX 210, HP 275-400)
	ac := max(20*level-140, 5*level)
	if archetypeOf(class) != archTank {
		ac = max(15*level-100, 4*level)
	}
	g := max(level-10, 0)
	pl.Gear = Gear{WeaponCond: 1, WeaponDamage: max(10*level, 10), AC: ac, MaxHP: 5 * level,
		Str: 3 * g, Sta: 2 * g, Agi: 5 * g, Dex: 3 * g}
	pl.Worn[14], pl.AttackType = true, 1
	if archetypeOf(class) == archTank {
		pl.Worn[17] = true
	} else {
		pl.Worn[15] = true
	}
	return pl
}

// simTarget is what a con's fights must come to: the share won at least and
// at most, and the hit points left in a fight won at most (a fight won with
// more is trivial). 0 and 1: no bound.
type simTarget struct {
	minWon, maxWon, maxHP float64
}

var simTargets = map[int]simTarget{
	0: {minWon: 0.95, maxWon: 1, maxHP: 0.92}, // even: won, comfortably
	3: {minWon: 0, maxWon: 0.35, maxHP: 1},    // red: a lone player mostly loses
	5: {minWon: 0, maxWon: 0.10, maxHP: 1},    // a deep red: a group's
}

// simMisses is every cell of the archetypes' fights under b that misses its
// target, as lines. The stand-ins are archetypes, not classes: the class
// rows are left out (the classes' shares are fitted on the real kits, which
// balancesim_real_test.go holds to the user's yellow and red targets of
// 2026-10-30); here the level and con tables are held to the shape: an even
// fight won, a red three or five up a lone player's loss.
func simMisses(b *Balance, fights int) []string {
	if b != nil {
		b = b.WithClasses(nil).WithClassLevels(nil)
	}
	var miss []string
	for _, class := range []int{classWarrior, 6} {
		for _, level := range []int{10, 20, 30, 40, 50, 60} {
			pl := simKit(class, level)
			for _, diff := range []int{0, 3, 5} {
				c := SimCellOf(pl, SimNPC{}, diff, b, fights, int64(100*level+diff), 10*time.Minute)
				t := simTargets[diff]
				won := c.WinShare()
				if won < t.minWon || won > t.maxWon || (c.Won > 0 && c.HPLeft > t.maxHP) {
					miss = append(miss, fmt.Sprintf("%s L%d +%d: won %.0f%%, %.0f%% HP left, %.0f s",
						ArchetypeName(class), level, diff, 100*won, 100*c.HPLeft, c.Secs))
				}
			}
		}
	}
	return miss
}

// simKnownGaps are the cells the seed rows miss, each with its cause
// (claude-notes/balance-20261029.md): none under the shape of 2026-10-30.
var simKnownGaps = map[string]string{}

// The seed rows meet the targets, archetype by archetype, but for the gaps
// named.
func TestBalanceSeedMeetsTheTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("fights a few thousand fights")
	}
	for _, m := range simMisses(SeedBalance(), 24) {
		key := m[:strings.Index(m, ":")]
		if why, known := simKnownGaps[key]; known {
			t.Logf("known gap: %s (%s)", m, why)
			continue
		}
		t.Error(m)
	}
}

// The control: the server as it was, no balance, misses them (every NPC a
// pushover: a red five levels up beaten with most of the hit points left).
func TestBalanceNoneMissesTheTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("fights a few thousand fights")
	}
	miss := simMisses(nil, 8)
	if len(miss) < 10 {
		t.Errorf("without the balance only %d cells miss the targets: %v", len(miss), miss)
	}
}

// One fight is the same fight for the same seed: the simulator's dice are
// its own.
func TestSimulatedFightsRepeat(t *testing.T) {
	pl := simKit(classWarrior, 30)
	a := SimulateFight(pl, SimNPC{Level: 32}, SeedBalance(), 7, 10*time.Minute)
	b := SimulateFight(pl, SimNPC{Level: 32}, SeedBalance(), 7, 10*time.Minute)
	if a != b {
		t.Errorf("one seed, two fights: %+v and %+v", a, b)
	}
	if !a.Won && !a.Died {
		t.Errorf("a fight that ended neither way: %+v", a)
	}
}

// The simulator's charm play (SimPlay.Charm): an enchanter's charm takes a
// creature of its level less one and the fight is the charmed creature's;
// the control: a creature past the charm's cap is no one's to charm.
func TestSimulatedCharmFightsWithTheCreature(t *testing.T) {
	pl := SimPlayer{Class: classEnchanter, Race: 0, Level: 20, Base: SimBaseStats(classEnchanter, 0, 20),
		Play: SimPlay{Spells: true, Charm: true, CharmDiff: -1}}
	pl.Spells = []CastSpell{{SpellID: 700173, DisplayIndex: 1, Level: 20, Scope: 1, Name: "Cajole", CastTime: 5 * time.Second,
		Recast: 30 * time.Second, PowerCost: 175, Range: 30, Def: ParseSpellText("Target, Charm ; Increase 25% based on CHA, 2.5min")}}
	if !SimCharmable(pl, -1) || SimCharmable(pl, 1) {
		t.Fatalf("charmable: -1 %v, +1 %v (the charm takes level 20 and below)", SimCharmable(pl, -1), SimCharmable(pl, 1))
	}
	r := SimulateFight(pl, SimNPC{Level: 18}, SeedBalance(), 3, 10*time.Minute)
	if !r.HadPet || r.PetLevel != 19 {
		t.Errorf("no charmed creature fought: %+v", r)
	}
}

// A group's fight (balancegroup.go): three of the synthetic warrior beat an
// NPC five levels up that one alone mostly loses to, and the control: one
// "group" of a single member fights as a lone player does (the same share
// won over the same seeds).
func TestAGroupBeatsWhatOneCannot(t *testing.T) {
	if testing.Short() {
		t.Skip("fights a few hundred fights")
	}
	b := SeedBalance()
	pl := simKit(classWarrior, 30)
	alone := SimCellOf(pl, SimNPC{}, 5, b, 16, 7, 10*time.Minute)
	trio := SimGroupCellOf([]SimPlayer{pl, pl, pl}, SimNPC{}, 5, b, 16, 7, 10*time.Minute)
	if trio.WinShare() <= alone.WinShare() || trio.WinShare() < 0.75 {
		t.Errorf("+5: alone won %.0f%%, three won %.0f%%", 100*alone.WinShare(), 100*trio.WinShare())
	}
	one := SimGroupCellOf([]SimPlayer{pl}, SimNPC{}, 2, b, 12, 3, 10*time.Minute)
	solo := SimCellOf(pl, SimNPC{}, 2, b, 12, 3, 10*time.Minute)
	if d := one.WinShare() - solo.WinShare(); d > 0.2 || d < -0.2 {
		t.Errorf("a group of one won %.0f%%, the lone player %.0f%%", 100*one.WinShare(), 100*solo.WinShare())
	}
}

// The low point (SimResult.LowHP, the user's "take you down to"): a fight
// is never ended below it; a stand-in that heals itself (a heal of a third
// of its hit points, cast below half) ends its fights above the low point
// it was taken down to; the control: the same stand-in without the heal
// ends each fight it wins at its low point or within a tick's blow of it.
func TestSimLowPointIsTheFightsLeast(t *testing.T) {
	b := SeedBalance().WithClasses(nil).WithClassLevels(nil)
	plain := simKit(classWarrior, 20)
	healer := simKit(classWarrior, 20)
	heal := ParseSpellText("Self, Increase Hitpoints by 200")
	healer.Spells = append(healer.Spells, CastSpell{SpellID: 9103, DisplayIndex: 3, Level: 20, Scope: 0, Name: "a heal",
		CastTime: time.Second, Recast: 12 * time.Second, PowerCost: 20, Def: heal})
	var healed, ended, plainWon int
	for seed := int64(1); seed <= 12; seed++ {
		r := SimulateFight(healer, SimNPC{Level: 21}, b, seed, 10*time.Minute)
		if r.LowHP > r.HP || r.LowShare() > r.HPShare() {
			t.Fatalf("seed %d: ended at %d below its low point %d", seed, r.HP, r.LowHP)
		}
		if r.Won {
			ended++
			if r.HP > r.LowHP {
				healed++
			}
		}
		p := SimulateFight(plain, SimNPC{Level: 21}, b, seed, 10*time.Minute)
		if p.Won {
			plainWon++
			if p.HP-p.LowHP > p.MaxHP/10 {
				t.Errorf("seed %d, no heal: ended at %d, low point %d of %d", seed, p.HP, p.LowHP, p.MaxHP)
			}
		}
	}
	if ended == 0 || plainWon == 0 || healed*2 < ended {
		t.Errorf("the healer won %d and ended above its low point in %d; without the heal %d won", ended, healed, plainWon)
	}
}

// A fitting group (SimGroupFor): a duo is the class and the role it lacks,
// a cleric for a tank or a melee class, a warrior for a priest or a caster;
// a trio a warrior, the class and a cleric; the control: a warrior's or a
// cleric's trio takes a wizard for the class's own place.
func TestAFittingGroupFillsTheRoleItLacks(t *testing.T) {
	kit := func(c int) (SimPlayer, error) { return SimPlayer{Class: c}, nil }
	for _, c := range []struct {
		class, size int
		want        []int
	}{
		{6, 2, []int{6, 9}}, {2, 2, []int{2, 9}}, {13, 2, []int{0, 13}}, {10, 2, []int{0, 10}}, {7, 2, []int{0, 7}},
		{9, 2, []int{0, 9}}, {6, 3, []int{0, 6, 9}}, {13, 3, []int{0, 13, 9}}, {0, 3, []int{0, 13, 9}}, {9, 3, []int{0, 13, 9}},
	} {
		g, err := SimGroupFor(c.class, 20, c.size, kit)
		if err != nil {
			t.Fatal(err)
		}
		var got []int
		for _, pl := range g {
			got = append(got, pl.Class)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("class %d, %d: %v, want %v", c.class, c.size, got, c.want)
		}
	}
}

// The simulator's foe fights by a class template (SimNPC.Template, read off a
// database by SimTemplatesFromDB): a roamer's crit every 25-35 s from 10-20 s
// in, 7.4 a level, fixed and through armour, lands on the simulated player at
// its clock (the fight's log names each), so a long fight costs the player
// more than the same fight with no template, on the same dice; the control:
// with no template the fight is the one before (no crit in its log).
func TestASimulatedFoeFightsByItsTemplate(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/sim.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tmpl, err := SimTemplatesFromDB(db) // the seeded roamer (database.SeedClassTemplates)
	if err != nil {
		t.Fatal(err)
	}
	roamer := tmpl["roamer"]
	if roamer == nil || roamer.SpecialEvery != 30 {
		t.Fatalf("the roamer read: %+v", roamer)
	}
	b := SeedBalance().WithClasses(nil).WithClassLevels(nil)
	pl := simKit(classWarrior, 40)
	var plain, crits strings.Builder
	fight := func(foe SimNPC, to *strings.Builder) SimResult {
		simLogTo = to
		defer func() { simLogTo = nil }()
		return SimulateFight(pl, foe, b, 11, 10*time.Minute)
	}
	without := fight(SimNPC{Level: 40}, &plain)
	with := fight(SimNPC{Level: 40, Template: roamer}, &crits)
	n := strings.Count(crits.String(), "Critical Hit hits Simmer for 296")
	if n == 0 || !with.Won || !without.Won {
		t.Fatalf("crits of 296 (7.4 x 40) in the templated fight: %d; won %v / %v", n, with.Won, without.Won)
	}
	if want := int(without.Time/(30*time.Second)) - 1; n < want || n > int(with.Time/(25*time.Second))+1 {
		t.Errorf("%d crits in a fight of %v (want one every 25-35 s from 10-20 s in)", n, with.Time)
	}
	if strings.Contains(plain.String(), "Critical Hit") {
		t.Errorf("the control: a crit with no template")
	}
	if with.HPShare() >= without.HPShare() {
		t.Errorf("hit points left: %.2f with the crits, %.2f without", with.HPShare(), without.HPShare())
	}
}

// A simulated foe's template leaves the pets their own: the world a fight is
// set in keeps the kit's pet templates and taps (SimPetsFromDB: the seeded
// pettemplate rows) beside the roamer it gives the creature. Until 2026-10-30
// the foe's template replaced every template of that world, and a pet against
// a roamer fought by none ("no template" in the summoning's line).
func TestASimulatedFoesTemplateKeepsThePetsOwn(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/sim.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tmpl, err := SimTemplatesFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	pets, err := SimPetsFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(pets.pets) == 0 {
		t.Fatal("no pet templates read off a new database: the check tells nothing")
	}
	pl := simKit(11, 40)
	pl.Pets = pets
	for _, foe := range []SimNPC{{Level: 40}, {Level: 40, Template: tmpl["roamer"]}} {
		f := newSimFight(pl, foe, SeedBalance())
		if got := len(f.w.templates.pets); got != len(pets.pets) {
			t.Errorf("foe template %v: the world has %d pet templates, want the kit's %d", foe.Template != nil, got, len(pets.pets))
		}
		if foe.Template != nil && f.npc.TemplateName() != "roamer" {
			t.Errorf("the foe fights by %q, want the roamer", f.npc.TemplateName())
		}
	}
}
