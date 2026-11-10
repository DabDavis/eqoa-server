package entity

import (
	"strings"
	"testing"
	"time"
)

// The magician's level 60 pets over a real server's database with
// tools/fix_mage_pets_1450.py applied (EQOA_REAL_DB, copied; petition 1450):
// each summoning makes its workbook pet with the kit the tool gave it, which
// fights the rig's crab (level 60, a million hit points, not fighting back) as
// magepets_test.go holds it to: the Djinni Vizier at 1.3 with its three spells
// at 1, 3 and 5 s and every 12 s (1000, 1300, 3000 on its target), the Efreeti
// Lord's quad of 600 and its 2000 inferno every 30 s, the Badi Elder's taunt
// every 15 s and its 2000 crit every 30 s (its buff on it), the Ghilan Noble's
// chill and frost wave 3 s apart every 30 s (its blessing on its owner). The
// world here has no balance: the figures land as they are.
func TestRealMagePets1450(t *testing.T) {
	db := simRealDB(t)
	fx := map[int]uint32{}
	for id := 905450; id <= 905457; id++ {
		ts, ok, err := db.TemplateSpellByID(id)
		if err != nil || !ok {
			t.Fatalf("spell %d: there %v, %v: tools/fix_mage_pets_1450.py not applied to this database?", id, ok, err)
		}
		fx[id] = ts.FX
	}
	summon := func(t *testing.T, id int) CastSpell {
		t.Helper()
		ts, ok, err := db.TemplateSpellByID(id)
		if err != nil || !ok {
			t.Fatalf("the summoning %d: there %v, %v", id, ok, err)
		}
		return *templateSpellOf(ts)
	}
	rig := func(t *testing.T) *petRig {
		t.Helper()
		r := newPetRig(t)
		if _, err := r.w.LoadClassTemplates(db); err != nil {
			t.Fatal(err)
		}
		r.p.Class, r.p.Level = 10, 60
		r.enemy.Level = 60
		return r
	}
	kit := func(r *petRig) string {
		var out []string
		for _, a := range r.w.pets[r.p.ServerID].petAbilities() {
			if a.pinned != nil {
				out = append(out, a.Kind+" "+a.pinned.Name)
			} else {
				out = append(out, a.Kind)
			}
		}
		return strings.Join(out, ", ")
	}
	for _, c := range []struct {
		name   string
		summon int
		hp     int
		kit    string
		check  func(t *testing.T, r *petRig, f *swarmFight)
		setup  func(r *petRig) func(pet *NPC)
	}{
		{"Djinni Vizier", 900761, 3440, "nuke Vizier's Bolt, nuke Vizier's Thunderclap, aoe Vizier's Tempest, buff Djinni's Grace",
			func(t *testing.T, r *petRig, f *swarmFight) {
				checkVizier1450(t, f, fx[905450], fx[905451], fx[905452])
			}, nil},
		{"Efreeti Lord", 701087, 4540, "nuke Efreeti's Inferno, buff Efreeti's Fire",
			func(t *testing.T, r *petRig, f *swarmFight) { checkEfreeti1450(t, f, fx[905453]) }, nil},
		{"Badi Elder", 701086, 7070, "taunt, buff Badi's Durability",
			func(t *testing.T, r *petRig, f *swarmFight) { checkBadi1450(t, r, f, true) },
			func(r *petRig) func(pet *NPC) {
				return func(pet *NPC) {
					pet.Effects = append(pet.Effects, &ActiveEffect{SpellID: 902157, Name: "Badi's Durability", Until: r.at(time.Hour)})
				}
			}},
		{"Ghilan Noble", 701088, 4650,
			"heal Ghilan's Lay Hands, heal Ghilan's Mending, buff Ghilan's Blessing, nuke Ghilan's Chill, nuke Ghilan's Frost Wave, " +
				"nuke Purifying Tide",
			func(t *testing.T, r *petRig, f *swarmFight) {
				checkGhilan1450(t, r, f, fx[905454], fx[905455], steps(14700, 30000, lastOf1450), steps(17700, 30000, lastOf1450))
			},
			func(r *petRig) func(pet *NPC) {
				r.p.Effects = append(r.p.Effects, &ActiveEffect{SpellID: 902167, Name: "Ghilan's Blessing", Until: r.at(time.Hour)})
				return func(*NPC) {}
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := rig(t)
			var setup []func(pet *NPC)
			if c.setup != nil {
				setup = append(setup, c.setup(r))
			}
			f := swarmFightFor(t, r, summon(t, c.summon), c.hp, fight1450, setup...)
			if f.pet.sheet == nil {
				t.Fatalf("%q: no workbook pet", f.pet.Name)
			}
			// what tools/fix_pets_1110.py adds comes after them (petitions 1478 and
			// 1495: pets1110_real_test.go)
			if k := kit(r); !strings.HasPrefix(k, c.kit) {
				t.Errorf("its abilities: %s\nwant %s first", k, c.kit)
			}
			c.check(t, r, f)
			t.Logf("%s: %q level %d, %d HP, size %.2f, swing %d, template %q", c.name, f.pet.Name, f.pet.Level, f.pet.MaxHP,
				f.pet.Size, npcSwingKind(f.pet), r.w.pets[r.p.ServerID].tmpl.tmpl.Name)
		})
	}
}
