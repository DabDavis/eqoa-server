package entity

import (
	"testing"
	"time"
)

// The owner, 2026-11-14: "rogue wounds aren't strong enough ... they are modded by strength ... make wounds 75%
// stronger also they apply snare". tools/data_1114c_20261114.py gives each wound 1.75 times its hit (its STR scale
// kept) and a slow by tier; the wounds had a 30% slow already (their line said "Root"). Measured here on the live
// records, before and as the tool writes them: a level 60 rogue of STR 300 on a creature.
func TestRogueWoundsStrongerAndSnare(t *testing.T) {
	for _, c := range []struct {
		name          string
		js            string
		hit, slow     int
		tick, seconds int
	}{
		{"Minor Wound (before)", minorWoundOldJSON, 330 + 90, 30, 0, 18},
		{"Minor Wound", minorWoundJSON, 578 + 90, 30, 0, 18},
		{"Mortal Wound (before)", mortalWoundOldJSON, 1400 + 90 + 2*25, 30, 25, 24},
		{"Mortal Wound", mortalWoundJSON, 2450 + 90 + 2*44, 40, 44, 24},
		{"Ascended Sothimus Critical Wounding", ascSothimusJSON, 3080 + 99, 40, 0, 18},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newPetRig(t)
			noResist(t)
			r.p.Class, r.p.Level, r.p.Str = 6, 60, 300
			crab := r.enemy
			crab.Level = 55
			crab.HP, crab.MaxHP = 5000000, 5000000
			s := wbSpell(9601, 20, 1, c.name, c.js)
			if len(s.Def.Unsupported) > 0 {
				t.Fatalf("not read: %v", s.Def.Unsupported)
			}
			r.w.mu.Lock()
			r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
			r.w.mu.Unlock()
			if got := 5000000 - crab.HP; got != c.hit {
				t.Errorf("its landing took %d, want %d (the base, 30%% of STR 300 and a DoT's first tic twice)", got, c.hit)
			}
			want := float32(100-c.slow) / 100
			end := time.Duration(c.seconds) * time.Second
			if f := crab.snareFactor(r.at(2 * time.Second)); f != want {
				t.Errorf("its pace snared: %.2f, want %.2f", f, want)
			}
			if f := crab.snareFactor(r.at(end - time.Second)); f != want {
				t.Errorf("its pace at %v: %.2f, want %.2f", end, f, want)
			}
			if f := crab.snareFactor(r.at(end + 2*time.Second)); f != 1 {
				t.Errorf("its pace after the spell: %.2f, want 1", f)
			}
			tick := 0
			for _, e := range crab.Effects {
				tick -= e.HPTick
			}
			if tick != c.tick {
				t.Errorf("its damage a tic %d, want %d", tick, c.tick)
			}
		})
	}
}

// The records as they were (the live server's, 2026-11-14) and as the tool writes them.
const (
	minorWoundOldJSON  = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":18,"stack":0,"level":20,"min_level":1,"req_equipment":"1HP","sheet_type":"Statistic Absolute","effects":[{"id":"0607","type":"Statistic Change","kind":"speed","amount":-30},{"id":"0608","type":"Dmg/Heal","kind":"hp","amount":-330,"scale":"str","scale_pct":30}]}`
	minorWoundJSON     = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":18,"stack":0,"level":20,"min_level":1,"req_equipment":"1HP","sheet_type":"Statistic Absolute","effects":[{"id":"0607","type":"Statistic Change","kind":"speed","amount":-30},{"id":"0608","type":"Dmg/Heal","kind":"hp","amount":-578,"scale":"str","scale_pct":30}]}`
	mortalWoundOldJSON = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,"stack":0,"level":49,"min_level":1,"req_equipment":"1HP","sheet_type":"Statistic Absolute","effects":[{"id":"064A","type":"Statistic Change","kind":"speed","amount":-30},{"id":"064B","type":"Dmg/Heal","kind":"hp","amount":-1400,"scale":"str","scale_pct":30},{"id":"064C","type":"Statistic Change","kind":"hp_tick","amount":-25}]}`
	mortalWoundJSON    = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":24,"stack":0,"level":49,"min_level":1,"req_equipment":"1HP","sheet_type":"Statistic Absolute","effects":[{"id":"064A","type":"Statistic Change","kind":"speed","amount":-40},{"id":"064B","type":"Dmg/Heal","kind":"hp","amount":-2450,"scale":"str","scale_pct":30},{"id":"064C","type":"Statistic Change","kind":"hp_tick","amount":-44}]}`
	ascSothimusJSON    = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"duration_s":18,"stack":0,"level":60,"min_level":1,"req_equipment":"1HP","sheet_type":"Statistic Absolute","effects":[{"id":"0673","type":"Statistic Change","kind":"speed","amount":-40},{"id":"0674","type":"Dmg/Heal","kind":"hp","amount":-3080,"scale":"str","scale_pct":33}]}`
)
