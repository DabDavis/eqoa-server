package handlers

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// surname.go — a character's surname (Live petition 5648: "could some how some
// way, add the ability to give yourself a Surname, whether at a certain level or
// via quest"; the owner approved: from level 20, the pets' and mercs' name
// filter, one a character, changed with a wait, a GM's clear).
//
//   - /surname <name> (#surname; a PS2's "/t surname <name>"): from level
//     surnameLevel, one word held to a pet's or a merc's name rules
//     (petNameShape: letters, 3 to 15, the name filter, the staff words, a
//     character's name rules), cased as one, and not a character's name nor a
//     named individual's of the world (who a surname would pass for). Kept in
//     characters.surname (database/surname.go). The first is taken at once; a
//     change waits surnameEvery from the last one taken. A name the filter
//     refuses is logged "Surname: FILTERED", as a pet's rename is.
//   - /surname alone says the character's surname and the rules; /surname
//     clear takes it off (the wait still runs from the last one taken).
//   - A GM's #surname clear <character> clears any character's, online or not,
//     and its wait with it: a GM's answer to a bad name.
//
// A merc's surname (mercsurname.go, petition 3430) rides the same list, by
// the merc's name.
//
// Where it shows: after the name in /who (every client), and on vanilla's name
// plates and its details box (□, a player's), by OpcodeSurnameOffer and
// OpcodeSurnames. Not on a PS2's name plates: the entity channel's name is 24
// bytes at 116 (entity.ShownNameMost, 23 with its NUL), already shared by the
// tags drawn before the name ([GM], [OG], [HC], [AFK]: ogtag.go), and the name
// drawn is how both clients name a character back (a target's name in a tell,
// an invite, a trade, a guild's and a group's lists: PlainName strips the tags
// before it, not words after it), so "First Last" in it would not fit beside the
// tags and would send "Last" as a second word wherever the name is typed back.

// surnameLevel is the least level a surname is taken at.
const surnameLevel = 20

// surnameEvery is how long a surname is kept before it may be changed.
const surnameEvery = 7 * 24 * time.Hour

// surnameLayout is OpcodeSurnames' layout.
const surnameLayout = 1

// The lines.
var (
	surnameUsage = fmt.Sprintf("/surname <name>: a surname after your name, from level %d: one word of %d to %d letters, "+
		"as a character's name. /surname clear takes it off. You may change it once every %d days.",
		surnameLevel, CharNameMin, CharNameMax, int(surnameEvery/(24*time.Hour)))
)

const (
	surnameLow       = "You may take a surname from level %d."
	surnameTaken     = "You are now %s %s."
	surnameSame      = "Your surname is %s already."
	surnameWait      = "You may change your surname again in %s."
	surnameNone      = "You have no surname."
	surnameIs        = "Your surname is %s."
	surnameCleared   = "Your surname %s is cleared."
	surnameOwnName   = "%s is your own name: choose another."
	surnameFailed    = "Your surname could not be kept. Try again."
	surnameGMUsage   = "#surname clear <character> - clears a character's surname (online or not) and its wait"
	surnameGMCleared = "%s's surname %q is cleared; it may take another at once."
	surnameGMNone    = "%s has no surname."
	surnameGMNoChar  = "No character is named %q."
	surnameGMTold    = "A GM cleared your surname %s."
)

// Surnames is the characters' surnames: the command, the read of each in the
// world, and what vanilla's name plates are sent.
type Surnames struct {
	world *entity.World
	db    *database.SQLiteDB
	sm    *session.Manager
	now   func() time.Time

	mu    sync.Mutex
	known map[uint32]string // a character's surname as read or taken, by its id
}

// surnamesH is the handler RegisterDefaultHandlers made (/surname's and /who's).
var surnamesH atomic.Pointer[Surnames]

// NewSurnames is the surnames over the World, the database and the sessions.
func NewSurnames(w *entity.World, db *database.SQLiteDB, sm *session.Manager) *Surnames {
	return &Surnames{world: w, db: db, sm: sm, now: time.Now, known: map[uint32]string{}}
}

// RegisterHandlers registers the offer's answer, reads each character's surname
// as it enters the world (told to the clients that draw them) and makes the
// handler /surname's.
func (s *Surnames) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeSurnameOffer, s.HandleAnswer)
	if s.world != nil {
		s.world.OnPlayerEntered(s.entered)
	}
	surnamesH.Store(s)
}

// SendSurnameOffer offers the session's client the surnames for its name
// plates (a PS2 ignores it).
func SendSurnameOffer(sess *session.Session) {
	if !QoLOnFor(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeSurnameOffer, build(protocol.OpcodeSurnameOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(surnameLayout)
	}))
}

// HandleAnswer is the client's answer: a u8, 1 to be sent the surnames (every
// character's in the world at once), 0 to stop.
func (s *Surnames) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("Surname: a short answer from %s", sess.RemoteAddr())
		return nil
	}
	on := qolAnswerFor(sess, data[0] != 0)
	sess.SetSurnames(on)
	if on {
		queueReliable(sess, protocol.OpcodeSurnames, buildSurnames(s.inWorld()))
	}
	return nil
}

// surnamePair is a character's name and its surname ("" none).
type surnamePair struct{ name, surname string }

// buildSurnames is OpcodeSurnames of the pairs.
func buildSurnames(pairs []surnamePair) []byte {
	return build(protocol.OpcodeSurnames, 8+len(pairs)*34, func(w *buffer.Writer) {
		w.WriteByte(surnameLayout)
		w.WriteUint16(uint16(min(len(pairs), 0xFFFF)))
		for _, p := range pairs[:min(len(pairs), 0xFFFF)] {
			writeName8(w, p.name)
			writeName8(w, p.surname)
		}
	})
}

// Of is the character's surname ("" none), read once and kept while it plays.
func (s *Surnames) Of(serverID uint32) string {
	if s == nil || serverID == 0 {
		return ""
	}
	s.mu.Lock()
	v, ok := s.known[serverID]
	s.mu.Unlock()
	if ok || s.db == nil {
		return v
	}
	v, _, err := s.db.CharacterSurname(serverID)
	if err != nil {
		if !errors.Is(err, database.ErrCharacterNotFound) {
			log.Printf("Surname: character %d's surname: %v", serverID, err)
		}
		return ""
	}
	s.keep(serverID, v)
	return v
}

// keep holds a character's surname as read or taken.
func (s *Surnames) keep(serverID uint32, v string) {
	s.mu.Lock()
	s.known[serverID] = v
	s.mu.Unlock()
}

// inWorld is every character in the world with a surname, by its own name.
func (s *Surnames) inWorld() []surnamePair {
	if s.world == nil {
		return nil
	}
	type who struct {
		id   uint32
		name string
	}
	var all []who
	s.world.ForEachPlayer(func(p *entity.Player) { all = append(all, who{p.CharServerID, p.Name}) })
	var out []surnamePair
	for _, w := range all {
		if v := s.Of(w.id); v != "" {
			out = append(out, surnamePair{w.name, v})
		}
	}
	// and every merc's (mercsurname.go): a merc's name is the world's, so its
	// pair names it wherever it stands, out or not yet
	if s.db != nil {
		mercs, err := s.db.MercSurnames()
		if err != nil {
			log.Printf("Surname: the mercs' surnames: %v", err)
		}
		names := make([]string, 0, len(mercs))
		for n := range mercs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out = append(out, surnamePair{n, mercs[n]})
		}
	}
	return out
}

// entered reads the character's surname afresh as it enters the world (a GM's
// edit since its last login taken) and tells the clients that draw them.
func (s *Surnames) entered(_ *session.Session, p *entity.Player) {
	if p == nil || s.db == nil {
		return
	}
	v, _, err := s.db.CharacterSurname(p.CharServerID)
	if err != nil {
		return
	}
	s.keep(p.CharServerID, v)
	if v != "" {
		s.tell(surnamePair{p.Name, v})
	}
}

// tell sends every client that draws the surnames the pairs.
func (s *Surnames) tell(pairs ...surnamePair) {
	if s.sm == nil || len(pairs) == 0 {
		return
	}
	msg := buildSurnames(pairs)
	s.sm.ForEach(func(sess *session.Session) bool {
		if sess.Surnames() {
			queueReliable(sess, protocol.OpcodeSurnames, msg)
		}
		return true
	})
}

// surnameLine reads a chat line that is /surname's (or #surname's): the rest
// after the word, and whether it is.
func surnameLine(message string) (rest string, ok bool) {
	word, rest, _ := strings.Cut(strings.TrimSpace(message), " ")
	if strings.EqualFold(word, "/surname") || strings.EqualFold(word, "#surname") {
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// waitWords is a wait as a line says it: days, else hours, else minutes.
func waitWords(d time.Duration) string {
	plural := func(n int, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	switch {
	case d >= 24*time.Hour:
		return plural(int((d+24*time.Hour-1)/(24*time.Hour)), "day")
	case d >= time.Hour:
		return plural(int((d+time.Hour-1)/time.Hour), "hour")
	}
	return plural(max(int((d+time.Minute-1)/time.Minute), 1), "minute")
}

// Command is /surname's (the file's doc): gm whether the session is a GM's (its
// #surname clear <character>, no level nor wait for its own).
func (s *Surnames) Command(sess *session.Session, p *entity.Player, args string, gm bool) {
	if s == nil || p == nil {
		return
	}
	if s.db == nil {
		SendErrorMessage(sess, "This server keeps no surnames.")
		return
	}
	args = strings.TrimSpace(args)
	word, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)
	switch {
	case gm && strings.EqualFold(word, "clear") && rest != "":
		s.gmClear(sess, p, rest)
		return
	case args == "":
		if v := s.Of(p.CharServerID); v != "" {
			SendClientMessage(sess, fmt.Sprintf(surnameIs, v))
		} else {
			SendClientMessage(sess, surnameNone)
		}
		SendClientMessage(sess, surnameUsage)
		if gm {
			SendClientMessage(sess, surnameGMUsage)
		}
		return
	case strings.EqualFold(args, "clear") || strings.EqualFold(args, "none") || strings.EqualFold(args, "off"):
		s.clearOwn(sess, p)
		return
	}
	s.take(sess, p, args, gm)
}

// take is /surname <name>: the name held to the rules, the level and the wait
// (none for a GM), kept, told.
func (s *Surnames) take(sess *session.Session, p *entity.Player, asked string, gm bool) {
	if !gm && p.Level < surnameLevel {
		SendErrorMessage(sess, fmt.Sprintf(surnameLow, surnameLevel))
		return
	}
	v, ok := petNameShape(asked)
	if ok {
		v = s.checkName(p, v)
	}
	acct := sess.AccountID()
	if v.refusal != "" {
		if v.filtered {
			log.Printf("Surname: FILTERED %s (character %d, account %d) asked for the surname %q: %s", p.Name, p.CharServerID, acct, asked, v.reason)
		} else {
			log.Printf("Surname: refused %s (character %d, account %d) the surname %q: %s", p.Name, p.CharServerID, acct, asked, v.reason)
		}
		SendErrorMessage(sess, v.refusal)
		return
	}
	cur, at, err := s.db.CharacterSurname(p.CharServerID)
	if err != nil {
		log.Printf("Surname: %s's surname: %v", p.Name, err)
		SendErrorMessage(sess, surnameFailed)
		return
	}
	if cur == v.name {
		SendClientMessage(sess, fmt.Sprintf(surnameSame, cur))
		return
	}
	now := s.now()
	if !gm && !at.IsZero() {
		if left := at.Add(surnameEvery).Sub(now); left > 0 {
			SendErrorMessage(sess, fmt.Sprintf(surnameWait, waitWords(left)))
			return
		}
	}
	if err := s.db.SetSurname(p.CharServerID, v.name, now); err != nil {
		log.Printf("Surname: %s's surname %q not kept: %v", p.Name, v.name, err)
		SendErrorMessage(sess, surnameFailed)
		return
	}
	s.keep(p.CharServerID, v.name)
	log.Printf("Surname: %s (character %d, account %d) takes the surname %q (was %q)", p.Name, p.CharServerID, acct, v.name, cur)
	SendClientMessage(sess, fmt.Sprintf(surnameTaken, p.Name, v.name))
	s.tell(surnamePair{p.Name, v.name})
}

// checkName holds a surname that passed petNameShape to the rules that need the
// world: not the character's own name, nor a character's (a GM's is the
// filter's), nor a named individual's.
func (s *Surnames) checkName(p *entity.Player, v petVerdict) petVerdict {
	name := v.name
	if strings.EqualFold(name, p.Name) {
		return petVerdict{refusal: fmt.Sprintf(surnameOwnName, name), reason: "its own name"}
	}
	named, gm, err := s.db.NamedCharacter(name)
	if err != nil {
		log.Printf("Surname: the characters named %q: %v", name, err)
		return petVerdict{refusal: surnameFailed, reason: "the characters could not be read"}
	}
	if gm {
		return petVerdict{refusal: renameFiltered, reason: "a GM's character's name", filtered: true}
	}
	if named {
		return petVerdict{refusal: fmt.Sprintf(renameCharacter, name), reason: "a character's name"}
	}
	if s.world != nil && s.world.NPCIndividualNamed(name) {
		return petVerdict{refusal: fmt.Sprintf(renameIndividual, name), reason: "a named individual's name"}
	}
	return v
}

// clearOwn is /surname clear: the character's surname off, its wait running on.
func (s *Surnames) clearOwn(sess *session.Session, p *entity.Player) {
	cur := s.Of(p.CharServerID)
	if cur == "" {
		SendClientMessage(sess, surnameNone)
		return
	}
	if err := s.db.SetSurname(p.CharServerID, "", time.Time{}); err != nil {
		log.Printf("Surname: %s's surname not cleared: %v", p.Name, err)
		SendErrorMessage(sess, surnameFailed)
		return
	}
	s.keep(p.CharServerID, "")
	log.Printf("Surname: %s (character %d) clears its surname %q", p.Name, p.CharServerID, cur)
	SendClientMessage(sess, fmt.Sprintf(surnameCleared, cur))
	s.tell(surnamePair{p.Name, ""})
}

// gmClear is a GM's #surname clear <character>.
func (s *Surnames) gmClear(sess *session.Session, gm *entity.Player, name string) {
	name = entity.PlainName(name)
	id, charName, old, err := s.db.ClearSurnameNamed(name)
	switch {
	case errors.Is(err, database.ErrCharacterNotFound):
		SendErrorMessage(sess, fmt.Sprintf(surnameGMNoChar, name))
		return
	case err != nil:
		log.Printf("Surname: %s's clear of %q: %v", gm.Name, name, err)
		SendErrorMessage(sess, surnameFailed)
		return
	}
	s.keep(id, "")
	if old == "" {
		SendClientMessage(sess, fmt.Sprintf(surnameGMNone, charName))
		return
	}
	log.Printf("Surname: GM %s (account %d) clears %s's (character %d) surname %q", gm.Name, sess.AccountID(), charName, id, old)
	SendClientMessage(sess, fmt.Sprintf(surnameGMCleared, charName, old))
	s.tell(surnamePair{charName, ""})
	if s.sm != nil && s.world != nil {
		// the sessions first, then the World (its lock is taken before the
		// sessions' elsewhere)
		var all []*session.Session
		s.sm.ForEach(func(ts *session.Session) bool { all = append(all, ts); return true })
		for _, ts := range all {
			if t := s.world.SessionPlayer(ts); t != nil && t.CharServerID == id {
				SendErrorMessage(ts, fmt.Sprintf(surnameGMTold, old))
				break
			}
		}
	}
}

// surnameAfter is a name with its character's surname after it (/who's), the
// name alone with none.
func surnameAfter(name string, serverID uint32) string {
	if v := surnamesH.Load().Of(serverID); v != "" {
		return name + " " + v
	}
	return name
}
