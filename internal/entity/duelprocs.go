package entity

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// duelprocs.go — what a melee hit sets off in a duel (petition 1135, Larrybird: "proc items
// dont work in duels, Bpoa or sbpb do not work in duels tested both multi times"). A duel's
// melee went straight to duelHurtLocked, so nothing a hit sets off on a creature ever ran.
// Now, as on a creature:
//
//   - a melee hit on a duelist, by its opponent or the opponent's pet, is answered by its spell
//     shields, its worn items (wornshield.go: the Breastplate of the Anointed, the Seraphic
//     Planar) and its defensive procs, at whoever struck;
//   - a duelist's melee hit runs its own procs: its effects' (proc.go, workbookmore.go) and its
//     weapon's (weaponproc.go).
//
// Their damage goes through duelHurtLocked as any duel hit does, so it is halved
// (DuelDamageShare), and leaves the loser standing. A heal or power given is whole: it is the
// item's figure, not a hit. A weapon's proc rolls no resist, as a duel's spells roll none; its
// calm has no hate to take here. Must be called with the lock held.

// duelMark is what a hit in a duel lands on: a duelist, or a duelist's pet.
type duelMark struct {
	t   *Player
	pet *NPC
}

func (m duelMark) id() EntityID {
	if m.t != nil {
		return m.t.ServerID
	}
	return m.pet.ServerID
}

func (m duelMark) level() int {
	if m.t != nil {
		return m.t.Level
	}
	return m.pet.Level
}

func (m duelMark) name() string {
	if m.t != nil {
		return m.t.Name
	}
	return m.pet.Name
}

// duelHurtMarkLocked lands n of from's duel damage on the mark, told to from as kind
// (numbertags.go); it reports the duel over, or the pet out of it.
func (w *World) duelHurtMarkLocked(d *Duel, from *Player, m duelMark, n int, now time.Time, kind numberKind) bool {
	if m.t != nil {
		return w.duelHurtLocked(d, from, from.ServerID, m.t, n, now, kind)
	}
	w.duelHurtPetLocked(d, from, m.pet, n, now, kind)
	return d.petOut[m.pet.Owner]
}

// duelAnswerLocked answers a melee hit landed on t with what t has that answers one, at the mark
// that struck (the file's doc). It reports the duel over, or the pet that struck out of it.
func (w *World) duelAnswerLocked(d *Duel, t *Player, m duelMark, now time.Time) bool {
	sess := w.findSessionByEntityIDLocked(t.ServerID)
	if len(t.Effects) > 0 {
		hits, _ := shieldAnswers(t.Effects, t.Level, m.level(), 0, func() bool { return false }, now, procIntn)
		for _, h := range hits {
			w.broadcastCastLocked(t, m.id(), 0, fxShieldHit)
			log.Printf("%s: %s's %q answers %s for %d (proc %v)", d.tag(), t.Name, h.name, m.name(), h.n, h.proc)
			if w.duelHurtMarkLocked(d, t, m, h.n, now, h.kind()) {
				return true
			}
		}
	}
	for _, s := range t.Gear.Shields {
		damage, heal, power := s.forWearer(t, procIntn)
		if damage > 0 && !s.Undead {
			w.broadcastCastLocked(t, m.id(), 0, fxShieldHit)
			log.Printf("%s: %s's %q answers %s for %d", d.tag(), t.Name, s.Item, m.name(), damage)
			if w.duelHurtMarkLocked(d, t, m, damage, now, numberProc) { // a worn item's answer: a proc's
				return true
			}
		}
		w.wornGiveLocked(t, sess, s, heal, power)
	}
	w.procHealsLocked(t, nil, true, now)
	w.duelProcSpellsLocked(d, t, sess, m, true, now)
	return w.duelOverForLocked(d, t, m)
}

// duelProcsLocked runs p's procs after its melee hit on the mark (the file's doc); it reports the
// duel over, or the pet hit out of it.
func (w *World) duelProcsLocked(d *Duel, p *Player, m duelMark, now time.Time) bool {
	sess := w.findSessionByEntityIDLocked(p.ServerID)
	hits, resisted := procRollsOf(p.Effects, p.StatNow, p.Level, m.level(), 0, func() bool { return false }, now, procIntn)
	if resisted > 0 {
		log.Printf("Duel %d: %s's proc resisted by %s", d.ID, p.Name, m.name())
	}
	for _, h := range p.procsOfRound(hits) {
		log.Printf("Duel %d: %s's proc hits %s for %d", d.ID, p.Name, m.name(), h.n)
		over := w.duelHurtMarkLocked(d, p, m, h.n, now, numberProc)
		w.procTapLocked(p, sess, h.ae, nil)
		if over {
			return true
		}
	}
	w.procHealsLocked(p, nil, false, now)
	w.duelProcSpellsLocked(d, p, sess, m, false, now)
	if w.duelOverForLocked(d, p, m) {
		return true
	}
	return w.duelWeaponProcLocked(d, p, sess, m, now)
}

// duelOverForLocked reports the duel over for p, or the mark's pet out of it.
func (w *World) duelOverForLocked(d *Duel, p *Player, m duelMark) bool {
	if d.pvp {
		// in no table: over when either side is down, or no longer foes
		return !d.who[0].pvpFoe(d.who[1]) || (m.pet != nil && d.petOut[m.pet.Owner])
	}
	return w.duelOnLocked(p.ServerID) != d || (m.pet != nil && d.petOut[m.pet.Owner])
}

// duelProcSpellsLocked fires p's spell procs at the mark, as procSpellsLocked does at a creature.
func (w *World) duelProcSpellsLocked(d *Duel, p *Player, sess *session.Session, m duelMark, defense bool, now time.Time) {
	standing := func() bool { return !w.duelOverForLocked(d, p, m) }
	w.procSpellsOnLocked(p, sess, defense, now, standing, func() bool { return false }, m.name(), func(cs *castState) {
		cs.spell.duel = true
		cs.targets = []EntityID{m.id()}
		if m.t != nil {
			w.duelLandLocked(cs, p, sess, m.t, now)
		} else {
			w.duelLandPetLocked(cs, p, m.pet, now)
		}
	})
}

// duelWeaponProcLocked runs the proc of the hand that landed p's hit, if it procs; it reports the
// duel over, or the pet hit out of it.
func (w *World) duelWeaponProcLocked(d *Duel, p *Player, sess *session.Session, m duelMark, now time.Time) bool {
	wp := handProcLocked(p, p.RoundHits)
	if wp == nil || p.IsDead() {
		return false
	}
	if wp.Only != "" && !strings.Contains(strings.ToLower(m.name()), wp.Only) {
		return false
	}
	if wp.Kind == ProcForm {
		w.formProcLocked(sess, p, wp, now)
		return false
	}
	if procIntn(100) >= wp.chancePct(p.StatNow(StatDex)) {
		return false
	}
	n := wp.Damage
	if wp.DamageTo > wp.Damage {
		n += procIntn(wp.DamageTo - wp.Damage + 1)
	}
	switch wp.Kind {
	case ProcRestore:
		w.restoreProcLocked(sess, p, wp, n)
		return false
	case ProcCalm:
		w.broadcastCastLocked(p, p.ServerID, 0, wp.FX)
		return false
	case ProcDoT:
		w.broadcastCastLocked(p, m.id(), 0, wp.FX)
		log.Printf("%s: %s's %q lays %d a tick on %s", d.tag(), p.Name, wp.Item, n, m.name())
		cs := weaponDoTCast(p, wp, m.id(), n)
		cs.spell.duel = true
		if m.t != nil {
			w.duelLandLocked(cs, p, sess, m.t, now)
		} else {
			w.duelLandPetLocked(cs, p, m.pet, now)
		}
		return w.duelOverForLocked(d, p, m)
	}
	w.broadcastCastLocked(p, m.id(), 0, wp.FX)
	log.Printf("%s: %s's %q hits %s for %d", d.tag(), p.Name, wp.Item, m.name(), n)
	over := w.duelHurtMarkLocked(d, p, m, n, now, numberProc)
	if wp.Kind == ProcTap && !p.IsDead() {
		w.procHealLocked(p, sess, p, d.hit(n))
	}
	return over
}
