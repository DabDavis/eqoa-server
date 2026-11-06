package handlers

import (
	"net"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lootLinesOf are the chat lines among messages sent.
func lootLinesOf(msgs [][]byte) []string {
	var out []string
	for _, m := range msgs {
		if opOf(m) == protocol.OpcodeClientMessage {
			line, _ := readWideLine(m[2:])
			out = append(out, line)
		}
	}
	return out
}

// Petition 1119: "loot messeges arent working. toggled on n off, even reset
// client." A take from a corpse says what it gave, a take-all each thing; the
// client's Loot Messages option (0x00CA, state 1 hides) sends none, the coins'
// line included, and on again sends them. On the code before, 0x00CA only logged
// and no item ever had a line.
func TestLootLinesFollowTheOption(t *testing.T) {
	f := newCorpseFixture(t, 2)
	f.loadLists()
	h := &FactionHandler{}
	option := func(state byte) {
		t.Helper()
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLootMessages)
		msg.SetData([]byte{state, 1})
		if err := h.HandleLootMessages(f.sess, msg); err != nil {
			t.Fatal(err)
		}
	}
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	if got := lootLinesOf(f.sent()); len(got) != 1 || !strings.HasPrefix(got[0], "You receive ") || !strings.HasSuffix(got[0], " from the corpse.") {
		t.Fatalf("a take said %q, want what it gave", got)
	}
	option(1)
	f.send(protocol.OpcodeLootTakeAll, nil)
	if got := lootLinesOf(f.sent()); len(got) != 0 {
		t.Errorf("a take-all with the option off said %q", got)
	}
	if !f.sess.LootMessagesOff() {
		t.Error("0x00CA state 1: the session does not hold it off")
	}

	g := newCorpseFixture(t, 2)
	g.loadLists()
	g.h.OpenCorpse(g.sess, g.npc)
	g.sent()
	g.send(protocol.OpcodeLootTakeAll, nil)
	if got := lootLinesOf(g.sent()); len(got) != 2 {
		t.Errorf("a take-all of two said %q, want a line each", got)
	}

	if got := lootStackLine(3, "arrow"); got != "You receive 3 arrow from the corpse." {
		t.Errorf("a stack's line %q", got)
	}
	option(0)
	if f.sess.LootMessagesOff() {
		t.Error("0x00CA state 0: the session still holds it off")
	}
}

// The coins' line goes with the option too.
func TestTheCoinsLineFollowsTheOption(t *testing.T) {
	f := newCorpseFixture(t, 0)
	f.sess.SetLootMessagesOff(true)
	f.h.giveCorpseCoins(f.sess, f.npc, 3)
	if got := lootLinesOf(f.sent()); len(got) != 0 {
		t.Errorf("with the option off: %q", got)
	}
	f.sess.SetLootMessagesOff(false)
	f.h.giveCorpseCoins(f.sess, f.npc, 3)
	if got := lootLinesOf(f.sent()); len(got) != 1 || got[0] != "You receive 3 tunar from the corpse." {
		t.Errorf("with it on: %q", got)
	}
}

// Petition 1145, Larrybird: "i cant see what my other grp is loooting could be uber rare gear". A take
// from a corpse says what it gave to the looter's group, each by its own Loot Messages option; a take-all a
// line an item. The control: with no group, nobody else hears.
func TestTheGroupHearsWhatIsLooted(t *testing.T) {
	f := newCorpseFixture(t, 2)
	f.loadLists()
	mate := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 77), Port: 7077}, 77, 1, 1, 2, true)
	off := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 78), Port: 7078}, 78, 1, 1, 2, true)
	opt := &FactionHandler{}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLootMessages)
	msg.SetData([]byte{1, 1})
	if err := opt.HandleLootMessages(off, msg); err != nil {
		t.Fatal(err)
	}
	f.h.groupOf = func(*entity.Player) []*session.Session { return []*session.Session{mate, off} }
	lines := func(s *session.Session) []string {
		var msgs [][]byte
		for _, m := range s.Queue().GetPending() {
			msgs = append(msgs, append([]byte(nil), m.Data()...))
		}
		return lootLinesOf(msgs)
	}
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })
	got := lines(mate)
	if len(got) != 1 || !strings.Contains(got[0], " receives ") || !strings.HasSuffix(got[0], " from the corpse.") {
		t.Fatalf("the group heard %q", got)
	}
	if h := lines(off); len(h) != 0 {
		t.Errorf("a member with the option off heard %q", h)
	}
	f.send(protocol.OpcodeLootTakeAll, nil)
	if h := lines(mate); len(h) != 1 {
		t.Errorf("a take-all of the one left: the group heard %q", h)
	}
	g := newCorpseFixture(t, 1)
	g.loadLists()
	g.h.groupOf = func(*entity.Player) []*session.Session { return nil }
	g.h.OpenCorpse(g.sess, g.npc)
	g.sent()
	g.send(protocol.OpcodeLootTakeAll, nil)
	if h := lootLinesOf(g.sent()); len(h) != 1 {
		t.Errorf("no group: the looter heard %q", h)
	}
}
