package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// answerOptions is a client's answer to the race options' offer.
func answerOptions(t *testing.T, h *RaceOptionsHandler, s *session.Session, v uint32) {
	t.Helper()
	m := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceOptions)
	m.SetData(u32b(v))
	if err := h.HandleRaceOptions(s, m); err != nil {
		t.Fatal(err)
	}
}

// optionsSent is the race options a session was last sent, put back together
// from their fragments (every OpcodeRaceOptionsData queued since from).
func optionsSent(s *session.Session, from int) []byte {
	var whole []byte
	pending := s.Queue().PeekPending()
	for _, m := range pending[from:] {
		if d := m.Data(); len(whole) > 0 || opcode(d) == protocol.OpcodeRaceOptionsData {
			if len(whole) > 0 && opcode(d) == protocol.OpcodeRaceLooks {
				break
			}
			whole = append(whole, d...)
		}
	}
	return whole
}

// On a server with the skin shades (world.skin_shades) and no custom race, the
// offer is made; a client that answers 2 (vanilla since 2026-11-11, which shows
// a game race's rows under the four) is sent the game races' rows, one that
// answers 1 (an older vanilla, which would show them instead of the four) the
// options without them. A human made with a shade by the first is a charlook
// row and every client that asked is told its look, a PS2 nothing. The control:
// a server without the shades offers nothing.
func TestSkinShadesOfferAndCreation(t *testing.T) {
	r := newCreationRig(t, entity.CreationRules{Skins: true})
	if _, err := r.db.Exec(`DELETE FROM racedefs`); err != nil {
		t.Fatal(err)
	}
	entity.SetRaceDefs(nil)
	h := NewRaceOptionsHandler(r.h.gameWorld, r.db)
	van, old, ps2 := r.session(), r.session(), r.session()
	for _, s := range []*session.Session{van, old, ps2} {
		r.h.SendCharacterSelectEQOA(s)
		if sentOp(s, protocol.OpcodeRaceOptions) == nil {
			t.Fatal("no offer on a server with the shades")
		}
	}
	at := len(van.Queue().PeekPending())
	answerOptions(t, h, van, 2)
	if got, want := optionsSent(van, at), append([]byte{0x23, 0x7E}, entity.EncodeRaceOptionsFor(true)...); !bytes.Equal(got, want) || got[3] != 10 {
		t.Fatalf("answered 2: %d bytes, want %d (10 races)", len(got), len(want))
	}
	at = len(old.Queue().PeekPending())
	answerOptions(t, h, old, 1)
	if got := optionsSent(old, at); !bytes.Equal(got, []byte{0x23, 0x7E, 1, 0}) {
		t.Fatalf("answered 1: %x, want no race", got)
	}
	if !van.RaceSkins() || old.RaceSkins() {
		t.Fatalf("skins: %v %v", van.RaceSkins(), old.RaceSkins())
	}

	rec := r.createWith(van, "Ebonia", 0, 0, 1, choices(entity.OptionChoice{Key: "skin", Step: 8}))
	if rec == nil || uint32(rec.ModelID) != 0x0D537335 {
		t.Fatalf("a dark human woman: %+v", rec)
	}
	looks, err := r.db.CharLooks(uint32(rec.ServerID))
	if err != nil || len(looks) != 1 || looks[0].Part != "skin" || looks[0].Value>>24 != entity.ModeShade {
		t.Fatalf("her look: %+v %v", looks, err)
	}
	for name, s := range map[string]*session.Session{"answered 2": van, "answered 1": old} {
		if d := sentOp(s, protocol.OpcodeRaceLooks); !bytes.Contains(d, []byte("Ebonia")) {
			t.Errorf("%s: not told her look: %x", name, d)
		}
	}
	if d := sentOp(ps2, protocol.OpcodeRaceLooks); d != nil {
		t.Errorf("a PS2 was told the looks: %x", d)
	}

	none := newCreationRig(t, entity.CreationRules{})
	if _, err := none.db.Exec(`DELETE FROM racedefs`); err != nil {
		t.Fatal(err)
	}
	entity.SetRaceDefs(nil)
	s := none.session()
	none.h.SendCharacterSelectEQOA(s)
	if sentOp(s, protocol.OpcodeRaceOptions) != nil {
		t.Fatal("the control: an offer with no shades and no custom race")
	}
	if rec := none.createWith(s, "Paler", 0, 0, 0, choices(entity.OptionChoice{Key: "skin", Step: 3})); rec != nil {
		t.Fatalf("the control: a shade made on a server without them: %+v", rec)
	}
}

// Character select's Customize re-chooses the shade: the 0x002A of a client
// that answered 2 carries it after the five ints, and the character's skin row
// is replaced (Natural takes it away). Left alone: a PS2's 0x002A (five ints
// alone), the same block from a client that answered 1 or never answered, and
// a block no client could make.
func TestCustomizeRechoosesTheShade(t *testing.T) {
	entity.SetCreationRules(entity.CreationRules{Skins: true})
	t.Cleanup(func() { entity.SetCreationRules(entity.CreationRules{}) })
	ann := annika() // an erudite
	r := newTakeoverRig(t, ann)
	h := NewRaceOptionsHandler(r.w, r.db)
	skin := func() []database.CharLook {
		t.Helper()
		l, err := r.db.CharLooks(ann.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	play := func(level uint32, tail []byte) {
		t.Helper()
		s := r.window(7)
		if level > 0 {
			answerOptions(t, h, s, level)
		}
		data := u32b(ann.ServerID)
		for _, v := range []uint32{1, 2, 2, 0} {
			data = append(data, u32b(v)...)
		}
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectedChar)
		msg.SetData(append(data, tail...))
		if err := r.h.HandleSelectedChar(s, msg); err != nil {
			t.Fatal(err)
		}
		r.w.RemovePlayerBySession(s)
	}
	light := choices(entity.OptionChoice{Key: "skin", Step: 3})
	play(1, light)
	play(0, light)
	play(2, nil)
	play(2, choices(entity.OptionChoice{Key: "skin", Step: 99}))
	play(2, choices(entity.OptionChoice{Key: "body", Step: 1}))
	if l := skin(); len(l) != 0 {
		t.Fatalf("the controls kept %+v", l)
	}
	play(2, light)
	if l := skin(); len(l) != 1 || l[0].Part != "skin" || l[0].Value>>24 != entity.ModeShade {
		t.Fatalf("Light: %+v", l)
	}
	play(2, choices(entity.OptionChoice{Key: "skin", Step: 0}))
	if l := skin(); len(l) != 0 {
		t.Fatalf("Natural again: %+v", l)
	}
}
