package handlers

import (
	"log"
	"strings"
	"unicode"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The profanity filter (F3 20261021-011737: "need chat and name filters").
// It is the server's in the game too: CLIENT's action 0x0B052000 sends
// opcode 0x0045 with nothing in it, and the server answers 0x0045 with the
// filter's new state, a zigzag on/off, which Msg0045 shows as "Profanity
// Filter: On" or "Off" (WORDS.TXT 550-552). The server masks what a player
// with the filter on hears; it is on at every login. A character's name is
// refused at creation when it carries one of the plainest roots.

// profaneRoots mask any word they begin ("fucking", "shitty"): roots no
// clean word begins.
var profaneRoots = []string{
	"fuck", "shit", "cunt", "bitch", "nigg", "faggot", "whore", "wank",
	"dildo", "twat", "jizz", "pussy", "penis", "vagina", "motherfuck",
}

// profaneWords are masked as whole words only: each is the start of a clean
// word too ("ass" in "assist", "cock" in "cockatrice", which Tunaria has).
var profaneWords = map[string]bool{
	"ass": true, "asses": true, "asshole": true, "assholes": true,
	"arse": true, "arsehole": true, "bastard": true, "bastards": true,
	"cock": true, "cocks": true, "cocksucker": true, "dick": true,
	"dicks": true, "dickhead": true, "fag": true, "fags": true,
	"slut": true, "sluts": true, "slutty": true, "piss": true,
	"pissed": true, "prick": true, "pricks": true, "cum": true,
	"tits": true, "titties": true, "boobs": true, "boner": true,
	"retard": true, "retarded": true, "rape": true, "douche": true,
	"douchebag": true,
}

// nameRoots refuse a character name that has one anywhere in it: a name is
// one word, so "Shitlord" hides its root, and none of these is inside a
// fair name (Grapes, Hancock, Cassandra, Titania and Dickens pass).
var nameRoots = []string{
	"fuck", "shit", "cunt", "bitch", "nigg", "faggot", "whore", "wank",
	"dildo", "twat", "jizz", "pussy", "penis", "vagina", "asshole", "slut",
}

// profane is whether one word (letters, any case) is masked.
func profane(word string) bool {
	w := strings.ToLower(word)
	if profaneWords[w] {
		return true
	}
	for _, r := range profaneRoots {
		if strings.HasPrefix(w, r) {
			return true
		}
	}
	return false
}

// lineFor is the line p hears: masked unless p turned the filter off.
func lineFor(p *entity.Player, line string) string {
	if p == nil || p.ProfanityFilterOff {
		return line
	}
	return maskProfanity(line)
}

// maskProfanity is text with every profane word's letters starred; the
// spaces, the punctuation and the clean words stay as they were.
func maskProfanity(text string) string {
	runes := []rune(text)
	changed := false
	for i := 0; i < len(runes); {
		if !unicode.IsLetter(runes[i]) {
			i++
			continue
		}
		j := i
		for j < len(runes) && unicode.IsLetter(runes[j]) {
			j++
		}
		if profane(string(runes[i:j])) {
			for k := i; k < j; k++ {
				runes[k] = '*'
			}
			changed = true
		}
		i = j
	}
	if !changed {
		return text
	}
	return string(runes)
}

// profaneName is whether a new character's name is refused.
func profaneName(name string) bool {
	n := strings.ToLower(name)
	for _, r := range nameRoots {
		if strings.Contains(n, r) {
			return true
		}
	}
	return false
}

// HandleProfanityFilter is 0x0045 from the client: the filter turned over,
// and its new state told (Msg0045's zigzag). The retail client's menus never
// send it (no entry carries action 0x0B052000), so /filter in the chat line
// does the same (HandleSayChat); msg may be nil.
func (h *ChatHandler) HandleProfanityFilter(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	p.ProfanityFilterOff = !p.ProfanityFilterOff
	on := int64(1)
	if p.ProfanityFilterOff {
		on = 0
	}
	queueReliable(sess, protocol.OpcodeProfanityFilter, build(protocol.OpcodeProfanityFilter, 2+10, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(on)
	}))
	log.Printf("ProfanityFilter: %s turned it %v", p.Name, on == 1)
	return nil
}
