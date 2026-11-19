package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// Petition 2411 (the owner, 2026-11-19: "Build the cast event"): a quest event "cast" lands the spell of its id
// on the player as a gift (World.LandOnSelf): Illusion: Brownie's form, its effect on the player's channel, as a
// board's or Bruce Buffer's. A spell id with no record or no effects lands nothing.
func TestAQuestCastEventLandsItsSpell(t *testing.T) {
	f := newQuestFixture(t)
	f.exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (700789, 60, 0, 0, 16544, 5, 120, -1809115846, -1569239993, 1, 1, -1, 'Illusion: Brownie', 'Target, Form, 10min')`)
	f.exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (700789, 'Target, Gives Form, 10min', 2021015237, 'test', ?)`,
		`{"v":1,"scope":"target","target_type":"Friendly","harmful":false,"duration_s":600,"stack":4,"level":60,"min_level":36,"req_equipment":"Any","sheet_type":"Alter Appearance","arch_type":"Illusion","effects":[{"id":"02F3","type":"Alter Appearance","kind":"illusion","sprite":"0x52EB7643"}]}`)
	f.exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power, iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (700790, 60, 0, 0, 16544, 5, 120, 0, 0, 1, 1, -1, 'Nothing Here', '')`)

	f.tw.p.HP, f.tw.p.MaxHP = 500, 500 // alive: a dead player is given nothing
	evs := []database.QuestEvent{{On: database.EventDone, Do: database.EventCast, Spell: 700790}, {On: database.EventDone, Do: database.EventCast, Spell: 1}}
	qu := &database.Quest{ID: 10010, Name: "The pincers"}
	f.qe.mu.Lock()
	c := f.qe.namedLocked(f.tw.p.CharServerID, f.tw.p.Name)
	f.qe.runEventsLocked(f.tw.s, f.tw.p, c, qu, 1, 1, evs)
	f.qe.mu.Unlock()
	if len(f.tw.p.Buffs) != 0 {
		t.Fatalf("a spell with no effects, or no record, landed: %+v", f.tw.p.Buffs)
	}

	f.qe.mu.Lock()
	f.qe.runEventsLocked(f.tw.s, f.tw.p, c, qu, 1, 1, []database.QuestEvent{{On: database.EventDone, Do: database.EventCast, Spell: 700789}})
	f.qe.mu.Unlock()
	found := false
	for _, b := range f.tw.p.Buffs {
		if b.Name == "Illusion: Brownie" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Illusion: Brownie did not land: buffs %+v", f.tw.p.Buffs)
	}
}

// A cast event is checked as the panel saves it: it must name a spell.
func TestACastEventNamesASpell(t *testing.T) {
	bad := database.EventProblems(`[{"on":"done","do":"cast"}]`, true)
	if len(bad) != 1 || !strings.Contains(bad[0], "cast names no spell") {
		t.Fatalf("a cast with no spell: %v", bad)
	}
	if ok := database.EventProblems(`[{"on":"done","do":"cast","spell":700789}]`, true); len(ok) != 0 {
		t.Fatalf("a cast of 700789: %v", ok)
	}
}
