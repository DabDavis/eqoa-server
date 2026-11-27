package entity

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// moogie.go — Mr. Moogie (petition 1246; the user: build it): an NPC who clears a
// player's buffs or heals it whole, for anyone not flagged for PvP and not in a duel (handlers'
// moogie.go asks; this does). "Clear my buffs" ends every effect on the player as a GM's
// #clearbuffs ends them (ClearBuffs), but the town's Boon (boon.go): a blessing for playing, not a
// spell's buff. "Fully heal me" brings its hit points and power to their most.

// MoogieName is his name: an NPC of it is Mr. Moogie.
const MoogieName = "Mr. Moogie"

// Why Mr. Moogie will not help (ours).
const (
	MoogieLinePvP  = "Mr. Moogie will not help you while you are flagged for PvP."
	MoogieLineDuel = "Mr. Moogie will not help you while you are in a duel."
	MoogieLineDead = "Mr. Moogie cannot help you now."
	MoogieLineFoe  = "Mr. Moogie will not help you while you fight him."
)

// MoogieBusy is why sess's player may not have Mr. Moogie's help now ("" when it may): he, moogie,
// will not help one he fights (a GM's petition 1251 made him attackable), one flagged for
// PvP or one in a duel.
func (w *World) MoogieBusy(sess *session.Session, moogie EntityID) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	switch {
	case p == nil || p.IsDead():
		return MoogieLineDead
	case p.pvp.on:
		return MoogieLinePvP
	case w.duelOnLocked(p.ServerID) != nil:
		return MoogieLineDuel
	case w.metalDuelOfLocked(p.ServerID):
		return MetalLineDueling // metalmoogie.go
	}
	if npc := w.NPCs[moogie]; npc != nil {
		if _, foe := npc.Threats[p.ServerID]; foe || npc.CurrentEnemy == p.ServerID || isMoogie(npc) && npc.moogieConsented(p.ServerID, time.Now()) {
			return MoogieLineFoe // or one who consented to fight him, while it lasts (petition 3331)
		}
	}
	return ""
}

// MoogieClear ends every effect on sess's player but the Boon, each as its time running out ends it
// (removeEffectLocked). It returns the player's name and how many it ended; ok false with no player.
func (w *World) MoogieClear(sess *session.Session) (name string, n int, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", 0, false
	}
	for _, ae := range append([]*ActiveEffect(nil), p.Effects...) {
		if outlivesDeath(ae.SpellID) || ae.Rules.Kept { // the Boon, a GM's reward, his own Duelist's Favor (metalmoogie.go)
			continue
		}
		w.removeEffectLocked(p, ae.SpellID)
		n++
	}
	p.StatsDirty = true
	log.Printf("Moogie: %s's buffs cleared (%d effects)", p.Name, n)
	return p.Name, n, true
}

// MoogieResetID is the lockout of Mr. Moogie's reset itself (a GM's petition 1444: "allow the
// player to reset all of their ability cool downs ... can only be used on a player once every 10
// minutes"): a lockout of no spell (an id no ability has), shown on no ability, so it is kept as every
// lockout is over a logout (charlockouts: lockoutsave.go) and a restart that keeps the sessions (keep.go),
// on the wall clock; MoogieResetEvery is how often a character may have it.
const (
	MoogieResetID    = -1444
	MoogieResetEvery = 10 * time.Minute
)

// MoogieReset ends every lockout of sess's player's abilities still running (cast.go: each spell's and
// art's, and the arts' shared cooldown) as their time running out ends them: each one's badge told 0x00CD
// of 1 ms and then 0 (sendLockoutCleared), once a display index. Not its pet's, nor anyone else's; not its items' cooldowns.
// Once in MoogieResetEvery a character (its own lockout, MoogieResetID): within it nothing is ended and
// wait is what is left of it. It returns the player's name and how many lockouts it ended; ok false
// with no living player, or within its ten minutes. The PvP flag and a duel are MoogieBusy's, asked
// first.
func (w *World) MoogieReset(sess *session.Session, now time.Time) (name string, n int, wait time.Duration, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", 0, 0, false
	}
	for _, l := range p.Lockouts {
		if l.spellID == MoogieResetID && now.Before(l.until) {
			return p.Name, 0, l.until.Sub(now), false
		}
	}
	told := map[int]bool{}
	for _, l := range p.Lockouts {
		if l.spellID == MoogieResetID {
			continue // its own, over: another below
		}
		if now.Before(l.until) {
			n++
		}
		if !told[l.displayIndex] { // a badge of no ability (-1) is sent nothing
			told[l.displayIndex] = true
			sendLockoutCleared(sess, l.displayIndex)
		}
	}
	if now.Before(p.artReady) {
		n++
	}
	p.artReady = time.Time{}
	p.Lockouts = append(p.Lockouts[:0], castLockout{spellID: MoogieResetID, displayIndex: -1, until: now.Add(MoogieResetEvery)})
	log.Printf("Moogie: %s's ability lockouts reset (%d running); again from %s", p.Name, n, now.Add(MoogieResetEvery).UTC().Format("15:04:05"))
	return p.Name, n, 0, true
}

// sendLockoutCleared tells an ability's lockout over before its time (Mr. Moogie's reset): 0x00CD of
// 1 ms and then of 0. The 0 alone is what CLIENT reads as the lockout's end, but vanilla passes over an
// end that comes with more than a second of the lockout it keeps still to run (Heirloom petition 3012:
// the old lockout's end following a new cast), and so kept every badge and sweep after a reset
// (petitions 3083, 3103). The 1 ms lockout first is one of no time, so the 0 after it is its own end to
// every client; CLIENT's badge of 1 ms is gone at its next frame either way.
func sendLockoutCleared(sess *session.Session, displayIndex int) {
	sendCooldown(sess, displayIndex, time.Millisecond)
	sendCooldown(sess, displayIndex, 0)
}

// MoogieHeal brings sess's player's hit points and power to their most. It returns the player's
// name; ok false with no player.
func (w *World) MoogieHeal(sess *session.Session) (name string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.IsDead() {
		return "", false
	}
	p.HP, p.Power = p.MaxHPNow(), p.MaxPowerNow()
	p.StatsDirty = true
	log.Printf("Moogie: %s healed whole (%d HP, %d power)", p.Name, p.HP, p.Power)
	return p.Name, true
}

// MoogieOutmatched is the line the live client's 0x077A case added in red, as a 2009 tape filmed it
// ("You are outmatched. Attempting to engage again is not advised."; board 1784): Frontiers' disc
// has it as word 1196, "You are outmatched. Attacking again is not advised." (VIClient's receive
// loop at 0x00BD280C), which a PS2 here still shows for 0x077A itself. A harmful spell at Mr.
// Moogie is refused with these words, as a swing at him is refused with 0x077A itself (the owner,
// 2026-11-19: "if players accidently attack mr. moogie lets use it"): the first time (below).
const MoogieOutmatched = "You are outmatched. Attempting to engage again is not advised."

// Mr. Moogie fought by those who mean it (the GM jib's Heirloom petition 3331, 2026-11-27: "Mr moogie
// should be attackable with all of his special mechanics and extremely high hp, ignore all player
// tickets to remove this"; the owner: "the first warning should say outmatched, but if they attack
// again it should let them"). A player's first harmful act at him (a press of attack, a harmful spell
// or a thrown potion begun at him) is warned off as it was (0x077A for a press, which also stops
// CLIENT's auto-attack; MoogieOutmatched for a spell or a throw) and nothing of it lands. Another such
// act within MoogieAgainWindow of the warning goes (on a PS2 the next press of attack: the 0x077A let
// the button go), and from then the player has consented: Mr. Moogie is a creature to it as any other
// (a very hard one: his template's hit points and mechanics are its own and run as they always ran),
// its swings, spells, areas, thrown potions, and its own pets and mercs all hurt him, and his hate
// takes them. The consent lasts while the player is on his hate list (or his enemy) and
// MoogieConsentAfter after its last hate on him; lapsed, the next attack is warned again. A
// warning not followed up within the window lapses too.
//
// Those who have not consented are kept from him as before (moogieSpares): another's area never
// takes him in on their behalf, a group member's pet or merc fights him only if its own master
// consented (a pet's or merc's consent is its master's, never the group's), and his help
// (handlers/moogie.go) is refused one consented while it lasts (MoogieBusy). Creatures fight him as
// they always could (the GM triggers' raids). His metal twins' duels (metalmoogie.go) are no part of it.
const (
	MoogieAgainWindow  = 2 * time.Minute
	MoogieConsentAfter = time.Minute
	// moogieAgainAfter is how soon after a warning a second act may be the "again": one press read
	// twice (a 0x000F and its unsent-press reading, a duplicate) is no second attack
	moogieAgainAfter = time.Second
)

// isMoogie reports whether npc is Mr. Moogie: he clears buffs and heals (handlers/moogie.go), and no
// player may hurt him but one who consented (moogieSpares). Creatures still may: the GM triggers'
// raids on him ("Nagafen Attacks Mr. Moogie" ...) set gods on him, so this is asked at the players'
// paths, not in challengeBars (which guards creatures' threat too). His metal twins are duelled
// (metalmoogie.go) and are not him.
func isMoogie(npc *NPC) bool {
	return npc != nil && strings.EqualFold(strings.TrimSpace(npc.Name), MoogieName)
}

// moogieSpares reports whether npc is Mr. Moogie and the player id has not consented to fight him
// (the doc above): nothing of that player's, its pets' or its mercs' may touch him. id is the player
// (moogiePlayer for a pet's or a merc's).
func moogieSpares(npc *NPC, id EntityID, now time.Time) bool {
	return isMoogie(npc) && !npc.moogieConsented(id, now)
}

// moogieConsented is whether the player id has consented to fight Mr. Moogie, n, and still may: on
// his hate list, his enemy, or within its time after.
func (n *NPC) moogieConsented(id EntityID, now time.Time) bool {
	until, ok := n.moogieOK[id]
	if !ok || id == 0 {
		return false
	}
	if _, foe := n.Threats[id]; foe || n.CurrentEnemy == id {
		return true
	}
	return now.Before(until)
}

// moogiePlayer is the player whose consent a pet's or a merc's is: a class pet's or a merc's owner, a
// merc's pet's merc's owner; a player's own id is itself.
func moogiePlayer(n *NPC) EntityID {
	if n == nil {
		return 0
	}
	if ms := n.mercPet; ms != nil && ms.npc != nil {
		return ms.npc.Owner
	}
	return n.Owner
}

// moogieSparesLocked is moogieSpares for any id: a player, or a pet or a merc by its master. Must be
// called with the lock held.
func (w *World) moogieSparesLocked(npc *NPC, id EntityID, now time.Time) bool {
	if !isMoogie(npc) {
		return false
	}
	if _, ok := w.Players[id]; !ok {
		if o := w.NPCs[id]; o != nil {
			id = moogiePlayer(o)
		}
	}
	return moogieSpares(npc, id, now)
}

// moogieAttackLocked is p's harmful act aimed at npc, the act's own (a press, a harmful spell or a
// thrown potion begun at him): whether it may go. On anyone but Mr. Moogie it may. On him: consented,
// it goes and the consent is kept; warned within MoogieAgainWindow (and not the same act read twice),
// it is the "again" and consents; else it is warned, the caller refusing it with its own words. Must
// be called with the lock held.
func (w *World) moogieAttackLocked(p *Player, npc *NPC, now time.Time) bool {
	if p == nil || !isMoogie(npc) {
		return true
	}
	if npc.moogieConsented(p.ServerID, now) {
		npc.moogieKeep(p.ServerID, now)
		return true
	}
	at, warned := npc.moogieWarned[p.ServerID]
	if warned && !now.Before(at) && now.Sub(at) < moogieAgainAfter {
		return false // the warning's own act, read again: warned still, the window from the first
	}
	if warned && !now.Before(at) && now.Sub(at) <= MoogieAgainWindow {
		delete(npc.moogieWarned, p.ServerID)
		npc.moogieKeep(p.ServerID, now)
		log.Printf("Moogie: %s attacks %s again after the warning: consented, the fight is on (petition 3331)", p.Name, npc.Name)
		return true
	}
	if npc.moogieWarned == nil {
		npc.moogieWarned = map[EntityID]time.Time{}
	}
	for id, t := range npc.moogieWarned { // the lapsed warnings, put away
		if now.Sub(t) > MoogieAgainWindow {
			delete(npc.moogieWarned, id)
		}
	}
	npc.moogieWarned[p.ServerID] = now
	log.Printf("Moogie: %s attacks %s: outmatched, warned (again within %v to fight him)", p.Name, npc.Name, MoogieAgainWindow)
	return false
}

// moogieKeep keeps the player id's consent on Mr. Moogie, n, to MoogieConsentAfter from now (never
// shortened), the lapsed consents of others put away.
func (n *NPC) moogieKeep(id EntityID, now time.Time) {
	if n.moogieOK == nil {
		n.moogieOK = map[EntityID]time.Time{}
	}
	for o, until := range n.moogieOK {
		if o != id && !n.moogieConsented(o, now) && now.Sub(until) > MoogieAgainWindow {
			delete(n.moogieOK, o)
		}
	}
	if until := now.Add(MoogieConsentAfter); until.After(n.moogieOK[id]) {
		n.moogieOK[id] = until
	}
}

// moogieHateLocked keeps a consent at a hate on Mr. Moogie (addThreatLocked): the hate of a consented
// player, its pet's or its merc's, carries the consent on to MoogieConsentAfter after it. Must be
// called with the lock held.
func (w *World) moogieHateLocked(npc *NPC, id EntityID, now time.Time) {
	if npc.moogieOK == nil {
		return // none consented: nothing to keep
	}
	if _, ok := w.Players[id]; !ok {
		if o := w.NPCs[id]; o != nil {
			id = moogiePlayer(o)
		}
	}
	if _, ok := npc.moogieOK[id]; ok {
		npc.moogieKeep(id, now)
	}
}

// moogieSparedAmong is the Mr. Moogie among a harmful cast's targets the player id has not consented
// to fight (nil for none). Must be called with the lock held.
func (w *World) moogieSparedAmongLocked(id EntityID, targets []EntityID, now time.Time) *NPC {
	for _, t := range targets {
		if npc := w.NPCs[t]; moogieSpares(npc, id, now) {
			return npc
		}
	}
	return nil
}

// sendOutMatchedLocked sends 0x077A (no payload): CLIENT adds word 1196 in red, stops auto-attack
// and answers 0x077B (handlers.SendOutMatched's message, from here). Must be called with the lock
// held.
func (w *World) sendOutMatchedLocked(sess *session.Session) {
	if sess == nil {
		return
	}
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeOutMatched)
	wr := buffer.NewWriter(msg.Span())
	wr.WriteUint16(uint16(protocol.OpcodeOutMatched))
	msg.SetSize(wr.Position())
	sess.Queue().AddWithSequence(msg)
}
