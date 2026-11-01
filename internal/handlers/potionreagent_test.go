package handlers

import (
	"bytes"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A potion whose line costs an item and gives one (potion.go): the shape of
// Envar's gems (tools/fix_eqoaemu_places.py), each sending its user up one
// chamber and needing the cracked gem the last one left. Against the
// scribe's fixture in a World with one gate row; the names, places and
// numbers are made up.

// opCount is how many of the messages are of op (opsOf, guildfixture_test.go).
func opCount(msgs [][]byte, op protocol.GameOpcode) int {
	n := 0
	for _, o := range opsOf(msgs) {
		if o == op {
			n++
		}
	}
	return n
}

// Without the cracked stone the last one left, the stone is refused in red
// and kept: nothing taken, given or sent (the old reading used it, the
// reagent unread). With it, the stone and the cracked stone go (0x003C
// each), its own cracked stone arrives (0x003B), and its user goes to the
// row's place at the next tick, alone.
func TestAGemTakesTheCrackedGemAndLeavesItsOwn(t *testing.T) {
	f := newScribeFixture(t)
	w := f.potionWorld()
	w.SessionManager = session.NewManager()
	var sent []entity.GateTravel
	w.OnTravel(func(tr entity.GateTravel) { sent = append(sent, tr) })
	w.SetSpellGates(map[string]database.SpellGate{"amber stone": {Spell: "amber stone", Kind: database.GateSelf,
		Place: "Upper Hall of the Glass Spire", World: 1, X: 500, Y: 91.4, Z: 600}})
	stone := f.potion("Amber Stone", -1, 1, 0,
		"Self, Reagent Cracked Opal Stone x1, Teleport to the Upper Hall of the Glass Spire, Summon Cracked Amber Stone x1", "", "")
	cracked := f.item("Cracked Opal Stone", false, 32767, 0, 1)
	f.item("Cracked Amber Stone", false, 32767, 0, 1)
	f.own(stone, 1)
	f.loadList()

	msgs := f.use(0)
	if !bytes.Contains(bytes.Join(msgs, nil), utf16le("Amber Stone needs 1 Cracked Opal Stone; you have 0.")) {
		t.Errorf("without the cracked stone: sent %v, want the red refusal", opsOf(msgs))
	}
	if opCount(msgs, protocol.OpcodeRemoveInvItem) != 0 || opCount(msgs, protocol.OpcodeAddInvItem) != 0 {
		t.Errorf("without the cracked stone: sent %v, want nothing taken or given", opsOf(msgs))
	}
	w.Tick(time.Now())
	if f.stackOf("Amber Stone") != 1 || f.stackOf("Cracked Amber Stone") != 0 || len(sent) != 0 {
		t.Fatalf("without the cracked stone: %d stones, %d of its cracked ones, %d sent; want 1, 0, 0",
			f.stackOf("Amber Stone"), f.stackOf("Cracked Amber Stone"), len(sent))
	}

	f.own(cracked, 1)
	f.loadList()
	msgs = f.use(0)
	if n := opCount(msgs, protocol.OpcodeRemoveInvItem); n != 2 {
		t.Errorf("used: %d taken off (0x003C), want 2 (the stone, the cracked stone): %v", n, opsOf(msgs))
	}
	if n := opCount(msgs, protocol.OpcodeAddInvItem); n != 1 {
		t.Errorf("used: %d given (0x003B), want 1 (its cracked stone): %v", n, opsOf(msgs))
	}
	if f.stackOf("Amber Stone") != 0 || f.stackOf("Cracked Opal Stone") != 0 || f.stackOf("Cracked Amber Stone") != 1 {
		t.Errorf("used: %d stones, %d cracked opal, %d cracked amber; want 0, 0, 1",
			f.stackOf("Amber Stone"), f.stackOf("Cracked Opal Stone"), f.stackOf("Cracked Amber Stone"))
	}
	// the client's list as the database: the cracked amber stone alone
	if l := sessionInvList(f.sess); l == nil || len(l.recs) != 1 || l.recs[0].qty != 1 {
		t.Errorf("the client's list: %+v, want the cracked amber stone alone", l)
	}
	w.Tick(time.Now())
	if len(sent) != 1 || sent[0].P != f.player || sent[0].To.World != 1 || sent[0].To.X != 500 || sent[0].To.Z != 600 ||
		sent[0].To.Place != "Upper Hall of the Glass Spire" || sent[0].Spell != "Amber Stone" {
		t.Errorf("sent %+v, want its user alone to the row's place (no ground: as written)", sent)
	}
}

// The control: a potion of no reagent and no summoning is used as before,
// one off its stack and nothing given.
func TestAPotionOfNoReagentTakesOnlyItself(t *testing.T) {
	f := newScribeFixture(t)
	f.potionWorld()
	d := f.potion("Plain Draught", -1, 3, 0, "Self, Increase Power by 25", "", "")
	f.own(d, 3)
	f.loadList()
	msgs := f.use(0)
	if opCount(msgs, protocol.OpcodeRemoveInvItem) != 1 || opCount(msgs, protocol.OpcodeAddInvItem) != 0 || f.stackOf("Plain Draught") != 2 {
		t.Errorf("sent %v, %d left; want one taken off and nothing given", opsOf(msgs), f.stackOf("Plain Draught"))
	}
}
