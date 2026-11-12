package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// On a server with the shades and the barbarians' tattoo (world.tattoos), a
// client that answers the offer 3 (vanilla since 2026-11-12, which draws it) is
// sent the tattoo row, one that answers 2 the rows it was sent before the
// tattoo; a barbarian man made with the crest by the first is a charlook row
// "tattoo" and both are told his look, a PS2 nothing. A client that answered 2
// may still choose it (the server holds a choice to the race's rows, not to
// what it was shown).
func TestTattooOfferAndCreation(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Skins: true, Tattoos: true})
	if _, err := r.db.Exec(`DELETE FROM racedefs`); err != nil {
		t.Fatal(err)
	}
	entity.SetRaceDefs(nil)
	h := NewRaceOptionsHandler(r.h.gameWorld, r.db)
	van, old, ps2 := r.session(), r.session(), r.session()
	for _, s := range []*session.Session{van, old, ps2} {
		r.h.SendCharacterSelectEQOA(s)
	}
	at := len(van.Queue().PeekPending())
	answerOptions(t, h, van, 3)
	if got, want := optionsSent(van, at), append([]byte{0x23, 0x7E}, entity.EncodeRaceOptionsAt(3)...); !bytes.Equal(got, want) || !bytes.Contains(got, []byte("Tattoo")) {
		t.Fatalf("answered 3: %d bytes, want %d with the tattoo", len(got), len(want))
	}
	at = len(old.Queue().PeekPending())
	answerOptions(t, h, old, 2)
	if got, want := optionsSent(old, at), append([]byte{0x23, 0x7E}, entity.EncodeRaceOptionsFor(true)...); !bytes.Equal(got, want) || bytes.Contains(got, []byte("Tattoo")) {
		t.Fatalf("answered 2: %d bytes, want %d without the tattoo", len(got), len(want))
	}
	if van.RaceLooksLevel() != 3 || old.RaceLooksLevel() != 2 || !van.RaceSkins() {
		t.Fatalf("levels %d %d", van.RaceLooksLevel(), old.RaceLooksLevel())
	}

	rec := r.createWith(van, "Woadrik", 6, 0, 0, choices(entity.OptionChoice{Key: "tattoo", Step: 3}))
	if rec == nil || uint32(rec.ModelID) != 0x61CA3EDF {
		t.Fatalf("a barbarian man: %+v", rec)
	}
	looks, err := r.db.CharLooks(uint32(rec.ServerID))
	if err != nil || len(looks) != 1 || looks[0].Part != "tattoo" || looks[0].Value != entity.Tattoo(2) {
		t.Fatalf("his look: %+v %v", looks, err)
	}
	for name, s := range map[string]*session.Session{"answered 3": van, "answered 2": old} {
		if d := sentOp(s, protocol.OpcodeRaceLooks); !bytes.Contains(d, []byte("Woadrik")) {
			t.Errorf("%s: not told his look: %x", name, d)
		}
	}
	if d := sentOp(ps2, protocol.OpcodeRaceLooks); d != nil {
		t.Errorf("a PS2 was told the looks: %x", d)
	}
	if rec := r.createWith(old, "Woadra", 6, 0, 1, choices(entity.OptionChoice{Key: "tattoo", Step: 1})); rec == nil {
		t.Fatal("her bands from a client that answered 2: refused")
	}
}
