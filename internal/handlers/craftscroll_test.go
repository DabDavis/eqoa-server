package handlers

import (
	"fmt"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A recipe scroll used from the inventory (0x003E, the item menu's Use):
// its recipe into the book, one scroll taken (0x003C after CLIENT's own "You
// have scribed a new recipe.", 1); used again, the book holding it, our line
// and nothing taken. The scroll carries the spell scrolls' art, so the
// recipe is looked for first (else: "The spell on this scroll cannot be
// learned here yet.").
func TestCraftRecipeScrollUsed(t *testing.T) {
	f := newCraftFixture(t)
	scroll := f.item("Test Pattern Scroll", -1, 1, 5, 0, 0, scrollIcon, "")
	f.recipe(301, 1, 0, 1, "Test Patterned Plate", f.plate, 1, 10, 40, false, scroll,
		fmt.Sprintf(`[{"item": %d, "count": 1, "keep": false}]`, f.ore))
	f.give(f.hammer, 1, 100, equipHeld)
	f.give(scroll, 2, 0, -1) // list index 1, key 1
	f.loadList()
	eh := &EquipHandler{inventoryStore: f.inv, craft: f.h}
	eh.playerOf = func(*session.Session) *entity.Player {
		return &entity.Player{CharServerID: f.sid, Name: "Smith", Level: 20}
	}
	use := func() [][]byte {
		payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
			w.WriteUint32(1) // the scroll's key
			w.WriteUint32(0)
			w.WriteByte(0)
		})[2:]
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
		msg.SetData(payload)
		if err := eh.HandleInteractItem(f.sess, msg); err != nil {
			t.Fatal(err)
		}
		return f.sent()
	}
	got := use()
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeRemoveInvItem)
	mustBytes(t, "1", got[0], reply(1).u32(0).b)
	mustBytes(t, "the scroll", got[1], removeMsg(1, 1))
	book, err := f.db.CharRecipeBook(f.sid)
	if err != nil || len(book) != 1 || book[0].Recipe != 301 {
		t.Fatalf("the book: %+v (%v)", book, err)
	}
	if f.stack(scroll) != 1 {
		t.Fatalf("%d scrolls left, want 1", f.stack(scroll))
	}
	got = use()
	mustOps(t, got, protocol.OpcodeColoredChat)
	mustBytes(t, "known", got[0], redLine("You already know that recipe."))
	if f.stack(scroll) != 1 {
		t.Fatal("a known recipe's scroll was taken")
	}
	// the control: an item no recipe names is not taken for one (a spell
	// scroll's answer, with no spell store here)
	other := f.item("Test Other Scroll", -1, 1, 5, 0, 0, scrollIcon, "")
	f.give(other, 1, 0, -1)
	f.loadList()
	payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(2)
		w.WriteUint32(0)
		w.WriteByte(0)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(payload)
	eh.HandleInteractItem(f.sess, msg)
	if got := f.sent(); !containsLine(got, scribeNoSpell) {
		t.Fatalf("another scroll: %s, want the spell scroll's refusal", ops(got))
	}
}
