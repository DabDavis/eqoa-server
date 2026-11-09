package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// summonportal.go — the Summoning Portal (the user, 2026-11-07: "lets add
// heirloom item like the genie lamp, fang of SoW, it is summoning portal
// infinite charges to cast on anyone, it spawns portal NPC, players talks to
// it, just type name of player to summon(doesnt have to be in group so
// different from Call of the Hero)").
//
// The item is a record named SummoningPortalName, kept as the lamp and the fang
// are (a character with none is given one at its login, giveSummoningPortal;
// the record and its startitems rows are a data step, like theirs). It is
// CLIENT's gem type (procanim 7: VIInfoText::IsUsable, the item menu's Use and
// the HUD's gem quick slot, as the fang: gearsets.go); a quick slot's press is
// the same 0x003E as the menu's. Its Use is never spent and opens a portal in
// front of its holder (entity/summonportal.go), and says so in a chat line: no
// box, since the item menu is up when it is used from there.
//
// Talking to the portal (anyone may) asks who to call, in a box under mark
// portalBoxMark. CLIENT has no text field a server can open, and its answer box
// holds the keys while it is up (VIWndResponse: Enter picks the answer under the
// cursor), so the box's answer closes it and the name is then typed on the chat
// line: the player's next plain line within PortalNameWait is the name (TakeName,
// in all three of a typed line's ways to the server: 0x000B in say, group and
// whisper, 0x0C01 in shout, 0x0578 in guild), handled here and shown to no one.
// A line beginning / or # is a command and passes, the name still awaited, and so
// is a line right after a 0x000E (ModeChanged: /g, /s, /gu and /say come as the
// mode, the bare text, the mode back; the client strips the command, so the text
// alone looks typed). The player closing the box, walking away from the portal
// or the minute running out ends the wait, and the next line is chat.
//
// The name is matched without case among the players in the world, in any world
// and any group. A refusal is a red line to the caller; a call that goes out
// counts against PortalCallEvery. Then, while PortalAsksFirst, the called player
// is asked in a box; a yes sends it through the portal (World.PortalBring: the
// travel a Call of the Hero uses, handlers' travelPlayer, the world loaded when
// it changes), to where the portal's owner stood when it opened. The caller is
// told each answer.
//
// A 0x0034 that comes while the game window is not on its plain screen is
// declined by CLIENT itself (Msg0034: 255 within a round trip, in a menu, say):
// that is no answer. The name's box leaves the wait as it is and says the same in
// a line; a call's box says who calls in a line once and is asked again every
// portalAskAgain until the call lapses, so the box shows once the menu is shut.
// (Vanilla keeps one such box until its menu closes, sending no 255.)
//
// Measured: CLIENT's boxes and chat routes, read off the decomp and vanilla (the
// genie's note, claude-notes/genie-lamp-windows-20261106.md, has the boxes).
// Ours, chosen: every word, every time below, the accept box (PortalAsksFirst,
// next), and one call at a time on a target (a second caller is told to wait, so
// a player never holds two of these boxes).

// PortalAsksFirst is the one rule left for the user to turn: true, the called
// player is asked and may say no; false, it comes at once. Without the asking
// anyone could pull anyone anywhere at will, a low-level player into a raid
// boss's lair for one. Change this one line to summon straight away.
const PortalAsksFirst = true

// SummoningPortalName is the item's record's name, as the NPC is named.
const SummoningPortalName = entity.PortalName

// portalBoxMark marks a portal's question's id (beside the quests' 0x51, the
// triggers' 0x52, the duels' 0x53, the rehabilitators' 0x54, the advisors' 0x55,
// the dyers' 0x56, the coachmen's ledgers' 0x57, a genie's 0x58 and a board's
// 0x59). portalCallBit tells a call's box from the name's question.
const (
	portalBoxMark = 0x5A000000
	portalCallBit = 0x00800000
)

// The portal's times (ours).
const (
	// PortalNameWait is how long a player's next line is awaited as the name
	PortalNameWait = time.Minute
	// PortalCallWait is how long a called player has to answer
	PortalCallWait = time.Minute
	// PortalCallEvery is how soon a caller's next call may go out
	PortalCallEvery = 5 * time.Second
	// portalAskAgain is how soon a box CLIENT declined at once is sent again
	portalAskAgain = 3 * time.Second
	// portalDeclinedWithin is how soon a closed box counts as one CLIENT
	// declined itself rather than one the player closed (the genie's)
	portalDeclinedWithin = genieDeclinedWithin
	// portalOneLine is how soon after a 0x000E a line is a /command's own:
	// the client sends them together, and no hand changes the mode in a menu
	// and types a line that fast
	portalOneLine = time.Second
	// portalSweepEvery is how often the waits and the calls are looked at
	portalSweepEvery = time.Second
)

// The portal's words (ours).
const (
	portalOpened = "A summoning portal opens before you. Anyone may talk to it and call a player here, in any world."

	portalQuestion = "Who do you call? Choose below, then type the player's name in chat and press Enter."
	portalWillType = "I will type the name"
	portalListens  = "The portal listens: type the player's name and press Enter. Only the portal will hear it."
	portalDeaf     = "The portal has stopped listening. Talk to it again."

	portalNoSuch    = "No player named %q is online."
	portalSelf      = "You cannot call yourself."
	portalDead      = "%s is dead and cannot answer a call."
	portalNoWorld   = "%s's game cannot load this world."
	portalBusy      = "%s is already answering a call. Try again in a moment."
	portalGathering = "The portal is gathering itself. Try again in a few seconds."
	portalCalling   = "You call %s through the portal."
	portalNoCome    = "%s cannot come."

	portalAsk     = "%s calls you through a summoning portal. Go?"
	portalAskMenu = "%s calls you through a summoning portal. Close your menu to answer."
	portalPassed  = "That call has passed."
	portalYouDead = "You cannot go while you are dead."
	portalThrough = "You step through the summoning portal."

	portalAnswered  = "%s answers your call."
	portalDeclined  = "%s declines your call."
	portalNoAnswer  = "%s does not answer your call."
	portalLeft      = "%s left before answering your call."
	portalLapsed    = "%s's call has passed."
	portalCallerOff = "%s's call has ended."
	portalClosed    = "The summoning portal has closed."
)

var portalAnswers = []string{"Yes", "No"}

// IsPortalBox reports a box a portal asked.
func IsPortalBox(id uint32) bool { return id&0xFF000000 == portalBoxMark }

// IsSummoningPortal is whether an item record is the Summoning Portal: worn
// nowhere and so named.
func IsSummoningPortal(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), SummoningPortalName)
}

// portalWait is a player whose next line is awaited as the name.
type portalWait struct {
	box    uint32
	portal entity.EntityID
	asked  time.Time // when the box went out
	until  time.Time
	modeAt time.Time // the last 0x000E during the wait
}

// portalCall is a call out: its box on the target's screen.
type portalCall struct {
	box        uint32
	portal     entity.EntityID
	caller     entity.EntityID
	callerName string
	target     entity.EntityID
	targetName string
	asked      time.Time // when the box last went out
	until      time.Time
	again      time.Time // when a box CLIENT declined goes out again; zero: none
	told       bool      // a declined box's line has been said
}

// Portal runs the Summoning Portals' talk: the waits for a name and the calls.
type Portal struct {
	gw       *entity.World
	now      func() time.Time
	askFirst bool // PortalAsksFirst (a test's own to turn)

	mu     sync.Mutex
	seq    uint32
	waits  map[entity.EntityID]*portalWait // by the player typing
	calls  map[entity.EntityID]*portalCall // by the player called
	called map[entity.EntityID]time.Time   // when each caller last called
}

// NewPortal makes the portals' talk over the world. Start begins its clock.
func NewPortal(gw *entity.World) *Portal {
	return &Portal{gw: gw, now: time.Now, askFirst: PortalAsksFirst,
		waits: map[entity.EntityID]*portalWait{}, calls: map[entity.EntityID]*portalCall{}, called: map[entity.EntityID]time.Time{}}
}

// Start begins the sweep over the waits and the calls.
func (g *Portal) Start() {
	go func() {
		t := time.NewTicker(portalSweepEvery)
		defer t.Stop()
		for range t.C {
			g.sweep()
		}
	}()
}

// nextBox is a new box id of the kind (0 or portalCallBit). Must be called with
// the mutex held.
func (g *Portal) nextBox(kind uint32) uint32 {
	g.seq++
	return portalBoxMark | kind | (g.seq & (portalCallBit - 1))
}

// portalTell is a white line to a session, if there is one.
func portalTell(sess *session.Session, line string) {
	if sess != nil {
		SendClientMessage(sess, line)
	}
}

// portalRefuse is a red line to a session, if there is one.
func portalRefuse(sess *session.Session, line string) {
	if sess != nil {
		SendErrorMessage(sess, line)
	}
}

// Open is the item used: the player's portal opens. The dead are given nothing
// and told nothing, as the fang's.
func (g *Portal) Open(sess *session.Session, p *entity.Player) {
	if g == nil || g.gw == nil || p == nil {
		return
	}
	if _, err := g.gw.OpenPortal(sess, g.now()); err != nil {
		return
	}
	SendClientMessage(sess, portalOpened)
}

// Talk is a portal talked to: anyone is asked who to call, and its next line
// waited for. It reports whether the NPC was a portal.
func (g *Portal) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if g == nil || g.gw == nil || p == nil || npc == nil || npc.PortalOf == 0 {
		return false
	}
	forgetDialogs(p)
	now := g.now()
	g.mu.Lock()
	a := g.waits[p.ServerID]
	if a == nil || a.portal != npc.ServerID {
		// a second talk to the same portal keeps its box's id: CLIENT
		// declines the second box while the first is up, which it answers
		a = &portalWait{box: g.nextBox(0), portal: npc.ServerID}
		g.waits[p.ServerID] = a
	}
	a.asked, a.until = now, now.Add(PortalNameWait)
	box := a.box
	g.mu.Unlock()
	SendQuestPopupMulti(sess, box, portalQuestion, []string{portalWillType})
	return true
}

// ModeChanged is a 0x000E: within portalOneLine the next line is a /command's.
func (g *Portal) ModeChanged(p *entity.Player) {
	if g == nil || p == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if a := g.waits[p.ServerID]; a != nil {
		a.modeAt = g.now()
	}
}

// TakeName is every typed line's first question: a line awaited as a name is
// taken and the call made, and it reports true; the caller shows it to no one.
// Any other line, a /command or a #command among them, is the chat's.
func (g *Portal) TakeName(sess *session.Session, p *entity.Player, line string) bool {
	if g == nil || g.gw == nil || p == nil {
		return false
	}
	name := strings.TrimSpace(line)
	if name == "" || name[0] == '/' || name[0] == '#' {
		return false
	}
	now := g.now()
	g.mu.Lock()
	a := g.waits[p.ServerID]
	if a == nil || (!a.modeAt.IsZero() && now.Sub(a.modeAt) < portalOneLine) {
		g.mu.Unlock()
		return false
	}
	delete(g.waits, p.ServerID)
	portal, until := a.portal, a.until
	g.mu.Unlock()
	if now.After(until) || !g.gw.PortalHears(portal, p.ServerID) {
		return false // walked away, or too late: it talks to the room
	}
	g.call(sess, p, portal, name, now)
	return true
}

// call is the name taken: the refusals, then the call.
func (g *Portal) call(sess *session.Session, caller *entity.Player, portal entity.EntityID, name string, now time.Time) {
	t := g.gw.PlayerByName(name)
	var ts *session.Session
	if t != nil {
		ts = g.inWorld(t.ServerID)
	}
	if ts == nil {
		portalRefuse(sess, fmt.Sprintf(portalNoSuch, name))
		return
	}
	if t.ServerID == caller.ServerID {
		portalRefuse(sess, portalSelf)
		return
	}
	if snap, ok := g.gw.PlayerSnapshot(t.ServerID); !ok || snap.IsDead() {
		portalRefuse(sess, fmt.Sprintf(portalDead, t.Name))
		return
	}
	spot, ok := g.gw.PortalSpot(portal)
	if !ok {
		portalRefuse(sess, portalClosed)
		return
	}
	if !CanLoadWorld(ts, spot.World) {
		portalRefuse(sess, fmt.Sprintf(portalNoWorld, t.Name))
		return
	}
	c := &portalCall{portal: portal, caller: caller.ServerID, callerName: caller.Name, target: t.ServerID, targetName: t.Name,
		asked: now, until: now.Add(PortalCallWait)}
	g.mu.Lock()
	if last, ok := g.called[caller.ServerID]; ok && now.Sub(last) < PortalCallEvery {
		g.mu.Unlock()
		portalRefuse(sess, portalGathering)
		return
	}
	if g.calls[t.ServerID] != nil {
		g.mu.Unlock()
		portalRefuse(sess, fmt.Sprintf(portalBusy, t.Name))
		return
	}
	g.called[caller.ServerID] = now
	c.box = g.nextBox(portalCallBit)
	if g.askFirst {
		g.calls[t.ServerID] = c
	}
	g.mu.Unlock()
	log.Printf("Portal: %s calls %s", caller.Name, t.Name)
	SendClientMessage(sess, fmt.Sprintf(portalCalling, t.Name))
	if !g.askFirst {
		g.come(sess, ts, c)
		return
	}
	SendQuestPopupMulti(ts, c.box, fmt.Sprintf(portalAsk, c.callerName), portalAnswers)
}

// come is a call answered yes: the target goes through the portal, or both are
// told why not.
func (g *Portal) come(cs, ts *session.Session, c *portalCall) {
	spot, ok := g.gw.PortalSpot(c.portal)
	if !ok {
		portalRefuse(ts, portalClosed)
		portalRefuse(cs, portalClosed)
		return
	}
	// the travel keeps a player from such a place in silence: its owner fell
	// through the ground, say, when it opened
	if _, err := ValidateTeleport(spot.X, spot.Y, spot.Z, spot.World); err != nil {
		log.Printf("Portal: %s's call to %s: no place to land (%v)", c.callerName, c.targetName, err)
		portalRefuse(ts, portalClosed)
		portalRefuse(cs, fmt.Sprintf(portalNoCome, c.targetName))
		return
	}
	if !g.gw.PortalBring(c.portal, c.target, ts) {
		portalRefuse(ts, portalYouDead)
		portalRefuse(cs, fmt.Sprintf(portalNoCome, c.targetName))
		return
	}
	portalTell(ts, portalThrough)
	portalTell(cs, fmt.Sprintf(portalAnswered, c.targetName))
}

// Answer is HandleDialogueOption's question for a portal's box. It reports
// whether the box was a portal's (an old one does nothing but say so).
func (g *Portal) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsPortalBox(box) {
		return false
	}
	if g == nil || g.gw == nil || p == nil {
		return true
	}
	if box&portalCallBit != 0 {
		g.answerCall(sess, p, box, option)
	} else {
		g.answerName(sess, p, box, option)
	}
	return true
}

// answerName is the name's box answered: its answer, or CLIENT's declining it
// at once, leave the wait on; the player closing it ends it.
func (g *Portal) answerName(sess *session.Session, p *entity.Player, box uint32, option byte) {
	now := g.now()
	g.mu.Lock()
	a := g.waits[p.ServerID]
	if a == nil || a.box != box {
		g.mu.Unlock()
		if a == nil && option != 255 {
			portalTell(sess, portalDeaf)
		}
		return
	}
	if option == 255 && now.Sub(a.asked) >= portalDeclinedWithin {
		delete(g.waits, p.ServerID)
		g.mu.Unlock()
		return
	}
	a.until = now.Add(PortalNameWait)
	g.mu.Unlock()
	portalTell(sess, portalListens)
}

// answerCall is a call's box answered: yes, no, or the box closed, which is a
// no, unless CLIENT closed it itself (a menu was up), which is no answer.
func (g *Portal) answerCall(sess *session.Session, p *entity.Player, box uint32, option byte) {
	now := g.now()
	g.mu.Lock()
	c := g.calls[p.ServerID]
	if c == nil || c.box != box {
		g.mu.Unlock()
		if option == 0 {
			portalTell(sess, portalPassed)
		}
		return
	}
	if option == 255 && now.Sub(c.asked) < portalDeclinedWithin {
		c.again = now.Add(portalAskAgain)
		first := !c.told
		c.told = true
		name := c.callerName
		g.mu.Unlock()
		if first {
			portalTell(sess, fmt.Sprintf(portalAskMenu, name))
		}
		return
	}
	delete(g.calls, p.ServerID)
	g.mu.Unlock()
	cs := g.inWorld(c.caller)
	if option == 0 {
		log.Printf("Portal: %s answers %s's call", c.targetName, c.callerName)
		g.come(cs, sess, c)
		return
	}
	log.Printf("Portal: %s declines %s's call", c.targetName, c.callerName)
	portalTell(cs, fmt.Sprintf(portalDeclined, c.targetName))
}

// portalLine is a line the sweep says once its mutex is let go.
type portalLine struct {
	sess *session.Session
	line string
}

// sweep ends the waits and the calls whose time is up, or whose portal or people
// are gone, and sends again the boxes CLIENT declined. Every portalSweepEvery.
// The mutex is taken before the World's lock here, and never the other way.
func (g *Portal) sweep() {
	if g == nil || g.gw == nil {
		return
	}
	now := g.now()
	var lines []portalLine
	type askAgain struct {
		sess   *session.Session
		box    uint32
		caller string
	}
	var again []askAgain
	g.mu.Lock()
	for id, at := range g.called {
		if now.Sub(at) >= PortalCallEvery {
			delete(g.called, id)
		}
	}
	for id, a := range g.waits {
		if now.After(a.until) || !g.gw.PortalHears(a.portal, id) {
			delete(g.waits, id)
			lines = append(lines, portalLine{g.inWorld(id), portalDeaf})
		}
	}
	for id, c := range g.calls {
		ts, cs := g.inWorld(id), g.inWorld(c.caller)
		_, stands := g.gw.PortalSpot(c.portal)
		switch {
		case ts == nil:
			lines = append(lines, portalLine{cs, fmt.Sprintf(portalLeft, c.targetName)})
		case cs == nil:
			lines = append(lines, portalLine{ts, fmt.Sprintf(portalCallerOff, c.callerName)})
		case !stands:
			lines = append(lines, portalLine{ts, portalClosed}, portalLine{cs, portalClosed})
		case now.After(c.until):
			lines = append(lines, portalLine{ts, fmt.Sprintf(portalLapsed, c.callerName)},
				portalLine{cs, fmt.Sprintf(portalNoAnswer, c.targetName)})
		default:
			if !c.again.IsZero() && !now.Before(c.again) {
				c.again, c.asked = time.Time{}, now
				again = append(again, askAgain{ts, c.box, c.callerName})
			}
			continue
		}
		delete(g.calls, id)
	}
	g.mu.Unlock()
	for _, l := range lines {
		portalTell(l.sess, l.line)
	}
	for _, a := range again {
		SendQuestPopupMulti(a.sess, a.box, fmt.Sprintf(portalAsk, a.caller), portalAnswers)
	}
}

// inWorld is the player's session while it is in the world, else nil.
func (g *Portal) inWorld(id entity.EntityID) *session.Session {
	if _, ok := g.gw.PlayerSnapshot(id); !ok {
		return nil
	}
	return g.gw.FindSessionByEntityID(id)
}

// giveSummoningPortal gives a character with none one at its login, as
// giveGenieLamp gives the lamp.
func (h *CharacterHandler) giveSummoningPortal(serverID uint32, room listSizes) {
	if h.store == nil || h.inventoryStore == nil {
		return
	}
	h.giveKeptItem(serverID, keptItemID(h.store.DB(), SummoningPortalName, "Portal"), "Portal", "summoning portal", room)
}
