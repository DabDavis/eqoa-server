package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/protocol"
)

// The ready check as typed and as vanilla's window answers it (readycheck.go,
// raidcmds.go; petition 1711): /readycheck and a PS2's "/t readycheck", /ready and
// "/t notready" while there is a check to answer, and "/t ready" a tell when there is
// none; the offer's answer and the window's reply.
func TestReadyCheckSpellingsAndWindow(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	cara, _ := c.join(3, "Cara", 25020, 16000, 0)
	c.gw.GroupInvite(alice, c.gw.IDSeenBy(alice, pb))
	c.gw.GroupAccept(bob, c.gw.IDSeenBy(bob, pa))
	c.gw.GroupInvite(alice, c.gw.IDSeenBy(alice, c.gw.Players[3]))
	c.gw.GroupAccept(cara, c.gw.IDSeenBy(cara, pa))
	rh := NewRaidHandler(c.gw, nil)
	// Bob's client answers the offer: the window
	if err := rh.handleReadyAnswer(bob, incoming(protocol.OpcodeReadyCheckAnswer, []byte{1})); err != nil {
		t.Fatal(err)
	}
	if got := buildReadyCheckOffer(); len(got) != 3 || got[2] != 1 || protocol.GameOpcode(binary.LittleEndian.Uint16(got)) != protocol.OpcodeReadyCheckOffer {
		t.Fatalf("the offer % x", got)
	}
	flushQueue(alice)
	flushQueue(bob)
	flushQueue(cara)
	c.say(cara, "/t ready") // no check: a tell to "ready", no one of the name
	if c.gw.HasReadyCheck(cara) {
		t.Fatal("a check from nothing")
	}
	flushQueue(cara)
	c.say(alice, "/t readycheck") // a PS2's
	if got := strings.Join(texts(heard(t, alice)), "|"); !strings.Contains(got, "Ready check sent to your group (2 asked") {
		t.Fatalf("Alice: %q", got)
	}
	if got := strings.Join(texts(heard(t, cara)), "|"); !strings.Contains(got, "Alice asks: Are you ready?") {
		t.Fatalf("Cara: %q", got)
	}
	var id uint32
	for _, m := range bob.Queue().PeekPending() {
		d := m.Data()
		if protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeReadyCheck {
			id = binary.LittleEndian.Uint32(d[2:])
			if d[6] != 30 || string(d[8:8+int(d[7])]) != "Alice" {
				t.Fatalf("Bob's window % x", d)
			}
		}
	}
	if id == 0 {
		t.Fatal("Bob got no window")
	}
	flushQueue(alice)
	reply := binary.LittleEndian.AppendUint32(nil, id)
	if err := rh.handleReadyReply(bob, incoming(protocol.OpcodeReadyCheckReply, append(reply, 1))); err != nil {
		t.Fatal(err)
	}
	if got := texts(heard(t, alice)); len(got) != 0 {
		t.Fatalf("Alice told %q of a yes", got)
	}
	c.say(cara, "/t notready")
	got := texts(heard(t, alice))
	if strings.Join(got, "|") != "Group not ready|Not ready: Cara" {
		t.Fatalf("Alice's report %q", got)
	}
	// /readycheck and #ready as typed by a vanilla
	flushQueue(alice)
	c.say(bob, "/readycheck")
	c.say(alice, "#ready")
	c.say(cara, "/ready")
	if got := texts(heard(t, bob)); !strings.Contains(strings.Join(got, "|"), "Everyone is ready") {
		t.Fatalf("Bob's report %q", got)
	}
}
