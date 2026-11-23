package handlers

// /pizza: a Chicago pizza fact in a box, everyone's (the user, 2026-11-22: "add a
// joke command to all players /pizza ... it displays a text pop up with a chicago
// pizza fact 'Chicago pizza is better than new york pizza' ... everytime they use it
// have it display a pre-written fact about the various types of chicago pizza. Find
// 10 different facts ... /pizza should randomly pick one and display it for them.
// Only positive facts about pizza allowed.").
//
// The box is a dialogue box (0x0046, SendDialogueBox), as a monster's name and level
// are shown to a PS2 (mobinfo.go): only the player sees it, closes it, and nothing is
// said aloud. Its first line is the user's own, then one of pizzaFacts at random,
// never the one the player saw last. A PS2 keeps an unknown /command to itself, so
// its player types #pizza or "/t pizza"; vanilla forwards the slash.
//
// The facts were checked on 2026-11-22 against Wikipedia (Ike Sewell, Chicago-style
// pizza, Tavern-style pizza, Lou Malnati's, Burt Katz), National Geographic and CBS
// Chicago (deep dish's 1943 start at 29 East Ohio Street), Chicago Magazine (most
// Chicagoans eat thin crust cut in squares), Nancy's and Giordano's own histories
// (stuffed pizza from an Italian Easter pie) and the day's calendars (April 5, the day
// Uno opened its first restaurant outside Chicago in 1979).

import (
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// pizzaWord is /pizza's word: "/pizza", "#pizza" and a PS2's "/t pizza".
const pizzaWord = "pizza"

// pizzaHeadline is every box's first line, the user's own.
const pizzaHeadline = "Chicago pizza is better than New York pizza!"

// pizzaFacts are the box's facts, about Chicago's kinds of pizza (deep dish, stuffed,
// tavern-style thin crust, pan), every one in praise of pizza. With the headline and
// its number each fits one box (questBoxChars, TestPizzaFactsFitTheBox) and holds no
// '%' (CLIENT's text expander reads it as a token).
var pizzaFacts = []string{
	"Deep dish pizza was born in Chicago in 1943 at Pizzeria Uno on East Ohio Street, and that very spot still bakes it today.",
	"A deep dish is built upside down: cheese on the crust first, then the toppings, and the tomato sauce on top keeps it all gooey through the long bake.",
	"Stuffed pizza was invented in Chicago in the 1970s by Nancy's and Giordano's, inspired by a double-crusted Italian Easter pie.",
	"A stuffed pizza tucks its cheese and fillings between two layers of dough, then crowns the top crust with tomato sauce.",
	"Tavern-style thin crust is the pizza Chicagoans eat most: a crispy, cracker-thin crust cut into squares, the famous party cut.",
	"Tavern-style pizza is named for the Chicago neighborhood taverns that served its crispy little squares to their patrons.",
	"The corner pieces of a party-cut pizza are the crispiest bites on the whole pie, and Chicagoans treasure them.",
	"Lou Malnati's has baked Chicago deep dish since 1971, and its classic sausage pie lays a single patty of Italian sausage under the tomatoes.",
	"Chicago pan pizza can wear a ring of caramelized cheese baked onto its crust, a crispy golden crown made famous by pizza legend Burt Katz.",
	"April 5 is Deep Dish Pizza Day, a whole day to celebrate Chicago's most famous pie.",
}

// pizzaEvery is the least time between one player's boxes: a held key or a pasted
// line does not stack a pile of boxes to close.
const pizzaEvery = 2 * time.Second

// pizzaShown is a character's last /pizza: when, and which fact.
type pizzaShown struct {
	at   time.Time
	fact int
}

// pizzaPick is a whole number in [0, n): math/rand/v2's IntN (a test sets it).
var pizzaPick = rand.IntN

// pizzaLine is whether a typed line is /pizza: "/pizza" or "#pizza" with anything
// after it, or "/t pizza" (or "/tell") with nothing after it (with text it is a
// tell), in any case.
func pizzaLine(line string) bool {
	line = strings.TrimSpace(line)
	if word, _, _ := strings.Cut(line, " "); strings.EqualFold(word, "/"+pizzaWord) || strings.EqualFold(word, "#"+pizzaWord) {
		return true
	}
	name, body, ok := parseTell(line)
	return ok && strings.EqualFold(name, pizzaWord) && strings.TrimSpace(body) == ""
}

// pizzaBox is the box's text for fact i (0-based).
func pizzaBox(i int) string {
	return fmt.Sprintf("%s\nPizza Fact #%d: %s", pizzaHeadline, i+1, pizzaFacts[i])
}

// nextPizzaFact is a fact at random other than last (-1: none yet).
func nextPizzaFact(last int) int {
	n := len(pizzaFacts)
	if last < 0 || last >= n || n < 2 {
		return pizzaPick(n)
	}
	return (last + 1 + pizzaPick(n-1)) % n // any but last, each alike
}

// pizzaFact shows player a fact, at most one every pizzaEvery.
func (h *ChatHandler) pizzaFact(sess *session.Session, player *entity.Player, now time.Time) {
	h.pizzaMu.Lock()
	if h.pizzaSeen == nil {
		h.pizzaSeen = map[uint32]pizzaShown{}
	}
	last, seen := h.pizzaSeen[player.CharServerID]
	if seen && now.Sub(last.at) < pizzaEvery {
		h.pizzaMu.Unlock()
		return
	}
	prev := -1
	if seen {
		prev = last.fact
	}
	i := nextPizzaFact(prev)
	h.pizzaSeen[player.CharServerID] = pizzaShown{at: now, fact: i}
	h.pizzaMu.Unlock()
	SendDialogueBox(sess, 0, pizzaBox(i))
	log.Printf("Pizza: %s was shown fact %d", player.Name, i+1)
}
