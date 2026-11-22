package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/session"

	"github.com/eqoa/server/internal/entity"
)

// npcwords.go — words of the travel NPCs and the spiritmasters as the live
// game had them, NPC by NPC (jibrish's video audits of live-era footage,
// boards 659, 745, 813, 823). The live game's NPCs did not share one wording
// (two coachmen filmed spoke two), so each filmed NPC has its own entry here;
// one with no entry keeps the shared words.

// bindWords is a spiritmaster's: its question, its one answer (the client adds
// Exit, which binds no one) and what it says once the player is bound, a line
// each.
type bindWords struct {
	Ask, Answer string
	Bound       []string
}

// defaultBind is a spiritmaster's with no words of its own: the live game's at
// a Fayspires spiritmaster (board 813), "You are now bound to this place." once
// bound (ours).
var defaultBind = bindWords{Ask: "Would you like to be bound?", Answer: "Yes, bind me.", Bound: []string{boundLine}}

// spiritmasterWords are the filmed spiritmasters', each over the default where
// it has a word ("" and nil: the default's).
var spiritmasterWords = map[string]bindWords{
	// Qeynos (board 813, a 2012 tape; "honorabe" as filmed)
	"Spiritmaster Lo": {Ask: "Are you here to learn the arts or to bind your spirit to this area?",
		Answer: "I need to bind my spirit to this area, Master Lo.",
		Bound:  []string{"The winds of Norrath shall bring your spirit back to this area if you die. Make sure your death is an honorabe one."}},
	// board 659's notes: Halas, Murnf
	"Spiritmaster Kerr": {Bound: []string{"You are now bound just outside the great city of Halas."}},
	"Spiritmaster Keldan": {Ask: "Stopping for a night of rest here in Murnf?", Answer: "I would like to be bound.",
		Bound: []string{"Very well, it is done. Have a fine day."}},
	// board 813's notes
	"Spiritmaster Duelau":  {Bound: []string{"If I must."}},
	"Spiritmaster Colanin": {Bound: []string{"Another weakling are ye? Here ye go.", "Go slay some beasts now, if ye die... well... don't die."}},
}

// bindWordsOf is the spiritmaster's words (spiritmasterWords over defaultBind).
func bindWordsOf(name string) bindWords {
	w := defaultBind
	if o, ok := spiritmasterWords[name]; ok {
		if o.Ask != "" {
			w.Ask = o.Ask
		}
		if o.Answer != "" {
			w.Answer = o.Answer
		}
		if len(o.Bound) > 0 {
			w.Bound = o.Bound
		}
	}
	return w
}

// coachWords is a coachman's: his menu's question (Greet), each destination
// as Dest words it (a fmt format of the place: the town as Names writes it,
// else its own name, its fee after it as destLabel puts it) and what he says
// once his ledger is signed (Signed; "" the shared ledgerSigned). "More...",
// "Back" and the every-ledger offer keep their words.
type coachWords struct {
	Greet, Dest, Signed string
	Names               map[string]string
}

// label is a destination in his words.
func (w coachWords) label(d entity.CoachDest) string {
	if w.Dest == "" {
		return destLabel(d)
	}
	if n, ok := w.Names[d.Name]; ok {
		d.Name = n
	}
	return fmt.Sprintf(w.Dest, destLabel(d))
}

// coachWordsByName are the filmed coachmen's (board 659: live tapes of
// 2012-2013); every other coachman asks "Greetings, traveler! Where would you
// like to go?" and lists the towns by name.
var coachWordsByName = map[string]coachWords{
	"Coachman Ronks": {Greet: "Welcome my friend, do you need to take a horse somewhere?", Dest: "Get me a horse to %s.",
		Names: map[string]string{"Highpass Hold": "Highpass", "Muniel's Tea Garden": "the Muniel's Tea Garden"}},
	"Coachman Billfer": {Greet: "Greetings, are you looking to travel or to sign my Ledger?", Dest: "I would like to travel to %s."},
	"Coachman Dudley": {Greet: "Come to drink some ale? Maybe not, looking to travel then?", Dest: "Travel sir, I would like to go to %s.",
		Signed: "Well, ya can come back to here from Castle Lightwolf and Moradhim now... those dwarves sure know how to drink."},
	"Coachwoman Dainei": {Greet: "Do you need to travel?", Dest: "Yes, I would like to go to %s.",
		Signed: "Sure, you may now come back to the village of Murnf from Wyndhaven or Castle Lightwolf."},
	"Coachman Rizkar": {Greet: "Can I help you with something?", Dest: "I want to go to %s.",
		Names: map[string]string{"Bobble-by-Water": "Bobble-by-water", "Fayspire": "Fayspires"}},
	"Coachman Huff": {Greet: "Hello there, need a horse?", Dest: "How about one to %s?",
		Names: map[string]string{"Highpass Hold": "Highpass", "Fayspire": "Fayspires"}},
	"Coachman Frender": {Greet: "Hail traveler."},
}

// wayWords is a waysender's own words, step by step (board 745): Greet the
// first question (a "%s" in it the player's name), each trip its answer by
// place in Labels (else the place's name); then, where set, Tell a box with
// the one answer "Done", Ask a question answered Yes or No, and Gone said
// before the warp (wayGo where unset). The client's Exit, or No, closes the
// box with nothing said; the level, fee and unlock checks stay travel's.
type wayWords struct {
	Greet        string
	Labels       map[string]string
	Tell         string
	Ask, Yes, No string
	Gone         string
}

// The steps of wayWords a player can have open (Player.WayStep).
const (
	wayStepPick = 1 + iota
	wayStepTell
	wayStepAsk
)

// greet is the first question to the player.
func (w wayWords) greet(player string) string {
	if strings.Contains(w.Greet, "%s") {
		return fmt.Sprintf(w.Greet, player)
	}
	return w.Greet
}

// labels are the trips as its first question's answers.
func (w wayWords) labels(dests []entity.CoachDest) []string {
	out := make([]string, 0, len(dests))
	for _, d := range dests {
		if l, ok := w.Labels[d.Name]; ok {
			out = append(out, l)
		} else {
			out = append(out, destLabel(d))
		}
	}
	return out
}

// wayScripts are the waysenders with their own words, by name.
var wayScripts = map[string]wayWords{
	// Freeport's dock, a 2012 tape read frame by frame (board 745)
	"Dockmaster Yarnning": {Greet: "What can I do for you today?",
		Labels: map[string]string{"Arcadin": "I want to explore Frontiers."},
		Tell:   "I hear there are some exciting places there. I can arrange for your travel.",
		Ask:    "Are you certain that is where you want to go?", Yes: "Yes.", No: "No.",
		Gone: "Off you go then."},
}

// answerWay takes the answer to a waysender's box of its own words: a trip
// picked goes on to its Tell, its Ask, then the trip; anything past the box's
// answers (the client's Exit), or No, closes it with nothing said.
func (h *InteractHandler) answerWay(sess *session.Session, player *entity.Player, option int) {
	w, dests, step := wayScripts[player.CoachBy], player.PendingCoachDests, player.WayStep
	var dest entity.CoachDest
	switch {
	case step == wayStepPick && option < len(dests):
		dest = dests[option]
	case (step == wayStepTell || step == wayStepAsk) && option == 0 && len(dests) == 1:
		dest = dests[0]
	default:
		log.Printf("Waysender: %s closed %s's box (step %d, answer %d)", player.Name, player.CoachBy, step, option)
		forgetDialogs(player)
		return
	}
	if step < wayStepTell && w.Tell != "" {
		player.PendingCoachDests, player.WayStep = []entity.CoachDest{dest}, wayStepTell
		h.sendOptionBox(sess, w.Tell, []string{"Done"})
		return
	}
	if step < wayStepAsk && w.Ask != "" {
		player.PendingCoachDests, player.WayStep = []entity.CoachDest{dest}, wayStepAsk
		h.sendOptionBox(sess, w.Ask, []string{w.Yes, w.No})
		return
	}
	forgetDialogs(player)
	h.travel(sess, player, dest)
}
