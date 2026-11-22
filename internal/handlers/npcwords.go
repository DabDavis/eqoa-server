package handlers

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
