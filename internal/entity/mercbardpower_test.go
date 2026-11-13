package entity

import (
	"testing"
	"time"
)

// A player bard's power songs reach its group's mercs (Deathcore's petition
// 1909: "bard power abilities dont affect mercs"): Power Dance (the group's
// power over time) and Power Ballad (the group's power now) as the database
// holds them, and Celedonicus' Grand Finale (a self spell whose +5000 power
// is its group's in its radius).
func TestMercTakesBardPower(t *testing.T) {
	type song struct {
		id                int
		name, line, json  string
		scope             int
		wantEffect, check bool
	}
	songs := []song{
		{164, "Power Dance", "Group, Increase Power per tic by 51 ; Increase 20% based on CHA, Radius 30, 24sec",
			`{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"duration_s":24,"stack":3,"radius":30,"effects":[{"id":"00AF","type":"Statistic Change","kind":"stat","stat":"pot","amount":51,"scale":"cha","scale_pct":20}]}`,
			2, true, false},
		{701200, "Power Ballad", "Group, Increase Power by 1000, Radius 30",
			`{"v":1,"scope":"group","target_type":"Friendly","harmful":false,"stack":3,"radius":30,"effects":[{"id":"00FA","type":"Statistic Change","kind":"power","amount":1000}]}`,
			2, false, true},
		{701118, "Celedonicus' Grand Finale", "Self, Increase Power by 5000, Decrease Power by 5000, PB AE",
			`{"v":1,"scope":"self","target_type":"Friendly","harmful":false,"radius":30,"effects":[{"id":"00F9","type":"Statistic Change","kind":"power","amount":5000,"scope":"group"},{"id":"00FB","type":"Statistic Change","kind":"power","amount":-5000,"scope":"self"}]}`,
			0, false, true},
	}
	for _, sg := range songs {
		t.Run(sg.name, func(t *testing.T) {
			r := newMercRig(t)
			r.p.Class = classBard
			ms := r.hire(r.kit(10, 0, 0), MercPassive)
			def := SpellDefFrom(sg.name, sg.line, "", sg.json) // as the book's loader reads it
			s := CastSpell{SpellID: sg.id, DisplayIndex: sg.id, Level: 1, Scope: sg.scope, Name: sg.name, Range: 30, Def: def, Area: ReadSpellArea(sg.name, sg.line, def)}
			m := ms.npc
			r.run(0, time.Second)
			m.Power = 0
			r.p.Power = r.p.MaxPowerNow()
			if !r.w.StartCast(r.sess, s, 0, r.at(2*time.Second)) {
				t.Fatal("the cast was refused")
			}
			r.run(2*time.Second, 4*time.Second)
			if sg.wantEffect {
				on := false
				for _, ae := range m.Effects {
					on = on || ae.SpellID == sg.id
				}
				if !on {
					t.Fatalf("%q is not on the merc: %d effects", sg.name, len(m.Effects))
				}
				if ms.body.PoTNow() <= ms.body.PoTBase() {
					t.Errorf("the merc's PoT %d, its base %d", ms.body.PoTNow(), ms.body.PoTBase())
				}
			}
			if sg.check && m.Power < 900 && m.Power < m.MaxPower {
				t.Errorf("the merc's power %d/%d after %q", m.Power, m.MaxPower, sg.name)
			}
		})
	}
}
