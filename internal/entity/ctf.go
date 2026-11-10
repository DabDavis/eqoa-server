package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// ctf.go — capture the flag in an arena world of its own (the owner, 2026-11-10: "since we have a template for
// making new worlds and zones lets add a new PvP world. One zone for Capture the Flag. players will join from PvP
// Events NPC next to Dabrish, will have 2 teams. we have flag model, teams will grab flag from middle and take to
// their base. decorate for a tactical experience", then, the same day, "for the ctf we are making it like warsong
// gulch from WoW": that game's rules, never its map, names or look). The arena is a server world composed of one
// disc zone (handlers/serverworlds.go, tools/worlds/arena.json); this file is its game.
//
// The rules (claude-notes/ctf-world-20261110.md has them whole, with every number):
//
//   - Joining: talking to the PvP Events herald beside Dabrish (handlers/ctf.go) signs a player up, and talking
//     again takes it off; /ctf says how things stand, /ctf leave leaves the queue or a match. A client that cannot
//     load the arena (a PS2, a page, an older vanilla) is told why and kept out.
//   - The start: once MinPerTeam a side are signed up, a Countdown runs (more may sign up meanwhile), then up to
//     TeamSize a side are split into two teams as even as they come (by count, then by levels), sent to their own
//     camps, and given Prep before the banners can be touched or anyone hurt.
//   - The banners: each team has its own, on its stand in its base. A player takes the other team's banner by
//     touching it (TouchReach), at its stand or where it lies; it is shown in the carrier's hand to everyone (the
//     channel's override item, ctfHand) and the carrier's name is green to every client. A carrier scores by
//     touching its own team's stand while its own banner stands there: a capture, and the banner goes back to its
//     stand. A carrier who dies, leaves the match or the world drops the banner where it was; a player of the
//     banner's own team returns it to its stand by touching it, an enemy takes it up again, and one left alone for
//     FlagReturn goes back by itself (so a banner dropped where no one can reach cannot stall the match).
//   - The fight: two players of a match on different teams are foes in the arena (pvpFoe): the PvP flag's own
//     fight (pvp.go), every hit at PvPDamageShare; a teammate never, nor anyone outside the match. A death in the
//     arena costs nothing (pvp.go's pvpDeathDebt, nothing lost) and the dead rise together at their team's camp,
//     in waves every WaveEvery (the first one at least WaveMinDead after the death).
//   - A carrier who has held a banner WoundAfter takes WoundStep percent more from every hit, and as much again
//     each further minute, at most WoundMax more: a match cannot stall on a carrier hiding in its base.
//   - Power-ups (PowerUps): a speed boost (Spirit of the Wolf's run speed for SpeedFor) and a heal (hit points and
//     power full), each taken by touch, back PowerUpEvery after.
//   - The end: ScoreLimit captures win; at TimeLimit the higher score wins and a tie is a draw; a team left with
//     nobody for ctfForfeitAfter loses. Everyone is told, the server too, and EndHold later every player of the
//     match is sent back beside the herald (a dead one rises there).
//
// Nothing outside the arena changes: a player of no match is no one's foe there, a match's player no one's outside
// it, and the PvP flag (world.pvp) is its own business in every other world. The words are ours.

// CTFSettings are a match's numbers (config world.ctf; DefaultCTFSettings has every default).
type CTFSettings struct {
	World        uint8         // the arena's world slot
	TeamSize     int           // the most a team holds
	MinPerTeam   int           // the fewest a team starts with
	ScoreLimit   int           // captures that win
	TimeLimit    time.Duration // the match's length, from the banners going live
	WaveEvery    time.Duration // the dead rise together this often
	WaveMinDead  time.Duration // and not before this long dead
	Countdown    time.Duration // from enough signed up to the start
	Prep         time.Duration // in the arena before the banners go live
	FlagReturn   time.Duration // a dropped banner left alone goes home after
	EndHold      time.Duration // from the end to everyone sent back
	WoundAfter   time.Duration // a carrier past this takes more from every hit (0: never)
	WoundStep    int           // percent more, and as much again each further minute
	WoundMax     int           // the most more, in percent
	PowerUpEvery time.Duration // a power-up comes back this long after it is taken
	SpeedFor     time.Duration // the speed boost lasts
	MinLevel     int           // the lowest level that may sign up
	TouchReach   float32       // how near a banner, a stand or a power-up a touch is
}

// DefaultCTFSettings are the numbers a server sets nothing of (the note's table).
func DefaultCTFSettings() CTFSettings {
	return CTFSettings{
		World:        11,
		TeamSize:     10,
		MinPerTeam:   2,
		ScoreLimit:   3,
		TimeLimit:    20 * time.Minute,
		WaveEvery:    30 * time.Second,
		WaveMinDead:  5 * time.Second,
		Countdown:    30 * time.Second,
		Prep:         30 * time.Second,
		FlagReturn:   30 * time.Second,
		EndHold:      15 * time.Second,
		WoundAfter:   4 * time.Minute,
		WoundStep:    25,
		WoundMax:     200,
		PowerUpEvery: 60 * time.Second,
		SpeedFor:     20 * time.Second,
		MinLevel:     1,
		TouchReach:   5,
	}
}

// ctfForfeitAfter is how long a team may stand empty in a running match before the other wins by it (time
// for the queue to fill it: ctfBackfill).
const ctfForfeitAfter = 30 * time.Second

// ctfBackfillLeft is how much of the time limit must be left for a signed-up player to be sent into a match
// that is on.
const ctfBackfillLeft = 2 * time.Minute

// ctfTouchAbove and ctfTouchBelow are how far over and under a spot's height a body touches it.
const (
	ctfTouchAbove float32 = 4
	ctfTouchBelow float32 = 3
)

// ctfSayEvery is how often a carrier at its stand is told its own banner must be there.
const ctfSayEvery = 4 * time.Second

// ctfStuckEvery is how often /stuck sends a match's player back to its camp.
const ctfStuckEvery = time.Minute

// ctfQuietAfterMove is how long a player the server put somewhere (a rise at its camp, a send there) touches
// nothing: its client's reports from before it heard of the move are not where it is.
const ctfQuietAfterMove = 2 * time.Second

// ctfArriveGrace is how long a player sent to its camp may take to be in the arena (the world loaded) before a
// match lets it go as gone from the arena.
const ctfArriveGrace = time.Minute

// ctfStrayGrace is how long a player in the arena's world with no match there (a restart's, a GM's move) is left
// before it is sent beside the herald.
const ctfStrayGrace = 5 * time.Second

// CTFHeraldName is the herald's name: an NPC of it signs players up (handlers/ctf.go), and the way back lands in
// front of it (else in front of Dabrish).
const CTFHeraldName = "PvP Events"

// ctfReturnBy is the NPC the way back lands by when the herald is not in the world.
const ctfReturnBy = "Dabrish"

// ctfReturnOut is how far in front of the herald the way back lands.
const ctfReturnOut float32 = 4

// CTFSpot is a place in the arena: where it is and the way a player put there faces (a 0x40's heading).
type CTFSpot struct {
	X, Y, Z float32
	Facing  float32
}

// CTFTeamLayout is one team's places and words.
type CTFTeamLayout struct {
	Name  string  // the team in a sentence: "the Ember Band"
	Short string  // in a score: "Ember"
	Flag  string  // its banner's name: "Ember Banner"
	Stand CTFSpot // where its banner stands, and where its carriers score
	Camp  CTFSpot // where its players start and rise
	// Body is its banner's body (a sprite's DictID) and Size its scale; 0: the layout's FlagBody and FlagSize
	Body uint32
	Size float32
}

// CTFPowerUpLayout is a power-up's kind (CTFSpeed, CTFHeal) and place.
type CTFPowerUpLayout struct {
	Kind string
	Name string
	At   CTFSpot
}

// The power-ups' kinds.
const (
	CTFSpeed = "speed"
	CTFHeal  = "heal"
)

// CTFLayout is the arena as its recipe has it (tools/worlds/arena.json's "arena"): its name, the teams' places, the
// power-ups', the field's edge, and the bodies it shows.
type CTFLayout struct {
	Name     string
	Teams    [2]CTFTeamLayout
	PowerUps []CTFPowerUpLayout
	// Field is the playing field's edge, a polygon in (x, z): a player of a match past it is put back at its camp.
	// Empty: no edge but the world's.
	Field [][2]float32
	// FlagBody is the body a banner's NPC stands in (a CHAR.ESF or ITEM.ESF sprite's DictID); FlagHand is the item
	// a carrier is shown holding (an ITEM.ESF model's DictID, the channel's override item), 0 none.
	FlagBody  uint32
	FlagHand  uint32
	FlagSize  float32
	SpeedBody uint32
	HealBody  uint32
}

// ctfPhase is where a match is.
type ctfPhase int

const (
	ctfPrep ctfPhase = iota // in the arena, the banners not yet live
	ctfPlay                 // the banners live
	ctfOver                 // the result told, everyone about to go back
)

// ctfFlagState is where a banner is.
type ctfFlagState int

const (
	ctfFlagHome ctfFlagState = iota
	ctfFlagCarried
	ctfFlagDropped
)

// ctfFlag is one team's banner.
type ctfFlag struct {
	team      int
	state     ctfFlagState
	carrier   EntityID
	takenAt   time.Time // its carrier took it (the wounds count from here)
	wounds    int       // the last wound step told
	x, y, z   float32   // where it lies when dropped
	droppedAt time.Time
	npc       EntityID // the NPC showing it at its stand or where it lies, 0 while carried
}

// ctfPowerUp is a power-up in the field.
type ctfPowerUp struct {
	lay   CTFPowerUpLayout
	npc   EntityID
	backAt time.Time // taken: back then
}

// ctfTeam is a team in a match.
type ctfTeam struct {
	members    []EntityID
	score      int
	emptySince time.Time
}

// ctfMatch is the match on.
type ctfMatch struct {
	id        uint32
	phase     ctfPhase
	began     time.Time // its players sent in
	opens     time.Time // the banners go live
	ends      time.Time // the time limit
	overAt    time.Time // the result told
	teams     [2]ctfTeam
	flags     [2]*ctfFlag
	powerUps  []*ctfPowerUp
	winner    int       // -1: a draw (or none yet)
	how       string    // how it ended, for the log
	minsTold  int       // the last "minutes left" said
	sentHome  bool
}

// ctfSignup is a player in the queue.
type ctfSignup struct {
	id  EntityID
	who *Player
	at  time.Time
}

// ctfState is the world's capture the flag.
type ctfState struct {
	on    bool
	set   CTFSettings
	lay   CTFLayout
	queue []ctfSignup
	// startAt is the countdown's end, zero while none runs
	startAt time.Time
	saidAt  time.Time // the countdown's last line
	m       *ctfMatch
	seq     uint32
	// strays are players in the arena's world with no match, by when they were first seen there
	strays map[EntityID]time.Time
	// clock is the last tick's time: what a logout or a line out of the tick goes by
	clock time.Time
}

// ctfNowLocked is the time a call out of the tick goes by: the last tick's, else the wall clock's.
func (w *World) ctfNowLocked() time.Time {
	if w.ctf.clock.IsZero() {
		return time.Now()
	}
	return w.ctf.clock
}

// ctfPlayer is a player's part in capture the flag.
type ctfPlayer struct {
	match   uint32 // the match it plays in, 0 none
	team    int    // its team's index, while match is set
	live    bool   // its match's banners are live: it fights the other team (pvpFoe)
	carries bool   // it carries the other team's banner (its name tagged: ShownName)
	hand    uint32 // the layout's banner item shown in its off hand (SerializePlayerUpdate's override item), 0 none
	home   bool   // its next respawn lands beside the herald (the match over while it lay dead)
	saidAt time.Time
	stuck  time.Time // /stuck again from here
	sentAt time.Time // sent to its camp (ctfToCampLocked): on its way there for ctfArriveGrace
	quiet  time.Time // touches nothing before this (ctfMayTouch): the server just put it somewhere
	// what it did, for the end's lines
	caps, returns, kills, deaths int
}

// inMatch reports p playing in match m.
func (p *Player) inMatch(m *ctfMatch) bool {
	return m != nil && p != nil && p.ctf.match != 0 && p.ctf.match == m.id
}

// CTFTeamOf is p's team in the arena's match, -1 for none.
func (w *World) CTFTeamOf(id EntityID) int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.Players[id]
	if p == nil || !p.inMatch(w.ctf.m) {
		return -1
	}
	return p.ctf.team
}

// SetCTF turns capture the flag on with these numbers and this arena, or off (on false). Players in a match when
// it goes off are sent back.
func (w *World) SetCTF(on bool, set CTFSettings, lay CTFLayout) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctf.on, w.ctf.set, w.ctf.lay = on, set, lay
	if w.ctf.strays == nil {
		w.ctf.strays = map[EntityID]time.Time{}
	}
	if !on {
		if w.ctf.m != nil {
			w.ctfEndLocked(-1, "the arena closed", w.ctfNowLocked())
		}
		w.ctf.queue = nil
		w.ctf.startAt = time.Time{}
	}
}

// CTFOn reports capture the flag on, and its arena's world.
func (w *World) CTFOn() (bool, uint8) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ctf.on, w.ctf.set.World
}

// ctfArenaLocked reports world the arena's.
func (w *World) ctfArenaLocked(world uint8) bool {
	return w.ctf.on && world == w.ctf.set.World
}

// The lines (ours). A PS2 never reads them: it cannot load the arena and is told so at the herald.
const (
	ctfLineClosed     = "The arena is closed on this server."
	ctfLineNoLoad     = "Your game cannot load the arena: it needs a client that builds the server's worlds (eqoa-rust-client-vanilla)."
	ctfLineLevel      = "You must be level %d to fight in the arena."
	ctfLineDead       = "You cannot sign up while you are dead."
	ctfLineDuel       = "You cannot sign up while you are in a duel."
	ctfLineJoined     = "You are signed up for capture the banner. %s"
	ctfLineLeft       = "You are no longer signed up."
	ctfLineInMatch    = "You are already fighting in the arena."
	ctfLineNotSigned  = "You are not signed up."
	ctfLineQueue      = "%d signed up; a match needs %d a side."
	ctfLineCountdown  = "A match begins in %d seconds."
	ctfLineWaitMatch  = "A match is on; you will be sent in as soon as a team has room."
	ctfLineNextMatch  = "A match is on; you will fight in the next one."
	ctfLineLeftMatch  = "You have left the match."
	ctfLineCancel     = "Too few are signed up now: the match will begin when %d a side are."
	ctfLineTeam       = "You fight for %s. Take %s from its stand and carry it to your own banner's stand, while your banner stands there."
	ctfLinePrep       = "The banners go live in %d seconds."
	ctfLineGo         = "The banners are live! First to %d captures wins; the match ends in %d minutes."
	ctfLineTaken      = "%s has taken %s!"
	ctfLineTakenUp    = "%s has picked up %s!"
	ctfLineDropped    = "%s has dropped %s."
	ctfLineReturned   = "%s has returned %s to its stand."
	ctfLineHomeAlone  = "%s has returned to its stand."
	ctfLineCaptured   = "%s has captured %s for %s! %s"
	ctfLineOwnAway    = "Your own banner must be on its stand for you to score."
	ctfLineWound      = "%s has carried %s too long and takes %d%% more from every hit."
	ctfLineMinutes    = "%d %s left in the match. %s"
	ctfLineWin        = "%s wins the match in %s, %s."
	ctfLineDraw       = "The match in %s ends in a draw, %s."
	ctfLineForfeit    = "%s has no one left: %s wins the match in %s, %s."
	ctfLineBack       = "You return from the arena."
	ctfLineYou        = "Your match: %d captures, %d returns, %d kills, %d deaths."
	ctfLineStray      = "There is no match here for you: you are sent back."
	ctfLineField      = "You have strayed from the field: back to your camp."
	ctfLineStuck      = "You are sent back to your camp."
	ctfLineStuckSoon  = "You can use /stuck again in %d seconds."
	ctfLineStuckFlag  = "You cannot use /stuck while you carry a banner."
	ctfLineSpeed      = "%s fills you with speed."
	ctfLineHeal       = "%s mends your wounds."
	ctfLineSignAll    = "A capture the banner match begins in %d seconds in %s. Talk to the PvP Events herald beside Dabrish to join."
	ctfLineWaveSoon   = "You rise with the next wave at your camp, in %d seconds."
	ctfLineTeamFull   = "Both teams are full; you will fight in the next match."
	ctfLineAlready    = "You are already signed up. %s"
	ctfLineNoArena    = "The arena cannot be reached now."
	ctfLineSameTeam   = "%s fights on your side."
	ctfLineNotLive    = "The banners are not live yet."
	ctfStatusNone     = "No match is on. %s"
	ctfStatusOn       = "A match is on in %s: %s, %d minutes left. %s"
	ctfStatusPrep     = "A match is about to begin in %s."
	ctfStatusOver     = "The match in %s is over, %s: everyone goes back in a moment."
)

// ctfBlue and ctfGold are the colours of a match's own lines: the game's own for an announcement.
var (
	ctfBlue = [3]byte{120, 200, 255}
	ctfGold = [3]byte{255, 215, 90}
)

// CTFJoin signs the session's player up, or says why not. It returns the lines to tell it.
func (w *World) CTFJoin(sess *session.Session, now time.Time) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	if !w.ctf.on {
		return []string{ctfLineClosed}
	}
	switch {
	case p.inMatch(w.ctf.m):
		return []string{ctfLineInMatch}
	case w.ctfQueuedLocked(p.ServerID) >= 0:
		return []string{fmt.Sprintf(ctfLineAlready, w.ctfQueueLineLocked())}
	case !w.loadsLocked(sess, w.ctf.set.World):
		return []string{ctfLineNoLoad}
	case p.Level < w.ctf.set.MinLevel:
		return []string{fmt.Sprintf(ctfLineLevel, w.ctf.set.MinLevel)}
	case p.IsDead() || p.respawnPending():
		return []string{ctfLineDead}
	case w.duelOnLocked(p.ServerID) != nil:
		return []string{ctfLineDuel}
	}
	w.ctf.queue = append(w.ctf.queue, ctfSignup{id: p.ServerID, who: p, at: now})
	log.Printf("CTF: %s signs up (%d in the queue)", p.Name, len(w.ctf.queue))
	return []string{fmt.Sprintf(ctfLineJoined, w.ctfQueueLineLocked())}
}

// CTFLeave takes the session's player off the queue, or out of its match (sent back beside the herald). It returns
// the line to tell it.
func (w *World) CTFLeave(sess *session.Session, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return ""
	}
	if i := w.ctfQueuedLocked(p.ServerID); i >= 0 {
		w.ctf.queue = append(w.ctf.queue[:i], w.ctf.queue[i+1:]...)
		log.Printf("CTF: %s leaves the queue (%d in it)", p.Name, len(w.ctf.queue))
		w.ctfCountdownLocked(now)
		return ctfLineLeft
	}
	if m := w.ctf.m; p.inMatch(m) {
		w.ctfDropOutLocked(p, "leaves", now)
		w.ctfSendBackLocked(p, 0)
		return ctfLineLeftMatch
	}
	return ctfLineNotSigned
}

// CTFStatus is how things stand, for the herald and /ctf.
func (w *World) CTFStatus(sess *session.Session, now time.Time) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.ctf.on {
		return []string{ctfLineClosed}
	}
	p := w.sessionPlayerLocked(sess)
	var out []string
	switch m := w.ctf.m; {
	case m == nil:
		out = append(out, fmt.Sprintf(ctfStatusNone, w.ctfQueueLineLocked()))
	case m.phase == ctfPrep:
		out = append(out, fmt.Sprintf(ctfStatusPrep, w.ctf.lay.Name))
	case m.phase == ctfOver:
		out = append(out, fmt.Sprintf(ctfStatusOver, w.ctf.lay.Name, w.ctfScoreLocked(m)))
	default:
		left := int(math.Ceil(m.ends.Sub(now).Minutes()))
		queue := w.ctfQueueLineLocked()
		if p := w.sessionPlayerLocked(sess); p.inMatch(m) {
			queue = ""
		}
		out = append(out, strings.TrimSpace(fmt.Sprintf(ctfStatusOn, w.ctf.lay.Name, w.ctfScoreLocked(m), max(left, 0), queue)))
	}
	if p != nil {
		switch {
		case p.inMatch(w.ctf.m):
			out = append(out, fmt.Sprintf("You fight for %s.", w.ctf.lay.Teams[p.ctf.team].Name))
		case w.ctfQueuedLocked(p.ServerID) >= 0:
			out = append(out, "You are signed up.")
		}
	}
	return out
}

// CTFQueued reports the session's player signed up.
func (w *World) CTFQueued(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && w.ctfQueuedLocked(p.ServerID) >= 0
}

// CTFInMatch reports the session's player fighting in the arena's match.
func (w *World) CTFInMatch(sess *session.Session) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	return p != nil && p.inMatch(w.ctf.m)
}

// ctfQueuedLocked is the player's place in the queue, -1 for none.
func (w *World) ctfQueuedLocked(id EntityID) int {
	for i, s := range w.ctf.queue {
		if s.id == id {
			return i
		}
	}
	return -1
}

// ctfQueueLineLocked says how many are signed up and what happens next.
func (w *World) ctfQueueLineLocked() string {
	switch {
	case w.ctf.m != nil && w.ctf.m.phase != ctfOver:
		return ctfLineWaitMatch
	case w.ctf.m != nil:
		return ctfLineNextMatch
	case !w.ctf.startAt.IsZero():
		return fmt.Sprintf(ctfLineCountdown, int(math.Ceil(w.ctf.startAt.Sub(w.ctfNowLocked()).Seconds())))
	}
	return fmt.Sprintf(ctfLineQueue, len(w.ctf.queue), w.ctf.set.MinPerTeam)
}

// ctfScoreLocked is the score as a line: "Ember 1, Frost 2".
func (w *World) ctfScoreLocked(m *ctfMatch) string {
	t := w.ctf.lay.Teams
	return fmt.Sprintf("%s %d, %s %d", t[0].Short, m.teams[0].score, t[1].Short, m.teams[1].score)
}

// ctfSystem is capture the flag's tick: the queue's countdown and start, the match's phases, its banners left
// alone, its waves, power-ups and wounds, its end and the way back, and the arena's strays. Every tick; must be
// called with the lock held.
func (w *World) ctfSystem(now time.Time) {
	if !w.ctf.on {
		return
	}
	w.ctf.clock = now
	w.ctfPruneQueueLocked()
	w.ctfStraysLocked(now)
	m := w.ctf.m
	if m == nil {
		w.ctfCountdownLocked(now)
		if !w.ctf.startAt.IsZero() && !now.Before(w.ctf.startAt) {
			w.ctfStartLocked(now)
		} else if !w.ctf.startAt.IsZero() {
			w.ctfCountdownLinesLocked(now)
		}
		return
	}
	w.ctfPruneTeamsLocked(m, now)
	switch m.phase {
	case ctfPrep:
		if !now.Before(m.opens) {
			w.ctfOpenLocked(m, now)
		}
	case ctfPlay:
		w.ctfBackfillLocked(m, now)
		w.ctfTouchAllLocked(m, now)
		w.ctfFollowLocked(m)
		w.ctfFlagsAloneLocked(m, now)
		w.ctfWoundsLocked(m, now)
		w.ctfPowerUpsLocked(m, now)
		w.ctfMinutesLocked(m, now)
		w.ctfForfeitLocked(m, now)
		if w.ctf.m == m && !now.Before(m.ends) {
			w.ctfTimeUpLocked(m, now)
		}
	case ctfOver:
		if !m.sentHome && !now.Before(m.overAt.Add(w.ctf.set.EndHold)) {
			w.ctfSendAllBackLocked(m)
		}
	}
}

// ctfPruneQueueLocked lets go of signed-up players who left the world (a new login is a new player: it signs up
// again).
func (w *World) ctfPruneQueueLocked() {
	keep := w.ctf.queue[:0]
	for _, s := range w.ctf.queue {
		if p := w.Players[s.id]; p != nil && p == s.who {
			keep = append(keep, s)
		}
	}
	for i := len(keep); i < len(w.ctf.queue); i++ {
		w.ctf.queue[i] = ctfSignup{}
	}
	w.ctf.queue = keep
}

// ctfReadyLocked is the queue's players that may go into the arena now, in their order.
func (w *World) ctfReadyLocked() []*Player {
	var out []*Player
	for _, s := range w.ctf.queue {
		p := w.Players[s.id]
		if p == nil || p != s.who || p.IsDead() || p.respawnPending() || w.duelOnLocked(p.ServerID) != nil {
			continue
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess == nil || !sess.CharacterInWorld() || sess.StateChannelsHeld() ||
			!w.loadsLocked(sess, w.ctf.set.World) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ctfCountdownLocked starts the countdown when enough are ready and none runs, and stops it when too few are.
func (w *World) ctfCountdownLocked(now time.Time) {
	if w.ctf.m != nil {
		w.ctf.startAt = time.Time{}
		return
	}
	enough := len(w.ctfReadyLocked()) >= 2*max(w.ctf.set.MinPerTeam, 1)
	switch {
	case enough && w.ctf.startAt.IsZero():
		w.ctf.startAt, w.ctf.saidAt = now.Add(w.ctf.set.Countdown), now
		secs := int(math.Round(w.ctf.set.Countdown.Seconds()))
		log.Printf("CTF: %d ready, the match begins in %d s", len(w.ctf.queue), secs)
		w.ctfTellQueueLocked(fmt.Sprintf(ctfLineCountdown, secs), ctfGold)
		w.ctfAnnounceLocked(fmt.Sprintf(ctfLineSignAll, secs, w.ctf.lay.Name), ctfGold, true)
	case !enough && !w.ctf.startAt.IsZero():
		w.ctf.startAt = time.Time{}
		log.Printf("CTF: the countdown stops: %d ready", len(w.ctfReadyLocked()))
		w.ctfTellQueueLocked(fmt.Sprintf(ctfLineCancel, w.ctf.set.MinPerTeam), ctfGold)
	}
}

// ctfCountdownLinesLocked tells the queue the countdown at 10 seconds.
func (w *World) ctfCountdownLinesLocked(now time.Time) {
	left := w.ctf.startAt.Sub(now)
	if left <= 10*time.Second && w.ctf.saidAt.Before(w.ctf.startAt.Add(-10*time.Second)) {
		w.ctf.saidAt = now
		w.ctfTellQueueLocked(fmt.Sprintf(ctfLineCountdown, int(math.Ceil(left.Seconds()))), ctfGold)
	}
}

// ctfTellQueueLocked tells everyone signed up a line.
func (w *World) ctfTellQueueLocked(line string, rgb [3]byte) {
	for _, s := range w.ctf.queue {
		if sess := w.findSessionByEntityIDLocked(s.id); sess != nil {
			w.sendColoredLocked(sess, line, rgb)
		}
	}
}

// ctfAnnounceLocked tells everyone in the world a line: all of them (everyone true), or those of the match.
func (w *World) ctfAnnounceLocked(line string, rgb [3]byte, everyone bool) {
	if w.SessionManager == nil {
		return
	}
	m := w.ctf.m
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.CharacterInWorld() || s.PendingTermination() {
			return true
		}
		if _, replaced := s.ReplacedFor(); replaced {
			return true
		}
		p := w.sessionPlayerLocked(s)
		if p == nil || (!everyone && !p.inMatch(m)) {
			return true
		}
		w.sendColoredLocked(s, line, rgb)
		return true
	})
}

// ctfTellMatchLocked tells the match's players a line.
func (w *World) ctfTellMatchLocked(m *ctfMatch, line string, rgb [3]byte) {
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			if sess := w.findSessionByEntityIDLocked(id); sess != nil {
				w.sendColoredLocked(sess, line, rgb)
			}
		}
	}
}

// ctfTellLocked tells one player a line.
func (w *World) ctfTellLocked(p *Player, line string, rgb [3]byte) {
	if p == nil {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendColoredLocked(sess, line, rgb)
	}
}

// ctfSplit puts players (in sign-up order) into two teams as even as they come: the highest level first, each to
// the team with fewer, of two as many the one with the lower sum of levels (ties to the first). It is the order
// the teams are sent in.
func ctfSplit(players []*Player) [2][]*Player {
	order := append([]*Player(nil), players...)
	sort.SliceStable(order, func(i, j int) bool { return order[i].Level > order[j].Level })
	var teams [2][]*Player
	var sum [2]int
	for _, p := range order {
		t := 0
		switch {
		case len(teams[1]) < len(teams[0]):
			t = 1
		case len(teams[1]) == len(teams[0]) && sum[1] < sum[0]:
			t = 1
		}
		teams[t] = append(teams[t], p)
		sum[t] += p.Level
	}
	return teams
}

// ctfStartLocked starts a match: the ready players split and sent to their camps, the banners on their stands,
// the power-ups out.
func (w *World) ctfStartLocked(now time.Time) {
	w.ctf.startAt = time.Time{}
	ready := w.ctfReadyLocked()
	set := w.ctf.set
	if len(ready) < 2*max(set.MinPerTeam, 1) {
		return
	}
	if len(ready) > 2*set.TeamSize {
		ready = ready[:2*set.TeamSize]
	}
	w.ctf.seq++
	m := &ctfMatch{id: w.ctf.seq, phase: ctfPrep, began: now, opens: now.Add(set.Prep), winner: -1}
	w.ctf.m = m
	teams := ctfSplit(ready)
	for t := range teams {
		for _, p := range teams[t] {
			w.ctfUnqueueLocked(p.ServerID)
			w.ctfEnterLocked(m, p, t, now)
		}
	}
	for t := range m.flags {
		m.flags[t] = &ctfFlag{team: t}
		w.ctfFlagHomeLocked(m.flags[t])
	}
	for _, pu := range w.ctf.lay.PowerUps {
		u := &ctfPowerUp{lay: pu}
		w.ctfPowerUpOutLocked(u)
		m.powerUps = append(m.powerUps, u)
	}
	log.Printf("CTF: match %d begins in %s: %s %v against %s %v; live in %v", m.id, w.ctf.lay.Name,
		w.ctf.lay.Teams[0].Short, w.ctfNamesLocked(m.teams[0].members), w.ctf.lay.Teams[1].Short, w.ctfNamesLocked(m.teams[1].members), set.Prep)
}

// ctfNamesLocked is the players' names, for the log.
func (w *World) ctfNamesLocked(ids []EntityID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if p := w.Players[id]; p != nil {
			out = append(out, p.Name)
		}
	}
	return out
}

// ctfUnqueueLocked takes a player off the queue.
func (w *World) ctfUnqueueLocked(id EntityID) {
	if i := w.ctfQueuedLocked(id); i >= 0 {
		w.ctf.queue = append(w.ctf.queue[:i], w.ctf.queue[i+1:]...)
	}
}

// ctfEnterLocked puts p on team t of m and sends it to its camp.
func (w *World) ctfEnterLocked(m *ctfMatch, p *Player, t int, now time.Time) {
	m.teams[t].members = append(m.teams[t].members, p.ServerID)
	m.teams[t].emptySince = time.Time{}
	p.ctf = ctfPlayer{match: m.id, team: t, live: m.phase == ctfPlay}
	w.breakInvisLocked(p, "the arena")
	lay := w.ctf.lay.Teams[t]
	other := w.ctf.lay.Teams[1-t]
	w.ctfTellLocked(p, fmt.Sprintf(ctfLineTeam, lay.Name, "the "+other.Flag), ctfBlue)
	switch m.phase {
	case ctfPrep:
		w.ctfTellLocked(p, fmt.Sprintf(ctfLinePrep, int(math.Ceil(m.opens.Sub(now).Seconds()))), ctfBlue)
	case ctfPlay:
		w.ctfTellLocked(p, w.ctfScoreLocked(m)+".", ctfBlue)
	}
	w.ctfToCampLocked(p, len(m.teams[t].members))
	log.Printf("CTF: %s joins %s in match %d", p.Name, lay.Short, m.id)
}

// ctfToCampLocked sends p to its team's camp, ring places round it (GateRingSpot).
func (w *World) ctfToCampLocked(p *Player, ring int) {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	if sess == nil {
		return
	}
	c := w.ctf.lay.Teams[p.ctf.team].Camp
	p.ctf.sentAt = w.ctfNowLocked()
	p.ctf.quiet = p.ctf.sentAt.Add(ctfQuietAfterMove)
	to := GateSpot{World: w.ctf.set.World, X: c.X, Y: c.Y, Z: c.Z, Place: w.ctf.lay.Teams[p.ctf.team].Name + "'s camp"}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Ring: ring % 9, Spell: w.ctf.lay.Name,
		Facing: c.Facing, HasFacing: true})
}

// ctfReturnSpotLocked is where the way back lands: ctfReturnOut in front of the herald, else in front of Dabrish.
func (w *World) ctfReturnSpotLocked() (GateSpot, bool) {
	for _, name := range []string{CTFHeraldName, ctfReturnBy} {
		if npc := w.npcByNameLocked(name); npc != nil && !npc.Dead {
			f := float64(npc.Facing)
			return GateSpot{World: npc.World, Y: npc.Y, Place: "beside " + npc.Name,
				X: npc.X + ctfReturnOut*float32(math.Sin(f)), Z: npc.Z + ctfReturnOut*float32(math.Cos(f))}, true
		}
	}
	return GateSpot{}, false
}

// ctfSendBackLocked sends p back from the arena beside the herald (ring its place round the spot), or, dead, marks
// its rise to land there.
func (w *World) ctfSendBackLocked(p *Player, ring int) {
	p.ctf.carries, p.ctf.hand = false, 0
	if p.IsDead() || p.respawnPending() {
		p.ctf.home = true
		return
	}
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	to, ok := w.ctfReturnSpotLocked()
	if sess == nil || !ok {
		log.Printf("CTF: %s cannot be sent back (no session or no herald)", p.Name)
		return
	}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: to, Ring: ring % 9, Spell: w.ctf.lay.Name})
	w.ctfTellLocked(p, ctfLineBack, ctfBlue)
}

// CTFRespawnSpot is where a dead player of the arena rises: its camp while it plays, beside the herald when its
// match ended while it lay dead (ok false: neither, the usual home).
func (w *World) CTFRespawnSpot(id EntityID) (spot GateSpot, facing float32, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.Players[id]
	if p == nil || !w.ctf.on {
		return GateSpot{}, 0, false
	}
	if p.ctf.home {
		p.ctf.home = false
		to, found := w.ctfReturnSpotLocked()
		return to, 0, found
	}
	if m := w.ctf.m; p.inMatch(m) {
		p.ctf.quiet = w.ctfNowLocked().Add(ctfQuietAfterMove)
		c := w.ctf.lay.Teams[p.ctf.team].Camp
		at := GateSpot{World: w.ctf.set.World, X: c.X, Y: c.Y, Z: c.Z, Place: w.ctf.lay.Teams[p.ctf.team].Name + "'s camp"}
		return at, c.Facing, true
	}
	return GateSpot{}, 0, false
}

// ctfRespawnDueLocked decides a dead player's rise when the arena has it: in a match, at the wave (the first
// WaveMinDead after its death); its match over, at once. handled false: the usual PlayerRespawnDelay.
func (w *World) ctfRespawnDueLocked(p *Player, now time.Time) (due, handled bool) {
	if !w.ctf.on {
		return false, false
	}
	if p.ctf.home {
		return true, true
	}
	m := w.ctf.m
	if !p.inMatch(m) {
		return false, false
	}
	if m.phase == ctfOver {
		p.ctf.home = true
		return true, true
	}
	return !now.Before(w.ctfWaveForLocked(m, p.DiedAt)), true
}

// ctfWaveForLocked is the wave a player dead since died rises with: the waves come every WaveEvery from the banners
// going live, and it rises with the first at least WaveMinDead after its death (one dead before the banners went
// live, at their going live).
func (w *World) ctfWaveForLocked(m *ctfMatch, died time.Time) time.Time {
	every := max(w.ctf.set.WaveEvery, time.Second)
	earliest := died.Add(w.ctf.set.WaveMinDead)
	if m.phase == ctfPrep || !earliest.After(m.opens) {
		return m.opens
	}
	k := (earliest.Sub(m.opens) + every - 1) / every
	return m.opens.Add(time.Duration(max(k, 1)) * every)
}

// ctfOpenLocked makes the banners live.
func (w *World) ctfOpenLocked(m *ctfMatch, now time.Time) {
	m.phase = ctfPlay
	m.opens = now
	m.ends = now.Add(w.ctf.set.TimeLimit)
	m.minsTold = int(math.Ceil(w.ctf.set.TimeLimit.Minutes()))
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			if p := w.Players[id]; p.inMatch(m) {
				p.ctf.live = true
			}
		}
	}
	log.Printf("CTF: match %d is live (%d against %d)", m.id, len(m.teams[0].members), len(m.teams[1].members))
	w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineGo, w.ctf.set.ScoreLimit, int(math.Round(w.ctf.set.TimeLimit.Minutes()))), ctfGold)
}

// ctfPruneTeamsLocked lets go of a match's players who are gone (a new login under the same id is not one of them:
// its player was another): their banner drops where they were.
func (w *World) ctfPruneTeamsLocked(m *ctfMatch, now time.Time) {
	for t := range m.teams {
		keep := m.teams[t].members[:0]
		for _, id := range m.teams[t].members {
			p := w.Players[id]
			if p == nil || !p.inMatch(m) {
				w.ctfFlagOfCarrierGoneLocked(m, id, now)
				continue
			}
			if p.World != w.ctf.set.World && !w.ctfOnTheWayLocked(p) {
				// gone from the arena by some other way (a gate, a GM's move): out of the match
				w.ctfFlagOfCarrierGoneLocked(m, id, now)
				log.Printf("CTF: %s left the arena (world %d): out of match %d", p.Name, p.World, m.id)
				p.ctf = ctfPlayer{}
				continue
			}
			keep = append(keep, id)
		}
		m.teams[t].members = keep
		if len(keep) == 0 && m.teams[t].emptySince.IsZero() {
			m.teams[t].emptySince = now
		}
	}
}

// ctfOnTheWayLocked reports a player of a match not yet in the arena: sent in and loading, or a travel queued.
func (w *World) ctfOnTheWayLocked(p *Player) bool {
	for _, t := range w.gates.due {
		if t.P == p {
			return true
		}
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil && sess.StateChannelsHeld() {
		return true // loading the arena
	}
	return !p.ctf.sentAt.IsZero() && w.ctfNowLocked().Sub(p.ctf.sentAt) < ctfArriveGrace
}

// ctfDropOutLocked takes p out of its match (it leaves, or logs out): its banner drops where it is, its team
// loses it, and the match is told.
func (w *World) ctfDropOutLocked(p *Player, why string, now time.Time) {
	m := w.ctf.m
	if !p.inMatch(m) {
		return
	}
	w.ctfFlagOfCarrierGoneLocked(m, p.ServerID, now)
	t := p.ctf.team
	members := m.teams[t].members[:0]
	for _, id := range m.teams[t].members {
		if id != p.ServerID {
			members = append(members, id)
		}
	}
	m.teams[t].members = members
	if len(members) == 0 {
		m.teams[t].emptySince = now
	}
	log.Printf("CTF: %s %s match %d", p.Name, why, m.id)
	p.ctf = ctfPlayer{}
}

// ctfLeavingLocked is p leaving the world (a logout): off the queue, out of its match, its banner dropped.
func (w *World) ctfLeavingLocked(p *Player) {
	w.ctfUnqueueLocked(p.ServerID)
	if p.inMatch(w.ctf.m) {
		w.ctfDropOutLocked(p, "logs out of", w.ctfNowLocked())
	}
	delete(w.ctf.strays, p.ServerID)
}

// ctfBackfillLocked sends signed-up players into a match on with room: each to the team with fewer (of two as
// many, the one with the lower score), while enough of the time is left.
func (w *World) ctfBackfillLocked(m *ctfMatch, now time.Time) {
	if len(w.ctf.queue) == 0 || m.ends.Sub(now) < ctfBackfillLeft {
		return
	}
	for _, p := range w.ctfReadyLocked() {
		t := 0
		a, b := len(m.teams[0].members), len(m.teams[1].members)
		switch {
		case b < a:
			t = 1
		case a == b && m.teams[1].score < m.teams[0].score:
			t = 1
		}
		if len(m.teams[t].members) >= w.ctf.set.TeamSize {
			return
		}
		w.ctfUnqueueLocked(p.ServerID)
		w.ctfEnterLocked(m, p, t, now)
	}
}

// ctfForfeitLocked ends a match a team has stood empty in for ctfForfeitAfter: the other wins, or nobody when
// both are empty.
func (w *World) ctfForfeitLocked(m *ctfMatch, now time.Time) {
	var empty [2]bool
	for t := range m.teams {
		empty[t] = len(m.teams[t].members) == 0 && !m.teams[t].emptySince.IsZero() && now.Sub(m.teams[t].emptySince) >= ctfForfeitAfter
	}
	switch {
	case empty[0] && empty[1]:
		w.ctfEndLocked(-1, "both teams left", now)
	case empty[0]:
		w.ctfEndLocked(1, "forfeit", now)
	case empty[1]:
		w.ctfEndLocked(0, "forfeit", now)
	}
}

// ctfMinutesLocked tells the match the minutes left at 10, 5 and 1.
func (w *World) ctfMinutesLocked(m *ctfMatch, now time.Time) {
	left := m.ends.Sub(now)
	for _, mins := range []int{10, 5, 1} {
		if m.minsTold > mins && left <= time.Duration(mins)*time.Minute && left > 0 {
			m.minsTold = mins
			unit := "minutes"
			if mins == 1 {
				unit = "minute"
			}
			w.ctfTellMatchLocked(m, fmt.Sprintf(ctfLineMinutes, mins, unit, w.ctfScoreLocked(m)+"."), ctfGold)
		}
	}
}

// ctfTimeUpLocked is the time limit: the higher score wins, a tie is a draw.
func (w *World) ctfTimeUpLocked(m *ctfMatch, now time.Time) {
	a, b := m.teams[0].score, m.teams[1].score
	switch {
	case a > b:
		w.ctfEndLocked(0, "time", now)
	case b > a:
		w.ctfEndLocked(1, "time", now)
	default:
		w.ctfEndLocked(-1, "time", now)
	}
}

// ctfEndLocked ends the match: winner the team that won (-1 a draw), told to its players and the server, the
// banners and power-ups taken in; EndHold later everyone goes back (ctfSendAllBackLocked).
func (w *World) ctfEndLocked(winner int, how string, now time.Time) {
	m := w.ctf.m
	if m == nil || m.phase == ctfOver {
		return
	}
	m.phase, m.overAt, m.winner, m.how = ctfOver, now, winner, how
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			if p := w.Players[id]; p.inMatch(m) {
				p.ctf.live = false // nobody's foe while it waits to go back
			}
		}
	}
	for _, f := range m.flags {
		if f == nil {
			continue
		}
		w.ctfLetGoLocked(f)
		w.ctfFlagNPCGoneLocked(f)
	}
	for _, u := range m.powerUps {
		w.ctfPowerUpGoneLocked(u)
	}
	lay := w.ctf.lay
	score := w.ctfScoreLocked(m)
	var line string
	switch {
	case how == "forfeit":
		line = fmt.Sprintf(ctfLineForfeit, upFirstWord(lay.Teams[1-winner].Name), lay.Teams[winner].Name, lay.Name, score)
	case winner >= 0:
		line = fmt.Sprintf(ctfLineWin, upFirstWord(lay.Teams[winner].Name), lay.Name, score)
	default:
		line = fmt.Sprintf(ctfLineDraw, lay.Name, score)
	}
	log.Printf("CTF: match %d ends (%s): %s", m.id, how, line)
	w.ctfAnnounceLocked(line, ctfGold, true)
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			if p := w.Players[id]; p != nil {
				w.ctfTellLocked(p, fmt.Sprintf(ctfLineYou, p.ctf.caps, p.ctf.returns, p.ctf.kills, p.ctf.deaths), ctfBlue)
			}
		}
	}
}

// ctfSendAllBackLocked sends every player of the ended match back beside the herald, and lets the match go.
func (w *World) ctfSendAllBackLocked(m *ctfMatch) {
	m.sentHome = true
	ring := 0
	for t := range m.teams {
		for _, id := range m.teams[t].members {
			p := w.Players[id]
			if p == nil || !p.inMatch(m) {
				continue
			}
			ring++
			w.ctfSendBackLocked(p, ring)
			home := p.ctf.home
			p.ctf = ctfPlayer{home: home}
		}
	}
	log.Printf("CTF: match %d: everyone sent back", m.id)
	w.ctf.m = nil
	w.ctfCountdownLocked(w.ctfNowLocked())
}

// ctfStraysLocked sends beside the herald, after ctfStrayGrace, a player standing in the arena's world in no match
// (a keep restart's, a GM's move of another): a GM himself may stay.
func (w *World) ctfStraysLocked(now time.Time) {
	if w.ctf.strays == nil {
		w.ctf.strays = map[EntityID]time.Time{}
	}
	m := w.ctf.m
	for id, p := range w.Players {
		if p.World != w.ctf.set.World || p.GM || p.inMatch(m) {
			delete(w.ctf.strays, id)
			continue
		}
		sess := w.findSessionByEntityIDLocked(id)
		if sess == nil || !sess.CharacterInWorld() || sess.StateChannelsHeld() {
			continue
		}
		first, seen := w.ctf.strays[id]
		if !seen {
			w.ctf.strays[id] = now
			continue
		}
		if now.Sub(first) < ctfStrayGrace || w.ctfOnTheWayLocked(p) {
			continue
		}
		delete(w.ctf.strays, id)
		log.Printf("CTF: %s stands in the arena with no match: sent back", p.Name)
		w.ctfTellLocked(p, ctfLineStray, ctfBlue)
		w.ctfSendBackLocked(p, 0)
	}
}

// CTFStuck is /stuck for a player of a match: back to its camp (none while it carries a banner, at most every
// ctfStuckEvery). handled false: not a match's player (the usual /stuck).
func (w *World) CTFStuck(sess *session.Session, now time.Time) (line string, handled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !p.inMatch(w.ctf.m) {
		return "", false
	}
	switch {
	case p.IsDead() || p.respawnPending():
		return stuckLineDead, true
	case p.ctf.carries:
		return ctfLineStuckFlag, true
	case now.Before(p.ctf.stuck):
		return fmt.Sprintf(ctfLineStuckSoon, int(math.Ceil(p.ctf.stuck.Sub(now).Seconds()))), true
	}
	p.ctf.stuck = now.Add(ctfStuckEvery)
	w.ctfToCampLocked(p, 1)
	log.Printf("CTF: %s uses /stuck: back to its camp", p.Name)
	return ctfLineStuck, true
}

// upFirstWord is s with its first letter a capital ("the Ember Band" to "The Ember Band").
func upFirstWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ctfRefusal is why p may not attack t in the arena, for the line; "" when nothing need be said.
func ctfRefusal(p, t *Player) string {
	switch {
	case p.IsDead() || t.IsDead():
		return ""
	case p.ctf.match == 0 || t.ctf.match == 0 || p.ctf.match != t.ctf.match:
		return ""
	case p.ctf.team == t.ctf.team:
		return fmt.Sprintf(ctfLineSameTeam, t.Name)
	case !p.ctf.live:
		return ctfLineNotLive
	}
	return ""
}

// ctfViewLocked is a match's player as sess sees it (visibility.go, after pvpViewLocked): to a player of the
// match, the other team's name green (channel byte 157 bit 0: CLIENT's ApplyLook colour 2) and its own team's
// blue; the other team's carrier of its banner gold (byte 142 0: ApplyLook's colour 0, and to TargetInRange as
// swingable as a foe's 4, so it is set only where pvpFoe made it a foe). Anyone else sees players as ever. Must
// be called with the lock held.
func (w *World) ctfViewLocked(sess *session.Session, p *Player, buf []byte) {
	m := w.ctf.m
	if !p.inMatch(m) || len(buf) <= 157 {
		return
	}
	viewer := w.sessionPlayerLocked(sess)
	if !viewer.inMatch(m) {
		return
	}
	if viewer.ctf.team == p.ctf.team {
		buf[157] &^= 1
		return
	}
	buf[157] |= 1
	if p.ctf.carries && buf[142] == pvpFoeByte {
		buf[142] = 0
	}
}

// CTFArenaName is the arena's name ("" while it is closed).
func (w *World) CTFArenaName() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.ctf.on {
		return ""
	}
	return w.ctf.lay.Name
}

// CTFReturnSpot is where the way back from the arena lands (in front of the herald, else Dabrish), and the
// arena's world: a login saved in the arena starts there (handlers' loginSpawn), as matches are not kept.
func (w *World) CTFReturnSpot() (spot GateSpot, arena uint8, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.ctf.on {
		return GateSpot{}, 0, false
	}
	spot, ok = w.ctfReturnSpotLocked()
	return spot, w.ctf.set.World, ok
}
