package entity

import (
	"fmt"
	"os"
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

// On a server's own kits (EQOA_REAL_DB): a rooting druid and wizard of 50
// (root, then their spells from range) lose a red five up under the seed's
// root chances, and win it nearly always with the chances at 0 (the
// control: the game before them, every root held to its end).
func TestRealRootPlayHeldToTheCon(t *testing.T) {
	db := simRealDB(t)
	none := SeedBalance().WithConColumn([][2]float64{{0, 0}}, func(r *database.BalanceCon, v float64) {
		r.RootResistPct, r.RootBreakPct = v, v
	})
	for _, class := range []int{7, 13} {
		pl, err := SimKitFromDB(db, class, 0, 50, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		pl.Play.Root, pl.Play.Melee = true, false
		if red := SimCellOf(pl, SimNPC{}, 5, SeedBalance(), 16, 1, 10*time.Minute); red.WinShare() > 0.4 {
			t.Errorf("class %d rooting under the seed: +5 won %.0f%%", class, 100*red.WinShare())
		}
		if was := SimCellOf(pl, SimNPC{}, 5, none, 8, 1, 10*time.Minute); was.WinShare() < 0.85 {
			t.Errorf("class %d rooting with no root chances: +5 won only %.0f%% (the control)", class, 100*was.WinShare())
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
