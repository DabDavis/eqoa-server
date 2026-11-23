package handlers

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// welcome.go — the welcome guide (petition 1457: "Add a game
// popup when people first log in, a dialogue pop up, for the day ... 1: Ask if
// they would like a guide of the server 2: Ask if they never want to see this
// on that character again ... Make sure to tell them that /tutorial will bring
// this menu up again at any time").
//
// A character's first world entry of a UTC calendar day asks, welcomeAfter
// after its client says it is in the world (as the login's other notices wait:
// the bug line's 15 s, the boon's 20), in the game's own question box (0x0034,
// CLIENT's Msg0034, as every NPC's question here, so a PS2 and vanilla show it
// alike): a guide of the server, its commands and features, or never this box
// again on this character. The day is taken (charwelcome.shown_day) as the box
// goes out, so a second entry that day, a takeover's included, is not asked;
// the box's own Close changes nothing else, and the next day asks again.
//
// Every window of it is such a box, one at a time (a GM's work entry of
// 2026-11-17 and its review of 2026-11-23: an Exit on every window, the tour
// closed at any time, no train of boxes left behind): the guide's pages, the
// Topics box (Graphics, Game features, UI: welcometopics.go) and each topic's
// pages, each with Next while a page follows and a way back (Back to the
// start, Back to topics). CLIENT's own Close (VIWndResponse adds More, Back
// and Close, WORDS 245, 217, 161, after three answers a page) is every
// window's exit, on a PS2 and vanilla alike: no answer of ours says Exit
// beside it, and no window has more than three answers, so none is paged.
// Each window has its own id (welcomeBoxMark), so an answer to an older one
// does nothing, and keeps the actions of its answers in their order
// (welcomeAsk.actions): an answer acts by its place, an answer past them does
// nothing.
//
// The guide is what this server has (welcomeFacts), a part a page or more,
// packed into 127 characters a page: the kept items a login gives (the
// Genie's Lamp, the Heirloom Fang of the Wolf, the Summoning Portal, the Rock
// of Freeport) and the Heirloom Outfitters' free gear, Dabrish and the Spell
// Archivist (in Freeport when they stand by its Announcements board), "And
// more!", how to report a bug, raids and their loot rules, the PC client's
// Options > Extended, and /tutorial. Each line in the spelling the player's
// client can use: a client of ours (vanilla: it answered the offers,
// Session.AckMasks) forwards a /command it does not know, a PS2 keeps one to
// itself and types "/t tutorial".
//
// Never again is kept (charwelcome.never) through logouts and restarts; only
// /tutorial (#tutorial, "/t tutorial") shows the box then, and choosing the
// guide there leaves it off: no answer turns the daily box back on (the user's
// choice, 2026-11-10); a GM does, by the row (never 0, or deleted).
//
// It never goes out over something the player must see to first: while the
// character is dead or on its way back from a death, or loading a world (its
// state channels held), it waits, looking again every welcomeRetry, until
// welcomeGiveUp after it was asked for (then the day was not taken, and the
// next entry that day asks). A box CLIENT declines itself because a menu is up
// (Msg0034: a 255 within welcomeDeclined of its sending) goes out again
// welcomeRetry later until then (vanilla holds such a box until its menu closes
// instead), every window of the tour alike. A 255 after that is the player's
// Close: the tour ends there, nothing more is sent or tried; the window last
// sent still takes a pick, as a PS2 sends 255 just before the real pick when
// two boxes were open.
//
// world.welcome_guide false (SetWelcomeGuide) turns all of it off. Its words
// and times are ours.

// welcomeBoxMark marks the welcome box's id (beside the quests' 0x51, the
// triggers' 0x52 and the others, the Teleporter's 0x5E among them).
const welcomeBoxMark = 0x61000000

// The welcome box's times (ours).
const (
	// welcomeAfter is how long after the world entry the day's box goes out
	welcomeAfter = 10 * time.Second
	// welcomeRetry is how soon a box that waited or was declined is tried again
	welcomeRetry = 3 * time.Second
	// welcomeGiveUp is how long after it was asked for a box is still tried
	welcomeGiveUp = 3 * time.Minute
	// welcomeDeclined is how soon a 255 counts as CLIENT's own decline (the
	// genie's measure)
	welcomeDeclined = genieDeclinedWithin
	// welcomeFactsFor is how long what the server has is taken as read
	welcomeFactsFor = 5 * time.Minute
	// welcomeNear is how near the Announcements board a helper stands "by" it
	welcomeNear = 60
	// welcomeAsksKept is how long an unanswered box is remembered
	welcomeAsksKept = time.Hour
)

// tutorialWord is /tutorial's word: "/tutorial", "#tutorial" and a PS2's
// "/t tutorial".
const tutorialWord = "tutorial"

// welcomeBoardName is Freeport's Announcements board, the guide's landmark.
const welcomeBoardName = "Freeport Announcements"

// The box (127 characters a question, 63 an answer: questAskChars,
// questAnswerChars), its answers in order, and the lines around it.
const (
	welcomeQuestionDaily    = "Welcome, %s! Would you like a quick guide to this server? Close this to be asked again another day."
	welcomeQuestionAnon     = "Welcome! Would you like a quick guide to this server? Close this to be asked again another day."
	welcomeQuestionTutorial = "Would you like a quick guide to this server, or its commands and features?"
	welcomeGuideAnswer      = "Yes, show me the guide"
	welcomeCommandsAnswer   = "Commands/features"
	welcomeNeverAnswer      = "Never show this again on this character"
	welcomeNextAnswer       = "Next"
	welcomeStartAnswer      = "Back to the start"
	welcomeTopicsAnswer     = "Back to topics"

	welcomeNeverLine = "You will not be asked again on %s. Type %s any time to see the guide."
	welcomeNeverFail = "That could not be kept just now: try again."
	welcomeOffLine   = "There is no welcome guide on this server."
)

// The guide's parts, a sentence each, packed into pages by part
// (welcomeGuidePages).
// No '%' or '|' (CLIENT's text expander reads both as codes) and nothing past
// ASCII, which a PS2's font has.
const (
	welcomeLamp      = "Your Genie's Lamp calls a genie anywhere: your bank, heirloom wares, repairs and more."
	welcomeOutfitter = "Heirloom gear is free and grows with your level: an Heirloom Outfitter stands by your first quest giver."
	welcomeFang      = "Put the Heirloom Fang of the Wolf on a quick slot: it gives you run speed."
	welcomePortal    = "The Summoning Portal calls any player to you."
	welcomePortalFP  = "The Summoning Portal calls any player to you, or takes you to Freeport."
	welcomeRock      = "The Rock of Freeport takes you to Freeport."

	welcomeDabrishBoard   = "In Freeport, by the Announcements board, Dabrish sets your level from 5 to 60 in steps of 5 and adds class mastery points."
	welcomeDabrish        = "Dabrish sets your level from 5 to 60 in steps of 5 and adds class mastery points."
	welcomeArchivistThere = "The Spell Archivist there has your class's spells."
	welcomeArchivistBoard = "In Freeport, by the Announcements board, the Spell Archivist has your class's spells."
	welcomeArchivist      = "The Spell Archivist has your class's spells."
	welcomeAndMore        = "And more!"

	welcomeRaidsOurs    = "Things to explore: raids, with the Raid List window and the raid commands (/help lists them)."
	welcomeRaidLootOurs = "A raid's leader sets its loot rules in Community > Group > Raid: free for all, need before greed or master looter."
	welcomeRaidsPS2     = "Things to explore: raids, with the raid commands (/t help lists them)."
	welcomeRaidLootPS2  = "A raid's leader sets its loot rules with /t raidloot: free for all, need before greed or master looter."
	welcomeExtendedOurs = "Options > Extended has more graphics options and settings."
	welcomeExtendedPS2  = "On the PC or browser client, Options > Extended has more graphics options and settings."
	welcomeTutorial     = "Type %s any time to bring this guide back. Have fun!"
)

var welcomeOn = func() *atomic.Bool {
	var b atomic.Bool
	b.Store(true)
	return &b
}()

// SetWelcomeGuide turns the welcome guide on or off (world.welcome_guide; on
// unless set false).
func SetWelcomeGuide(on bool) { welcomeOn.Store(on) }

// WelcomeGuideOn reports world.welcome_guide.
func WelcomeGuideOn() bool { return welcomeOn.Load() }

// welcomeStore is where a character's day and never are kept (database.SQLiteDB).
type welcomeStore interface {
	WelcomeDue(serverID uint32, day string) (bool, error)
	TakeWelcomeDay(serverID uint32, day string) (bool, error)
	SetWelcomeNever(serverID uint32, never bool) error
}

// WelcomeGuide runs the welcome box and its guide.
type WelcomeGuide struct {
	gw    *entity.World
	store welcomeStore
	db    *database.SQLiteDB // the kept items' records (welcomeFacts); nil: none

	now   func() time.Time
	later func(time.Duration, func()) // time.AfterFunc's; a test's own

	mu      sync.Mutex
	seq     uint32
	asked   map[uint32]*welcomeAsk // the box last asked, by the character's serverid
	facts   welcomeFacts
	factsAt time.Time
	// factsOf reads what the server has (readFacts); a test's own
	factsOf func() welcomeFacts
}

// welcomeAsk is a box asked: its character and session, its id, question and
// answers with what each does, whether it is the day's, when it last went out
// and until when it is tried.
type welcomeAsk struct {
	char     uint32
	sess     *session.Session
	box      uint32
	question string
	answers  []string
	actions  []welcomeAction // what answers[i] does
	daily    bool
	taken    bool      // the day's box: the day taken (it went out)
	sent     time.Time // when it last went out; zero: not yet
	asked    time.Time
	until    time.Time
	waiting  bool // a try is on its way
}

// NewWelcomeGuide makes the welcome guide over db's charwelcome.
func NewWelcomeGuide(gw *entity.World, db *database.SQLiteDB) *WelcomeGuide {
	g := &WelcomeGuide{gw: gw, db: db, now: time.Now, asked: map[uint32]*welcomeAsk{},
		later: func(d time.Duration, f func()) { time.AfterFunc(d, f) }}
	if db != nil {
		g.store = db
	}
	g.factsOf = g.readFacts
	return g
}

// welcomeStep is a window of the tour, or the never answer.
type welcomeStep int

const (
	stepStart  welcomeStep = iota // the start box (/tutorial's question)
	stepGuide                     // a page of the guide
	stepTopics                    // the Topics box
	stepTopic                     // a page of a topic
	stepNever                     // never again on this character
)

// welcomeAction is what an answer does: the window it opens (a topic's, a
// page's), or never.
type welcomeAction struct {
	step  welcomeStep
	topic welcomeTopicID
	page  int
}

// welcomeWindow is a window's question and its answers with their actions.
type welcomeWindow struct {
	question string
	answers  []string
	actions  []welcomeAction
}

// add is an answer and what it does.
func (w *welcomeWindow) add(answer string, a welcomeAction) {
	w.answers = append(w.answers, answer)
	w.actions = append(w.actions, a)
}

// startWindow is the start box: the day's question (by name), or
// /tutorial's.
func startWindow(name string, daily bool) welcomeWindow {
	w := welcomeWindow{question: welcomeQuestionTutorial}
	if daily {
		w.question = fmt.Sprintf(welcomeQuestionDaily, name)
		if len([]rune(w.question)) > questAskChars || strings.ContainsAny(name, "%|") {
			w.question = welcomeQuestionAnon
		}
	}
	w.add(welcomeGuideAnswer, welcomeAction{step: stepGuide})
	w.add(welcomeCommandsAnswer, welcomeAction{step: stepTopics})
	w.add(welcomeNeverAnswer, welcomeAction{step: stepNever})
	return w
}

// window is the window an action opens for the session, on a server that has
// f; ok false for none (a topic gone since).
func window(act welcomeAction, f welcomeFacts, sess *session.Session) (welcomeWindow, bool) {
	ours := sess.AckMasks()
	switch act.step {
	case stepStart:
		return startWindow("", false), true
	case stepGuide:
		return pageWindow(welcomeGuidePages(f, ours, sess.BugReports()), act, welcomeStartAnswer,
			welcomeAction{step: stepStart}), true
	case stepTopics:
		w := welcomeWindow{question: welcomeTopicsQuestion}
		for _, t := range welcomeTopics(f, ours, QoLOnFor(sess)) {
			w.add(t.name, welcomeAction{step: stepTopic, topic: t.id})
		}
		return w, len(w.answers) > 0
	case stepTopic:
		t, ok := welcomeTopicOf(welcomeTopics(f, ours, QoLOnFor(sess)), act.topic)
		if !ok {
			return welcomeWindow{}, false
		}
		return pageWindow(welcomePack([][]string{t.lines}), act, welcomeTopicsAnswer, welcomeAction{step: stepTopics}), true
	}
	return welcomeWindow{}, false
}

// pageWindow is page act.page of pages (the last if past it): Next while a
// page follows, then the way back.
func pageWindow(pages []string, act welcomeAction, back string, backTo welcomeAction) welcomeWindow {
	if len(pages) == 0 {
		pages = []string{welcomeAndMore}
	}
	i := min(max(act.page, 0), len(pages)-1)
	w := welcomeWindow{question: pages[i]}
	if i+1 < len(pages) {
		next := act
		next.page = i + 1
		w.add(welcomeNextAnswer, next)
	}
	w.add(back, backTo)
	return w
}

// welcomeDay is t's UTC calendar day, as charwelcome keeps it.
func welcomeDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// Entered is OnPlayerEntered: the day's box, welcomeAfter on, when it is due
// (its day not taken, never not said).
func (g *WelcomeGuide) Entered(sess *session.Session, p *entity.Player) {
	if g == nil || g.store == nil || p == nil || !WelcomeGuideOn() {
		return
	}
	due, err := g.store.WelcomeDue(p.CharServerID, welcomeDay(g.now()))
	if err != nil {
		log.Printf("Welcome: %s's box: %v", p.Name, err)
		return
	}
	if !due {
		return
	}
	a, _ := g.ask(sess, p, startWindow(p.Name, true), true)
	g.tryIn(a, welcomeAfter)
}

// Show is /tutorial: the box, whatever the day and never (it waits as the
// day's does for a death or a world loading). Typed while the day's box waits
// to go out, it is that box: the day is taken.
func (g *WelcomeGuide) Show(sess *session.Session, p *entity.Player) {
	if g == nil || p == nil || !WelcomeGuideOn() {
		SendErrorMessage(sess, welcomeOffLine)
		return
	}
	a, old := g.ask(sess, p, startWindow(p.Name, false), false)
	if old != nil && old.daily && !old.taken && g.store != nil {
		if _, err := g.store.TakeWelcomeDay(p.CharServerID, welcomeDay(g.now())); err != nil {
			log.Printf("Welcome: %s's day: %v", p.Name, err)
		}
	}
	g.try(a)
}

// ask makes p's next box, the window w, in place of any it has, which it
// returns (nil: none).
func (g *WelcomeGuide) ask(sess *session.Session, p *entity.Player, w welcomeWindow, daily bool) (a, old *welcomeAsk) {
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	for char, x := range g.asked {
		if now.Sub(x.asked) > welcomeAsksKept {
			delete(g.asked, char)
		}
	}
	g.seq++
	a = &welcomeAsk{char: p.CharServerID, sess: sess, box: welcomeBoxMark | (g.seq & 0xFFFFFF), question: w.question,
		answers: w.answers, actions: w.actions, daily: daily, asked: now, until: now.Add(welcomeGiveUp)}
	old = g.asked[a.char]
	g.asked[a.char] = a
	return a, old
}

// tryIn tries a again d from now, once however often it is asked for.
func (g *WelcomeGuide) tryIn(a *welcomeAsk, d time.Duration) {
	g.mu.Lock()
	if a.waiting || g.asked[a.char] != a {
		g.mu.Unlock()
		return
	}
	a.waiting = true
	g.mu.Unlock()
	g.later(d, func() {
		g.mu.Lock()
		a.waiting = false
		g.mu.Unlock()
		g.try(a)
	})
}

// try sends a's box when its player is free to answer it, taking the day for
// the day's; else it looks again welcomeRetry on, until a.until.
func (g *WelcomeGuide) try(a *welcomeAsk) {
	free, gone := g.free(a)
	now := g.now()
	g.mu.Lock()
	if g.asked[a.char] != a {
		g.mu.Unlock()
		return // answered, or another box asked
	}
	if gone || (!free && !now.Before(a.until)) {
		delete(g.asked, a.char)
		g.mu.Unlock()
		return
	}
	if !free {
		g.mu.Unlock()
		g.tryIn(a, welcomeRetry)
		return
	}
	if a.daily && !a.taken {
		took, err := g.store.TakeWelcomeDay(a.char, welcomeDay(now))
		if err != nil || !took {
			delete(g.asked, a.char)
			g.mu.Unlock()
			if err != nil {
				log.Printf("Welcome: character %d's day: %v", a.char, err)
			}
			return // a takeover's box, say, went out first
		}
		a.taken = true
	}
	a.sent = now
	sess, box, question, answers := a.sess, a.box, a.question, a.answers
	g.mu.Unlock()
	SendQuestPopupMulti(sess, box, question, answers)
}

// free is whether a's player may be asked now; gone, that its session is not
// its character's in the world any more (it left, was replaced or ends).
func (g *WelcomeGuide) free(a *welcomeAsk) (free, gone bool) {
	s := a.sess
	if s == nil || !s.CharacterInWorld() || s.PendingTermination() {
		return false, true
	}
	if _, replaced := s.ReplacedFor(); replaced {
		return false, true
	}
	if g.gw == nil {
		return true, false
	}
	p := g.gw.SessionPlayer(s)
	if p == nil || p.CharServerID != a.char {
		return false, true
	}
	snap, found := g.gw.PlayerSnapshot(p.ServerID)
	if !found {
		return false, true
	}
	if snap.IsDead() || !snap.DiedAt.IsZero() || s.StateChannelsHeld() {
		return false, false // a death, its way back or a world loading first
	}
	return true, false
}

// IsWelcomeBox reports a welcome box's id.
func IsWelcomeBox(id uint32) bool { return id&0xFF000000 == welcomeBoxMark }

// Answer is HandleDialogueOption's question for a welcome box: the window
// its answer opens, or never again. It reports whether the box was one (an
// old one, or an answer past the window's, does nothing).
func (g *WelcomeGuide) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsWelcomeBox(box) {
		return false
	}
	if g == nil || p == nil {
		return true
	}
	now := g.now()
	g.mu.Lock()
	a := g.asked[p.CharServerID]
	if a == nil || a.box != box {
		g.mu.Unlock()
		return true
	}
	if option == 255 {
		// CLIENT's own decline (a menu up) goes out again; a later 255 is the
		// player's Close, which ends the tour: nothing more is sent, and the
		// window takes a pick that follows it (the file's doc)
		declined := !a.sent.IsZero() && now.Sub(a.sent) < welcomeDeclined
		g.mu.Unlock()
		if declined {
			g.tryIn(a, welcomeRetry)
		}
		return true
	}
	if int(option) >= len(a.actions) {
		g.mu.Unlock()
		return true
	}
	act := a.actions[option]
	delete(g.asked, p.CharServerID)
	g.mu.Unlock()
	if act.step == stepNever {
		g.never(sess, p)
		return true
	}
	if act.step == stepGuide && act.page == 0 {
		log.Printf("Welcome: %s asked for the guide", p.Name)
	}
	w, ok := window(act, g.factsNow(), sess)
	if !ok {
		return true
	}
	next, _ := g.ask(sess, p, w, false)
	g.try(next)
	return true
}

// never keeps never again on p's character, and says how to see the guide.
func (g *WelcomeGuide) never(sess *session.Session, p *entity.Player) {
	if g.store == nil {
		SendErrorMessage(sess, welcomeNeverFail)
		return
	}
	if err := g.store.SetWelcomeNever(p.CharServerID, true); err != nil {
		log.Printf("Welcome: %s's never not kept: %v", p.Name, err)
		SendErrorMessage(sess, welcomeNeverFail)
		return
	}
	SendColoredChat(sess, fmt.Sprintf(welcomeNeverLine, p.Name, tutorialSpelling(sess.AckMasks())), 255, 255, 0)
	log.Printf("Welcome: %s is not to be asked again", p.Name)
}

// tutorialSpelling is /tutorial as the player's client can send it: a PS2
// keeps an unknown /command to itself, so its player types "/t tutorial".
func tutorialSpelling(ours bool) string {
	if ours {
		return "/" + tutorialWord
	}
	return "/t " + tutorialWord
}

// tutorialLine is whether a typed line is /tutorial: "/tutorial" or
// "#tutorial" with anything after it, or "/t tutorial" (or "/tell") with
// nothing after it (with text it is a tell), in any case.
func tutorialLine(line string) bool {
	line = strings.TrimSpace(line)
	if word, _, _ := strings.Cut(line, " "); strings.EqualFold(word, "/"+tutorialWord) || strings.EqualFold(word, "#"+tutorialWord) {
		return true
	}
	name, body, ok := parseTell(line)
	return ok && strings.EqualFold(name, tutorialWord) && strings.TrimSpace(body) == ""
}

// welcomeFacts is what this server has that the guide speaks of.
type welcomeFacts struct {
	lamp, fang, portal, rock bool // the kept items a login gives (genie.go, wolffang.go, summonportal.go, rockfreeport.go)
	outfitters               bool // an Heirloom Outfitter stands (heirloom gear, free)
	dabrish, archivist       bool // Dabrish and a Spell Archivist stand
	// dabrishBoard and archivistBoard: they stand within welcomeNear of Freeport's
	// Announcements board
	dabrishBoard, archivistBoard bool

	// the Game features topic's (welcometopics.go)
	mercs                       bool        // world.mercs (MercsOn)
	teleporter, teleporterBoard bool        // world.teleporter and a Teleporter standing (by the board)
	boon                        entity.Boon // the towns' boon (Boon.On: the boards give one)
	ascended                    bool        // Ascended records exist
	conLevel                    bool        // world.con_level (ConLevelOn)
}

// factsNow is what the server has, read at most every welcomeFactsFor.
func (g *WelcomeGuide) factsNow() welcomeFacts {
	now := g.now()
	g.mu.Lock()
	if !g.factsAt.IsZero() && now.Sub(g.factsAt) < welcomeFactsFor {
		f := g.facts
		g.mu.Unlock()
		return f
	}
	g.mu.Unlock()
	f := g.factsOf()
	g.mu.Lock()
	g.facts, g.factsAt = f, now
	g.mu.Unlock()
	return f
}

// readFacts reads what the server has: the kept items' records (given at a
// login only with world.heirlooms on), the Rock of Freeport's switch, and the
// helpers standing in the world.
func (g *WelcomeGuide) readFacts() welcomeFacts {
	var f welcomeFacts
	if g.db != nil && heirloomsGiven.Load() {
		f.lamp = genieLampID(g.db) != 0
		f.fang = keptItemID(g.db, WolfFangName, "Welcome") != 0
		f.portal = keptItemID(g.db, SummoningPortalName, "Welcome") != 0
	}
	f.rock = RockOfFreeportOn()
	f.mercs = MercsOn()
	f.ascended = len(entity.AscendedItems()) > 0
	f.conLevel = ConLevelOn()
	if g.gw == nil {
		return f
	}
	f.boon = g.gw.Boon()
	key := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	spots := g.gw.NPCSpotsNamed(map[string]bool{key(DabrishName): true, key(SpellArchivistName): true,
		key(heirloomOutfitterName): true, key(welcomeBoardName): true, key(TeleporterName): true})
	boards := spots[key(welcomeBoardName)]
	byBoard := func(of []entity.NPCSpot) bool {
		for _, s := range of {
			for _, b := range boards {
				if s.World == b.World && math.Hypot(float64(s.X-b.X), float64(s.Z-b.Z)) <= welcomeNear {
					return true
				}
			}
		}
		return false
	}
	dab, arch := spots[key(DabrishName)], spots[key(SpellArchivistName)]
	f.dabrish, f.archivist = len(dab) > 0, len(arch) > 0
	f.dabrishBoard, f.archivistBoard = byBoard(dab), byBoard(arch)
	f.outfitters = len(spots[key(heirloomOutfitterName)]) > 0
	if TeleporterOn() {
		tele := spots[key(TeleporterName)]
		f.teleporter, f.teleporterBoard = len(tele) > 0, byBoard(tele)
	}
	return f
}

// welcomeGuidePages is the guide, a question box's page at a time, for a
// server that has f, to a client of ours (vanilla) or not (a PS2), that
// reports with F3 or not.
func welcomeGuidePages(f welcomeFacts, ours, f3 bool) []string {
	var items []string
	if f.lamp {
		items = append(items, welcomeLamp)
	}
	if f.outfitters {
		items = append(items, welcomeOutfitter)
	}
	if f.fang {
		items = append(items, welcomeFang)
	}
	switch {
	case f.portal && f.rock:
		items = append(items, welcomePortal, welcomeRock)
	case f.portal:
		items = append(items, welcomePortalFP)
	case f.rock:
		items = append(items, welcomeRock)
	}

	var freeport []string
	switch {
	case f.dabrish && f.dabrishBoard:
		freeport = append(freeport, welcomeDabrishBoard)
	case f.dabrish:
		freeport = append(freeport, welcomeDabrish)
	}
	switch {
	case f.archivist && f.archivistBoard && f.dabrish && f.dabrishBoard:
		freeport = append(freeport, welcomeArchivistThere)
	case f.archivist && f.archivistBoard:
		freeport = append(freeport, welcomeArchivistBoard)
	case f.archivist:
		freeport = append(freeport, welcomeArchivist)
	}
	if len(freeport) > 0 {
		freeport = append(freeport, welcomeAndMore)
	}

	bug := reportNoticeText
	if f3 {
		bug = reportNoticeF3
	}
	raids, loot, extended := welcomeRaidsPS2, welcomeRaidLootPS2, welcomeExtendedPS2
	if ours {
		raids, loot, extended = welcomeRaidsOurs, welcomeRaidLootOurs, welcomeExtendedOurs
	}
	// /tutorial's line last, on the closing page
	return welcomePack([][]string{items, freeport, {bug}, {raids, loot}, {extended, fmt.Sprintf(welcomeTutorial, tutorialSpelling(ours))}})
}

// welcomePack packs parts' sentences into pages of a question box's
// questAskChars, a part's never sharing a page with another's; a sentence
// longer than a page is cut where splitSay cuts it.
func welcomePack(parts [][]string) []string {
	var pages []string
	for _, part := range parts {
		var page string
		for _, s := range part {
			switch {
			case page == "":
				page = s
			case len([]rune(page))+1+len([]rune(s)) <= questAskChars:
				page += " " + s
			default:
				pages = append(pages, splitSay(page, questAskChars)...)
				page = s
			}
		}
		if page != "" {
			pages = append(pages, splitSay(page, questAskChars)...)
		}
	}
	return pages
}
