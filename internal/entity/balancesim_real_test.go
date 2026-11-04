package entity

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// simRealDB opens a copy of the server database EQOA_REAL_DB names, or
// skips.
func simRealDB(t *testing.T) *database.SQLiteDB {
	t.Helper()
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/sim.db"
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestRealBalanceProbe prints a few fights, for looking at by hand
// (EQOA_SIM_PROBE=1).
func TestRealBalanceProbe(t *testing.T) {
	if os.Getenv("EQOA_SIM_PROBE") == "" {
		t.Skip("EQOA_SIM_PROBE=1 prints sample fights")
	}
	db := simRealDB(t)
	for _, class := range []int{0, 6, 9, 13, 11, 10} {
		for _, level := range []int{5, 15, 30, 45, 60} {
			pl, err := SimKitFromDB(db, class, 0, level, 0.5)
			if err != nil {
				t.Fatal(err)
			}
			if class == 11 || class == 10 {
				pl.Play.Pet = true
				pl.Play.Melee = false
			}
			line := fmt.Sprintf("class %2d L%2d DMG %3d AC %4d spells %3d:", class, level, pl.Gear.WeaponDamage, pl.Gear.AC, len(pl.Spells))
			for d := 0; d <= 5; d++ {
				won, hp, secs, n := 0, 0.0, 0.0, 40
				for s := range n {
					r := SimulateFight(pl, SimNPC{Level: level + d}, nil, int64(s+1), 10*time.Minute)
					if r.Won {
						won++
						hp += r.HPShare()
					}
					secs += r.Time.Seconds()
				}
				line += fmt.Sprintf(" +%d %3.0f%% %3.0f%%hp %4.0fs |", d, 100*float64(won)/float64(n), 100*hp/float64(max(won, 1)), secs/float64(n))
			}
			t.Log(line)
		}
	}
}

// TestRealBalanceOneFight logs one fight as the server logs it
// (EQOA_SIM_ONE=class,level,npclevel[,play[,charmdiff]]: play 1 with its
// pet, 2 rooting, 3 with a creature charmdiff levels from its own charmed).
func TestRealBalanceOneFight(t *testing.T) {
	spec := os.Getenv("EQOA_SIM_ONE")
	if spec == "" {
		t.Skip("EQOA_SIM_ONE=class,level,npclevel")
	}
	var class, level, foe, pet, charm int
	fmt.Sscanf(spec, "%d,%d,%d,%d,%d", &class, &level, &foe, &pet, &charm)
	db := simRealDB(t)
	pl, err := SimKitFromDB(db, class, 0, level, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	switch pet {
	case 1:
		pl.Play.Pet, pl.Play.Melee = true, false
	case 2:
		pl.Play.Root, pl.Play.Melee = true, false // a rooting caster
	case 3:
		pl.Play.Charm, pl.Play.CharmDiff, pl.Play.Melee = true, charm, false // a creature charmed
	}
	for _, s := range pl.Spells {
		t.Logf("book: L%d %q role %d cast %v recast %v power %d dmg %.0f heal %.0f", s.Level, s.Name, simRole(&s), s.CastTime, s.Recast, s.PowerCost, simDamage(&s, &Player{}), simHeal(&s, &Player{}))
	}
	simLogTo = os.Stderr
	defer func() { simLogTo = nil }()
	r := SimulateFight(pl, SimNPC{Level: foe}, SeedBalance(), 1, 10*time.Minute)
	t.Logf("%+v", r)
}

// TestRealBalanceGearCurve prints the kits' gear by level (EQOA_SIM_GEAR=1):
// the synthetic outfits of balancesim_test.go are read off it.
func TestRealBalanceGearCurve(t *testing.T) {
	if os.Getenv("EQOA_SIM_GEAR") == "" {
		t.Skip("EQOA_SIM_GEAR=1")
	}
	db := simRealDB(t)
	for _, class := range []int{0, 6, 9, 13} {
		for level := 1; level <= 60; level += 1 {
			if level%5 != 0 && level != 1 {
				continue
			}
			pl, err := SimKitFromDB(db, class, 0, level, 0.5)
			if err != nil {
				t.Fatal(err)
			}
			g := pl.Gear
			t.Logf("class %2d L%2d DMG %3d 2H %v AC %4d STR %3d STA %3d AGI %3d DEX %3d WIS %3d INT %3d HP %3d POW %3d HoT %v PoT %v worn %v type %d",
				class, level, g.WeaponDamage, g.WeaponTwoHanded, g.AC, g.Str, g.Sta, g.Agi, g.Dex, g.Wis, g.Int, g.MaxHP, g.MaxPower, g.HoTItems, g.PoTItems,
				[]bool{pl.Worn[14], pl.Worn[15], pl.Worn[16], pl.Worn[17], pl.Worn[20]}, pl.AttackType)
		}
	}
}

// On a server's own kits, books and pets (EQOA_REAL_DB): the seed rows make
// the warrior and the paladin meet the targets at 30 and 50 (an even won, a +2
// a tough fight, a +5 lost more than won), and the control, no balance, makes
// a +5 a pushover for them. The monk is left out: its self heals and arts beat
// its yellows at 30 with three quarters of its hit points (a gap the balance
// notes name).
func TestRealBalanceSeedOnRealKits(t *testing.T) {
	db := simRealDB(t)
	for _, class := range []int{classWarrior, 2} {
		for _, level := range []int{30, 50} {
			pl, err := SimKitFromDB(db, class, 0, level, 0.5)
			if err != nil {
				t.Fatal(err)
			}
			even := SimCellOf(pl, SimNPC{}, 0, SeedBalance(), 16, 1, 10*time.Minute)
			yellow := SimCellOf(pl, SimNPC{}, 2, SeedBalance(), 16, 1, 10*time.Minute)
			red := SimCellOf(pl, SimNPC{}, 5, SeedBalance(), 16, 1, 10*time.Minute)
			if even.WinShare() < 0.95 || (yellow.Won > 0 && yellow.HPLeft > 0.75) || red.WinShare() > 0.7 {
				t.Errorf("class %d L%d under the seed: even won %.0f%%, +2 %.0f%% HP left, +5 won %.0f%%",
					class, level, 100*even.WinShare(), 100*yellow.HPLeft, 100*red.WinShare())
			}
			was := SimCellOf(pl, SimNPC{}, 5, nil, 8, 1, 10*time.Minute)
			if was.WinShare() < 0.95 || was.HPLeft < 0.6 {
				t.Errorf("class %d L%d with no balance: +5 won %.0f%% with %.0f%% HP left (the control)", class, level,
					100*was.WinShare(), 100*was.HPLeft)
			}
		}
	}
}

// On a server's own kits (EQOA_REAL_DB): a rooting druid of 50 (root, then
// its spells from range) loses a red five up under the seed's root chances
// and, with the chances at 0 (the game before them, every root held to its
// end), wins it nearly always given the time (15-20 minutes under the con
// rows of 2026-10-30, so the control's fights may last 30); a rooting wizard
// of 50 pays for a yellow two up under the chances, and with none wins it
// nine times in ten or more with half its hit points or more (the control:
// untouched under the old Arcane Shackles, which held 50 s from a 1 s cast;
// the workbook's end at a hit, so the wizard roots with its Arcane
// Bindings-Wiz, 36 s from a 3 s cast, and a re-root costs it a blow or two,
// more under the re-fit's share of 2026-11-01: 94% won with 53% left, where
// the chances leave it 38%). A wizard's roots do not take a red at all.
func TestRealRootPlayHeldToTheCon(t *testing.T) {
	db := simRealDB(t)
	none := SeedBalance().WithConColumn([][2]float64{{0, 0}}, func(r *database.BalanceCon, v float64) {
		r.RootResistPct, r.RootBreakPct = v, v
	})
	kit := func(class int) SimPlayer {
		pl, err := SimKitFromDB(db, class, 0, 50, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		pl.Play.Root, pl.Play.Melee = true, false
		return pl
	}
	druid := kit(7)
	if red := SimCellOf(druid, SimNPC{}, 5, SeedBalance(), 16, 1, 10*time.Minute); red.WinShare() > 0.4 {
		t.Errorf("a druid rooting under the seed: +5 won %.0f%%", 100*red.WinShare())
	}
	if was := SimCellOf(druid, SimNPC{}, 5, none, 8, 1, 30*time.Minute); was.WinShare() < 0.85 {
		t.Errorf("a druid rooting with no root chances: +5 won only %.0f%% (the control)", 100*was.WinShare())
	}
	wizard := kit(13)
	paid := SimCellOf(wizard, SimNPC{}, 2, SeedBalance(), 16, 1, 10*time.Minute)
	free := SimCellOf(wizard, SimNPC{}, 2, none, 16, 1, 10*time.Minute)
	if free.WinShare() < 0.9 || free.HPLeft < 0.5 {
		t.Errorf("a wizard rooting with no root chances: +2 won %.0f%% with %.0f%% left (the control)", 100*free.WinShare(), 100*free.HPLeft)
	}
	if paid.WinShare() > 0.9 && paid.HPLeft > 0.75 {
		t.Errorf("a wizard rooting under the seed: +2 won %.0f%% with %.0f%% left, as if its roots held", 100*paid.WinShare(), 100*paid.HPLeft)
	}
}

// TestRealCharmTemplateProbe prints an enchanter's fights with a creature it
// charmed at its cap (the highest level its book's charms take), fighting by
// each charm template the database has (tools/seed_npc_charm.py: "charm
// tree", ...) and as itself, against creatures of its own level and two and
// three above: won, died and unfinished in ten minutes, the seconds of the
// wins and the enchanter's hit points left; beside them its own summoned pet's
// fights, for the measure; the enchanter casts nothing unless
// EQOA_SIM_CHARM_CASTS is set (EQOA_SIM_CHARM=templates; EQOA_SIM_CHARM_LEVELS,
// EQOA_SIM_CHARM_ONLY a template's name).
func TestRealCharmTemplateProbe(t *testing.T) {
	if os.Getenv("EQOA_SIM_CHARM") != "templates" {
		t.Skip("EQOA_SIM_CHARM=templates prints charmed creatures' fights by the charm templates")
	}
	db := simRealDB(t)
	tmpls, err := SimTemplatesFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range tmpls {
		if strings.HasPrefix(n, "charm ") && (os.Getenv("EQOA_SIM_CHARM_ONLY") == "" || n == os.Getenv("EQOA_SIM_CHARM_ONLY")) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	levels := []int{40, 45, 50, 55, 60}
	if l := os.Getenv("EQOA_SIM_CHARM_LEVELS"); l != "" {
		levels = nil
		for _, f := range strings.Split(l, ",") {
			n, _ := strconv.Atoi(f)
			levels = append(levels, n)
		}
	}
	cell := func(k SimPlayer, level int) string {
		line := ""
		for _, d := range []int{0, 2, 3} {
			won, died, open, n := 0, 0, 0, 12
			var secs, left float64
			for s := range n {
				r := SimulateFight(k, SimNPC{Level: level + d}, SeedBalance(), int64(s+1), 10*time.Minute)
				switch {
				case r.Won:
					won++
					secs += r.Time.Seconds()
					left += float64(r.HP) / float64(max(r.MaxHP, 1))
				case r.Died:
					died++
				default:
					open++
				}
			}
			line += fmt.Sprintf(" +%d won %2d died %2d open %2d %4.0fs hp %3.0f%% |", d, won, died, open,
				secs/float64(max(won, 1)), 100*left/float64(max(won, 1)))
		}
		return line
	}
	for _, level := range levels {
		pl, err := SimKitFromDB(db, classEnchanter, 0, level, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		cd := 0
		for d := -1; d >= -20; d-- {
			if SimCharmable(pl, d) {
				cd = d
				break
			}
		}
		if cd == 0 {
			continue
		}
		// the enchanter stands by: the pet's fight alone (its own casts would
		// draw the creature off a pet whose hate comes in a crit's jumps:
		// keepsHate's 80% reads a spell's damage, not its hate)
		pl.Play.Spells = os.Getenv("EQOA_SIM_CHARM_CASTS") != ""
		own := pl
		own.Play.Pet, own.Play.Melee = true, false
		t.Logf("L%2d its own pet:              %s", level, cell(own, level))
		for _, n := range append([]string{""}, names...) {
			k := pl
			k.Play.Charm, k.Play.CharmDiff, k.Play.Melee = true, cd, false
			label := "as itself"
			if n != "" {
				k.Play.CharmTemplate, label = tmpls[n], n
			}
			t.Logf("L%2d charm %+d %-20s %s", level, cd, label, cell(k, level))
		}
	}
}

// TestRealCharmProbe prints fights with a charmed creature, for the report
// on charm under the balance (EQOA_SIM_CHARM=1): won, died and unfinished
// after ten minutes, the seconds of the fights decided, and the NPC's hit
// points left in the fights not won.
func TestRealCharmProbe(t *testing.T) {
	if os.Getenv("EQOA_SIM_CHARM") == "" {
		t.Skip("EQOA_SIM_CHARM=1 prints fights with a charmed creature")
	}
	db := simRealDB(t)
	for _, class := range []int{12, 11} {
		for _, level := range []int{20, 30, 40, 50, 55, 60} {
			pl, err := SimKitFromDB(db, class, 0, level, 0.5)
			if err != nil {
				t.Fatal(err)
			}
			for _, cd := range []int{-5, -3, -1, 0, 3} {
				if !SimCharmable(pl, cd) {
					continue
				}
				k := pl
				k.Play.Charm, k.Play.CharmDiff, k.Play.Melee = true, cd, false
				line := fmt.Sprintf("class %2d L%2d charm %+d:", class, level, cd)
				for _, d := range []int{0, 2, 3, 5} {
					won, died, open, n := 0, 0, 0, 12
					var secs, left float64
					for s := range n {
						r := SimulateFight(k, SimNPC{Level: level + d}, SeedBalance(), int64(s+1), 10*time.Minute)
						switch {
						case r.Won:
							won++
							secs += r.Time.Seconds()
						case r.Died:
							died++
							left += float64(r.NPCHP) / float64(max(r.NPCMaxHP, 1))
						default:
							open++
							left += float64(r.NPCHP) / float64(max(r.NPCMaxHP, 1))
						}
					}
					line += fmt.Sprintf(" +%d won %2d died %2d open %2d %4.0fs npc %3.0f%% |", d, won, died, open,
						secs/float64(max(won, 1)), 100*left/float64(max(died+open, 1)))
				}
				t.Log(line)
			}
		}
	}
}

// balanceAt0130 is the balance as eqoaclassic had it at 01:30 UTC on
// 2026-10-30 (the tuning of 2026-10-29 with pets' hit points at seven
// tenths), which the user played and called too easy.
func balanceAt0130() *Balance {
	lv := func(l int, hp, dmg, con, php, pdm float64) database.BalanceLevel {
		return database.BalanceLevel{Level: l, HPPct: hp, DamagePct: dmg, ConPct: con, PetHPPct: php, PetDamagePct: pdm,
			ArtCooldownMS: 1500, RegenPct: 5}
	}
	cn := func(d int, dmg, tk, rr, rb float64) database.BalanceCon {
		return database.BalanceCon{Diff: d, DamagePct: dmg, TakenPct: tk, RootResistPct: rr, RootBreakPct: rb}
	}
	return NewBalance([]database.BalanceLevel{
		lv(1, 300, 250, 0, 300, 250), lv(5, 215, 265, 0, 215, 265), lv(8, 225, 260, 0, 225, 260), lv(10, 220, 225, 25, 220, 225),
		lv(15, 230, 255, 70, 161, 280), lv(18, 230, 250, 100, 161, 300), lv(25, 250, 220, 100, 175, 285), lv(30, 300, 200, 100, 210, 300),
		lv(35, 300, 195, 100, 210, 340), lv(45, 330, 190, 100, 287, 430), lv(50, 340, 190, 100, 357, 475), lv(60, 400, 160, 100, 560, 560),
	}, []database.BalanceCon{cn(0, 100, 100, 0, 5), cn(1, 112, 91, 5, 10), cn(2, 125, 83, 15, 15), cn(3, 135, 79, 35, 25),
		cn(4, 142, 76, 50, 30), cn(5, 150, 72, 65, 35)}).
		WithClasses([]database.BalanceClass{{Class: 1, MeleeTakenPct: 70}, {Class: 5, MeleeTakenPct: 80}, {Class: 6, MeleeTakenPct: 65}})
}

// The calibration (EQOA_REAL_DB): the live log the user called "too easy",
// Dust, a level 5 shadow knight of 170 hit points, against a level 6 fire
// bat (786 hit points, hits of 8 every 3 s): he killed it in 24 s and kept
// 65%. The simulator's level 5 shadow knight, its book only Chilling Aura
// (Dust cast no nuke: 786 in 24 s of ~85 a swing), under the 01:30 tables
// fights the same fight (the bat's 786, its 8 a hit) to the same end; under
// the seed of 2026-10-30 the yellow ended under 40% of its hit points, and
// was still won as a rule. Since the new players' curve (2026-11-04, the
// user: "Lets make it easier for new players, 1-5 mobs should really be easy
// and 6-9 little tougher") a creature of 6 is a new player's yellow of the
// middle step: the fight is won and ends under the calibration's (the one
// the user called "too easy"), above the old tuning's 40%, and under the
// tables as they were before the curve it still ends under 40%.
func TestRealDustAndTheFireBat(t *testing.T) {
	db := simRealDB(t)
	pl, err := SimKitFromDB(db, 3, 0, 5, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	var book []CastSpell
	for _, s := range pl.Spells {
		if simRole(&s) == roleBuff {
			book = append(book, s)
		}
	}
	pl.Spells = book
	bat := SimNPC{Name: "fire bat", Level: 6}
	fight := func(b *Balance) (won, hp, secs float64, npcHP int) {
		n := 40
		for i := range n {
			r := SimulateFight(pl, bat, b, int64(i+1), 10*time.Minute)
			npcHP = r.NPCMaxHP
			if r.Won {
				won++
				hp += r.HPShare()
				secs += r.Time.Seconds()
			}
		}
		if won > 0 {
			hp /= won
			secs /= won
		}
		return won / float64(n), hp, secs, npcHP
	}
	won, hp, secs, batHP := fight(balanceAt0130())
	t.Logf("01:30 tables: won %.0f%%, %.0f%% HP left, %.0f s, the bat %d HP (Dust: 24 s, 65%%, 786)", 100*won, 100*hp, secs, batHP)
	if batHP != 786 || won < 0.95 || hp < 0.55 || hp > 0.75 || secs < 18 || secs > 40 {
		t.Errorf("the calibration fight is not Dust's")
	}
	calibration := hp
	tuned := simRealBalance(t, db)
	won, hp, secs, batHP = fight(tuned)
	t.Logf("the tuning: won %.0f%%, %.0f%% HP left, %.0f s, the bat %d HP", 100*won, 100*hp, secs, batHP)
	if won < 0.9 || hp <= 0.40 || hp >= calibration {
		t.Errorf("under the tuning the fire bat: won %.0f%% with %.0f%% left (want won, between 40%% and the "+
			"calibration's %.0f%%)", 100*won, 100*hp, 100*calibration)
	}
	if bw, bh, _, _ := fight(balanceBeforeNewPlayers(tuned)); bw < 0.6 || bh > 0.40 {
		t.Errorf("before the new players' curve the fire bat: won %.0f%% with %.0f%% left (want the old tuning's: "+
			"won, under 40%%)", 100*bw, 100*bh)
	}
	// the bat by the template its name has (tools/seed_roamers.py: a roamer), whose crit comes from level 10:
	// a level 6 bat fights with its melee alone, the same fight
	if foe := simRealFoe(t, db, "fire bat"); foe.Template != nil {
		bat.Template = foe.Template
		w2, h2, s2, _ := fight(tuned)
		if w2 != won || h2 != hp || s2 != secs {
			t.Errorf("the level 6 roamer's fight: won %.0f%%, %.0f%% left, %.0f s; without the template %.0f%%, %.0f%%, %.0f s",
				100*w2, 100*h2, s2, 100*won, 100*hp, secs)
		}
	}
}

// simRealFoe is the simulated foe a real database gives a creature of the
// name ("" a generic creature: any name npctemplate gives the roamer): the
// template npctemplate gives it (tools/seed_roamers.py), else none.
func simRealFoe(t *testing.T, db *database.SQLiteDB, name string) SimNPC {
	t.Helper()
	uses, err := db.NPCTemplates()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := uses[database.NPCClassKey(name)]
	if name == "" {
		for _, v := range uses {
			if v == "roamer" {
				tmpl = v
				break
			}
		}
	}
	if tmpl == "" {
		return SimNPC{}
	}
	all, err := SimTemplatesFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if all[tmpl] == nil {
		t.Fatalf("npctemplate names %q, which classtemplates has not", tmpl)
	}
	t.Logf("the foe fights by the %q template", tmpl)
	return SimNPC{Template: all[tmpl]}
}

// simRealBalance is the balance a real database fights under: its own tables
// once the tuning is written into them (balanceclasslevel has rows: tools/
// tune_balance.py and tools/retune_balance.py applied), else the seed rows
// (the same tuning, for a database it is not written into yet).
func simRealBalance(t *testing.T, db *database.SQLiteDB) *Balance {
	t.Helper()
	cll, err := db.BalanceClassLevels()
	if err != nil {
		t.Fatal(err)
	}
	if len(cll) == 0 {
		t.Log("the seed rows (the database's balance is not the tuning's)")
		return SeedBalance()
	}
	ls, err := db.BalanceLevels()
	if err != nil {
		t.Fatal(err)
	}
	cs, err := db.BalanceCons()
	if err != nil {
		t.Fatal(err)
	}
	cls, err := db.BalanceClasses()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("the database's own tables (%d class and level rows)", len(cll))
	return NewBalance(ls, cs).WithClasses(cls).WithClassLevels(cll)
}

// The user's targets of 2026-10-30 on the real kits (EQOA_REAL_DB), past the
// new players' levels (1-12 are balanced apart since 2026-11-04:
// TestRealNewPlayersCurve): both
// yellows won as a rule (one up four times in five, two up half the time)
// and taking a player down below about a third of its hit points (its low
// point, averaged over the fights won one and two up: the tuning's fit aims
// at a quarter), a red three up a lone player's loss (a healer's too), even
// fights won (a priest's at levels 1-3 97%), two of the level a red three
// up's winners and three a red five up's, under the database's own tables
// once the tuning is written into them (simRealBalance), against the
// creature the database gives (simRealFoe: a roamer); the controls: under
// the 01:30 tables most yellows left a player above a third, and the rows
// each check was added for miss it (the priests' under the tables as they
// were before the new players' curve, which makes any row's even fight at
// 1-3 an easy one; and at 70, since the first tuning's 47 is won 98% of the
// time once the cleric's Divine Protection answers every hit).
func TestRealBalanceYellowsAndReds(t *testing.T) {
	db := simRealDB(t)
	kit := func(c, l int) SimPlayer {
		pl, err := SimKitFromDB(db, c, 0, l, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		return pl
	}
	seed, old := simRealBalance(t, db), balanceAt0130()
	foe := simRealFoe(t, db, "") // a creature as the database's creatures fight: a roamer once seed_roamers.py is applied
	yellow := func(pl SimPlayer, b *Balance, fights int, foe SimNPC) (SimCell, SimCell, float64) {
		y1 := SimCellOf(pl, foe, 1, b, fights, 1, 10*time.Minute)
		y2 := SimCellOf(pl, foe, 2, b, fights, 1, 10*time.Minute)
		w1, w2 := float64(y1.Won), float64(y2.Won)
		return y1, y2, (w1*y1.LowLeft + w2*y2.LowLeft) / max(w1+w2, 1)
	}
	easy, cells := 0, 0
	for _, class := range []int{0, 2, 4, 6} {
		for _, level := range []int{20, 40, 55} { // 5 until the new players' curve
			pl := kit(class, level)
			y1, y2, low := yellow(pl, seed, 16, foe)
			if y1.WinShare() < 0.7 || y2.WinShare() < 0.5 || low > 0.36 {
				t.Errorf("class %d L%d: +1 won %.0f%%, +2 won %.0f%%, taken down to %.0f%% on average",
					class, level, 100*y1.WinShare(), 100*y2.WinShare(), 100*low)
			}
			if r := SimCellOf(pl, foe, 3, seed, 16, 1, 10*time.Minute); r.WinShare() > 0.4 {
				t.Errorf("class %d L%d +3 alone: won %.0f%%", class, level, 100*r.WinShare())
			}
			cells++
			if _, _, l := yellow(pl, old, 8, SimNPC{}); l > 0.36 { // the 01:30 creatures: no template
				easy++
			}
		}
	}
	if easy*2 < cells {
		t.Errorf("the control: under the 01:30 tables only %d of %d yellows left a player above a third", easy, cells)
	}
	// even fights won as a rule ("even and below kept"), healers included: a
	// priest at levels 1-3 at least 97% of 40 (the coordinator's check of
	// 2026-10-30: under the first tuning's row of 47 a level 1 cleric won 70%
	// of them on the 04:20 data); the control is that row
	for _, class := range []int{7, 8, 9} {
		for _, level := range []int{1, 2, 3} {
			if e := SimCellOf(kit(class, level), foe, 0, seed, 40, 1, 10*time.Minute); e.WinShare() < 0.97 {
				t.Errorf("class %d L%d even: won %.0f%%, low %.0f%%", class, level, 100*e.WinShare(), 100*e.LowLeft)
			}
		}
	}
	// a red three up a healer's loss alone too: a priest out-heals a low enough
	// share in a long fight (under the share fit alone a druid of 14 won 100%
	// of its reds in about three minutes, a cleric of 25 too, a shaman of 40);
	// the control is the druid of 14 at the share fit's 26
	for _, class := range []int{7, 8, 9} {
		for _, level := range []int{14, 18, 25, 40, 45} {
			if r := SimCellOf(kit(class, level), foe, 3, seed, 16, 1, 10*time.Minute); r.WinShare() > 0.4 {
				t.Errorf("class %d L%d +3 alone: won %.0f%% (%.0f s)", class, level, 100*r.WinShare(), r.Secs)
			}
		}
	}
	fitted := []database.BalanceClassLevel{{Class: 7, Level: 14, MeleeTakenPct: 26, PetHPPct: 100, DealtPct: 100}}
	for _, r := range seed.ClassLevelRows() {
		if r.Class != 7 || r.Level != 14 { // the share fit's row in place of the table's (or of none there)
			fitted = append(fitted, r)
		}
	}
	if r := SimCellOf(kit(7, 14), foe, 3, seed.WithClassLevels(fitted), 16, 1, 10*time.Minute); r.WinShare() < 0.6 {
		t.Errorf("the control: a druid of 14 at the share fit's 26 won %.0f%% of its reds alone: the check tells nothing", 100*r.WinShare())
	}
	var first []database.BalanceClassLevel
	before := balanceBeforeNewPlayers(seed)
	for _, r := range before.ClassLevelRows() {
		if r.Class == 9 && r.Level <= 2 {
			r.MeleeTakenPct = 70 // tools/tune_balance.py's cleric of 1 was 47 (and 2, by the row of 1 it reaches)
		}
		first = append(first, r)
	}
	ctl := SimCellOf(kit(9, 1), foe, 0, before.WithClassLevels(first), 40, 1, 10*time.Minute)
	t.Logf("the control: a level 1 cleric's even fights under the first tuning's row: won %.0f%%", 100*ctl.WinShare())
	if ctl.WinShare() >= 0.97 {
		t.Errorf("the control: under the first tuning's row a level 1 cleric won %.0f%% of its even fights: the check tells nothing", 100*ctl.WinShare())
	}
	for _, level := range []int{10, 30, 50} {
		duo, err := SimGroupFor(6, level, 2, func(c int) (SimPlayer, error) { return kit(c, level), nil })
		if err != nil {
			t.Fatal(err)
		}
		if g := SimGroupCellOf(duo, foe, 3, seed, 12, 1, 10*time.Minute); g.WinShare() < 0.6 {
			t.Errorf("L%d +3, a rogue and a cleric: won %.0f%%", level, 100*g.WinShare())
		}
		trio, err := SimGroupFor(6, level, 3, func(c int) (SimPlayer, error) { return kit(c, level), nil })
		if err != nil {
			t.Fatal(err)
		}
		if g := SimGroupCellOf(trio, foe, 5, seed, 12, 1, 10*time.Minute); g.WinShare() < 0.5 {
			t.Errorf("L%d +5, a warrior, a rogue and a cleric: won %.0f%%", level, 100*g.WinShare())
		}
	}
}

// The coordinator's low-level check of 2026-10-30 (jib: "jibrush got beat up
// by a level 1 while he was level 3 alc"): at levels 1-10 every class wins
// its even fights about 95% of the time and its blue and green ones about
// all of it (97% of 40), by hand AND in its own play (a wizard's root from 3,
// an alchemist's from 4, a pet class's pet), in a new character's starting
// gear at 1-5 (SimStartKitFromDB; a level 1's blue and green fights are its
// even one, a creature of level 1: no creature is under it). Until the new
// players' curve (2026-11-04) one cell was short of it, an enchanter of 2 by
// hand at 92% (the NPC of 2 hit 317%: its first blows killed a
// 25-hit-point caster); its creature hits at 133% now. The control: an
// alchemist of 2 by hand under the share fit alone (no dealt), under the
// tables as they were before the curve, wins under half of its even fights.
func TestRealLowLevelsEvenAndBelow(t *testing.T) {
	db := simRealDB(t)
	b := simRealBalance(t, db)
	foe := simRealFoe(t, db, "")
	known := map[[3]int]float64{} // class, level, play (0 by hand): none short since the curve
	for class := range 15 {
		for level := 1; level <= 10; level++ {
			var pl SimPlayer
			var err error
			if level <= 5 {
				pl, err = SimStartKitFromDB(db, class, 0, level)
			} else {
				pl, err = SimKitFromDB(db, class, 0, level, 0.5)
			}
			if err != nil {
				t.Fatal(err)
			}
			plays := []SimPlayer{pl}
			switch {
			case class == 10 || class == 11 || class == 12:
				k := pl
				k.Play.Pet, k.Play.Melee = true, false
				plays = append(plays, k)
			case (class == 13 && level >= 3) || (class == 14 && level >= 4):
				k := pl
				k.Play.Root, k.Play.Melee = true, false
				plays = append(plays, k)
			}
			for i, k := range plays {
				for _, diff := range []int{-2, -1, 0} {
					want := 0.97
					if diff == 0 || level+diff < 1 { // no creature under level 1: a level 1's blue and green are its even fight
						want = 0.95
					}
					if w, ok := known[[3]int{class, level, i}]; ok && diff == 0 {
						want = w
					}
					if c := SimCellOf(k, foe, diff, b, 40, 1, 10*time.Minute); c.WinShare() < want {
						t.Errorf("class %d L%d %s %+d: won %.0f%% (want %.0f%%)", class, level, []string{"by hand", "own play"}[i],
							diff, 100*c.WinShare(), 100*want)
					}
				}
			}
		}
	}
	var fitted []database.BalanceClassLevel
	before := balanceBeforeNewPlayers(b)
	for _, r := range before.ClassLevelRows() {
		if r.Class == 14 && r.Level <= 3 {
			r.DealtPct = 100
		}
		fitted = append(fitted, r)
	}
	alc, err := SimStartKitFromDB(db, 14, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if c := SimCellOf(alc, foe, 0, before.WithClassLevels(fitted), 40, 1, 10*time.Minute); c.WinShare() >= 0.5 {
		t.Errorf("the control: an alchemist of 2 by hand with no dealt won %.0f%% of its even fights: the check tells nothing", 100*c.WinShare())
	}
}

// balanceBeforeNewPlayers is b with levels 1-12 as they were before the new
// players' curve (tools/refit_new_players.py, 2026-11-04): the creatures'
// hit points and melee of the tuning of 2026-10-30, all of a player's melee
// landing (no player_melee_pct, no melee_dealt_pct).
func balanceBeforeNewPlayers(b *Balance) *Balance {
	hp := [...]float64{300, 278.75, 257.5, 236.25, 215, 655.0 / 3, 665.0 / 3, 225, 222.5, 220, 222, 224}
	dmg := [...]float64{188, 317, 225, 294, 243, 285, 245, 276, 230, 236, 221, 247}
	ls, cs := b.Rows()
	var out []database.BalanceLevel
	for _, r := range ls {
		if r.Level >= 1 && r.Level <= 12 {
			r.HPPct, r.DamagePct, r.PlayerMeleePct = hp[r.Level-1], dmg[r.Level-1], 100
		}
		out = append(out, r)
	}
	var cl []database.BalanceClassLevel
	for _, r := range b.ClassLevelRows() {
		r.MeleeDealtPct = 100
		cl = append(cl, r)
	}
	return NewBalance(out, cs).WithClasses(b.ClassRows()).WithClassLevels(cl)
}

// The new players' curve (EQOA_REAL_DB; the user, 2026-11-04: "1-5 mobs
// should really be easy and 6-9 little tougher. 10-12 stronger, the mob crits
// at 10 do make it more difficult", and the healers' and casters' melee
// raised at 1-10): in a new character's starting gear at 1-5 and the middling
// gear of its last ten levels from 6, each class in its own play, the
// yellows against a creature (the players one and two levels under it, the
// median of their low points) end above half a player's hit points against
// creatures of 2-5, between a third and three fifths against 6-9 and between
// a fifth and two fifths against 10-12; the median class keeps four fifths
// through an even fight at 1-5; and a healer's even fight by hand at 1-10
// takes at most one and a half times the melee classes' median (over its
// level and the one on either side). The
// controls: under the tables as they were before the curve the yellows of
// 2-5 end below half (16-24% in the fit), and without the healers' melee a
// cleric of 3-5 took over twice the melee classes' median.
func TestRealNewPlayersCurve(t *testing.T) {
	db := simRealDB(t)
	b := simRealBalance(t, db)
	foe := simRealFoe(t, db, "")
	kit := func(c, l int, own bool) SimPlayer {
		var pl SimPlayer
		var err error
		if l <= 5 {
			pl, err = SimStartKitFromDB(db, c, 0, l)
		} else {
			pl, err = SimKitFromDB(db, c, 0, l, 0.5)
		}
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case !own:
		case c == 10 || c == 11 || c == 12:
			pl.Play.Pet, pl.Play.Melee = true, false
		case (c == 13 && l >= 3) || (c == 14 && l >= 4):
			pl.Play.Root, pl.Play.Melee = true, false
		}
		return pl
	}
	low := func(pl SimPlayer, c SimCell) float64 {
		if pl.Play.Pet {
			return c.PetLowLeft
		}
		return c.LowLeft
	}
	median := func(xs []float64) float64 {
		if len(xs) == 0 {
			return 0
		}
		sort.Float64s(xs)
		if n := len(xs); n%2 == 1 {
			return xs[n/2]
		} else {
			return (xs[n/2-1] + xs[n/2]) / 2
		}
	}
	// the yellows against a creature of level n: its +1 for players of n-1, its +2 for n-2
	yellows := func(b *Balance, n int) float64 {
		var meds []float64
		for _, d := range []int{1, 2} {
			if n-d < 1 {
				continue
			}
			var lows []float64
			for c := range 15 {
				pl := kit(c, n-d, true)
				if cell := SimCellOf(pl, foe, d, b, 40, 1, 10*time.Minute); cell.Won > 0 {
					lows = append(lows, low(pl, cell))
				}
			}
			meds = append(meds, median(lows))
		}
		var sum float64
		for _, m := range meds {
			sum += m
		}
		return sum / float64(len(meds))
	}
	for n := 2; n <= 12; n++ {
		lo, hi := 0.5, 1.0
		switch {
		case n >= 10:
			lo, hi = 0.2, 0.4
		case n >= 6:
			lo, hi = 1.0/3, 0.6
		}
		if y := yellows(b, n); y < lo || y > hi {
			t.Errorf("the yellows against a creature of %d end at %.0f%% (want %.0f-%.0f%%)", n, 100*y, 100*lo, 100*hi)
		}
	}
	for n := 1; n <= 5; n++ {
		var lows []float64
		for c := range 15 {
			pl := kit(c, n, true)
			lows = append(lows, low(pl, SimCellOf(pl, foe, 0, b, 40, 1, 10*time.Minute)))
		}
		if m := median(lows); m < 0.8 {
			t.Errorf("an even fight at %d leaves the median class %.0f%% (want 80%%)", n, 100*m)
		}
	}
	// a healer's even fight by hand against the melee classes' median, taken
	// over the level and the one on either side (as the fit took it: one
	// level's median moves with its kits, 14 s at 8 between 19 and 22)
	secs := func(b *Balance, c, l int) float64 {
		return SimCellOf(kit(c, l, false), foe, 0, b, 40, 1, 10*time.Minute).Secs
	}
	meleeAt := map[int]float64{}
	for l := 1; l <= 11; l++ {
		var melee []float64
		for c := range 7 {
			melee = append(melee, secs(b, c, l))
		}
		meleeAt[l] = median(melee)
	}
	for l := 1; l <= 10; l++ {
		var near []float64
		for _, x := range []int{l - 1, l, l + 1} {
			if m, ok := meleeAt[x]; ok {
				near = append(near, m)
			}
		}
		m := median(near)
		for _, c := range []int{7, 8, 9} {
			if s := secs(b, c, l); s > 1.5*m {
				t.Errorf("class %d L%d's even fight by hand takes %.0f s, the melee classes' median %.0f s", c, l, s, m)
			}
		}
	}
	before := balanceBeforeNewPlayers(b)
	if y := (yellows(before, 2) + yellows(before, 3) + yellows(before, 4) + yellows(before, 5)) / 4; y >= 0.5 {
		t.Errorf("the control: before the curve the yellows of 2-5 end at %.0f%%: the check tells nothing", 100*y)
	}
	if s, m := secs(balanceBeforeNewPlayersMeleeOnly(b), 9, 4), meleeAt[4]; s < 2*m {
		t.Errorf("the control: a cleric of 4 without its melee took %.0f s, the melee classes' median %.0f s: the check tells nothing", s, m)
	}
}

// balanceBeforeNewPlayersMeleeOnly is b with the healers' and casters' melee
// as it was before the new players' curve (melee_dealt_pct 100), the rest of
// the curve kept.
func balanceBeforeNewPlayersMeleeOnly(b *Balance) *Balance {
	var cl []database.BalanceClassLevel
	for _, r := range b.ClassLevelRows() {
		r.MeleeDealtPct = 100
		cl = append(cl, r)
	}
	return b.WithClassLevels(cl)
}

// A wisp is a yellow like any other (the user's "28%" of 2026-10-30): a
// crag wisp fights by the template the database gives it (tools/
// seed_roamers.py: the wisp, jib's wizard at 28% of a player's figures), and
// a warrior, a rogue and a cleric by hand and a wizard and an alchemist
// rooting win one up at least 70% of the time and two up half of it, at 25,
// 35 and 45. The control: under jib's wizard as he made it (35%) the same
// check fails somewhere (2026-11-02: a rogue of 45 one up, an alchemist of 25
// and 35 two up). It was a rogue of 35 losing its one up until a creature's
// effects counted in its fights (2026-11-01: its debuffs on the wizard win
// it) and its yellows were fitted again on a backstab that needs its caster
// behind (it deals 115% at 35, to win its wisps: one cell the fit moves).
func TestRealWispsAreYellows(t *testing.T) {
	db := simRealDB(t)
	b := simRealBalance(t, db)
	foe := simRealFoe(t, db, "crag wisp")
	if foe.Template == nil || foe.Template.Name != "wisp" {
		t.Skip("npctemplate gives a crag wisp no wisp template: tools/seed_roamers.py first")
	}
	kit := func(c, l int) SimPlayer {
		pl, err := SimKitFromDB(db, c, 0, l, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		if c == 13 || c == 14 {
			pl.Play.Root, pl.Play.Melee = true, false
		}
		return pl
	}
	for _, class := range []int{0, 6, 9, 13, 14} {
		for _, level := range []int{25, 35, 45} {
			pl := kit(class, level)
			y1 := SimCellOf(pl, foe, 1, b, 16, 1, 10*time.Minute)
			y2 := SimCellOf(pl, foe, 2, b, 16, 1, 10*time.Minute)
			if y1.WinShare() < 0.7 || y2.WinShare() < 0.5 {
				t.Errorf("class %d L%d against a wisp: +1 won %.0f%%, +2 won %.0f%%", class, level, 100*y1.WinShare(), 100*y2.WinShare())
			}
		}
	}
	all, err := SimTemplatesFromDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if all["wizard"] == nil {
		t.Fatal("no wizard template to fight the control by")
	}
	wiz := SimNPC{Template: all["wizard"]}
	var failed []string
	for _, class := range []int{0, 6, 9, 13, 14} {
		for _, level := range []int{25, 35, 45} {
			pl := kit(class, level)
			y1 := SimCellOf(pl, wiz, 1, b, 16, 1, 10*time.Minute)
			y2 := SimCellOf(pl, wiz, 2, b, 16, 1, 10*time.Minute)
			if y1.WinShare() < 0.7 || y2.WinShare() < 0.5 {
				failed = append(failed, fmt.Sprintf("class %d L%d %.0f%%/%.0f%%", class, level, 100*y1.WinShare(), 100*y2.WinShare()))
			}
		}
	}
	t.Logf("the control: under jib's wizard (35%%) the check fails at %v", failed)
	if len(failed) == 0 {
		t.Errorf("the control: under jib's wizard (35%%) every class won its yellows as against a wisp: the check tells nothing")
	}
}

// The pet classes' pets are yellows like any other (the pets' fit of
// 2026-10-30, tools/tune_pets.py, and its re-fit on the workbook's pets,
// tools/refit_eqoaemu_balance.py): a magician, a necromancer and an
// enchanter with the pet its book gives at 10, 20, 30, 40, 50 and 60 win one
// up at least 75% and two up half the time against the creature the
// database gives (a roamer), the pet taken down to under 45% of its hit
// points at its lowest on average, and lose a red three up alone more often
// than not. The control: the workbook's pets under the pet columns the four
// tunings left (balancelevel's pet_hp_pct 100-540 and pet_damage_pct
// 225-560, fitted for the server's own pets, which hit with a creature's
// roll) keep over 70% through a +1 and win their reds alone, a magician's
// and a necromancer's at 30 and 45.
func TestRealPetsAreYellows(t *testing.T) {
	db := simRealDB(t)
	b := simRealBalance(t, db)
	foe := simRealFoe(t, db, "")
	kit := func(c, l int) SimPlayer {
		pl, err := SimKitFromDB(db, c, 0, l, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		pl.Play.Pet, pl.Play.Melee = true, false
		return pl
	}
	for _, class := range []int{10, 11, 12} {
		for _, level := range []int{10, 20, 30, 40, 50, 60} {
			pl := kit(class, level)
			y1 := SimCellOf(pl, foe, 1, b, 16, 1, 10*time.Minute)
			y2 := SimCellOf(pl, foe, 2, b, 16, 1, 10*time.Minute)
			w1, w2 := float64(y1.Won), float64(y2.Won)
			low := (w1*y1.PetLowLeft + w2*y2.PetLowLeft) / max(w1+w2, 1)
			if y1.WinShare() < 0.75 || y2.WinShare() < 0.5 || low > 0.45 {
				t.Errorf("class %d L%d with its pet: +1 won %.0f%%, +2 won %.0f%%, the pet down to %.0f%% on average",
					class, level, 100*y1.WinShare(), 100*y2.WinShare(), 100*low)
			}
			if r := SimCellOf(pl, foe, 3, b, 16, 1, 10*time.Minute); r.WinShare() > 0.4 {
				t.Errorf("class %d L%d +3 alone with its pet: won %.0f%%", class, level, 100*r.WinShare())
			}
		}
	}
	// the four tunings' pet columns, as their rows' line reads (tools/tune_balance.py's LEVEL_WANT and LEVELS_0130)
	old := b.WithLevelColumn([][2]float64{{8, 100}, {10, 140}, {15, 155}, {18, 155}, {25, 190}, {30, 205}, {35, 250},
		{45, 375}, {50, 450}, {60, 540}}, func(r *database.BalanceLevel, v float64) { r.PetHPPct = v }).
		WithLevelColumn([][2]float64{{1, 250}, {5, 265}, {8, 260}, {10, 225}, {15, 280}, {18, 300}, {25, 285}, {30, 300},
			{35, 340}, {45, 430}, {50, 475}, {60, 560}}, func(r *database.BalanceLevel, v float64) { r.PetDamagePct = v })
	for _, class := range []int{10, 11} {
		for _, level := range []int{30, 45} {
			pl := kit(class, level)
			y1 := SimCellOf(pl, foe, 1, old, 16, 1, 10*time.Minute)
			red := SimCellOf(pl, foe, 3, old, 16, 1, 10*time.Minute)
			// 0.6, not the 0.7 of 2026-10-30: the magician's pet at 30 keeps 0.79 of its hit points since every
			// damage shield answers each hit (tools/refit_damage_shields.py), 67% through a +1 under these columns
			if y1.PetLowLeft < 0.6 || red.WinShare() < 0.6 {
				t.Errorf("the control: class %d L%d's pet under the four tunings' pet columns kept %.0f%% through a +1 and "+
					"won %.0f%% of its reds alone", class, level, 100*y1.PetLowLeft, 100*red.WinShare())
			}
		}
	}
}
