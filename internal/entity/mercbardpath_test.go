package entity

import (
	"testing"
	"time"
)

// A level 60 bard merc of the power path sings its path's power songs
// (Kytank's petition 1993: "Bard merc doesn't seem to be using power path
// spells (at 60)"). Its book holds Power Jig and Power Vibe (the path's 40
// and 49) and Harmony (55, of the Concerto line), all three the workbook's
// stack 3 +PoT line as the database holds them: by rank Harmony took the
// power songs out of the book, so the bard never sang them. Now Power Vibe,
// the line's biggest figure, is kept and sung, and lands on its owner.
func TestMercBardSingsPowerPath(t *testing.T) {
	song := func(id int, name string, level int, recast time.Duration, power int, line, json string) CastSpell {
		def := SpellDefFrom(name, line, "", json) // as the book's loader reads it
		return CastSpell{SpellID: id, DisplayIndex: id, Level: level, Scope: 2, Name: name, Range: 30, Def: def,
			Recast: recast, PowerCost: power, Area: ReadSpellArea(name, line, def)}
	}
	jig := song(192, "Power Jig", 40, 48*time.Second, 440, "Group, Increase Power per tic by 101 ; Increase 20% based on CHA, Radius 30, 24sec",
		`{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":24,"stack":3,"level":40,"min_level":24,"radius":30,"effects":[{"id":"00D1","type":"Statistic Change","kind":"stat","stat":"pot","amount":101,"scale":"cha","scale_pct":20}]}`)
	vibe := song(700631, "Power Vibe", 49, 48*time.Second, 540, "Group, Increase Power per tic by 134 ; Increase 20% based on CHA, Radius 30, 24sec",
		`{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":24,"stack":3,"level":49,"min_level":29,"radius":30,"effects":[{"id":"00E0","type":"Statistic Change","kind":"stat","stat":"pot","amount":134,"scale":"cha","scale_pct":20}]}`)
	harmony := song(700714, "Harmony", 55, 5*time.Second, 40, "Group, Increase Power per tic by 30 ; Increase 5% based on CHA, Radius 30, 150sec",
		`{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":150,"stack":3,"level":55,"min_level":33,"radius":30,"effects":[{"id":"00E9","type":"Statistic Change","kind":"stat","stat":"pot","amount":30,"scale":"cha","scale_pct":5}]}`)
	r := newMercRig(t)
	r.p.Level = 60
	ms := r.hire(r.kit(classBard, 0, 0, jig, harmony, vibe), MercAssist)
	var kept []string
	for _, sp := range ms.mercBook() {
		if sp.use == useBuff {
			kept = append(kept, sp.s.Name)
		}
	}
	if len(kept) != 1 || kept[0] != "Power Vibe" {
		t.Fatalf("the bard's songs kept: %q, want Power Vibe alone", kept)
	}
	// sung within its first turns, then on its recast: on its owner and the
	// bard as it lands, each time
	var onOwner, onBard int
	for d := time.Duration(0); d <= 2*time.Minute; d += 100 * time.Millisecond {
		r.tick(d)
		for _, e := range r.p.Effects {
			if e.SpellID == vibe.SpellID {
				onOwner++
			}
		}
		if buffOn(ms.npc.Effects, &vibe) {
			onBard++
		}
	}
	if _, sung := ms.ready[vibe.SpellID]; !sung {
		t.Fatal("Power Vibe was never sung")
	}
	// 24 s of every 48 s recast: about half of two minutes' ticks
	if onOwner < 400 || onBard < 400 {
		t.Errorf("Power Vibe on the owner %d, the bard %d of 1200 ticks", onOwner, onBard)
	}
}
