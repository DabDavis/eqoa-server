package handlers

import (
	"testing"

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
