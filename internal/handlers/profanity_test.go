package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The words a tester shouted (F3 20261021-011737) are starred, whatever
// their case or suffix and with their punctuation kept; words that only
// begin like one, or hold one inside, are not (the control).
func TestMaskProfanity(t *testing.T) {
	for in, want := range map[string]string{
		"shit cunt penis":                          "**** **** *****",
		"FUCKING hell, shitty!":                    "******* hell, ******!",
		"Jibrish shouts: penis":                    "Jibrish shouts: *****",
		"Scunthorpe cockatrice assist class grass": "Scunthorpe cockatrice assist class grass",
		"you ass": "you ***",
	} {
		if got := maskProfanity(in); got != want {
			t.Errorf("maskProfanity(%q) = %q, want %q", in, got, want)
		}
	}
}

// A name with a root anywhere in it is refused at creation; the names that
// hide a short word inside a fair one are not.
func TestProfaneNamesAreRefused(t *testing.T) {
	h := &CharacterHandler{}
	for _, n := range []string{"Shitlord", "Fuckface", "Penisman", "Bigbitch"} {
		if h.isValidName(n) {
			t.Errorf("%s was taken", n)
		}
	}
	for _, n := range []string{"Grapes", "Hancock", "Cassandra", "Titania", "Dickens", "Funyun", "Jibrish"} {
		if !h.isValidName(n) {
			t.Errorf("%s was refused", n)
		}
	}
}

// Each hearer gets the line its own filter makes: on at login, so masked;
// turned off with 0x0045 (answered with the new state, zigzag 0), the line
// as said; on again (zigzag 1), masked.
func TestProfanityFilterIsEachHearers(t *testing.T) {
	c := newChatWorld(t)
	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	cs, _ := c.join(3, "Cid", 120, 120, 0)
	if err := c.h.HandleProfanityFilter(cs, incoming(protocol.OpcodeProfanityFilter, nil)); err != nil {
		t.Fatal(err)
	}
	reply := cs.Queue().GetPending()
	if len(reply) != 1 || string(reply[0].Data()) != "\x45\x00\x00" {
		t.Fatalf("Cid's toggle: % x", reply[0].Data())
	}
	c.say(as, "well shit")
	if got := texts(heard(t, bs)); len(got) != 1 || got[0] != "Ann: well ****" {
		t.Fatalf("Bob (filter on) heard %v", got)
	}
	if got := texts(heard(t, cs)); len(got) != 1 || got[0] != "Ann: well shit" {
		t.Fatalf("Cid (filter off) heard %v", got)
	}
	cs.Queue().GetPending()
	c.h.HandleProfanityFilter(cs, incoming(protocol.OpcodeProfanityFilter, nil))
	if reply := cs.Queue().GetPending(); len(reply) != 1 || string(reply[0].Data()) != "\x45\x00\x02" {
		t.Fatalf("Cid's second toggle: % x", reply[0].Data())
	}
}

// /filter in the chat line turns the filter over as 0x0045 does (the
// retail menus never send it), for anyone, and is not said; the control:
// a line that only starts with the word is said.
func TestSlashFilterTurnsItOver(t *testing.T) {
	c := newChatWorld(t)
	as, ap := c.join(1, "Ann", 100, 100, 0)
	c.say(as, "/FILTER")
	if !ap.ProfanityFilterOff {
		t.Fatal("/FILTER left the filter on")
	}
	if got := as.Queue().GetPending(); len(got) != 1 || string(got[0].Data()) != "\x45\x00\x00" {
		t.Fatalf("/filter's answer: %d messages", len(got))
	}
	c.say(as, "/profanity")
	if ap.ProfanityFilterOff {
		t.Fatal("/profanity left the filter off")
	}
	as.Queue().GetPending()
	c.say(as, "filtered water")
	if got := texts(heard(t, as)); len(got) != 1 || got[0] != "Ann: filtered water" {
		t.Fatalf("a plain line: %v", got)
	}
}

// The client's setting (OpcodeProfanityChoice, petition 1490) sets the
// hearer's filter, never turns it over: 0 off (the line as said), 0 again
// still off, 1 on (masked); nothing is answered, and the speaker's own filter
// does not change what another hears. The controls: a short answer changes
// nothing, and Bob, who never answered (a PS2), keeps his on.
func TestProfanityChoiceSetsTheHearersFilter(t *testing.T) {
	c := newChatWorld(t)
	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	cs, cp := c.join(3, "Cid", 120, 120, 0)
	if got := buildProfanityChoiceOffer(); string(got) != "\x60\x7e\x01" {
		t.Fatalf("the offer: % x", got)
	}
	choose := func(b ...byte) {
		t.Helper()
		if err := c.h.HandleProfanityChoice(cs, incoming(protocol.OpcodeProfanityChoice, b)); err != nil {
			t.Fatal(err)
		}
	}
	choose()
	if cp.ProfanityFilterOff {
		t.Fatal("a short answer turned the filter off")
	}
	choose(0)
	choose(0, 9)
	if !cp.ProfanityFilterOff {
		t.Fatal("0 left the filter on")
	}
	if got := cs.Queue().GetPending(); len(got) != 0 {
		t.Fatalf("the choice was answered: %d messages", len(got))
	}
	c.say(as, "well shit")
	if got := texts(heard(t, cs)); len(got) != 1 || got[0] != "Ann: well shit" {
		t.Fatalf("Cid (off by his setting) heard %v", got)
	}
	if got := texts(heard(t, bs)); len(got) != 1 || got[0] != "Ann: well ****" {
		t.Fatalf("Bob (no setting) heard %v", got)
	}
	cs.Queue().GetPending()
	choose(1)
	if cp.ProfanityFilterOff {
		t.Fatal("1 left the filter off")
	}
	c.say(as, "well shit")
	if got := texts(heard(t, cs)); len(got) != 1 || got[0] != "Ann: well ****" {
		t.Fatalf("Cid (on again) heard %v", got)
	}
}

// The three tiers (petition 1625), on one line with a swear, "retarded", two
// racial slurs (built from parts) and the controls a slur is inside or begins
// like ("snigger", "spice", "chink" of armour): Off hears it as said; Medium
// only the slurs starred, the swear and "retarded" kept; High all of them.
func TestProfanityTiers(t *testing.T) {
	s1, s2 := "ni"+"gga", "k"+"ike"
	line := "shit retarded " + s1 + " " + s2 + "s snigger spice chink"
	p := &entity.Player{}
	p.ProfanityFilterOff = true
	if got := lineFor(p, line); got != line {
		t.Errorf("off: %q", got)
	}
	p.ProfanityFilterOff, p.ProfanityFilterMedium = false, true
	if got, want := lineFor(p, line), "shit retarded ***** ***** snigger spice chink"; got != want {
		t.Errorf("medium: %q, want %q", got, want)
	}
	p.ProfanityFilterMedium = false
	if got, want := lineFor(p, line), "**** ******** ***** ***** snigger spice chink"; got != want {
		t.Errorf("high: %q, want %q", got, want)
	}
	// every slur is High's too
	for w := range slurWords {
		if maskProfanity(w) == w {
			t.Errorf("high missed a slur word (%d letters)", len(w))
		}
	}
	for _, r := range slurRoots {
		if maskProfanity(r+"s") == r+"s" {
			t.Errorf("high missed a slur root (%d letters)", len(r))
		}
	}
	// Medium masks no word of the High lists that is not a slur
	for w := range profaneWords {
		if !slur(w) && maskWords(w, slur) != w {
			t.Errorf("medium masked %q", w)
		}
	}
}

// The client's setting by tier: 2 Medium, 1 (an older client's On) and any
// other nonzero High, 0 Off; /filter from Medium turns it off, then on at High.
func TestProfanityChoiceTiers(t *testing.T) {
	c := newChatWorld(t)
	as, _ := c.join(1, "Ann", 100, 100, 0)
	cs, cp := c.join(3, "Cid", 120, 120, 0)
	choose := func(b byte) {
		t.Helper()
		if err := c.h.HandleProfanityChoice(cs, incoming(protocol.OpcodeProfanityChoice, []byte{b})); err != nil {
			t.Fatal(err)
		}
	}
	slurred := "well shit " + "wet" + "back"
	hear := func(want string) {
		t.Helper()
		cs.Queue().GetPending()
		c.say(as, slurred)
		if got := texts(heard(t, cs)); len(got) != 1 || got[0] != "Ann: "+want {
			t.Fatalf("Cid heard %v, want %q", got, want)
		}
	}
	choose(2)
	hear("well shit *******")
	choose(1)
	hear("well **** *******")
	choose(2)
	choose(7)
	hear("well **** *******")
	choose(0)
	hear(slurred)
	choose(2)
	c.h.HandleProfanityFilter(cs, nil)
	if !cp.ProfanityFilterOff {
		t.Fatal("/filter from Medium left it on")
	}
	c.h.HandleProfanityFilter(cs, nil)
	if cp.ProfanityFilterOff || cp.ProfanityFilterMedium {
		t.Fatal("/filter from Off is not High")
	}
}
