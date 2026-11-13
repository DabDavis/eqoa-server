package entity

import (
	"testing"
	"time"
)

// Petition 1949 (the GM Jibwiz): the wizard's hate-lowering line, Stupefy its top, "should
// be about equal to removing 1 sorcerer MCA nuke of threat". The Sorcerer's nuke is the
// Fury (Icy, Flaming, Thunderous: 1455 + 21.39 a level + 40% DEX, and its second hit on
// the same target 856 + 31.37 a level + 40% DEX). Measured with the records as they are on
// the live server, cast by a level 60 wizard of DEX 620 (Jibwiz's) on a level 60 creature: the
// Fury put on 5,797, the old Stupefy (400) took off 399. tools/stupefy_1949_20261113.py gives it
// the Fury's own figures, both hits as one (2311 + 52.76 a level + 80% DEX): within a tenth of it.
func TestStupefyMeasured(t *testing.T) {
	r := newPetRig(t)
	noResist(t)
	r.p.Class, r.p.Level, r.p.Dex = 13, 60, 620
	crab := r.enemy
	crab.Level = 60
	fury := wbSpell(9501, 60, 1, "Icy Fury", `{"v":1,"scope":"target_aoe","target_type":"Enemy","harmful":true,"stack":0,"level":60,"min_level":1,"req_equipment":"Any","sheet_type":"Dmg/Heal","effects":[{"id":"08EB","type":"Dmg/Heal","kind":"hp","amount":-1455,"scale":"dex","scale_pct":40,"level_pct":2139,"resist":"cr"},{"id":"08EB","type":"Dmg/Heal","kind":"hp2","amount":-856,"scale":"dex","scale_pct":40,"level_pct":3137,"resist":"cr"}],"radius":10}`)
	stupefy := wbSpell(9502, 55, 1, "Stupefy", stupefyJSON)
	old := wbSpell(9503, 55, 1, "Stupefy (before)", stupefyOldJSON)
	land := func(s CastSpell) {
		r.w.mu.Lock()
		defer r.w.mu.Unlock()
		r.w.landOnNPCLocked(&castState{spell: s, caster: r.p.ServerID, targets: []EntityID{crab.ServerID}}, r.p, r.sess, crab, r.at(time.Second))
	}
	crab.HP, crab.MaxHP = 5000000, 5000000
	land(fury)
	nuke := crab.Threats[r.p.ServerID]
	land(old)
	was := nuke - crab.Threats[r.p.ServerID]
	land(fury)
	before := crab.Threats[r.p.ServerID]
	land(stupefy)
	removed := before - crab.Threats[r.p.ServerID]
	t.Logf("one Fury: hate %d; the old Stupefy took off %d, the new %d", nuke, was, removed)
	if was != 399 {
		t.Errorf("the old Stupefy took off %d, measured 399", was)
	}
	if removed < nuke*9/10 || removed > nuke*11/10 {
		t.Errorf("Stupefy took off %d of a Fury's %d", removed, nuke)
	}
}

const stupefyOldJSON = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Modify Hate","effects":[{"id":"08D5","type":"Modify Hate","kind":"hate","amount":-400}]}`

// stupefyJSON is Stupefy's record as tools/stupefy_1949_20261113.py writes it.
const stupefyJSON = `{"v":1,"scope":"target","target_type":"Enemy","harmful":true,"stack":0,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Modify Hate","effects":[{"id":"08D5","type":"Modify Hate","kind":"hate","amount":-2311,"level_pct":5276,"scale":"dex","scale_pct":80}]}`
