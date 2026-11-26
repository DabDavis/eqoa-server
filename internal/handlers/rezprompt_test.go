package handlers

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// rezPrompts is each 0x00D3 in msgs after index from, as Msg00D3 reads it:
// the caster's name and the form byte (1: resurrect).
func rezPrompts(t *testing.T, msgs [][]byte, from int) (names []string, forms []byte) {
	t.Helper()
	for _, d := range msgs[from:] {
		if opcode(d) != protocol.OpcodeSummonPrompt {
			continue
		}
		if len(d) < 2+4 {
			t.Fatalf("a short 0x00D3: % x", d)
		}
		n := int(binary.LittleEndian.Uint32(d[2:]))
		if len(d) < 2+4+n+1 {
			t.Fatalf("a short 0x00D3: % x", d)
		}
		names = append(names, string(d[6:6+n]))
		forms = append(forms, d[6+n])
	}
	return names, forms
}

// lastDump is the index in msgs of the last MemoryDump, -1 for none.
func lastDump(msgs [][]byte) int {
	at := -1
	for i, d := range msgs {
		if opcode(d) == protocol.OpcodeMemoryDump {
			at = i
		}
	}
	return at
}

// Live petition 5665 (2026-11-26), a player: "when sent rez before left where
// i died was sent rez and when got back to spiritmaster was unable to get
// rez". A rez landed 3 s before the respawn: its prompt went up, and the
// respawn's dump took it down unanswered (CLIENT's Msg000D, SetMode(1)),
// while the offer kept waiting here. Now the prompt goes again after the
// dump, the same caster, the resurrect form, the offer's time as it was; its
// Yes takes the player back to the body in the world it died in, and that
// dump shows nothing more. The control: an offer whose time is over is not
// shown (nor is one at a respawn with no rez at all).
func TestARespawnShowsAWaitingRezOfferAgain(t *testing.T) {
	f := newRespawnFixture(t, 3, 4768, 100, 4500)
	f.w.OnRezOffer(func(s *session.Session, caster string) { SendSummonPrompt(s, caster, true) })
	until := time.Now().Add(entity.RezOfferFor - 3*time.Second)
	f.p.RezReturn = entity.RezReturn{World: 3, X: 4768, Y: 100, Z: 4500, Until: until, Caster: "Sloth"}
	f.w.Tick(f.t0.Add(entity.PlayerRespawnDelay))

	sent := f.sent()
	dump := lastDump(sent)
	if dump < 0 || f.p.World != 0 {
		t.Fatalf("no respawn: dump at %d, world %d", dump, f.p.World)
	}
	names, forms := rezPrompts(t, sent, dump+1)
	if len(names) != 1 || names[0] != "Sloth" || forms[0] != 1 {
		t.Fatalf("after the respawn's dump: prompts %q forms %v, want one resurrect prompt from Sloth", names, forms)
	}
	if f.p.RezReturn.Until != until {
		t.Errorf("the offer's time moved: %v, want %v", f.p.RezReturn.Until, until)
	}

	// the client's 0x0014 answers the dump; then the Yes
	if err := f.h.HandleCharacterInWorld(f.sess, protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)); err != nil {
		t.Fatal(err)
	}
	before := len(f.sent())
	yes := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSummonAnswer)
	yes.SetData([]byte{1})
	if err := f.h.handleRezAnswer(f.sess, yes); err != nil {
		t.Fatal(err)
	}
	sent = f.sent()
	back := lastDump(sent)
	if back < before {
		t.Fatal("the Yes sent no dump")
	}
	if _, world, pos, _ := dumpPlace(t, sent[back]); world != 3 || pos != [3]float32{4768, 100, 4500} {
		t.Errorf("the Yes's dump: world %d at %v, want world 3 at the body", world, pos)
	}
	if names, _ := rezPrompts(t, sent, before); len(names) != 0 {
		t.Errorf("the Yes's dump showed the prompt again: %q", names)
	}
	if f.p.RezReturn != (entity.RezReturn{}) {
		t.Errorf("the offer is still there after its Yes: %+v", f.p.RezReturn)
	}

	// controls: an offer over before the respawn, and no offer at all
	for _, r := range []entity.RezReturn{
		{World: 3, X: 4768, Y: 100, Z: 4500, Until: time.Now().Add(-time.Second), Caster: "Sloth"},
		{},
	} {
		g := newRespawnFixture(t, 3, 4768, 100, 4500)
		g.w.OnRezOffer(func(s *session.Session, caster string) { SendSummonPrompt(s, caster, true) })
		g.p.RezReturn = r
		g.w.Tick(g.t0.Add(entity.PlayerRespawnDelay))
		if lastDump(g.sent()) < 0 {
			t.Fatal("control: no respawn")
		}
		if names, _ := rezPrompts(t, g.sent(), 0); len(names) != 0 {
			t.Errorf("control %+v: prompts %q, want none", r, names)
		}
	}
}
