package entity

import (
	"strings"
	"testing"
	"time"
)

// The Ghilan Noble over a real server's database with petfix180_20261109.py
// applied (EQOA_REAL_DB, copied; petition 1395): Summon Ghilan Noble (701088)
// makes its workbook pet with its own kit and Purifying Tide (905130) after it;
// against the rig's crab named as a creature npcundead calls undead (Festering
// Jack, the GMs' word in petition 1193) the tide goes every 30 s and lands 1500,
// and against the crab living it never goes. The world here has no balance.
// After tools/fix_mage_pets_1450.py (petition 1450) its kit is that tool's,
// the tide kept, and its other nuke the tool's chill.
func TestRealGhilanNoblesTideIsForTheUndead(t *testing.T) {
	db := simRealDB(t)
	tide, ok, err := db.TemplateSpellByID(905130)
	if err != nil || !ok {
		t.Fatalf("spell 905130: there %v, %v: petfix180 not applied to this database?", ok, err)
	}
	wantKit, nuke, nukesLeast := "heal Water Heal Level 55, buff Ghilan's Blessing, nuke Water Nuke Level 55, nuke Purifying Tide",
		902166, 8
	if _, ok, _ := db.TemplateSpellByID(905454); ok {
		wantKit, nuke, nukesLeast = "heal Ghilan's Lay Hands, heal Ghilan's Mending, buff Ghilan's Blessing, nuke Ghilan's Chill, "+
			"nuke Ghilan's Frost Wave, nuke Purifying Tide", 905454, 3
	}
	sum, ok, err := db.TemplateSpellByID(701088)
	if err != nil || !ok {
		t.Fatalf("the summoning 701088: there %v, %v", ok, err)
	}
	spell := *templateSpellOf(sum)
	for _, c := range []struct {
		name string
		want int
	}{{"Festering Jack", 4}, {"Desert Crab", 0}} {
		t.Run(c.name, func(t *testing.T) {
			r := newPetRig(t)
			if _, err := r.w.LoadClassTemplates(db); err != nil {
				t.Fatal(err)
			}
			if _, _, err := r.w.LoadUndeadTags(db); err != nil {
				t.Fatal(err)
			}
			r.p.Class, r.p.Level = 10, 60
			r.enemy.Name = c.name // after the templates' load: none of the name's (Festering Jack's iod template) on it
			f := swarmFightFor(t, r, spell, 4650, 110*time.Second)
			ps := r.w.pets[r.p.ServerID]
			var kit []string
			for _, a := range ps.petAbilities() {
				kit = append(kit, a.Kind+" "+a.pinned.Name)
			}
			if strings.Join(kit, ", ") != wantKit {
				t.Errorf("its abilities: %v", kit)
			}
			at, ms := f.of(tide.FX)
			landed := f.logged(`lands "Purifying Tide" on ` + c.name + ` for 1500 before`)
			water, _, err := db.TemplateSpellByID(nuke)
			if err != nil {
				t.Fatal(err)
			}
			nukes, _ := f.of(water.FX)
			if len(at) != c.want || landed != c.want || len(nukes) < nukesLeast {
				t.Errorf("against %s: the tide cast at %v (%v ms), %d landed for 1500, its %s %d times; want %d of the tide",
					c.name, at, ms, landed, water.Pattern.SpellName, len(nukes), c.want)
			}
			for i := 1; i < len(at); i++ {
				if at[i]-at[i-1] != 30000 {
					t.Errorf("the tide %d ms after the one before, want 30 s", at[i]-at[i-1])
				}
			}
			t.Logf("%s against %s (undead %v): %q level %d, %d HP; in 110 s the tide cast at %v ms (casts of %v ms), %d landed "+
				"for 1500; its %s %d times", spell.Name, c.name, r.w.undeadLocked(r.enemy), f.pet.Name, f.pet.Level,
				f.pet.MaxHP, at, ms, landed, water.Pattern.SpellName, len(nukes))
		})
	}
}
