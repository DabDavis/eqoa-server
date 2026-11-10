package entity

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// The Swarming Deaths over a real server's database with petfix176_20261109.py
// applied (EQOA_REAL_DB, copied; petition 1388): each of the four summonings
// (Swarming Death, and the masteries' Bone Swarming Death (60), Ghoulish Death
// and Haunting Death) makes its workbook pet on the Swarming Deaths' own
// template, which fights the rig's crab with the kit and nothing else: the
// damage over time kept up, the harm touch every 50 s, the tap every 30 s
// healing it, the proc on a fourth of its melee hits (the rig's dice). The
// world here has no balance: the figures land as they are.
func TestRealSwarmingDeathsFightWithTheirKit(t *testing.T) {
	db := simRealDB(t)
	fx := map[int]uint32{}
	for _, id := range []int{905100, 905101, 905102} {
		ts, ok, err := db.TemplateSpellByID(id)
		if err != nil || !ok {
			t.Fatalf("spell %d: there %v, %v: petfix176 not applied to this database?", id, ok, err)
		}
		fx[id] = ts.FX
	}
	for _, id := range []int{700823, 901004, 900994, 900995} {
		ts, ok, err := db.TemplateSpellByID(id)
		if err != nil || !ok {
			t.Fatalf("the summoning %d: there %v, %v", id, ok, err)
		}
		spell := *templateSpellOf(ts)
		t.Run(spell.Name, func(t *testing.T) {
			r := newPetRig(t)
			if _, err := r.w.LoadClassTemplates(db); err != nil {
				t.Fatal(err)
			}
			r.p.Class, r.p.Level = classNecromancer, 60
			f := swarmFightFor(t, r, spell, 100, 110*time.Second)
			ps := r.w.pets[r.p.ServerID]
			if ps == nil || ps.tmpl == nil || f.pet.sheet == nil {
				t.Fatalf("no workbook pet of a template")
			}
			tm := ps.tmpl.tmpl
			if !strings.HasPrefix(tm.Name, "pet swarming death (workbook pet ") || tm.ProcChance != 25 || tm.ProcDamage != 250 ||
				tm.ProcResist != "AR" || f.pet.Class != 3 || npcSwingKind(f.pet) != 5 {
				t.Errorf("template %q, proc %d%% %d %s, class %d, swing %d: want the Swarming Deaths', 25%% 250 AR, a shadow knight's quad",
					tm.Name, tm.ProcChance, tm.ProcDamage, tm.ProcResist, f.pet.Class, npcSwingKind(f.pet))
			}
			var kit []string
			for _, a := range ps.petAbilities() {
				kit = append(kit, a.Kind+" "+a.pinned.Name)
			}
			if strings.Join(kit, ", ") != "dot Swarming Plague, nuke Swarming Harm Touch, lifetap Swarming Leech" {
				t.Errorf("its abilities: %v", kit)
			}
			if a := ps.petAbilities()[0]; a.Kind != database.AbilityDoT || a.castTime(a.pinned) != 500*time.Millisecond {
				t.Errorf("its damage over time: %+v", a.TemplateAbility)
			}
			checkSwarmKit(t, f, fx[905100], fx[905101], fx[905102], true)
			dot, _ := f.of(fx[905100])
			touch, _ := f.of(fx[905102])
			tap, _ := f.of(fx[905101])
			procs, _ := f.of(fxShieldHit)
			t.Logf("%s: %q level %d, %d HP, template %q: in 110 s %d casts of its damage over time (%d ticks of 300, never "+
				"two, never down), %d harm touches (2000), %d taps (1000, healing it %v), %d procs (250) of %d melee hits",
				spell.Name, f.pet.Name, f.pet.Level, f.pet.MaxHP, tm.Name, len(dot), len(f.ticks), len(touch), len(tap), f.heals,
				len(procs), f.dice)
		})
	}
}
