package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/session"
)

// pizzaBoxes is the dialogue boxes (0x0046) s was sent, in order.
func pizzaBoxes(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().PeekPending() {
		if b := mobBox(m.Data()); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// Ten facts, all different, each with the headline and its number in one box
// (CLIENT drops a 0x0046's 218th character) and no '%' (its text expander's token).
func TestPizzaFactsFitTheBox(t *testing.T) {
	if len(pizzaFacts) != 10 {
		t.Fatalf("%d facts, want 10", len(pizzaFacts))
	}
	seen := map[string]bool{}
	for i, f := range pizzaFacts {
		if seen[f] {
			t.Errorf("fact %d twice: %q", i+1, f)
		}
		seen[f] = true
		box := pizzaBox(i)
		if n := len([]rune(box)); n > questBoxChars {
			t.Errorf("fact %d: box of %d characters, room for %d: %q", i+1, n, questBoxChars, box)
		}
		if strings.Contains(box, "%") {
			t.Errorf("fact %d holds a %%: %q", i+1, box)
		}
		if !strings.HasPrefix(box, pizzaHeadline+"\n") {
			t.Errorf("fact %d: no headline: %q", i+1, box)
		}
	}
}

// Every spelling a client sends shows the typer one box, the headline and a fact, and
// no one near hears a thing; a tell with words to a player named Pizza is still a tell.
func TestPizzaShowsTheTyperABox(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, _ := c.join(2, "Bob", 25010, 16000, 0)
	for _, l := range []string{"/pizza", "/PIZZA", "#pizza", "/t pizza", "/tell Pizza", "/pizza please"} {
		flushQueue(alice)
		flushQueue(bob)
		c.h.pizzaMu.Lock()
		delete(c.h.pizzaSeen, pa.CharServerID) // each spelling its own first use
		c.h.pizzaMu.Unlock()
		c.say(alice, l)
		got := pizzaBoxes(alice)
		if len(got) != 1 {
			t.Fatalf("%s: %d boxes %q", l, len(got), got)
		}
		i := c.h.pizzaSeen[pa.CharServerID].fact
		if got[0] != pizzaBox(i) {
			t.Fatalf("%s: box %q, want fact %d's %q", l, got[0], i+1, pizzaBox(i))
		}
		if n := len(bob.Queue().PeekPending()); n != 0 {
			t.Fatalf("%s: Bob was sent %d messages", l, n)
		}
	}
	for _, l := range []string{"/t pizza hello", "/pizzas", "pizza"} {
		if pizzaLine(l) {
			t.Errorf("%q read as /pizza", l)
		}
	}
	// /help lists it, and "/pi ?" finds it
	if !strings.Contains(strings.Join(playerHelpLines, "\n"), "/pizza") {
		t.Error("/help lacks /pizza")
	}
	if got := strings.Join(commandsBeginning("/pi", false), "\n"); !strings.Contains(got, "/pizza") {
		t.Errorf("/pi ? finds %q", got)
	}
}

// The pick is at random and never the fact the player saw last; a second /pizza
// within pizzaEvery shows nothing; each player's last fact is its own.
func TestPizzaPicksAnotherFactEachTime(t *testing.T) {
	c := newChatWorld(t)
	alice, pa := c.join(1, "Alice", 25000, 16000, 0)
	bob, pb := c.join(2, "Bob", 25010, 16000, 0)
	defer func(f func(int) int) { pizzaPick = f }(pizzaPick)

	// the first is any of the ten
	pizzaPick = func(n int) int {
		if n != len(pizzaFacts) {
			t.Fatalf("first pick of %d", n)
		}
		return 4
	}
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	flushQueue(alice)
	c.h.pizzaFact(alice, pa, now)
	if got := pizzaBoxes(alice); len(got) != 1 || got[0] != pizzaBox(4) {
		t.Fatalf("first: %q", got)
	}

	// within pizzaEvery: nothing
	flushQueue(alice)
	c.h.pizzaFact(alice, pa, now.Add(pizzaEvery-time.Millisecond))
	if got := pizzaBoxes(alice); len(got) != 0 {
		t.Fatalf("too soon: %q", got)
	}

	// after it: one of the other nine, every one of them reachable, never fact 5 again
	last, at := 4, now
	reached := map[int]bool{}
	for k := 0; k < len(pizzaFacts)-1; k++ {
		pizzaPick = func(n int) int {
			if n != len(pizzaFacts)-1 {
				t.Fatalf("a later pick of %d", n)
			}
			return k
		}
		// each k from the same last, so put it back each time
		c.h.pizzaMu.Lock()
		c.h.pizzaSeen[pa.CharServerID] = pizzaShown{at: at, fact: last}
		c.h.pizzaMu.Unlock()
		at = at.Add(pizzaEvery)
		flushQueue(alice)
		c.h.pizzaFact(alice, pa, at)
		i := c.h.pizzaSeen[pa.CharServerID].fact
		if i == last {
			t.Fatalf("pick %d showed fact %d again", k, last+1)
		}
		if got := pizzaBoxes(alice); len(got) != 1 || got[0] != pizzaBox(i) {
			t.Fatalf("pick %d: %q", k, got)
		}
		reached[i] = true
	}
	if len(reached) != len(pizzaFacts)-1 {
		t.Fatalf("reached %d of the other facts: %v", len(reached), reached)
	}

	// Bob's first is his own, whatever Alice saw
	pizzaPick = func(n int) int { return n - 1 }
	c.h.pizzaFact(bob, pb, at)
	if got := c.h.pizzaSeen[pb.CharServerID].fact; got != len(pizzaFacts)-1 {
		t.Fatalf("Bob's first: fact %d", got+1)
	}
}
