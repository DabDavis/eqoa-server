package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// hardcoreask.go — the Hardcore choice for a character made on a client that
// cannot offer it at creation: a PS2 (STATION's and CLIENT's creation pages
// send 0x002B's fixed fields, with nothing free for it), the account pages, an
// older vanilla. CLIENT's own question box (0x0034, as the welcome guide's and
// every NPC's question here) asks it at the character's first world entry,
// hardcoreAskAfter on (after the welcome box's 10 s), in two steps so a slip of
// the thumb cannot make a character Hardcore:
//
//	"Make Name a Hardcore character? ..."  Yes, make me Hardcore / No, a normal character
//	"Are you sure? ..."                     Yes, Hardcore for good / No, keep me normal
//
// A No (either box) makes the character normal for good; the second Yes makes
// it Hardcore (database.ChooseHardcore, its tag drawn at the next pass). A box
// closed (or never answered) leaves the choice open: it is asked again at the
// next entry, and "/hardcore" (a PS2's "/t hardcore") asks it at once. The
// choice is open only while the character is new (HardcoreState.ChoiceOpen:
// still at the level it was made at, no experience): it is a choice made at
// creation, not a badge to put on a character played up. Words ours; nothing
// a PS2's font lacks, no '%' or '|'.
//
// The [OG] choice (database/ogchar.go; the owner, 2026-11-19: "lets make [OG]
// status a char tag like [HC]") is asked the same way, in its own two boxes
// (ogBoxMark), where world.og_accounts is on and the character's [OG] choice
// is open (HardcoreState.OGChoiceOpen): never at the same moment as
// Hardcore's. A character with both open is asked Hardcore first and [OG]
// ogAfterHardcore after Hardcore's last box is answered or closed; one with
// only [OG] open is asked it hardcoreAskAfter on. "/og" (a PS2's "/t og") asks
// it again while it is open.

// hardcoreBoxMark marks the Hardcore boxes' ids (beside the welcome guide's
// 0x61, the call's 0x63 and the others).
const hardcoreBoxMark = 0x72000000

// ogBoxMark marks the [OG] boxes' ids.
const ogBoxMark = 0x73000000

// ogAfterHardcore is how long after Hardcore's boxes the [OG] box is asked,
// so the two never stand together.
const ogAfterHardcore = 6 * time.Second

// The boxes' times.
const (
	hardcoreAskAfter   = 25 * time.Second
	hardcoreAskRetry   = 3 * time.Second
	hardcoreAskGiveUp  = 3 * time.Minute
	hardcoreAsksKept   = time.Hour
	hardcoreAskDecline = welcomeDeclined
)

// The boxes' words (127 characters a question, 63 an answer).
const (
	hardcoreQ1       = "Make %s a Hardcore character? Its first death would be final: no respawn, no resurrection. This can never be undone."
	hardcoreQ1Anon   = "Make this a Hardcore character? Its first death would be final: no respawn, no resurrection. This can never be undone."
	hardcoreQ2       = "Are you sure? %s will be Hardcore for good, drawn [HC]. If %s dies, %s stays dead."
	hardcoreQ2Anon   = "Are you sure? This character will be Hardcore for good, drawn [HC]. If it dies, it stays dead."
	hardcoreYes1     = "Yes, make me Hardcore"
	hardcoreNo1      = "No, a normal character"
	hardcoreYes2     = "Yes, Hardcore for good"
	hardcoreNo2      = "No, keep me normal"
	hardcoreMadeLine = "%s is now a Hardcore character. Good luck: you only live once."
	hardcoreNormal   = "%s is a normal character."
	hardcoreFailLine = "That could not be kept just now: try again with %s."
	hardcoreIsLine   = "%s is a Hardcore character: its first death is final."
	hardcoreClosed   = "%s is a normal character: the Hardcore choice is made when a character is new."
	hardcoreOffLine  = "There are no Hardcore characters on this server."
	hardcoreLaterLn  = "You can still make %s Hardcore while it is new: type %s."
)

// The [OG] boxes' words (127 characters a question, 63 an answer): what [OG]
// gives up said at the choice (the basic upgrades only).
const (
	ogQ1       = "Make %s an [OG] character? Basic upgrades only: no map, minimap or item links, no mouse or touch. Permanent."
	ogQ1Anon   = "Make this an [OG] character? Basic upgrades only: no map, minimap or item links, no mouse or touch. Permanent."
	ogQ2       = "Are you sure? %s will be [OG] for good, drawn [OG] %s: the game as it was, with no way back."
	ogQ2Anon   = "Are you sure? This character will be [OG] for good, drawn [OG]: the game as it was, with no way back."
	ogYes1     = "Yes, make me [OG]"
	ogNo1      = "No, keep the additions"
	ogYes2     = "Yes, [OG] for good"
	ogNo2      = "No, keep me standard"
	ogMadeLine = "%s is now an [OG] character: the game as it was."
	ogNormal   = "%s is a standard character."
	ogIsLine   = "%s is an [OG] character."
	ogClosed   = "%s is a standard character: the [OG] choice is made when a character is new."
	ogOffLine  = "There are no [OG] characters on this server."
	ogLaterLn  = "You can still make %s [OG] while it is new: type %s."
)

// ogWord is /og's word: "/og", "#og", a PS2's "/t og" (no name is 2 letters).
const ogWord = "og"

// hardcoreWord is /hardcore's word: "/hardcore", "#hardcore", a PS2's "/t hardcore".
const hardcoreWord = "hardcore"

// hardcoreAsker is the box's asker (registry.go); nil: none.
var hardcoreAsker atomic.Pointer[HardcoreAsk]

// hardcoreAskStore is the character's state and choice (database.SQLiteDB).
type hardcoreAskStore interface {
	HardcoreOf(serverID uint32) (database.HardcoreState, error)
	ChooseHardcore(serverID uint32, on bool) (bool, error)
	ChooseOG(serverID uint32, on bool) (bool, error)
}

// HardcoreAsk runs the boxes.
type HardcoreAsk struct {
	gw    *entity.World
	store hardcoreAskStore

	now   func() time.Time
	later func(time.Duration, func())

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]*hardcoreAskBox // by the character's serverid
}

// hardcoreAskBox is a box asked.
type hardcoreAskBox struct {
	char    uint32
	sess    *session.Session
	box     uint32
	second  bool // the "Are you sure?" box
	og      bool // an [OG] box (else Hardcore's)
	sent    time.Time
	asked   time.Time
	until   time.Time
	waiting bool
}

// NewHardcoreAsk makes the asker over db.
func NewHardcoreAsk(gw *entity.World, db *database.SQLiteDB) *HardcoreAsk {
	a := &HardcoreAsk{gw: gw, now: time.Now, asked: map[uint32]*hardcoreAskBox{},
		later: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
	if db != nil {
		a.store = db
	}
	return a
}

// state reads p's Hardcore and [OG] columns.
func (a *HardcoreAsk) state(p *entity.Player) (database.HardcoreState, bool) {
	if a == nil || a.store == nil || p == nil {
		return database.HardcoreState{}, false
	}
	st, err := a.store.HardcoreOf(p.CharServerID)
	if err != nil {
		log.Printf("Hardcore: %s's choice: %v", p.Name, err)
		return st, false
	}
	return st, true
}

// open reads whether p's Hardcore choice is still open.
func (a *HardcoreAsk) open(p *entity.Player) (database.HardcoreState, bool) {
	if !HardcoreOn() {
		return database.HardcoreState{}, false
	}
	st, ok := a.state(p)
	return st, ok && st.ChoiceOpen()
}

// openOG reads whether p's [OG] choice is still open (world.og_accounts).
func (a *HardcoreAsk) openOG(p *entity.Player) (database.HardcoreState, bool) {
	if !OGAccounts() {
		return database.HardcoreState{}, false
	}
	st, ok := a.state(p)
	return st, ok && st.OGChoiceOpen()
}

// Entered is OnPlayerEntered: the first box, hardcoreAskAfter on, for a
// character whose choice is open: Hardcore's first, else [OG]'s (Hardcore's
// last answer asks [OG]'s: askOGNext).
func (a *HardcoreAsk) Entered(sess *session.Session, p *entity.Player) {
	if _, open := a.open(p); open {
		a.tryIn(a.newBox(sess, p, false, false), hardcoreAskAfter)
		return
	}
	if _, open := a.openOG(p); open {
		a.tryIn(a.newBox(sess, p, false, true), hardcoreAskAfter)
	}
}

// askOGNext asks p's [OG] choice ogAfterHardcore on when it is open, after
// Hardcore's boxes.
func (a *HardcoreAsk) askOGNext(sess *session.Session, p *entity.Player) {
	if _, open := a.openOG(p); open {
		a.tryIn(a.newBox(sess, p, false, true), ogAfterHardcore)
	}
}

// ShowOG is /og: the first [OG] box now when the choice is open, else what the
// character is.
func (a *HardcoreAsk) ShowOG(sess *session.Session, p *entity.Player) {
	if !OGAccounts() || p == nil {
		SendErrorMessage(sess, ogOffLine)
		return
	}
	st, open := a.openOG(p)
	switch {
	case open:
		a.try(a.newBox(sess, p, false, true))
	case st.OG:
		SendClientMessage(sess, fmt.Sprintf(ogIsLine, p.Name))
	default:
		SendClientMessage(sess, fmt.Sprintf(ogClosed, p.Name))
	}
}

// Show is /hardcore: the first box now when the choice is open, else what the
// character is.
func (a *HardcoreAsk) Show(sess *session.Session, p *entity.Player) {
	if !HardcoreOn() || p == nil {
		SendErrorMessage(sess, hardcoreOffLine)
		return
	}
	st, open := a.open(p)
	switch {
	case open:
		a.try(a.newBox(sess, p, false, false))
	case st.Hardcore:
		SendClientMessage(sess, fmt.Sprintf(hardcoreIsLine, p.Name))
	default:
		SendClientMessage(sess, fmt.Sprintf(hardcoreClosed, p.Name))
	}
}

func (a *HardcoreAsk) newBox(sess *session.Session, p *entity.Player, second, og bool) *hardcoreAskBox {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for c, x := range a.asked {
		if now.Sub(x.asked) > hardcoreAsksKept {
			delete(a.asked, c)
		}
	}
	a.seq++
	mark := uint32(hardcoreBoxMark)
	if og {
		mark = ogBoxMark
	}
	b := &hardcoreAskBox{char: p.CharServerID, sess: sess, box: mark | (a.seq & 0xFFFFFF), second: second, og: og,
		asked: now, until: now.Add(hardcoreAskGiveUp)}
	a.asked[b.char] = b
	return b
}

func (a *HardcoreAsk) tryIn(b *hardcoreAskBox, d time.Duration) {
	a.mu.Lock()
	if b.waiting || a.asked[b.char] != b {
		a.mu.Unlock()
		return
	}
	b.waiting = true
	a.mu.Unlock()
	a.later(d, func() {
		a.mu.Lock()
		b.waiting = false
		a.mu.Unlock()
		a.try(b)
	})
}

// try sends b when its player may answer, else looks again until b.until.
func (a *HardcoreAsk) try(b *hardcoreAskBox) {
	free, gone := a.free(b)
	now := a.now()
	a.mu.Lock()
	if a.asked[b.char] != b {
		a.mu.Unlock()
		return
	}
	if gone || (!free && !now.Before(b.until)) {
		delete(a.asked, b.char)
		a.mu.Unlock()
		return
	}
	if !free {
		a.mu.Unlock()
		a.tryIn(b, hardcoreAskRetry)
		return
	}
	b.sent = now
	sess, box, second, og := b.sess, b.box, b.second, b.og
	a.mu.Unlock()
	name := ""
	if p := a.gw.SessionPlayer(sess); p != nil {
		name = p.Name
	}
	q, answers := hardcoreQuestion(name, second)
	if og {
		q, answers = ogQuestion(name, second)
	}
	SendQuestPopupMulti(sess, box, q, answers)
}

// hardcoreQuestion is a box's question and answers for a character name.
func hardcoreQuestion(name string, second bool) (string, []string) {
	plain := name != "" && !strings.ContainsAny(name, "%|")
	if second {
		q := hardcoreQ2Anon
		if plain {
			if s := fmt.Sprintf(hardcoreQ2, name, name, name); len([]rune(s)) <= questAskChars {
				q = s
			}
		}
		return q, []string{hardcoreYes2, hardcoreNo2}
	}
	q := hardcoreQ1Anon
	if plain {
		if s := fmt.Sprintf(hardcoreQ1, name); len([]rune(s)) <= questAskChars {
			q = s
		}
	}
	return q, []string{hardcoreYes1, hardcoreNo1}
}

// ogQuestion is an [OG] box's question and answers for a character name.
func ogQuestion(name string, second bool) (string, []string) {
	plain := name != "" && !strings.ContainsAny(name, "%|")
	if second {
		q := ogQ2Anon
		if plain {
			if s := fmt.Sprintf(ogQ2, name, name); len([]rune(s)) <= questAskChars {
				q = s
			}
		}
		return q, []string{ogYes2, ogNo2}
	}
	q := ogQ1Anon
	if plain {
		if s := fmt.Sprintf(ogQ1, name); len([]rune(s)) <= questAskChars {
			q = s
		}
	}
	return q, []string{ogYes1, ogNo1}
}

// free is welcome.go's: the session still its character's in the world, alive,
// not loading a world.
func (a *HardcoreAsk) free(b *hardcoreAskBox) (free, gone bool) {
	s := b.sess
	if s == nil || !s.CharacterInWorld() || s.PendingTermination() {
		return false, true
	}
	if _, replaced := s.ReplacedFor(); replaced {
		return false, true
	}
	if a.gw == nil {
		return true, false
	}
	p := a.gw.SessionPlayer(s)
	if p == nil || p.CharServerID != b.char {
		return false, true
	}
	snap, ok := a.gw.PlayerSnapshot(p.ServerID)
	if !ok {
		return false, true
	}
	if snap.IsDead() || !snap.DiedAt.IsZero() || s.StateChannelsHeld() {
		return false, false
	}
	return true, false
}

// IsHardcoreBox reports a Hardcore box's id, or an [OG] box's.
func IsHardcoreBox(id uint32) bool {
	return id&0xFF000000 == hardcoreBoxMark || id&0xFF000000 == ogBoxMark
}

// Answer is HandleDialogueOption's for a Hardcore box; it reports whether the
// box was one.
func (a *HardcoreAsk) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsHardcoreBox(box) {
		return false
	}
	if a == nil || p == nil {
		return true
	}
	now := a.now()
	a.mu.Lock()
	b := a.asked[p.CharServerID]
	if b == nil || b.box != box {
		a.mu.Unlock()
		return true
	}
	if option == 255 {
		declined := !b.sent.IsZero() && now.Sub(b.sent) < hardcoreAskDecline
		og := b.og
		if !declined {
			delete(a.asked, p.CharServerID) // closed: left open, the next asked
		}
		a.mu.Unlock()
		switch {
		case declined:
			a.tryIn(b, hardcoreAskRetry) // CLIENT's own decline (a menu up)
		case og:
			SendClientMessage(sess, fmt.Sprintf(ogLaterLn, p.Name, ogSpelling(sess.AckMasks())))
		default:
			SendClientMessage(sess, fmt.Sprintf(hardcoreLaterLn, p.Name, hardcoreSpelling(sess.AckMasks())))
			a.askOGNext(sess, p)
		}
		return true
	}
	delete(a.asked, p.CharServerID)
	second, og := b.second, b.og
	a.mu.Unlock()
	if og {
		if _, open := a.openOG(p); !open {
			return true // played on meanwhile, or chosen in another window
		}
		switch {
		case option == 0 && !second:
			a.try(a.newBox(sess, p, true, true))
		case option == 0 && second:
			a.chooseOG(sess, p, true)
		case option == 1:
			a.chooseOG(sess, p, false)
		}
		return true
	}
	if _, open := a.open(p); !open {
		return true // played on meanwhile, or chosen in another window
	}
	switch {
	case option == 0 && !second:
		a.try(a.newBox(sess, p, true, false))
	case option == 0 && second:
		a.choose(sess, p, true)
		a.askOGNext(sess, p)
	case option == 1:
		a.choose(sess, p, false)
		a.askOGNext(sess, p)
	}
	return true
}

// chooseOG keeps the [OG] choice and tells the player; made [OG], its session
// and client are told at once (madeOG).
func (a *HardcoreAsk) chooseOG(sess *session.Session, p *entity.Player, on bool) {
	changed, err := a.store.ChooseOG(p.CharServerID, on)
	if err != nil || !changed {
		if err != nil {
			log.Printf("OG: %s's choice (%v) not kept: %v", p.Name, on, err)
		}
		SendErrorMessage(sess, fmt.Sprintf(hardcoreFailLine, ogSpelling(sess.AckMasks())))
		return
	}
	if on {
		madeOG(a.gw, sess)
		SendClientMessage(sess, fmt.Sprintf(ogMadeLine, p.Name))
	} else {
		SendClientMessage(sess, fmt.Sprintf(ogNormal, p.Name))
	}
	log.Printf("OG: %s chose %s in the game", p.Name, map[bool]string{true: "[OG]", false: "a standard character"}[on])
}

// ogSpelling is /og as the player's client can send it.
func ogSpelling(ours bool) string {
	if ours {
		return "/" + ogWord
	}
	return "/t " + ogWord
}

// ogLine is whether a typed line is /og ("/og", "#og", or "/t og" with
// nothing after it).
func ogLine(line string) bool {
	line = strings.TrimSpace(line)
	if word, rest, _ := strings.Cut(line, " "); (strings.EqualFold(word, "/"+ogWord) || strings.EqualFold(word, "#"+ogWord)) &&
		strings.TrimSpace(rest) == "" {
		return true
	}
	name, body, ok := parseTell(line)
	return ok && strings.EqualFold(name, ogWord) && strings.TrimSpace(body) == ""
}

// ogCommand is /og's way in.
func ogCommand(sess *session.Session, p *entity.Player) {
	if a := hardcoreAsker.Load(); a != nil {
		a.ShowOG(sess, p)
		return
	}
	SendErrorMessage(sess, ogOffLine)
}

// choose keeps the choice and tells the player.
func (a *HardcoreAsk) choose(sess *session.Session, p *entity.Player, on bool) {
	changed, err := a.store.ChooseHardcore(p.CharServerID, on)
	if err != nil || !changed {
		if err != nil {
			log.Printf("Hardcore: %s's choice (%v) not kept: %v", p.Name, on, err)
		}
		SendErrorMessage(sess, fmt.Sprintf(hardcoreFailLine, hardcoreSpelling(sess.AckMasks())))
		return
	}
	if on {
		if a.gw != nil {
			a.gw.SetPlayerHardcore(sess, true, false)
		}
		hcEntered.LoadOrStore(p.CharServerID, time.Now())
		SendColoredChat(sess, fmt.Sprintf(hardcoreMadeLine, p.Name), hardcoreAnnounceRGB[0], hardcoreAnnounceRGB[1], hardcoreAnnounceRGB[2])
	} else {
		SendClientMessage(sess, fmt.Sprintf(hardcoreNormal, p.Name))
	}
	log.Printf("Hardcore: %s chose %s in the game", p.Name, map[bool]string{true: "HARDCORE", false: "a normal character"}[on])
}

// hardcoreSpelling is /hardcore as the player's client can send it.
func hardcoreSpelling(ours bool) string {
	if ours {
		return "/" + hardcoreWord
	}
	return "/t " + hardcoreWord
}

// hardcoreLine is whether a typed line is /hardcore ("/hardcore", "#hardcore",
// or "/t hardcore" with nothing after it).
func hardcoreLine(line string) bool {
	line = strings.TrimSpace(line)
	if word, _, _ := strings.Cut(line, " "); strings.EqualFold(word, "/"+hardcoreWord) || strings.EqualFold(word, "#"+hardcoreWord) {
		return true
	}
	name, body, ok := parseTell(line)
	return ok && strings.EqualFold(name, hardcoreWord) && strings.TrimSpace(body) == ""
}

// hardcoreAnswer is HandleDialogueOption's way in.
func hardcoreAnswer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsHardcoreBox(box) {
		return false
	}
	return hardcoreAsker.Load().Answer(sess, p, box, option)
}

// hardcoreCommand is /hardcore's way in.
func hardcoreCommand(sess *session.Session, p *entity.Player) {
	if a := hardcoreAsker.Load(); a != nil {
		a.Show(sess, p)
		return
	}
	SendErrorMessage(sess, hardcoreOffLine)
}
