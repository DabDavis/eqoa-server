package entity

import (
	"fmt"
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// mercharm.go — /merc charm (the owner, 2026-11-11: "enchanter merc a charm
// command"): a merc whose book holds a charm (an enchanter's Entrancing lines
// and Binding Grasp; a bard's, a druid's and a necromancer's too, by the same
// rules) charms its hirer's target, and /merc charm off lets it go.
//
// The charm is the player's (charm.go), cast by the merc: of its book's
// charms the highest level that takes the target (the longer of two of a
// level: Entrancing Coercion over Coerce) (the creature's npccharm
// row, the charm's level cap, a necromancer's undead alone and a druid's
// animals alone: charmStartLocked, undeadStartLocked, targetKindStartLocked),
// ready and paid for, in its reach; its cast time, power and lockout as any
// of its casts (merccast.go). At its landing the creature's resist (its
// npccharm row's, else charmResistBase, the arcane debuffs on it taken off)
// may throw it off, and the creature comes at the merc; else it is taken for
// the charm's time plus its CHA share of the merc's CHA (charmDuration) and
// is the merc's pet (mercpet.go) in place of its summoned one (gone as the
// charm takes hold): it fights what the merc fights, follows it, and is kept
// healed and buffed by it; a kill of its is the hirer's. The charm breaking
// (its time out, /merc charm off) turns it on the merc; led past its grace
// (charmFar) it goes home at once; the merc going (a dismissal, a death, its
// hirer's logout) lets it go where it stands. Dead as a pet, its corpse is no
// one's to loot and it comes back itself (corpse.go), and the merc summons
// its own pet again.

// The lines of /merc charm (ours, but Sandstorm's "Attempt resisted!" and
// charm.go's refusals).
const (
	mercCharmNone    = "Your mercenary knows no charm."
	mercCharmNoFoe   = "Your mercenary cannot charm that."
	mercCharmBusy    = "Your mercenary is busy."
	mercCharmNotYet  = "Your mercenary cannot cast its charm yet."
	mercCharmTooFar  = "Your target is out of your mercenary's reach."
	mercCharmHasOne  = "Your mercenary already has a charmed pet: /merc charm off lets it go."
	mercCharmBegins  = "%s begins to charm %s."
	mercCharmTaken   = "%s has charmed %s."
	mercCharmNoneOut = "Your mercenary has no charmed pet."
	mercCharmLetGo   = "%s lets %s go."
	mercCharmWore    = "%s's charm on %s has worn off."
)

// MercCharm is /merc charm: the session's player's merc begins its charm at
// the player's target (the file's doc). It reports the begun line, or the
// refusal.
func (w *World) MercCharm(sess *session.Session, now time.Time) (line, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	ms := w.mercOfLocked(p)
	if ms == nil {
		return "", "You have no mercenary out."
	}
	m := ms.npc
	var charms []*CastSpell
	for i := range ms.kit.Spells {
		if s := &ms.kit.Spells[i]; s.Def.Charms() {
			charms = append(charms, s)
		}
	}
	if len(charms) == 0 {
		return "", mercCharmNone
	}
	if ps := w.mercPetOutLocked(ms); ps != nil && ps.npc.Charm != nil {
		return "", mercCharmHasOne
	}
	ent, ok := w.findEntityByObjectIDLocked(sess, p.TargetObjectID)
	t, isNPC := ent.(*NPC)
	if !ok || !isNPC || !mercMayFight(m, t) || t.Owner != 0 || t.Charm != nil {
		return "", mercCharmNoFoe
	}
	if now.Before(ms.busy) {
		return "", mercCharmBusy
	}
	// the highest level of its charms that takes it
	refusal = mercCharmNoFoe
	var best *CastSpell
	targets := []EntityID{t.ServerID}
	for _, s := range charms {
		why := ""
		if s.Def.undeadOnly() {
			why, _ = w.undeadStartLocked(targets)
		}
		if why == "" && s.Def.TargetKind != "" {
			why, _ = w.targetKindStartLocked(&s.Def, targets)
		}
		if why == "" {
			why = w.charmStartLocked(s, targets)
		}
		if why == "" && !mercReaches(m, s, t.X, t.Y, t.Z, npcReachExtra(t)) {
			why = mercCharmTooFar
		}
		if why == "" && !ms.mercReady(s, now) {
			why = mercCharmNotYet
		}
		if why != "" {
			if best == nil {
				refusal = why
			}
			continue
		}
		if best == nil || s.Level > best.Level || s.Level == best.Level && s.Def.Duration > best.Def.Duration {
			best = s
		}
	}
	if best == nil {
		return "", refusal
	}
	m.stopMoving()
	m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
	w.mercCastLocked(ms, best, t.ServerID, false, now)
	return fmt.Sprintf(mercCharmBegins, m.Name, t.Name), ""
}

// MercCharmOff is /merc charm off: the merc's charmed pet let go, turned on
// the merc (charm.go: a dismissed charm turns on its master). It reports the
// line, or the refusal.
func (w *World) MercCharmOff(sess *session.Session) (line, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	ms := w.mercOfLocked(w.sessionPlayerLocked(sess))
	if ms == nil {
		return "", "You have no mercenary out."
	}
	ps := ms.pet
	if ps == nil || ps.npc.Charm == nil || ps.npc.Dead {
		return "", mercCharmNoneOut
	}
	name := ps.npc.Name
	w.mercCharmReleaseLocked(ms, true)
	return fmt.Sprintf(mercCharmLetGo, ms.npc.Name, name), ""
}

// mercCharmLandLocked lands the merc's charm on the creature id (the file's
// doc). Must be called with the lock held.
func (w *World) mercCharmLandLocked(ms *mercState, s *CastSpell, id EntityID, now time.Time) {
	m, o := ms.npc, ms.owner
	t := w.NPCs[id]
	if t == nil || t.Dead || t.World != m.World || t.Owner != 0 || t.Charm != nil || !mercMayFight(m, t) {
		return
	}
	sess := w.findSessionByEntityIDLocked(o.ServerID)
	if ps := w.mercPetOutLocked(ms); ps != nil && ps.npc.Charm != nil {
		return // one came while it was cast
	}
	resist := w.charmResistLocked(t, now)
	if t.Dummy != NotDummy || charmIntn(100) < resist {
		m.LastCombatTime = now
		w.updateAggro(t, m.ServerID, 1) // it comes at the merc
		if sess != nil {
			w.sendColoredLocked(sess, charmResisted, castRefusalRGB)
		}
		log.Printf("Merc: %s's %q's %q resisted by %s (%d%%)", o.Name, m.Name, s.Name, t.Name, resist)
		return
	}
	if ms.pet != nil {
		w.mercPetGoesLocked(ms, now) // its summoned pet goes as the charm takes hold
	}
	hold := &charmHold{npcType: t.NPCType, faction: t.Faction, world: t.World, spell: s.Name}
	if d := charmDuration(s, ms.body.StatNow(StatCha)); d > 0 {
		hold.until = now.Add(d)
	}
	w.dropAggroLocked(t.ServerID)
	w.takeCharmedLocked(o, t, hold)
	ps := &petState{owner: o, npc: t, stance: petDefensive, last: now, merc: ms}
	ps.tmpl = w.charmTemplateLocked(t)
	t.mercPet = ms
	ms.pet = ps
	if ms.target == t.ServerID {
		ms.target = 0
		m.endRound()
	}
	if ms.ordered == t.ServerID {
		ms.ordered = 0
	}
	if sess != nil {
		w.sendChatToSession(sess, fmt.Sprintf(mercCharmTaken, m.Name, t.Name))
	}
	log.Printf("Merc: %s's %q charms %s (level %d) with %q until %v", o.Name, m.Name, t.Name, t.Level, s.Name, hold.until)
}

// mercCharmReleaseLocked lets the merc's charmed pet go: the creature its own
// again (charm.go's releaseCharmLocked: home at once past its grace), turned
// on the merc when hostile. Must be called with the lock held.
func (w *World) mercCharmReleaseLocked(ms *mercState, hostile bool) {
	ps := ms.pet
	if ps == nil || ps.npc.Charm == nil {
		return
	}
	ms.pet = nil
	npc := ps.npc
	npc.mercPet = nil
	w.releaseCharmLocked(ps, nil, "", false)
	if hostile && !npc.Dead && npc.Charm == nil && npc.World == ms.npc.World && !ms.npc.Dead {
		w.updateAggro(npc, ms.npc.ServerID, 1)
	}
}

// mercCharmTurnLocked keeps the merc's charm or ends it: its time out (the
// creature turned on the merc, its hirer told), past its grace from where it
// spawned or where the merc stands (let go, home at once). It reports the
// charm ended. Must be called with the lock held.
func (w *World) mercCharmTurnLocked(ms *mercState, sess *session.Session, now time.Time) bool {
	ps := ms.pet
	if ps == nil || ps.npc.Charm == nil || ps.npc.Dead {
		return false
	}
	pet, h := ps.npc, ps.npc.Charm
	switch {
	case !h.until.IsZero() && !now.Before(h.until):
		if sess != nil {
			w.sendChatToSession(sess, fmt.Sprintf(mercCharmWore, ms.npc.Name, pet.Name))
		}
		w.mercCharmReleaseLocked(ms, true)
		return true
	case charmFar(pet, h.world, pet.World, pet.X, pet.Z), charmFar(pet, h.world, ms.npc.World, ms.npc.X, ms.npc.Z):
		if sess != nil {
			w.sendChatToSession(sess, charmTooFar)
		}
		w.mercCharmReleaseLocked(ms, false)
		return true
	}
	return false
}
