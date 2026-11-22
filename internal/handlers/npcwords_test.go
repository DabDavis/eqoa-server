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

// The filmed coachmen's words (board 659): Coachman Ronks, with Highpass Hold's,
// Muniel's Tea Garden's and Bobble-by-Water's ledgers signed, asks "Welcome my
// friend, do you need to take a horse somewhere?" and offers each as "Get me a
// horse to <town>." in his writing ("Highpass", "the Muniel's Tea Garden"), and
// a pick still goes there; Coachman Billfer asks his question and offers "I
// would like to travel to <town>."; Coachman Dudley signs his ledger with his
// own line. The control: another coachman (Quicksteed) keeps today's question
// and the bare town.
func TestFilmedCoachmensWords(t *testing.T) {
	tw, ronks, book := coachWorld(t)
	book[44] = map[string]bool{"Freeport": true, "Highpass Hold": true, "Muniel's Tea Garden": true, "Bobble-by-Water": true}
	tw.talk(ronks)
	q, opts := askedBox(t, tw.s)
	if q != "Welcome my friend, do you need to take a horse somewhere?" {
		t.Errorf("Ronks asked %q", q)
	}
	want := []string{"Get me a horse to Bobble-by-Water.", "Get me a horse to Highpass.", "Get me a horse to the Muniel's Tea Garden."}
	slices.Sort(opts)
	if !slices.Equal(opts, want) {
		t.Errorf("Ronks offered %q, want %q (in any order)", opts, want)
	}

	billfer := tw.add(2, tw.w.NPCs[101428])
	tw.w.UpdatePosition(tw.p.ServerID, 4813, 58, 17141, 0)
	book[44]["Wyndhaven"] = true
	tw.w.NPCs[1003] = &entity.NPC{ServerID: 1003, Name: "Coachman Brumley", SpawnX: 4715, SpawnY: 54, SpawnZ: 13211, X: 4715, Y: 54, Z: 13211}
	tw.talk(billfer)
	if q, opts := askedBox(t, tw.s); q != "Greetings, are you looking to travel or to sign my Ledger?" ||
		!slices.Equal(opts, []string{"I would like to travel to Wyndhaven."}) {
		t.Errorf("Billfer asked %q %q", q, opts)
	}

	quick := tw.add(3, tw.w.NPCs[415])
	tw.w.UpdatePosition(tw.p.ServerID, 16781, 188, 15353, 0)
	tw.talk(quick)
	if q, opts := askedBox(t, tw.s); q != "Greetings, traveler! Where would you like to go?" || !slices.Contains(opts, "Freeport") {
		t.Errorf("the control, Quicksteed: %q %q", q, opts)
	}

	dudley := tw.add(4, &entity.NPC{ServerID: 1004, Name: "Coachman Dudley", SpawnX: 12971, SpawnY: 54, SpawnZ: 4684})
	tw.w.UpdatePosition(tw.p.ServerID, 12971, 54, 4681, 0)
	tw.talk(dudley)
	if got := said(tw); !slices.Contains(got, "Coachman Dudley: Well, ya can come back to here from Castle Lightwolf and Moradhim now... those dwarves sure know how to drink.") {
		t.Errorf("Dudley's signing said %q", got)
	}
}

// Spiritmaster Kerr's and Keldan's words (board 659's notes).
func TestKerrAndKeldansWords(t *testing.T) {
	if w := bindWordsOf("Spiritmaster Kerr"); w.Ask != "Would you like to be bound?" ||
		!slices.Equal(w.Bound, []string{"You are now bound just outside the great city of Halas."}) {
		t.Errorf("Kerr: %+v", w)
	}
	if w := bindWordsOf("Spiritmaster Keldan"); w.Ask != "Stopping for a night of rest here in Murnf?" || w.Answer != "I would like to be bound." ||
		!slices.Equal(w.Bound, []string{"Very well, it is done. Have a fine day."}) {
		t.Errorf("Keldan: %+v", w)
	}
}

// ronksLabels is the towns as Coachman Ronks's menu words them (board 659).
func ronksLabels(towns ...string) []string {
	var out []string
	for _, t := range towns {
		out = append(out, coachWordsByName["Coachman Ronks"].label(entity.CoachDest{Name: t}))
	}
	return out
}
