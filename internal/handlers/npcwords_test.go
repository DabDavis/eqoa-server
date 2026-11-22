package handlers

import (
	"encoding/binary"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// askedBox is the last option box queued for the session (taken): its
// question and its answers.
func askedBox(t *testing.T, s *session.Session) (string, []string) {
	t.Helper()
	q, opts, found := "", []string(nil), false
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 6 || protocol.GameOpcode(binary.LittleEndian.Uint16(d)) != protocol.OpcodeQuestPopupMulti {
			continue
		}
		p := 6
		str := func() string {
			n := int(binary.LittleEndian.Uint32(d[p:]))
			u := make([]uint16, n)
			for i := range u {
				u[i] = binary.LittleEndian.Uint16(d[p+4+2*i:])
			}
			p += 4 + 2*n
			return string(utf16.Decode(u))
		}
		q, opts, found = str(), nil, true
		n := int(d[p])
		p++
		for range n {
			opts = append(opts, str())
		}
	}
	if !found {
		t.Fatal("no option box")
	}
	return q, opts
}

// A spiritmaster with no words of its own asks the live game's "Would you
// like to be bound?" with the one answer "Yes, bind me." and says the shared
// bound line (board 813); a filmed one asks, answers and says its own:
// Spiritmaster Lo his question and answer and his line ("honorabe" as
// filmed), Spiritmaster Colanin his two lines in order.
func TestSpiritmastersWords(t *testing.T) {
	for _, c := range []struct {
		name, ask, answer string
		bound             []string
	}{
		{"Spiritmaster Savi", "Would you like to be bound?", "Yes, bind me.", []string{"You are now bound to this place."}},
		{"Spiritmaster Lo", "Are you here to learn the arts or to bind your spirit to this area?",
			"I need to bind my spirit to this area, Master Lo.",
			[]string{"The winds of Norrath shall bring your spirit back to this area if you die. Make sure your death is an honorabe one."}},
		{"Spiritmaster Duelau", "Would you like to be bound?", "Yes, bind me.", []string{"If I must."}},
		{"Spiritmaster Colanin", "Would you like to be bound?", "Yes, bind me.",
			[]string{"Another weakling are ye? Here ye go.", "Go slay some beasts now, if ye die... well... don't die."}},
	} {
		tw := newTalkWorld(t, 23594, 6304)
		tw.p.Y = 32.6
		id := tw.add(1, &entity.NPC{ServerID: 101714, Name: c.name, SpawnX: 23594.3, SpawnY: 32.6, SpawnZ: 6300.9})
		tw.talk(id)
		if q, opts := askedBox(t, tw.s); q != c.ask || !slices.Equal(opts, []string{c.answer}) {
			t.Errorf("%s asked %q %q, want %q [%q]", c.name, q, opts, c.ask, c.answer)
		}
		tw.pick(id, 0)
		var want []string
		for _, l := range c.bound {
			want = append(want, c.name+": "+l)
		}
		got := slices.DeleteFunc(said(tw), func(l string) bool { return l == "" })
		if !slices.Equal(got, want) {
			t.Errorf("%s said %q, want %q", c.name, got, want)
		}
		if _, bound := tw.w.BindOf(tw.p.ServerID); !bound {
			t.Errorf("%s: the answer bound no one", c.name)
		}
	}
}
