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

// hardcoreBoxMark marks the Hardcore boxes' ids (beside the welcome guide's
// 0x61, the call's 0x63 and the others).
const hardcoreBoxMark = 0x72000000

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

// hardcoreWord is /hardcore's word: "/hardcore", "#hardcore", a PS2's "/t hardcore".
const hardcoreWord = "hardcore"

// hardcoreAsker is the box's asker (registry.go); nil: none.
var hardcoreAsker atomic.Pointer[HardcoreAsk]

// hardcoreAskStore is the character's state and choice (database.SQLiteDB).
type hardcoreAskStore interface {
	HardcoreOf(serverID uint32) (database.HardcoreState, error)
	ChooseHardcore(serverID uint32, on bool) (bool, error)
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

// open reads whether p's choice is still open.
func (a *HardcoreAsk) open(p *entity.Player) (database.HardcoreState, bool) {
	if a == nil || a.store == nil || p == nil || !HardcoreOn() {
		return database.HardcoreState{}, false
	}
	st, err := a.store.HardcoreOf(p.CharServerID)
	if err != nil {
		log.Printf("Hardcore: %s's choice: %v", p.Name, err)
		return st, false
	}
	return st, st.ChoiceOpen()
}

// Entered is OnPlayerEntered: the first box, hardcoreAskAfter on, for a
// character whose choice is open.
func (a *HardcoreAsk) Entered(sess *session.Session, p *entity.Player) {
	if _, open := a.open(p); !open {
		return
	}
	b := a.newBox(sess, p, false)
	a.tryIn(b, hardcoreAskAfter)
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
		a.try(a.newBox(sess, p, false))
	case st.Hardcore:
		SendClientMessage(sess, fmt.Sprintf(hardcoreIsLine, p.Name))
	default:
		SendClientMessage(sess, fmt.Sprintf(hardcoreClosed, p.Name))
	}
}

func (a *HardcoreAsk) newBox(sess *session.Session, p *entity.Player, second bool) *hardcoreAskBox {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	for c, x := range a.asked {
		if now.Sub(x.asked) > hardcoreAsksKept {
			delete(a.asked, c)
		}
	}
	a.seq++
	b := &hardcoreAskBox{char: p.CharServerID, sess: sess, box: hardcoreBoxMark | (a.seq & 0xFFFFFF), second: second,
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
	sess, box, second := b.sess, b.box, b.second
	a.mu.Unlock()
	name := ""
	if p := a.gw.SessionPlayer(sess); p != nil {
		name = p.Name
	}
	q, answers := hardcoreQuestion(name, second)
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

// IsHardcoreBox reports a Hardcore box's id.
func IsHardcoreBox(id uint32) bool { return id&0xFF000000 == hardcoreBoxMark }

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
		a.mu.Unlock()
		if declined {
			a.tryIn(b, hardcoreAskRetry) // CLIENT's own decline (a menu up)
		} else {
			SendClientMessage(sess, fmt.Sprintf(hardcoreLaterLn, p.Name, hardcoreSpelling(sess.AckMasks())))
		}
		return true
	}
	delete(a.asked, p.CharServerID)
	second := b.second
	a.mu.Unlock()
	if _, open := a.open(p); !open {
		return true // played on meanwhile, or chosen in another window
	}
	switch {
	case option == 0 && !second:
		a.try(a.newBox(sess, p, true))
	case option == 0 && second:
		a.choose(sess, p, true)
	case option == 1:
		a.choose(sess, p, false)
	}
	return true
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
