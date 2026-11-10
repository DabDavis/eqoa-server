package entity

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// callhero.go — Call of the Hero: a magician's call that brings a member of
// its group to its side, from anywhere, if the member accepts (jib's PR #1,
// 7b79650, re-made on main's workbook spell; the GM Jibrish's ruling of
// 2026-10-29: "a critical mage spell, it had a long cast time and 1 hour cool
// down. It would summon a player from anywhere in the world to the mages
// current position, if the player accepted the call of the hero."; a test
// note of 2006: "Call of the Hero should work in group now").
//
// The workbook's record (spelleffect.effects): "Group, Summon Player"
// (EffectSummonPlayer), scope group, its radius 30, its lowest level 30; its
// spellpattern scope 2, cast 5 s, recast 1800 s, power 200, which this leaves
// as they are. Until 2026-11-10 it landed as a group spell: on the members in
// its range, each brought beside its caster at once and unasked, so the far
// members it is for were never called, and the near ones were moved from where
// they stood to beside it.
//
// The rules (ours where it says so; the PR's otherwise):
//   - it goes on its caster alone, whatever its record's scope
//     (castTargetsLocked), and lands on no area's others (landLocked);
//   - as it lands, each member of the caster's group not within HeroCallNear
//     of it (farther, or in another world), alive, in the world and not
//     loading one, and of the spell's lowest level (tooLowLocked: the caster
//     told, as for any spell), is asked through the hero-call hook
//     (handlers/callhero.go: a Yes/No box; a member whose game cannot load the
//     caster's world is not asked, and the caster is told); the caster is told
//     whom it called, or that no one was far enough (ours);
//   - Yes within the hook's wait lands the member beside the caster where the
//     caster stands then, as a group gate lands its members round their caster
//     (HeroCallBring: the travel hook, GateRingSpot, the world loaded when it
//     changed). No, or no answer in time: nothing. A caster gone, dead or
//     loading a world by then: the member stays, told the call has ended;
//   - it is no harmful spell, and costs what its record says, as any spell.

// The call's numbers, ours: HeroCallNear is how near its caster a member
// stands at its side already, and is not asked; HeroCallWait how long a call
// waits for its answer (handlers/callhero.go).
const (
	HeroCallNear float32 = 30
	HeroCallWait         = 60 * time.Second
)

// The call's words to its caster, ours: no recording of the live game's
// exists.
const (
	heroCalled   = "You call %s to your side."
	heroCallNone = "No one of your group is far enough to call."
)

// callsHero reports whether the spell calls its caster's group to it.
func (d *SpellDef) callsHero() bool {
	for _, e := range d.Effects {
		if e.Kind == EffectSummonPlayer {
			return true
		}
	}
	return false
}

// HeroCall is one member a call asks, for the hero-call hook: the caster,
// the member and its session, the caster's world (which the member's game
// must load), the spell's name, and the member's place round the caster.
type HeroCall struct {
	Caster, Member         EntityID
	CasterName, MemberName string
	Sess, CasterSess       *session.Session
	World                  uint8
	Spell                  string
	Ring                   int
}

// HeroCallFunc asks a member a call reaches (handlers' box). It runs on the
// tick goroutine after the tick, with the World lock let go.
type HeroCallFunc func(c HeroCall)

// heroCallState is the calls to ask and the hook that asks them.
type heroCallState struct {
	due []HeroCall
	ask HeroCallFunc
}

// OnHeroCall sets the hero-call hook.
func (w *World) OnHeroCall(fn HeroCallFunc) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls.ask = fn
}

// heroCallLandLocked is a call's landing on its caster (the file's rules):
// each member it reaches handed to the hook, the caster told. Must be called
// with the lock held.
func (w *World) heroCallLandLocked(cs *castState, caster *Player, casterSess *session.Session) {
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
		if w.tooLowLocked(caster, casterSess, m, &cs.spell) { // workbook.go: below its lowest level
			continue
		}
		w.calls.due = append(w.calls.due, HeroCall{Caster: caster.ServerID, Member: m.ServerID, CasterName: caster.Name,
			MemberName: m.Name, Sess: s, CasterSess: casterSess, World: caster.World, Spell: cs.spell.Name, Ring: len(names) + 1})
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

// HeroCallBring is a call's Yes: the member queued for the travel hook to
// land beside its caster, where the caster stands now, at its place round it
// (ring). It reports false when it cannot come: it or its caster dead, gone
// or loading a world (callerGone says it was the caster).
func (w *World) HeroCallBring(caster, member EntityID, sess *session.Session, ring int, spell string) (ok, callerGone bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	m := w.Players[member]
	if m == nil || m.IsDead() || m.respawnPending() || sess == nil || w.sessionPlayerLocked(sess) != m {
		return false, false
	}
	c := w.Players[caster]
	var cs *session.Session
	if c != nil {
		cs = w.findSessionByEntityIDLocked(c.ServerID)
	}
	if c == nil || c.IsDead() || c.respawnPending() || cs == nil || cs.StateChannelsHeld() {
		log.Printf("Call: %s accepts %q, but its caster is gone", m.Name, spell)
		return false, true
	}
	at := GateSpot{World: c.World, X: c.X, Y: c.Y, Z: c.Z, Place: c.Name + "'s side"}
	w.gates.due = append(w.gates.due, GateTravel{Sess: sess, P: m, To: at, Ring: ring, Spell: spell})
	log.Printf("Call: %s accepts %q: to %s at (%.1f, %.1f, %.1f) world %d", m.Name, spell, at.Place, at.X, at.Y, at.Z, at.World)
	return true, false
}
