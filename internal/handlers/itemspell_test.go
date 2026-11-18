package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// An item of table itemspell (itemspell.go): a rune whose Use calls a pet takes one of its charges a call,
// none while its pet is out (it says so); a rune whose spell is cast takes one at the cast and then rests;
// one the user's class may not use is refused with nothing spent; an item with no row is not one.
func TestItemSpellRunes(t *testing.T) {
	f := newScribeFixture(t)
	w := entity.NewWorld()
	w.SessionManager = session.NewManager()
	f.player.HP, f.player.MaxHP, f.player.Level, f.player.Power, f.player.MaxPower = 1000, 1000, 60, 10, 5000
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.h.gameWorld = w
	w.NPCs[900] = &entity.NPC{ServerID: 900, Name: "Thexian Bonewing", Level: 55, HP: 100, MaxHP: 100, ModelID: 0x6AF98A83,
		Size: 1, NPCType: 0x80, X: 20000, Z: 20000}
	const summon, defile = 760353, 760355
	for _, s := range []struct {
		id         int
		name, line string
		effects    string
	}{
		{summon, "Summon Thexian Bonewing", "Summons a Thexian Bonewing, a familiar",
			`{"v":1,"scope":"self","target_type":"Self","harmful":false,"stack":0,"level":55,"min_level":1,"req_equipment":"Any","sheet_type":"Summon Pet","effects":[{"id":"V001","type":"Summon Pet","kind":"summon","pet":{"sheet_pet_id":"VALK","name":"Thexian Bonewing"}}]}`},
		{defile, "Defilement", "Self, Increase Power by 150", ""},
	} {
		if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
			iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
			VALUES (?, 55, 0, 0, 16880, 0, 300, 0, 0, 0, 0, -1, ?, '')`, s.id, s.name); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`INSERT INTO spelleffect (spellid, line, fxdict, source, effects) VALUES (?, ?, 0, 'test', ?)`,
			s.id, s.line, s.effects); err != nil {
			t.Fatal(err)
		}
	}
	bonewing := f.item("Rune of the Bonewing", false, 0x7FFF, 55, 1)
	defilement := f.item("Rune of Defilement", false, 0x7C00, 55, 1) // the casters and the alchemist
	plain := f.item("Plain Rune", false, 0x7FFF, 1, 1)
	for _, r := range []struct {
		item, spell int
		rest        float64
	}{{bonewing, summon, 0}, {defilement, defile, 60}} {
		if _, err := f.db.Exec(`INSERT INTO itemeffect (patternid, line, name, charges, source) VALUES (?, 'Self, Increase PR by 1', 'x', 100, 'test')`, r.item); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`INSERT INTO itemspell (patternid, spellid, rest_s) VALUES (?, ?, ?)`, r.item, r.spell, r.rest); err != nil {
			t.Fatal(err)
		}
	}
	f.own(plain, 1)
	f.own(bonewing, 1)
	f.own(defilement, 1)
	f.loadList()
	key := func(pattern int) uint32 {
		t.Helper()
		for i, r := range f.rowsOf() {
			if r.Pattern.PatternID == pattern {
				return uint32(i)
			}
		}
		t.Fatalf("no item %d", pattern)
		return 0
	}
	charges := func(pattern int) int {
		t.Helper()
		for _, r := range f.rowsOf() {
			if r.Pattern.PatternID == pattern {
				return r.Charges
			}
		}
		return -1
	}
	pet := func() *entity.NPC {
		for _, n := range w.NPCs {
			if n.Owner == f.player.ServerID && !n.Dead {
				return n
			}
		}
		return nil
	}
	said := func(sent [][]byte, words string) bool {
		var wide []byte
		for _, c := range words {
			wide = append(wide, byte(c), 0)
		}
		for _, m := range sent {
			if bytes.Contains(m, wide) {
				return true
			}
		}
		return false
	}
	used := func(sent [][]byte) bool {
		for _, m := range sent {
			if opOf(m) == protocol.OpcodeInteractItem {
				return true
			}
		}
		return false
	}

	f.use(key(plain))
	if pet() != nil {
		t.Fatal("a rune of no row brought a pet")
	}
	if charges(bonewing) != 100 || charges(defilement) != 100 {
		t.Fatalf("a new rune holds %d and %d charges, want 100", charges(bonewing), charges(defilement))
	}
	sent := f.use(key(bonewing))
	p := pet()
	if p == nil || p.Name != "Thexian Bonewing" {
		t.Fatalf("the rune brought %+v (%s)", p, ops(sent))
	}
	if charges(bonewing) != 99 || !used(sent) || !said(sent, "answers the Rune of the Bonewing") {
		t.Errorf("after a call: %d charges, %s", charges(bonewing), ops(sent))
	}
	id := p.ServerID
	if sent = f.use(key(bonewing)); !said(sent, "already at your side") || used(sent) || charges(bonewing) != 99 || pet().ServerID != id {
		t.Errorf("with it out: %s, %d charges", ops(sent), charges(bonewing))
	}
	p.Dead = true
	if f.use(key(bonewing)); pet() == nil || charges(bonewing) != 98 {
		t.Errorf("after it fell: pet %v, %d charges", pet(), charges(bonewing))
	}

	// the cast, by a necromancer: no power asked (10 of its 300), a charge, then the rest
	f.player.Class = 11
	if sent = f.use(key(defilement)); !used(sent) || charges(defilement) != 99 {
		t.Fatalf("the Rune of Defilement: %s, %d charges", ops(sent), charges(defilement))
	}
	if f.player.Casting == nil || len(f.player.Lockouts) != 0 {
		t.Errorf("its cast: %v, lockouts %d (an item's has none)", f.player.Casting != nil, len(f.player.Lockouts))
	}
	f.player.Casting = nil
	if sent = f.use(key(defilement)); !said(sent, "is not ready yet") || charges(defilement) != 99 {
		t.Errorf("within its rest: %s, %d charges", ops(sent), charges(defilement))
	}
	// a warrior may not use it: refused, nothing spent
	f.h.itemSpellUsed = nil
	f.player.Class = 0
	if f.use(key(defilement)); charges(defilement) != 99 {
		t.Errorf("a warrior's use spent a charge: %d", charges(defilement))
	}
}
