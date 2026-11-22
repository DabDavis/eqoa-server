package handlers

import (
	"log"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// cityintro.go — a new character's first-login welcome, as the live game's
// tapes show it (work board 825, with the welcomes filmed for Moradhim's rogue
// and paladin, Highbourne's wizard and paladin, Fayspires' enchanter, the
// western monk and Freeport's bard): at the first world entry of a character
// with no experience, before anyone is spoken to, its city's boxes (table
// cityintro: a row a start place and class, a line a box) and then CLIENT's
// question box asks "do you need instructions on how to use the basic
// controls?" [Yes. / No.]. Yes gives the shared control boxes and the row's
// yes_last, No the row's no_last (each sends the player to its guildmaster).
// The welcome is taken (charintro.welcomed) as it goes out, so it is said once.
//
// A welcomed character's first merchant window asks, instead of opening, the
// live game's "Would you like a brief tutorial on how to use merchant NPCs?"
// [Yes. / No.] (charintro.merchant, once): No closes it, Yes says a few lines
// of ours (the tapes never took it); the next talk opens the shop.
//
// The ability tutorial asked after a guild's first ability reward is a quest
// step's choices (data on the step, the engine's own), not this file's.
//
// Timing, as the welcome guide's (welcome.go): the welcome goes out
// introAfter after the entry, once the player is free to answer (not dead or
// loading a world), else it looks again every introRetry until introGiveUp. A
// box CLIENT declines itself because another is up (a 255 within
// introDeclined) goes out again; the welcome guide's own box, Hardcore's and
// the [OG] choice wait for this one the same way. Unanswered boxes live in
// memory only: after a keep restart an old box's answer does nothing (the
// welcome was taken, so it is not said again).
//
// The control boxes name the pad's buttons in words: CLIENT's text box draws
// its own glyphs from `|` codes, which nothing here sends (a run of them
// kills a PS2's TextBox).

// introBoxMark marks the welcome's and the merchant question's box ids
// (beside the welcome guide's 0x61, Hardcore's 0x72 and the others).
const introBoxMark = 0x7A000000

// The welcome's times.
const (
	introAfter    = 3 * time.Second
	introRetry    = 3 * time.Second
	introGiveUp   = 3 * time.Minute
	introDeclined = welcomeDeclined
	introAsksKept = time.Hour
)

// The live game's words (the tapes'), and ours where marked.
const (
	introControlsAsk = "Before we get too far, do you need instructions on how to use the basic controls?"
	introMerchantAsk = "Would you like a brief tutorial on how to use merchant NPCs?"
	introYes         = "Yes."
	introNo          = "No."
)

// introControls is the controls branch's shared boxes (the rogue's tape, boxes
// 4-9: no city word in them), the pad's glyphs in words.
var introControls = []string{
	"Ok adventurer, here are the basic controls of Tunaria.",
	"The left analog stick controls the movement of your character.",
	"The right analog stick not only controls the zoom of the camera, but it will also turn the camera to the left or right.",
	"The R1 button is used to target characters and the L1 button is used to target your group members and yourself.",
	"The Square button is used to interact with targeted characters. This is how you interact with merchants and other characters.",
	"Start opens your options menu. You may select any option by pressing X or exit the menu by pressing Triangle.",
}

// introMerchantYes is the merchant question's Yes (ours: not on film).
var introMerchantYes = []string{
	"A merchant's window lists what it sells and its price. Pick an item to buy it.",
	"To sell, pick an item of your own in the window: a merchant pays less than it charges.",
	"Speak to me again to see my wares.",
}

// introKind is which box an ask is.
type introKind int

const (
	introWelcome introKind = iota
	introMerchant
)

// cityIntroStore is what the welcome keeps (database.SQLiteDB).
type cityIntroStore interface {
	CityIntroFor(world, race, class int, x, z float64) (database.CityIntro, bool, error)
	CharIntroOf(serverID uint32) (database.CharIntro, error)
	TakeIntroWelcome(serverID uint32) (bool, error)
	TakeIntroMerchant(serverID uint32) (bool, error)
}

// CityIntro runs the welcomes and the merchant question.
type CityIntro struct {
	gw    *entity.World
	store cityIntroStore
	now   func() time.Time
	later func(d time.Duration, f func())

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]*introAsk // by character
}

// introAsk is a box asked or waiting to be.
type introAsk struct {
	kind           introKind
	char           uint32
	sess           *session.Session
	box            uint32
	intro          database.CityIntro
	asked, sent    time.Time
	until          time.Time
	waiting, taken bool
	boxesSaid      bool
}

// NewCityIntro makes the welcomes' runner; db nil: none is said.
func NewCityIntro(gw *entity.World, db *database.SQLiteDB) *CityIntro {
	c := &CityIntro{gw: gw, now: time.Now, asked: map[uint32]*introAsk{},
		later: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
	if db != nil {
		c.store = db
	}
	return c
}

// Entered is OnPlayerEntered: the welcome, introAfter on, for a character with
// no experience that was never welcomed and stands at a row's start place.
func (c *CityIntro) Entered(sess *session.Session, p *entity.Player) {
	if c == nil || c.store == nil || p == nil || c.gw == nil {
		return
	}
	snap, ok := c.gw.PlayerSnapshot(p.ServerID)
	if !ok || snap.TotalXP != 0 {
		return
	}
	ci, err := c.store.CharIntroOf(snap.CharServerID)
	if err != nil {
		log.Printf("CityIntro: %s's row: %v", snap.Name, err)
		return
	}
	if ci.Welcomed {
		return
	}
	row, found, err := c.store.CityIntroFor(int(snap.World), snap.Race, snap.Class, float64(snap.X), float64(snap.Z))
	if err != nil {
		log.Printf("CityIntro: %s's welcome: %v", snap.Name, err)
		return
	}
	if !found {
		return
	}
	a := c.ask(sess, snap.CharServerID, introWelcome)
	a.intro = row
	c.tryIn(a, introAfter)
}

// ask makes the character's next box in place of any it had.
func (c *CityIntro) ask(sess *session.Session, char uint32, kind introKind) *introAsk {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, x := range c.asked {
		if now.Sub(x.asked) > introAsksKept {
			delete(c.asked, k)
		}
	}
	c.seq++
	a := &introAsk{kind: kind, char: char, sess: sess, box: introBoxMark | (c.seq & 0xFFFFFF), asked: now,
		until: now.Add(introGiveUp)}
	c.asked[char] = a
	return a
}

// tryIn tries a again d from now, once however often it is asked for.
func (c *CityIntro) tryIn(a *introAsk, d time.Duration) {
	c.mu.Lock()
	if a.waiting || c.asked[a.char] != a {
		c.mu.Unlock()
		return
	}
	a.waiting = true
	c.mu.Unlock()
	c.later(d, func() {
		c.mu.Lock()
		a.waiting = false
		c.mu.Unlock()
		c.try(a)
	})
}

// try sends a's boxes when its player is free to answer, taking the welcome;
// else it looks again introRetry on, until a.until.
func (c *CityIntro) try(a *introAsk) {
	free, gone := boxFree(c.gw, a.sess, a.char)
	now := c.now()
	c.mu.Lock()
	if c.asked[a.char] != a {
		c.mu.Unlock()
		return
	}
	if gone || (!free && !now.Before(a.until)) {
		delete(c.asked, a.char)
		c.mu.Unlock()
		return
	}
	if !free {
		c.mu.Unlock()
		c.tryIn(a, introRetry)
		return
	}
	if !a.taken {
		took, err := c.store.TakeIntroWelcome(a.char)
		if err != nil || !took {
			delete(c.asked, a.char)
			c.mu.Unlock()
			if err != nil {
				log.Printf("CityIntro: character %d's welcome: %v", a.char, err)
			}
			return // another session of it was welcomed first
		}
		a.taken = true
	}
	a.sent = now
	say := !a.boxesSaid
	a.boxesSaid = true
	sess, box, boxes := a.sess, a.box, a.intro.Boxes
	c.mu.Unlock()
	if say {
		for _, b := range boxes {
			SendDialogueBox(sess, 0, b)
		}
		log.Printf("CityIntro: character %d welcomed (cityintro %d)", a.char, a.intro.ID)
	}
	SendQuestPopupMulti(sess, box, introControlsAsk, []string{introYes, introNo})
}

// boxFree is whether a question may be put to sess's character char now (not
// dead, on its way back from a death or loading a world); gone, that the
// session is not that character's in the world any more.
func boxFree(gw *entity.World, s *session.Session, char uint32) (free, gone bool) {
	if s == nil || !s.CharacterInWorld() || s.PendingTermination() {
		return false, true
	}
	if _, replaced := s.ReplacedFor(); replaced {
		return false, true
	}
	if gw == nil {
		return true, false
	}
	p := gw.SessionPlayer(s)
	if p == nil || p.CharServerID != char {
		return false, true
	}
	snap, found := gw.PlayerSnapshot(p.ServerID)
	if !found {
		return false, true
	}
	if snap.IsDead() || !snap.DiedAt.IsZero() || s.StateChannelsHeld() {
		return false, false
	}
	return true, false
}

// MerchantAsk is a merchant window's request from a welcomed character never
// asked the merchant question: the question in its place (the caller clears
// the request). It reports whether it asked.
func (c *CityIntro) MerchantAsk(sess *session.Session, p *entity.Player) bool {
	if c == nil || c.store == nil || p == nil {
		return false
	}
	ci, err := c.store.CharIntroOf(p.CharServerID)
	if err != nil || !ci.Welcomed || ci.Merchant {
		return false
	}
	took, err := c.store.TakeIntroMerchant(p.CharServerID)
	if err != nil || !took {
		if err != nil {
			log.Printf("CityIntro: %s's merchant question: %v", p.Name, err)
		}
		return false
	}
	a := c.ask(sess, p.CharServerID, introMerchant)
	c.mu.Lock()
	a.sent, a.taken = c.now(), true
	c.mu.Unlock()
	SendQuestPopupMulti(sess, a.box, introMerchantAsk, []string{introYes, introNo})
	log.Printf("CityIntro: %s asked the merchant question", p.Name)
	return true
}

// IsIntroBox reports a welcome's or merchant question's box id.
func IsIntroBox(id uint32) bool { return id&0xFF000000 == introBoxMark }

// Answer is HandleDialogueOption's question for one of these boxes; it
// reports whether the box was one (an old one does nothing).
func (c *CityIntro) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsIntroBox(box) {
		return false
	}
	if c == nil || p == nil {
		return true
	}
	now := c.now()
	c.mu.Lock()
	a := c.asked[p.CharServerID]
	if a == nil || a.box != box {
		c.mu.Unlock()
		return true
	}
	if option == 255 {
		declined := a.kind == introWelcome && !a.sent.IsZero() && now.Sub(a.sent) < introDeclined
		if !declined {
			delete(c.asked, p.CharServerID) // the player's Exit
		}
		c.mu.Unlock()
		if declined {
			c.tryIn(a, introRetry)
		}
		return true
	}
	delete(c.asked, p.CharServerID)
	c.mu.Unlock()
	var boxes []string
	switch {
	case a.kind == introWelcome && option == 0:
		boxes = append(append(boxes, introControls...), a.intro.YesLast)
	case a.kind == introWelcome && option == 1:
		boxes = append(boxes, a.intro.NoLast)
	case a.kind == introMerchant && option == 0:
		boxes = introMerchantYes
	}
	for _, b := range boxes {
		if b != "" {
			SendDialogueBox(sess, 0, b)
		}
	}
	return true
}
