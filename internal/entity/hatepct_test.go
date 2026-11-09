package entity

import (
	"testing"
	"time"
)

// A workbook hit's hate_pct is the share of its damage that is its hate
// (petition 1372: Vylinnuza's Merciless Blizzard and Inferno, "little to no
// aggro generated"), kept as HateOff: 0 when absent, held to 0..100, and kept
// on the least a resisted hit does.
func TestAHitsHatePctIsRead(t *testing.T) {
	for _, c := range []struct {
		field string
		off   int
	}{
		{``, 0}, {`,"hate_pct":100`, 0}, {`,"hate_pct":0`, 100}, {`,"hate_pct":25`, 75}, {`,"hate_pct":150`, 0}, {`,"hate_pct":-5`, 100},
	} {
		d, err := ParseSpellJSON("Test Blizzard", "", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"effects":[
			{"id":"08FE","type":"Dmg/Heal","kind":"hp","amount":-2200,"scale":"dex","scale_pct":100,"level_pct":1909,"resist":"cr"`+c.field+`}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Effects) != 1 || d.Effects[0].Kind != EffectHP || d.Effects[0].HateOff != c.off {
			t.Fatalf("%q: %+v, want one hit of HateOff %d", c.field, d.Effects, c.off)
		}
		if l, ok := leastHarm(d.Effects[0]); !ok || l.HateOff != c.off || l.Amount != -2200 || l.LevelPct != 0 {
			t.Errorf("%q: the least of it %+v", c.field, l)
		}
	}
}

// Landed on a creature, a hit of hate_pct 0 does all its damage and makes no
// hate but the anger every harmful spell gives (1): a creature no one else
// angered fights its caster, and one whose hate someone holds stays on them
// however many land. The control is the same hit without the field: its
// damage is its hate, and it takes the creature off the one who held it.
func TestAHitOfNoHateLeavesTheCreatureOnItsTank(t *testing.T) {
	r := newPetRig(t)
	noResist(t)
	r.p.Class, r.p.Level = 13, 60 // a wizard
	crab := r.enemy
	const tank EntityID = 1002
	spell := func(id int, field string) CastSpell {
		return wbSpell(id, 60, 1, "Test Merciless Blizzard", `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any",
			"effects":[{"id":"08FE","type":"Dmg/Heal","kind":"hp","amount":-2200,"scale":"dex","scale_pct":100,"level_pct":1909,"resist":"cr"`+field+`}]}`)
	}
	quiet, loud := spell(9401, `,"hate_pct":0`), spell(9402, ``)
	reset := func(holder EntityID, held int64) {
		crab.HP, crab.MaxHP, crab.Dead = 5000000, 5000000, false
		crab.Threats, crab.CurrentEnemy = nil, 0
		if holder != 0 {
			crab.Threats = map[EntityID]int64{holder: held}
			crab.CurrentEnemy = holder
		}
	}
	land := func(s CastSpell) int {
		before := crab.HP
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
		return before - crab.HP
	}

	reset(0, 0)
	hit := land(quiet)
	if want := 2200 + 60*1909/100; hit != want {
		t.Fatalf("the quiet hit took %d, want %d (2200 and 19.09 a level)", hit, want)
	}
	if got := crab.Threats[r.p.ServerID]; got != 1 || crab.CurrentEnemy != r.p.ServerID {
		t.Errorf("alone: its hate %d, its enemy %d; want 1 and the caster", got, crab.CurrentEnemy)
	}

	reset(tank, 2000)
	for i := 0; i < 3; i++ {
		if got := land(quiet); got != hit {
			t.Fatalf("quiet hit %d took %d, want %d", i, got, hit)
		}
	}
	if got := crab.Threats[r.p.ServerID]; got != 3 || crab.CurrentEnemy != tank || crab.Threats[tank] != 2000 {
		t.Errorf("three quiet hits beside a tank of 2000: its hate %d, the tank's %d, its enemy %d", got, crab.Threats[tank], crab.CurrentEnemy)
	}

	reset(tank, 2000)
	if got := land(loud); got != hit {
		t.Fatalf("the control took %d, want %d", got, hit)
	}
	if got := crab.Threats[r.p.ServerID]; got != int64(hit)+1 || crab.CurrentEnemy != r.p.ServerID {
		t.Errorf("the control: its hate %d (want %d), its enemy %d (want the caster)", got, hit+1, crab.CurrentEnemy)
	}
}
