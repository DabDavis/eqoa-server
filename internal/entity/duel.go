package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// Duels (the user, 2026-10-29: "lets add dueling, the corsten had it in
// memory i believe", then "dueling damage is reduce by half vs players").
//
// SOE's live build (eqoa-live-1500, resident in the retail save states) had
// them: a duel part in VIClient (+0x50DE8), the VIWndDuel window and two
// opcodes of their own, 0x1501 up and 0x1502 down (duelwire.go,
// claude-notes/duel-20261029.md). The Frontiers disc every client of ours
// follows has none of it, and its TargetInRange refuses a player against a
// player or a pet, so a duel reaches a disc client the way Sandstorm's must
// (claude-notes/f3-pass-20261026c.md): its opponent shown to it as a
// creature (duelViewLocked). A client that took the duel offer (0x7E09:
// vanilla with the live window ported) is told the live way instead and sees
// players as players, as the live client did (its own TargetInRange asks its
// duel part).
//
// One duel at a time a player: asked (an invitation for DuelInviteTime) or
// on. Its two fight with melee, harmful spells and their pets, every hit at
// DuelDamageShare; nobody else can hurt them through it (a third player's
// harm is refused as before), and heals and buffs land as ever. Nothing dies
// of a duel: a hit the loser cannot take leaves it DuelLoserHP and ends the
// duel (no death, no XP, no debt, no faction, no loot), as does surrender,
// leaving the arena (DuelArenaRadius round where it began, or another world),
// a logout and DuelMaxTime (a draw). At the end both are players to each
// other again, their duel's spells come off each other, and those within
// DuelHeardWithin of it hear who won.

// The duel's numbers, one place.
const (
	// DuelDamageShare is what a duel's hits land at: the user's "reduce by
	// half vs players", for a duelist's or a pet's, melee or spell.
	DuelDamageShare = 0.5
	// DuelInviteTime is how long an invitation stands: the live client's
	// invitation record starts its timer at 20000 ms (0x1502 sub 2's handler,
	// live 0x012E4BA4) and its window counts the last ten seconds down.
	DuelInviteTime = 20 * time.Second
	// DuelStartRange is how near the two must stand to challenge and to begin
	// (ours: the live client checks no distance).
	DuelStartRange float32 = 30
	// DuelArenaRadius is how far from where the duel began either may go;
	// past it, or in another world, the duel is forfeit (ours; the live
	// client points its compass at the duel's centre, 0x1502 sub 8).
	DuelArenaRadius float32 = 60
	// DuelLoserHP is what a hit the loser could not take leaves it.
	DuelLoserHP = 1
	// DuelMaxTime ends a duel that long with no winner.
	DuelMaxTime = 10 * time.Minute
	// DuelAskAgain is how soon a player may challenge again (a box at a time).
	DuelAskAgain = 5 * time.Second
	// DuelHeardWithin is who hears a duel's end besides its two: the players
	// within it of the duel's centre (chat's say radius).
	DuelHeardWithin float32 = 200
	// duelSnare is a duel opponent's slow on the run speed (ours: no slow on
	// a player was ever recorded).
	duelSnare float32 = 0.5
	// DuelWagerTime is how long the live wager window may stay open before
	// the duel is called off (ours).
	DuelWagerTime = 2 * time.Minute
	// DuelMaxWager is the most a side may stake: the live deposit window's
	// eight digits (its cursor at 8, VIWndDeposit's m_digit[9]).
	DuelMaxWager = 99999999
)

// The live words (eqoa-live-1500's WORDS.TXT rows, the disc has none of them).
const (
	duelWordExpired     = "The duel invitation has expired."        // 1859
	duelWordDeclinedYou = "Your duel invitation has been declined." // 1860
	duelWordDeclinedBy  = "%s declined your duel invitation."       // 1861
	duelWordSelf        = "You cannot duel yourself."               // 1862
	duelWordInvalid     = "Invalid target."                         // 1863
	duelWordChallenges  = "%s challenges you to a duel."            // 1866
	duelWordCancel      = "Cancel duel?"                            // 1872
	duelWordInDuel      = "%s is in a duel."                        // 1883
	duelWordAccept      = "Accept"                                  // 1779
	duelWordDecline     = "Decline"                                 // 157
)

// The live client's colours for its duel lines (0x012D29F8's VIColor32F):
// orange for the duel's words, red for a refusal.
var (
	duelOrange = [3]byte{255, 163, 71} // (1.0, 0.64, 0.28)
	duelRed    = [3]byte{255, 64, 64}  // (1.0, 0.25, 0.25)
)

// duelPhase is where a duel stands.
type duelPhase byte

const (
	duelAsked duelPhase = iota + 1 // an invitation, waiting for its answer
	duelWager                      // the live wager window, both sides staking (both have it)
	duelOn                         // the fight
)

// Duel is one duel between two players.
type Duel struct {
	ID      uint32     // the live client's duel and invitation id (never 0, never above duelIDMask)
	A, B    EntityID   // the challenger and the challenged
	who     [2]*Player // the two as they were asked: a new login under the same id is not them
	Phase   duelPhase
	Expires time.Time // an invitation's end
	Began   time.Time // the fight's start
	World   uint8
	// Stake is the tunar each side stakes, [0] the challenger's; Wager the
	// challenge's own (both stake it: a client without the live window sets
	// its wager in the challenge); held: the stakes are out of the purses,
	// paid to the winner at the end (duelpurse.go)
	Stake    [2]int
	Wager    int
	held     bool
	accept   [2]bool   // the wager window's accepts
	until    time.Time // the wager window's end
	windowed bool      // it went through the wager window
	X, Y, Z  float32   // the centre: halfway between the two as the fight began

	// sent is the opponent's and its pet's object ids each side's live
	// window was last told (0x1502 sub 7), [0] the challenger's
	sent [2][2]uint32
	// pets: a side's pet fights in the duel; petOut: its pet was left at a
	// hit point and is out of it; petTarget: what it fights (the opponent
	// or the opponent's pet)
	pets      map[EntityID]bool
	petOut    map[EntityID]bool
	petTarget map[EntityID]EntityID
	// struck: the opponent hurt this side or its pet; petStruck: its pet
	// (the pets' stances: pet.go)
	struck    map[EntityID]bool
	petStruck map[EntityID]bool
}

// other is the opponent of id in the duel.
func (d *Duel) other(id EntityID) EntityID {
	if id == d.A {
		return d.B
	}
	return d.A
}

// side is id's index in sent: 0 the challenger, 1 the challenged.
func (d *Duel) side(id EntityID) int {
	if id == d.A {
		return 0
	}
	return 1
}

// duelIDMask keeps an id within a box's low 24 bits (duelBoxMark).
const duelIDMask = 0x00FFFFFF

// duelTable is the world's duels.
type duelTable struct {
	byID  map[uint32]*Duel
	of    map[EntityID]*Duel
	next  uint32
	asked map[EntityID]time.Time // each challenger's last challenge
}

func (t *duelTable) add(d *Duel) {
	if t.byID == nil {
		t.byID = make(map[uint32]*Duel)
		t.of = make(map[EntityID]*Duel)
	}
	t.byID[d.ID] = d
	t.of[d.A] = d
	t.of[d.B] = d
}

func (t *duelTable) remove(d *Duel) {
	delete(t.byID, d.ID)
	if t.of[d.A] == d {
		delete(t.of, d.A)
	}
	if t.of[d.B] == d {
		delete(t.of, d.B)
	}
}

// newID is the next duel id: never 0, never one in use.
func (t *duelTable) newID() uint32 {
	for {
		t.next = (t.next + 1) & duelIDMask
		if t.next == 0 {
			continue
		}
		if _, used := t.byID[t.next]; !used {
			return t.next
		}
	}
}

// DuelOf is the id of the duel p is in and whether it is on (the fight)
// rather than asked; 0 when none.
func (w *World) DuelOf(p *Player) (id uint32, on bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if d := w.duels.of[p.ServerID]; d != nil {
		return d.ID, d.Phase == duelOn
	}
	return 0, false
}

// duelOnLocked is the duel p fights, or nil.
func (w *World) duelOnLocked(id EntityID) *Duel {
	if d := w.duels.of[id]; d != nil && d.Phase == duelOn {
		return d
	}
	return nil
}

// duelFoeLocked reports whether target, a player or a pet, is on the other
// side of p's duel: its opponent or its opponent's pet.
func (w *World) duelFoeLocked(p *Player, target EntityID) bool {
	d := w.duelOnLocked(p.ServerID)
	if d == nil {
		return false
	}
	foe := d.other(p.ServerID)
	if target == foe {
		return true
	}
	if pet := w.NPCs[target]; pet != nil && pet.Owner == foe && !d.petOut[foe] {
		return true
	}
	return false
}

// duelistLocked is the duel's player id as it was asked, or nil: gone, or
// replaced by a new login of its character (CreatePlayer keeps the id).
func (w *World) duelistLocked(d *Duel, id EntityID) *Player {
	p := w.Players[id]
	if p == nil || p != d.who[d.side(id)] {
		return nil
	}
	return p
}

// duelSayLocked tells p a line in the duel's colour.
func (w *World) duelSayLocked(p *Player, text string, rgb [3]byte) {
	if p == nil {
		return
	}
	if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
		w.sendColoredLocked(sess, text, rgb)
	}
}

// DuelChallenge is a challenge from sess's player: to the player its client
// knows by objectID (its target), or, with name, to the player of that name.
// wager, when not 0, is the tunar each is to stake (a client without the live
// window names its wager here; two with it stake in the window instead).
func (w *World) DuelChallenge(sess *session.Session, objectID uint32, name string, wager int, now time.Time) {
	defer w.runDuelPurse()
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.Players[p.ServerID] != p {
		return
	}
	var t *Player
	if name = strings.TrimSpace(name); name != "" {
		for _, q := range w.Players {
			if strings.EqualFold(q.Name, name) {
				t = q
				break
			}
		}
		if t == nil {
			w.duelSayLocked(p, fmt.Sprintf("%s is not connected.", name), duelRed)
			return
		}
	} else if objectID == 0 || objectID == 0xFFFFFFFF {
		w.duelSayLocked(p, duelWordInvalid, duelRed)
		return
	} else if objectID == sess.PlayerObjectID() {
		t = p
	} else if e, ok := w.findEntityByObjectIDLocked(sess, objectID); ok {
		t, _ = e.(*Player)
	}
	if t == nil {
		w.duelSayLocked(p, duelWordInvalid, duelRed)
		return
	}
	if t.ServerID == p.ServerID {
		w.duelSayLocked(p, duelWordSelf, duelRed)
		return
	}
	if msg := w.duelRefusalLocked(p, t, now); msg != "" {
		w.duelSayLocked(p, msg, duelRed)
		return
	}
	if p.DuelWindow && t.DuelWindow {
		wager = 0 // the window's: each sets its own there
	}
	if wager < 0 || wager > DuelMaxWager {
		w.duelSayLocked(p, fmt.Sprintf("A wager is 1 to %d tunar.", DuelMaxWager), duelRed)
		return
	}
	if wager > p.Tunar {
		w.duelSayLocked(p, fmt.Sprintf("You do not have %d tunar to wager.", wager), duelRed)
		return
	}
	d := &Duel{ID: w.duels.newID(), A: p.ServerID, B: t.ServerID, who: [2]*Player{p, t}, Phase: duelAsked, Expires: now.Add(DuelInviteTime), Wager: wager}
	w.duels.add(d)
	if w.duels.asked == nil {
		w.duels.asked = make(map[EntityID]time.Time)
	}
	w.duels.asked[p.ServerID] = now
	log.Printf("Duel %d: %s challenges %s", d.ID, p.Name, t.Name)

	if p.DuelWindow {
		w.sendDuelIDLocked(sess, d.ID)
	}
	if wager > 0 {
		w.duelSayLocked(p, fmt.Sprintf("You challenge %s to a duel for %d tunar.", t.Name, wager), duelOrange)
	} else {
		w.duelSayLocked(p, fmt.Sprintf("You challenge %s to a duel.", t.Name), duelOrange)
	}
	w.duelInviteLocked(d, p, t)
}

// duelRefusalLocked is why p may not challenge t now, or "".
func (w *World) duelRefusalLocked(p, t *Player, now time.Time) string {
	switch {
	case w.duels.of[p.ServerID] != nil:
		return "You are already in a duel."
	case w.duels.of[t.ServerID] != nil:
		return fmt.Sprintf(duelWordInDuel, t.Name)
	case p.IsDead() || p.respawnPending():
		return "You cannot duel now."
	case t.IsDead() || t.respawnPending():
		return fmt.Sprintf("%s cannot duel now.", t.Name)
	case p.World != t.World || dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) > DuelStartRange:
		return fmt.Sprintf("%s is too far away to duel.", t.Name)
	case !w.duels.asked[p.ServerID].IsZero() && now.Sub(w.duels.asked[p.ServerID]) < DuelAskAgain:
		return "You must wait a moment before challenging again."
	}
	return ""
}

// duelInviteLocked shows t the invitation: the live window's (0x1502 sub 2)
// or a box with Accept and Decline, and a line either way.
func (w *World) duelInviteLocked(d *Duel, from, t *Player) {
	sess := w.findSessionByEntityIDLocked(t.ServerID)
	if sess == nil {
		return
	}
	question := fmt.Sprintf(duelWordChallenges, from.Name)
	if d.Wager > 0 {
		// the live words end in a stop: the wager goes before it
		question = fmt.Sprintf("%s challenges you to a duel for %d tunar.", from.Name, d.Wager)
	}
	if t.DuelWindow {
		title := w.masterClassLocked(from)
		w.sendDuelInviteLocked(sess, d.ID, from, title)
		if d.Wager > 0 {
			// the live invitation has no wager of its own: said beside it
			w.sendColoredLocked(sess, question, duelOrange)
		}
		return
	}
	w.sendColoredLocked(sess, question, duelOrange)
	w.sendDuelBoxLocked(sess, DuelBoxID(d.ID), question, []string{duelWordAccept, duelWordDecline})
}

// DuelAnswer is sess's player's answer to the invitation id (0: the one it
// was sent).
func (w *World) DuelAnswer(sess *session.Session, id uint32, accept bool, now time.Time) {
	defer w.runDuelPurse()
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if id == 0 {
		// /duel accept or decline: the invitation p was sent
		if d := w.duels.of[p.ServerID]; d != nil && d.Phase == duelAsked && d.B == p.ServerID {
			id = d.ID
		}
	}
	d := w.duels.byID[id]
	if d == nil || d.Phase != duelAsked || d.B != p.ServerID {
		if accept {
			w.duelSayLocked(p, duelWordExpired, duelOrange)
		}
		return
	}
	a := w.duelistLocked(d, d.A)
	if !accept {
		w.duelDeclineLocked(d, p, a)
		return
	}
	if a == nil {
		w.duels.remove(d)
		w.duelSayLocked(p, duelWordExpired, duelOrange)
		return
	}
	why := ""
	switch {
	case a.IsDead() || a.respawnPending():
		why = fmt.Sprintf("%s cannot duel now.", a.Name)
	case p.IsDead() || p.respawnPending():
		why = "You cannot duel now."
	case a.World != p.World || dist3(a.X, a.Y, a.Z, p.X, p.Y, p.Z) > DuelStartRange:
		why = fmt.Sprintf("%s is too far away to duel.", a.Name)
	}
	if why != "" {
		w.duels.remove(d)
		w.duelSayLocked(p, why, duelRed)
		w.duelGoneToChallengerLocked(a, d)
		return
	}
	if a.DuelWindow && p.DuelWindow {
		w.openWagerLocked(d, a, p, now) // duelpurse.go
		return
	}
	d.Stake = [2]int{d.Wager, d.Wager}
	if why := w.holdStakesLocked(d, a, p, now); why != "" {
		w.duels.remove(d)
		w.duelSayLocked(p, why, duelRed)
		w.duelSayLocked(a, why, duelRed)
		return
	}
	w.startDuelLocked(d, a, p, now)
}

// duelDeclineLocked is the challenged b declining d, the challenger a told:
// the live window's sub 5 with b's name, else the live word.
func (w *World) duelDeclineLocked(d *Duel, b, a *Player) {
	w.duels.remove(d)
	log.Printf("Duel %d: %s declines", d.ID, b.Name)
	if a == nil {
		return
	}
	if as := w.findSessionByEntityIDLocked(a.ServerID); as != nil && a.DuelWindow {
		w.sendDuelDeclinedLocked(as, d.ID, b.Name)
		return
	}
	w.duelSayLocked(a, fmt.Sprintf(duelWordDeclinedBy, b.Name), duelOrange)
}

// duelGoneToChallengerLocked tells the challenger its invitation came to
// nothing (lapsed, or could not begin): the live window's anonymous decline
// (sub 4), else the live word for a lapse.
func (w *World) duelGoneToChallengerLocked(a *Player, d *Duel) {
	if a == nil {
		return
	}
	if as := w.findSessionByEntityIDLocked(a.ServerID); as != nil && a.DuelWindow {
		w.sendDuelNoNameLocked(as, d.ID)
		return
	}
	w.duelSayLocked(a, duelWordExpired, duelOrange)
}

// duelGoneToChallengedLocked tells the challenged its invitation is gone
// (lapsed or withdrawn): the live window's sub 3 (which says the live word
// and closes it), else a line.
func (w *World) duelGoneToChallengedLocked(b *Player, d *Duel, line string) {
	if b == nil {
		return
	}
	if bs := w.findSessionByEntityIDLocked(b.ServerID); bs != nil && b.DuelWindow {
		w.sendDuelGoneLocked(bs, d.ID)
		return
	}
	w.duelSayLocked(b, line, duelOrange)
}

// startDuelLocked begins the fight between the challenger a and b.
func (w *World) startDuelLocked(d *Duel, a, b *Player, now time.Time) {
	d.Phase = duelOn
	d.Began = now
	d.World = a.World
	d.X, d.Y, d.Z = (a.X+b.X)/2, (a.Y+b.Y)/2, (a.Z+b.Z)/2
	d.pets = make(map[EntityID]bool)
	d.petOut = make(map[EntityID]bool)
	d.petTarget = make(map[EntityID]EntityID)
	d.struck = make(map[EntityID]bool)
	d.petStruck = make(map[EntityID]bool)
	d.sent = [2][2]uint32{}
	log.Printf("Duel %d: %s and %s begin at (%.1f, %.1f, %.1f) world %d", d.ID, a.Name, b.Name, d.X, d.Y, d.Z, d.World)
	for _, p := range []*Player{a, b} {
		foe := w.Players[d.other(p.ServerID)]
		sess := w.findSessionByEntityIDLocked(p.ServerID)
		if sess == nil {
			continue
		}
		if p.DuelWindow {
			if p == a && !d.windowed {
				w.sendDuelAcceptedLocked(sess, d.ID) // the live client declines its own queue
			}
			w.sendDuelStateLocked(sess, d, p)
			w.sendDuelPlaceLocked(sess, d)
		}
		w.sendColoredLocked(sess, fmt.Sprintf("The duel with %s has begun.", foe.Name), duelOrange)
	}
}

// DuelCancel is sess's player's withdrawal or surrender: an invitation it
// made is withdrawn, one it was sent declined, a wager window's duel called
// off, and its fight forfeit.
func (w *World) DuelCancel(sess *session.Session, now time.Time) {
	defer w.runDuelPurse()
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	d := w.duels.of[p.ServerID]
	if d == nil {
		w.duelSayLocked(p, "You are not in a duel.", duelRed)
		return
	}
	if d.Phase == duelOn {
		w.endDuelLocked(d, p.ServerID, duelSurrendered, now)
		return
	}
	if d.Phase == duelWager {
		w.closeWagerLocked(d, fmt.Sprintf("%s has called off the duel.", p.Name))
		return
	}
	if d.B == p.ServerID {
		w.duelDeclineLocked(d, p, w.duelistLocked(d, d.A))
		return
	}
	w.duels.remove(d)
	log.Printf("Duel %d: %s withdraws", d.ID, p.Name)
	w.duelSayLocked(p, "You withdraw your duel challenge.", duelOrange)
	w.duelGoneToChallengedLocked(w.duelistLocked(d, d.B), d, fmt.Sprintf("%s has withdrawn the duel challenge.", p.Name))
}

// DuelAskCancel shows sess's player the live game's "Cancel duel?" (a box
// with Yes and No: DuelCancelBox), when it is in a duel.
func (w *World) DuelAskCancel(sess *session.Session) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	if w.duels.of[p.ServerID] == nil {
		w.duelSayLocked(p, "You are not in a duel.", duelRed)
		return
	}
	w.sendDuelBoxLocked(sess, DuelCancelBox, duelWordCancel, []string{"Yes", "No"})
}

// DuelBoxAnswer is a 0x0035 to a duel's box (IsDuelBox): option 0 the first
// answer (Accept, Yes), 1 the second; 255, a box closed, is no answer (the
// PS2 sends it before the real pick when two boxes are open).
func (w *World) DuelBoxAnswer(sess *session.Session, box uint32, option byte, now time.Time) {
	if option == 255 {
		return
	}
	if box == DuelCancelBox {
		if option == 0 {
			w.DuelCancel(sess, now)
		}
		return
	}
	w.DuelAnswer(sess, box&duelIDMask, option == 0, now)
}

// The ways a duel ends.
type duelEnd byte

const (
	duelDefeated    duelEnd = iota + 1 // the loser could not take a hit
	duelSurrendered                    // the loser gave up
	duelFled                           // the loser left the arena or its world
	duelLeft                           // the loser left the world (a logout)
	duelFell                           // the loser died of something else
	duelDraw                           // DuelMaxTime: nobody
)

// endDuelLocked ends the fight, loser losing it (0: a draw): both players to
// each other again, their melee on each other let go, the duel's spells off
// them, each live window told, and the result said to both and to those near.
func (w *World) endDuelLocked(d *Duel, loser EntityID, how duelEnd, now time.Time) {
	w.duels.remove(d)
	a, b := w.duelistLocked(d, d.A), w.duelistLocked(d, d.B)
	for _, p := range []*Player{a, b} {
		if p == nil {
			continue
		}
		foe := d.other(p.ServerID)
		if p.AttackTarget == foe {
			p.disengage()
		} else if pet := w.NPCs[p.AttackTarget]; pet != nil && pet.Owner == foe {
			p.disengage()
		}
		w.clearDuelEffectsLocked(p, foe)
		if ps := w.pets[p.ServerID]; ps != nil && d.pets[p.ServerID] {
			ps.npc.endRound()
			ps.npc.stopMoving()
		}
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil && p.DuelWindow {
			w.sendDuelEndLocked(sess, d)
		}
	}
	// the names as they were asked: a player that left is named all the same
	nameA, nameB := d.who[0].Name, d.who[1].Name
	winner, lost := nameA, nameB
	if loser == d.A {
		winner, lost = nameB, nameA
	}
	var line string
	switch how {
	case duelDraw:
		line = fmt.Sprintf("The duel between %s and %s ends in a draw.", nameA, nameB)
	case duelDefeated:
		line = fmt.Sprintf("%s has defeated %s in a duel.", winner, lost)
	default:
		line = fmt.Sprintf("%s forfeits the duel to %s.", lost, winner)
	}
	log.Printf("Duel %d: %s (%d s)", d.ID, line, int(now.Sub(d.Began).Seconds()))
	w.duelAnnounceLocked(d, line)
	w.settleStakesLocked(d, loser) // duelpurse.go
}

// duelAnnounceLocked says a duel's end to its two and to every player in its
// world within DuelHeardWithin of its centre.
func (w *World) duelAnnounceLocked(d *Duel, line string) {
	told := map[EntityID]bool{}
	for _, id := range []EntityID{d.A, d.B} {
		if p := w.Players[id]; p != nil {
			w.duelSayLocked(p, line, duelOrange)
			told[id] = true
		}
	}
	for _, p := range w.Players {
		if told[p.ServerID] || p.World != d.World || dist3(p.X, p.Y, p.Z, d.X, d.Y, d.Z) > DuelHeardWithin {
			continue
		}
		w.duelSayLocked(p, line, duelOrange)
	}
}

// clearDuelEffectsLocked takes the duel's spells off p: what foe put on it.
func (w *World) clearDuelEffectsLocked(p *Player, foe EntityID) {
	for _, e := range append([]*ActiveEffect(nil), p.Effects...) {
		if e.duelFoe == foe {
			w.removeEffectLocked(p, e.SpellID)
		}
	}
}

// duelLeavingLocked is p leaving the world (a logout, a new login): its
// invitation lapses and its fight is forfeit.
func (w *World) duelLeavingLocked(p *Player) {
	d := w.duels.of[p.ServerID]
	if d == nil {
		return
	}
	now := time.Now()
	if d.Phase == duelOn {
		w.endDuelLocked(d, p.ServerID, duelLeft, now)
		return
	}
	if d.Phase == duelWager {
		w.closeWagerLocked(d, fmt.Sprintf("%s has called off the duel.", p.Name))
		return
	}
	w.duels.remove(d)
	if d.A == p.ServerID {
		w.duelGoneToChallengedLocked(w.duelistLocked(d, d.B), d, fmt.Sprintf("%s has withdrawn the duel challenge.", p.Name))
	} else {
		w.duelGoneToChallengerLocked(w.duelistLocked(d, d.A), d)
	}
}

// duelSystem lapses invitations past their time or whose players went, and
// ends fights whose players went, died, fled or ran out of time; it tells a
// live window of a new id for its opponent. Every tick; must be called with
// the lock held.
func (w *World) duelSystem(now time.Time) {
	if len(w.duels.byID) == 0 {
		return
	}
	for _, d := range w.duels.byID {
		a, b := w.duelistLocked(d, d.A), w.duelistLocked(d, d.B)
		if d.Phase == duelAsked {
			if a == nil || b == nil || now.After(d.Expires) {
				w.duels.remove(d)
				log.Printf("Duel %d: the invitation lapses", d.ID)
				w.duelGoneToChallengedLocked(b, d, duelWordExpired)
				w.duelGoneToChallengerLocked(a, d)
			}
			continue
		}
		if d.Phase == duelWager {
			switch {
			case a == nil || b == nil:
				w.closeWagerLocked(d, "The duel is off: your opponent has gone.")
			case now.After(d.until):
				w.closeWagerLocked(d, "The duel is off: the wager took too long.")
			case a.IsDead() || b.IsDead() || a.World != b.World || dist3(a.X, a.Y, a.Z, b.X, b.Y, b.Z) > DuelArenaRadius:
				w.closeWagerLocked(d, "The duel is off.")
			}
			continue
		}
		switch {
		case a == nil || b == nil:
			gone := d.A
			if a != nil {
				gone = d.B
			}
			w.endDuelLocked(d, gone, duelLeft, now)
		case a.IsDead() || a.respawnPending():
			w.endDuelLocked(d, d.A, duelFell, now)
		case b.IsDead() || b.respawnPending():
			w.endDuelLocked(d, d.B, duelFell, now)
		case w.duelOutLocked(d, a):
			w.endDuelLocked(d, d.A, duelFled, now)
		case w.duelOutLocked(d, b):
			w.endDuelLocked(d, d.B, duelFled, now)
		case now.Sub(d.Began) >= DuelMaxTime:
			w.endDuelLocked(d, 0, duelDraw, now)
		default:
			for _, p := range []*Player{a, b} {
				if p.DuelWindow {
					if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
						w.refreshDuelStateLocked(sess, d, p)
					}
				}
			}
		}
	}
}

// duelOutLocked reports p out of the duel's arena: another world, or
// farther than DuelArenaRadius from its centre.
func (w *World) duelOutLocked(d *Duel, p *Player) bool {
	return p.World != d.World || dist3(p.X, p.Y, p.Z, d.X, d.Y, d.Z) > DuelArenaRadius
}

// duelViewLocked shows a duelist its opponent as a creature: channel byte
// 142 (CLIENT's m_p256) 0 in place of a player's 2, which is all the disc's
// TargetInRange (and vanilla's, line for line) asks before a swing, and what
// Sandstorm must send. id is the player on the channel. A client with the
// live window goes by 0x1502 sub 7 and keeps seeing players. Must be called
// with the lock held.
func (w *World) duelViewLocked(sess *session.Session, id EntityID, buf []byte) {
	if len(w.duels.of) == 0 || len(buf) <= 142 {
		return
	}
	viewer := w.sessionPlayerLocked(sess)
	if viewer == nil || viewer.DuelWindow {
		return
	}
	if d := w.duelOnLocked(viewer.ServerID); d != nil && d.other(viewer.ServerID) == id {
		buf[142] = 0
	}
}

// duelViewPetLocked is duelViewLocked for a pet's channel: its byte 142 a
// creature's (0, not a pet's 3) to its owner's opponent while it is in the
// duel.
func (w *World) duelViewPetLocked(sess *session.Session, pet *NPC, buf []byte) {
	if pet.Owner == 0 || len(w.duels.of) == 0 {
		return
	}
	if d := w.duelOnLocked(pet.Owner); d == nil || d.petOut[pet.Owner] {
		return
	}
	w.duelViewLocked(sess, pet.Owner, buf)
}

// duelHalf is a hit at DuelDamageShare, never under 1.
func duelHalf(n int) int {
	return max(1, int(math.Round(float64(n)*DuelDamageShare)))
}

// duelHurtLocked lands n of attacker's duel damage (before the halving) on
// t: from the attacker itself, or from src, its pet. Told to both (0x00DB:
// the attacker's own figure as its own, as a pet's hits are its owner's);
// a hit t cannot take leaves it DuelLoserHP and ends the duel. It reports
// the end.
func (w *World) duelHurtLocked(d *Duel, attacker *Player, src EntityID, t *Player, n int, now time.Time) bool {
	dmg := duelHalf(n)
	attacker.LastCombatTime, t.LastCombatTime = now, now
	if as := w.findSessionByEntityIDLocked(attacker.ServerID); as != nil {
		if tid := w.objectIDSeenByLocked(as, t.ServerID); tid != 0 {
			w.sendDamageIndication(as, as.PlayerObjectID(), -dmg, tid)
		}
	}
	if ts := w.findSessionByEntityIDLocked(t.ServerID); ts != nil {
		if sid := w.objectIDSeenByLocked(ts, src); sid != 0 {
			w.sendDamageIndication(ts, sid, -dmg, ts.PlayerObjectID())
		}
	}
	d.struck[t.ServerID] = true
	t.StatsDirty = true
	if dmg < t.HP {
		t.HP -= dmg
		log.Printf("Duel %d: %s hits %s for %d (%d before the halving; HP %d/%d)", d.ID, attacker.Name, t.Name, dmg, n, t.HP, t.MaxHPNow())
		return false
	}
	t.HP = min(t.HP, DuelLoserHP)
	log.Printf("Duel %d: %s's %d leaves %s at %d", d.ID, attacker.Name, dmg, t.Name, t.HP)
	w.endDuelLocked(d, t.ServerID, duelDefeated, now)
	return true
}

// duelHurtPetLocked lands n of attacker's duel damage (before the halving)
// on the opponent's pet: told to the attacker; a hit it cannot take leaves it
// at one hit point, out of the duel (its owner told), as nothing dies of a
// duel.
func (w *World) duelHurtPetLocked(d *Duel, attacker *Player, pet *NPC, n int, now time.Time) {
	dmg := duelHalf(n)
	attacker.LastCombatTime = now
	if as := w.findSessionByEntityIDLocked(attacker.ServerID); as != nil {
		if tid := w.objectIDSeenByLocked(as, pet.ServerID); tid != 0 {
			w.sendDamageIndication(as, as.PlayerObjectID(), -dmg, tid)
		}
	}
	d.struck[pet.Owner] = true
	d.petStruck[pet.Owner] = true
	if dmg < pet.HP {
		pet.HP -= dmg
		return
	}
	pet.HP = 1
	d.petOut[pet.Owner] = true
	d.pets[pet.Owner] = false
	pet.endRound()
	pet.stopMoving()
	if ps := w.pets[pet.Owner]; ps != nil {
		ps.target = 0
	}
	log.Printf("Duel %d: %s's pet %q yields", d.ID, w.nameOfLocked(pet.Owner), pet.Name)
	w.duelSayLocked(w.Players[pet.Owner], "Your pet yields and leaves the duel.", duelOrange)
	if attacker.AttackTarget == pet.ServerID {
		attacker.disengage()
	}
}

// nameOfLocked is a player's name, or "".
func (w *World) nameOfLocked(id EntityID) string {
	if p := w.Players[id]; p != nil {
		return p.Name
	}
	return ""
}

// duelStandIn is a creature of the target's level: the line a hit on a
// duelist or its pet is rolled on ("as on a creature").
func duelStandIn(level int) *NPC {
	return &NPC{Level: level}
}

// duelEngageLocked is EngageAttack's duel half: a 0x000F at p's duel
// opponent or its pet engages it as a creature is engaged. It reports
// whether it took the press.
func (w *World) duelEngageLocked(p *Player, ent Entity, objectID uint32, swing Swing, now time.Time) bool {
	if ent == nil || !w.duelFoeLocked(p, ent.GetEntityID()) {
		return false
	}
	id := ent.GetEntityID()
	p.Swing = swing
	if p.AttackTarget == id && p.AttackObjectID == objectID {
		p.pressed(now) // a press with the client's timer run out (attack.go)
		return true
	}
	p.AttackTarget = id
	p.AttackObjectID = objectID
	p.pressed(now)
	w.breakInvisLocked(p, "an attack")
	log.Printf("Duel: %s engages %s (swing %d, %v, reach %.1f)", p.Name, entityNameLocked(ent), swing.Kind, swing.Delay, swing.Reach)
	return true
}

func entityNameLocked(e Entity) string {
	switch t := e.(type) {
	case *Player:
		return t.Name
	case *NPC:
		return t.Name
	}
	return "?"
}

// duelRoundLocked is attackSystem's duel half: p's melee on its duel
// opponent or the opponent's pet, in rounds as on a creature (attack.go), each
// hit duel damage. It reports whether p's fight was a duel's (handled here);
// a fight on a player whose duel is over is let go.
func (w *World) duelRoundLocked(p *Player, now time.Time) bool {
	target := p.AttackTarget
	t := w.Players[target]
	pet := w.NPCs[target]
	if t == nil && (pet == nil || pet.Owner == 0) {
		return false // a creature's fight: attack.go's
	}
	d := w.duelOnLocked(p.ServerID)
	if d == nil || !w.duelFoeLocked(p, target) || p.IsDead() || (pet != nil && pet.Dead) {
		p.disengage()
		return true
	}
	reach := func() bool {
		if p.Swing.Never {
			return false
		}
		if t != nil {
			return dist3(p.X, p.Y, p.Z, t.X, t.Y, t.Z) <= p.Swing.Reach
		}
		r := p.Swing.Reach + npcReachExtra(pet)
		dx, dy, dz := p.X-pet.X, p.Y-pet.Y, p.Z-pet.Z
		return dx*dx+dy*dy+dz*dz <= r*r
	}
	hit := func() bool {
		if !reach() {
			return false
		}
		if t != nil {
			n := playerMeleeHit(p, duelStandIn(t.Level), rand.Intn)
			return w.duelHurtLocked(d, p, p.ServerID, t, n, now)
		}
		n := playerMeleeHit(p, duelStandIn(pet.Level), rand.Intn)
		w.duelHurtPetLocked(d, p, pet, n, now)
		return p.AttackTarget == 0
	}
	hits := swingHits(p.Swing.Kind)
	land := func() bool {
		for p.RoundHits < len(hits) && !now.Before(p.RoundStart.Add(hits[p.RoundHits])) {
			p.RoundHits++
			if p.RoundHits == 1 {
				w.breakInvisLocked(p, "a swing")
			}
			if hit() {
				p.disengage()
				return true
			}
		}
		return false
	}
	if !p.RoundStart.IsZero() && land() {
		return true
	}
	// on the client's beat, as on a creature (attack.go roundStart)
	start, ok := p.roundStart(now)
	if !ok {
		return true
	}
	p.RoundStart, p.RoundHits = start, 0
	p.NextSwing = p.RoundStart.Add(p.Swing.Delay)
	land()
	return true
}

// SetDuelWindow makes the client's answer to the duel offer (0x7E09) its
// player's: on, the live window's messages and players seen as players. It
// reports false when the session has no player in the world.
func (w *World) SetDuelWindow(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.DuelWindow = on
	return true
}
