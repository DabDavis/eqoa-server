package entity

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// callhero.go — Call of the Hero: a magician's call that brings a member of
// its group to its side, from anywhere, if the member accepts (the owner's
// ruling of 2026-10-29: "a critical mage spell, it had a long cast time and 1
// hour cool down. It would summon a player from anywhere in the world to the
// mages current position, if the player accepted the call of the hero."; a
// test note of 2006: "Call of the Hero should work in group now").
//
// Its line, EQOA-Data's as spelleffect holds it: "Summons a player to where
// you are"; its record a group's (scope 2), cast 5 s, recast 3600 s, which
// this leaves as they are. Until now the line was read nowhere: the spell
// took its power and did nothing.
//
// The words (a line's clause): "Summon Player", "Summons a player", and what
// follows them ("to where you are").
//
// The rules (ours where it says so):
//   - the call goes by its caster's group, whatever its record's scope: it is
//     cast on its caster (castTargetsLocked), the 0x00B1 the caster's own;
//   - as it lands, every member of the caster's group not within HeroCallNear
//     of it (farther, or in another world), alive, in the world and not
//     loading one, whose game loads the caster's world (handlers'
//     LoadableWorlds), is asked through the hero-call hook (handlers: a Yes/No
//     box, 0x0034, its id under HeroCallBoxMark); the caster is told whom it
//     called, or that there was no one (ours);
//   - Yes (option 0) within HeroCallWait lands the member beside the caster,
//     where the caster stands when the answer comes, as a group gate lands
//     its members round their caster (gates.go: the travel hook at the next
//     tick, GateRingSpot, the world loaded when it changed; a game that cannot
//     load it stays, told). No, or no answer in time: nothing (a Yes too late
//     is told it passed, ours). A caster gone, dead or loading a world by
//     then: the member stays, told the call has ended (ours);
//   - it is no harmful spell, and costs what its record says, as any spell.

// The call's numbers, ours.
const (
	// HeroCallBoxMark marks a call's box (0x0034's u32, echoed by 0x0035),
	// as handlers' quests (0x51000000), triggers (0x52000000), duels
	// (0x53000000, duelwire.go) and rehabilitators (0x54000000) mark theirs.
	HeroCallBoxMark = 0x55000000
	// HeroCallNear is how near its caster a member stands at its side
	// already, and is not asked.
	HeroCallNear float32 = 30
	// HeroCallWait is how long a call waits for its answer.
	HeroCallWait = 60 * time.Second
)

// The call's words, ours: no recording of the live game's exists.
const (
	heroCalled     = "You call %s to your side."
	heroCallNone   = "No one of your group is far enough to call."
	heroCallPassed = "The call has passed."
	heroCallEnded  = "The call has ended."
)

// reCallHero is the call's clause, lower case.
var reCallHero = regexp.MustCompile(`^summons?\s+(?:a\s+)?player\b`)

// callHeroPart reads the call's clause.
func callHeroPart(part string) (SpellEffect, bool) {
	if !reCallHero.MatchString(part) {
		return SpellEffect{}, false
	}
	return SpellEffect{Kind: EffectCallHero}, true
}

// callsHero reports whether the spell calls its caster's group to it.
func (d *SpellDef) callsHero() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectCallHero {
			return true
		}
	}
	return false
}

// HeroCall is one member a call asks, for the hero-call hook: its box's id,
// the member's session and player, the caster's name and the spell's.
type HeroCall struct {
	Box    uint32
	Sess   *session.Session
	P      *Player
	Caster string
	Spell  string
}

// HeroCallFunc asks a member a call reaches (handlers' box). It runs on the
// tick goroutine after the tick, with the World lock released.
type HeroCallFunc func(c HeroCall)

// heroCallState is the calls waiting for their answers by box, those to ask,
// the hook that asks them, and the boxes' count.
type heroCallState struct {
	open map[uint32]heroCallOpen
	due  []HeroCall
	ask  HeroCallFunc
	seq  uint32
}

// heroCallOpen is a call waiting for one member's answer: the member, the
// caster, the spell's name, the member's place round the caster, and until
// when.
type heroCallOpen struct {
	member, caster EntityID
	spell          string
	ring           int
	until          time.Time
}

// IsHeroCallBox reports a 0x0035's id as a call's box.
func IsHeroCallBox(id uint32) bool { return id&0xFF000000 == HeroCallBoxMark }

// OnHeroCall sets the hero-call hook.
func (w *World) OnHeroCall(fn HeroCallFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls.ask = fn
}

// heroCallLandLocked is a call's landing (the file's rules): each member it
// reaches asked, the caster told. It reports whether the spell is a call.
// Must be called with the lock held.
func (w *World) heroCallLandLocked(cs *castState, caster *Player, casterSess *session.Session, now time.Time) bool {
	if !cs.spell.Def.callsHero() {
		return false
	}
	for box, c := range w.calls.open {
		if !now.Before(c.until) {
			delete(w.calls.open, box)
		}
	}
	var names []string
	for _, m := range w.groupMembersLocked(caster) {
		if m == caster || m.IsDead() || m.respawnPending() {
			continue
		}
		if m.World == caster.World && !outOfRange(caster, m.X, m.Y, m.Z, HeroCallNear) {
			continue // at its side already
		}
		s := w.findSessionByEntityIDLocked(m.ServerID)
		if s == nil || s.StateChannelsHeld() {
			continue // gone, or loading a world
		}
		if !w.loadsLocked(s, caster.World) {
			log.Printf("Call: %s's %q does not ask %s: its game cannot load world %d", caster.Name, cs.spell.Name, m.Name, caster.World)
			continue
		}
		w.calls.seq++
		box := uint32(HeroCallBoxMark | (w.calls.seq & 0xFFFFFF))
		if w.calls.open == nil {
			w.calls.open = make(map[uint32]heroCallOpen)
		}
		w.calls.open[box] = heroCallOpen{member: m.ServerID, caster: caster.ServerID, spell: cs.spell.Name, ring: len(names) + 1,
			until: now.Add(HeroCallWait)}
		w.calls.due = append(w.calls.due, HeroCall{Box: box, Sess: s, P: m, Caster: caster.Name, Spell: cs.spell.Name})
		names = append(names, m.Name)
	}
	line := heroCallNone
	if len(names) > 0 {
		line = fmt.Sprintf(heroCalled, strings.Join(names, ", "))
	}
	if casterSess != nil {
		w.sendChatToSession(casterSess, line)
	}
	log.Printf("Call: %s's %q asks %d of its group: %s", caster.Name, cs.spell.Name, len(names), strings.Join(names, ", "))
	return true
}

// runHeroCalls hands each member a call asks to the hero-call hook. Must be
// called WITHOUT the lock.
func (w *World) runHeroCalls() {
	w.mu.Lock()
	due, fn := w.calls.due, w.calls.ask
	w.calls.due = nil
	w.mu.Unlock()
	if fn == nil {
		return
	}
	for _, c := range due {
		fn(c)
	}
}

// AnswerHeroCall takes a member's answer (0x0035) to a call's box: Yes
// (option 0) in time queues its landing beside the caster for the travel
// hook (gates.go), at the next tick; No, or a box not the session's, drops
// it; a close (255) keeps it open, as a quest's box is kept (the PS2 sends
// one before the real pick when two boxes are open). It returns a line for
// the member, "" for none.
func (w *World) AnswerHeroCall(sess *session.Session, box uint32, option byte, now time.Time) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	c, ok := w.calls.open[box]
	if !ok || option == 255 {
		return ""
	}
	p := w.sessionPlayerLocked(sess)
	if p == nil || p.ServerID != c.member {
		return ""
	}
	delete(w.calls.open, box)
	if option != 0 {
		log.Printf("Call: %s declines %q", p.Name, c.spell)
		return ""
	}
	if !now.Before(c.until) {
		log.Printf("Call: %s accepts %q too late", p.Name, c.spell)
		return heroCallPassed
	}
	if p.IsDead() || p.respawnPending() {
		return ""
	}
	caster := w.Players[c.caster]
	var cs *session.Session
	if caster != nil {
		cs = w.findSessionByEntityIDLocked(caster.ServerID)
	}
	if caster == nil || caster.IsDead() || caster.respawnPending() || cs == nil || cs.StateChannelsHeld() {
		log.Printf("Call: %s accepts %q, but its caster is gone", p.Name, c.spell)
		return heroCallEnded
	}
	at := GateSpot{World: caster.World, X: caster.X, Y: caster.Y, Z: caster.Z, Place: caster.Name + "'s side"}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: p, To: at, Ring: c.ring, Spell: c.spell})
	log.Printf("Call: %s accepts %q: to %s at (%.1f, %.1f, %.1f) world %d", p.Name, c.spell, at.Place, at.X, at.Y, at.Z, at.World)
	return ""
}
