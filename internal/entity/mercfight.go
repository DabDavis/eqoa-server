package entity

import (
	"log"
	"time"
)

// mercfight.go — a merc's fight (merc.go): what it takes on, its melee, and
// its damage, whose kills are its owner's.
//
// It assists its owner (the owner's word): it never starts a fight of its own,
// never fights a player (no PvP, no duel's opponent or pet: duel.go leaves a
// merc out) and never what its owner may not attack (mercMayFight). By its
// stance (/merc stance): passive, only what /merc attack names; assist (its
// default), its owner's auto-attack's target, else the nearest creature within
// petAssist of its owner fighting its owner or itself, hurt by its owner, or
// fought by its owner's pet, and one fighting anyone of its owner's group (a
// player, its pet, its merc, that merc's pet) or its own pet
// (mercDefendsLocked; the owner, 2026-11-11: "when 2 players in a group have a
// merc, if one merc gets aggro, sometimes the other does not act to defend");
// aggressive, also one fought by any of its owner's group. /merc attack sets it on
// its owner's target whatever its stance, until that dies or goes. A target
// petLeash from its owner is let go, as a pet's; one not ordered and not its
// owner's auto-attack's, already past petAssist from its owner (its owner
// walked off from it), too (mercLeftBehindLocked).
//
// A tank or a melee fighter goes to its target and swings a player's rounds of
// its class and level (GetSwing on its heirlooms, rounds.go's timing), each hit
// a player's roll on its heirloom weapon, STR and DEX (offence.go) through the
// balance as a player's melee (meleeDealtLocked); a healer and a caster stand
// off, in reach of their spells, and do not swing. A hit is told to its owner
// as its merc's figure (numbertags.go; its pet's to a layout 1 client), angers the creature at the merc
// (updateAggro: it is the merc that a creature turns on), and a kill is its
// owner's as a pet's (petLandHitLocked): its experience by killexp.go's split
// of the players near (a merc takes no share and cuts none), its factions and
// its quests (factionKillLocked).

// mercMayFight is whether the merc may take on the creature: what a pet may
// (petMayFight: alive, in its world, attackable, nobody's pet or merc, not
// another's Metal Moogie), and never a Metal Moogie at all (a duel's).
func mercMayFight(m, npc *NPC) bool {
	return petMayFight(m, npc) && npc.challenge == nil
}

// mercTeamFightsLocked reports the creature fighting, or fought by, one of the
// merc's owner's group: a player of it, its pet, its merc. Must be called with
// the lock held.
func (w *World) mercTeamFightsLocked(ms *mercState, npc *NPC) bool {
	owner := ms.owner
	if owner.GroupID == 0 {
		return false
	}
	g := w.groups[owner.GroupID]
	if g == nil {
		return false
	}
	for _, id := range g.Members {
		if npc.CurrentEnemy == id {
			return true
		}
		if _, hurt := npc.Threats[id]; hurt {
			return true
		}
		if m := w.Players[id]; m != nil {
			if m.Attacking() && m.AttackTarget == npc.ServerID {
				return true
			}
			if ps := w.petStateLocked(m); ps != nil && (npc.CurrentEnemy == ps.npc.ServerID || ps.target == npc.ServerID) {
				return true
			}
		}
		if o := w.NPCs[id]; o.isMerc() && o.merc.target == npc.ServerID {
			return true
		}
	}
	return false
}

// mercDefendsLocked reports the creature fighting one the merc defends: its
// owner, itself or its own pet, and anyone of its owner's group: a player, its
// pet, its merc and that merc's pet (the file's doc). Must be called with the
// lock held.
func (w *World) mercDefendsLocked(ms *mercState, npc *NPC) bool {
	e := npc.CurrentEnemy
	if e == 0 {
		return false
	}
	ours := func(o *Player) bool {
		if o == nil {
			return false
		}
		if e == o.ServerID {
			return true
		}
		if pet := w.petOfLocked(o); pet != nil && e == pet.ServerID {
			return true
		}
		for _, om := range w.mercs[o.ServerID] {
			if om.owner == o && (e == om.npc.ServerID || om.pet != nil && e == om.pet.npc.ServerID) {
				return true
			}
		}
		return false
	}
	if ours(ms.owner) || e == ms.npc.ServerID {
		return true
	}
	g := w.groups[ms.owner.GroupID]
	if ms.owner.GroupID == 0 || g == nil {
		return false
	}
	for _, id := range g.Members {
		if id == e {
			return true // a member: a player or a merc
		}
		if p := w.Players[id]; p != nil && p != ms.owner && ours(p) {
			return true
		}
	}
	return false
}

// mercChooseTargetLocked keeps or lets go of the merc's target and gives one
// without a target what its stance takes on (the file's doc). Must be called
// with the lock held.
func (w *World) mercChooseTargetLocked(ms *mercState) {
	owner, m := ms.owner, ms.npc
	if w.duelEngagedLocked(owner.ServerID) {
		// its owner's duel is its owner's own: it stands down, whatever it
		// fought, until the duel is over (ownfight.go, petition 1901)
		if ms.target != 0 || ms.ordered != 0 {
			ms.target, ms.ordered = 0, 0
			m.endRound()
		}
		return
	}
	valid := func(id EntityID) *NPC {
		t := w.NPCs[id]
		if t == nil || !mercMayFight(m, t) || distXZ(owner.X, owner.Z, t.X, t.Z) > petLeash {
			return nil
		}
		return t
	}
	if ms.ordered != 0 {
		if valid(ms.ordered) != nil {
			ms.target = ms.ordered
			return
		}
		ms.ordered = 0
	}
	if ms.target != 0 && (valid(ms.target) == nil || w.mercLeftBehindLocked(ms, ms.target)) {
		ms.target = 0
		m.endRound()
	}
	if ms.stance == MercPassive {
		if ms.target != 0 {
			ms.target = 0
			m.endRound()
		}
		return
	}
	// its owner's own fight first: an assist follows its owner's switch
	if owner.Attacking() {
		if t := valid(owner.AttackTarget); t != nil {
			ms.target = t.ServerID
			return
		}
	}
	if ms.target != 0 {
		return
	}
	pet := w.petOfLocked(owner)
	var best *NPC
	var bestD float32
	for _, id := range w.SpatialIndex.QueryRadius(owner.X, owner.Z, petAssist) {
		npc := w.NPCs[id]
		if npc == nil || !mercMayFight(m, npc) {
			continue
		}
		mine := npc.CurrentEnemy == owner.ServerID || npc.CurrentEnemy == m.ServerID
		if !mine {
			_, mine = npc.Threats[owner.ServerID] // its owner hurt it
		}
		if !mine && pet != nil {
			ps := w.pets[owner.ServerID]
			mine = npc.CurrentEnemy == pet.ServerID || ps.target == npc.ServerID
		}
		if !mine {
			mine = w.mercDefendsLocked(ms, npc)
		}
		if !mine && ms.stance == MercAggressive {
			mine = w.mercTeamFightsLocked(ms, npc)
		}
		if !mine {
			continue
		}
		if d := distXZ(m.X, m.Z, npc.X, npc.Z); best == nil || d < bestD {
			best, bestD = npc, d
		}
	}
	if best != nil {
		ms.target = best.ServerID
	}
}

// mercLeftBehindLocked reports a target the merc's owner has walked off
// from: farther than petAssist from its owner (as far as the merc looks round
// its owner for a fight) and not its owner's auto-attack's. /merc attack's
// order is kept to petLeash (petition 1504, Lifeispain: "when I just left the
// dummy the merc is still fighting it by itself"). Must be called with the
// lock held.
func (w *World) mercLeftBehindLocked(ms *mercState, id EntityID) bool {
	owner, t := ms.owner, w.NPCs[id]
	if t == nil || id == ms.ordered || owner.Attacking() && owner.AttackTarget == id {
		return false
	}
	return distXZ(owner.X, owner.Z, t.X, t.Z) > petAssist
}

// mercTurnLocked is the merc's turn: its target, then a cast its role wants
// now (merccast.go: heals first, buffs out of a fight, its taunts and its
// attacks in one), else its place: at its target's side swinging (a tank or a
// melee fighter), in reach of it (a healer or a caster), else beside its owner
// or where it holds. Must be called with the lock held.
func (w *World) mercTurnLocked(ms *mercState, dt float32, now time.Time) {
	m := ms.npc
	w.mercChooseTargetLocked(ms)
	t := w.NPCs[ms.target]
	if ms.target == 0 {
		t = nil
	}
	// its bow (mercshoot.go): an arrow on its way lands, and the bow or its
	// melee hands in hand by its target's distance
	if w.mercShotLandLocked(ms, now) {
		t = w.NPCs[ms.target]
		if ms.target == 0 {
			t = nil
		}
	}
	w.mercHandsLocked(ms, t)
	if t != nil || now.Before(ms.busy) {
		ms.mercIdleNoteBusy(now) // no ease in a fight or a cast (mercidle.go)
	}
	if now.Before(ms.busy) {
		// a cast on its way: it stands and faces where it casts
		m.stopMoving()
		return
	}
	if w.mercCastTurnLocked(ms, t, now) {
		m.stopMoving()
		return
	}
	switch {
	case t == nil:
		w.mercHomeLocked(ms, dt, now)
	case ms.role == roleTank || ms.role == roleMelee:
		w.mercMeleeLocked(ms, t, dt, now)
	default:
		w.mercStandOffLocked(ms, t, dt, now)
	}
}

// mercReach is how near its target a merc swings from: a player's melee
// reach (attack.go's TargetInRange) and the target's allowance for its size,
// less a little so it is inside it.
func mercReach(t *NPC) float32 {
	return meleeReach + npcReachExtra(t) - 0.5
}

// mercMeleeLocked takes the merc to its target and swings there in its
// rounds, as a pet's (petFightLocked): each round's swing to all who see it,
// its hits at the swing's times. Must be called with the lock held.
func (w *World) mercMeleeLocked(ms *mercState, t *NPC, dt float32, now time.Time) {
	m := ms.npc
	reach := mercReach(t)
	if d := distXZ(m.X, m.Z, t.X, t.Z); d > reach || w.petStackedLocked(m, t.X, t.Y, t.Z, now) {
		// out of its reach: its bow's shot, or the pull it waits for
		// (mercshoot.go)
		if d > reach && w.mercShootLocked(ms, t, now) {
			m.loseDueHits(now)
			return
		}
		// out of its reach, or its target on the floor over or under it: round
		// what stands between (petwalk.go)
		x, z := m.X, m.Z
		if d > reach {
			x = t.X + (m.X-t.X)/d*(reach-1)
			z = t.Z + (m.Z-t.Z)/d*(reach-1)
		}
		w.petWalkLocked(m, t.ServerID, x, z, t.X, t.Y, t.Z, max(ms.owner.SpeedNow(), petRunSpeed), dt, now)
		m.loseDueHits(now) // closing in: no hit lands from out of reach
		return
	}
	m.stopMoving()
	m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
	length, delay, fx, hits := npcRound(m)
	if m.RoundTarget != t.ServerID {
		m.endRound()
		m.RoundTarget = t.ServerID
	}
	if m.RoundStart.IsZero() || !now.Before(m.RoundStart.Add(length)) {
		next := m.RoundStart.Add(length)
		if m.RoundStart.IsZero() || now.Sub(next) >= length {
			next = now
		}
		m.RoundStart, m.RoundHits = next, 0
		breakNPCInvis(m)
		w.broadcastNPCSwingLocked(m, t.ServerID, uint32(delay/time.Millisecond), fx)
	}
	for m.RoundHits < len(hits) && !now.Before(m.RoundStart.Add(hits[m.RoundHits])) {
		m.RoundHits++
		if w.mercMeleeHitLocked(ms, t, now) {
			return
		}
	}
}

// mercStandOffLocked keeps a healer or a caster in reach of its target: it
// follows its owner (or holds its place) while its spells reach the creature
// from there, and steps toward it otherwise, to a little inside its longest
// reach. Must be called with the lock held.
func (w *World) mercStandOffLocked(ms *mercState, t *NPC, dt float32, now time.Time) {
	m := ms.npc
	reach := w.mercReachOf(ms) + npcReachExtra(t)
	if dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) <= reach {
		w.mercHomeLocked(ms, dt, now)
		if dist3(m.X, m.Y, m.Z, t.X, t.Y, t.Z) <= reach && m.Loco == LocoStand {
			m.Facing = headingTo(t.X-m.X, t.Z-m.Z)
		}
		return
	}
	d := distXZ(m.X, m.Z, t.X, t.Z)
	x := t.X + (m.X-t.X)/d*(reach-2)
	z := t.Z + (m.Z-t.Z)/d*(reach-2)
	w.petWalkLocked(m, t.ServerID, x, z, t.X, t.Y, t.Z, max(ms.owner.SpeedNow(), petRunSpeed), dt, now)
}

// mercStandOffReach is the most a healer or a caster stands from its target
// when none of its spells says (or none reaches farther).
const mercStandOffReach float32 = 18

// mercReachOf is the reach of the merc's spells at a creature: the longest of
// its book's, at most mercStandOffReach, at least a melee reach. Must be
// called with the lock held.
func (w *World) mercReachOf(ms *mercState) float32 {
	r := float32(meleeReach)
	for i := range ms.kit.Spells {
		s := &ms.kit.Spells[i]
		if mercUseOf(s).atFoe() && s.Range > r {
			r = s.Range
		}
	}
	return min(r, mercStandOffReach)
}

// mercMeleeHitLocked is one landed hit of the merc's round: a player's roll on
// its weapon, STR and DEX (offence.go's playerShotHit, a bow's too), the
// balance's share for a player's melee of its class and level, then its damage.
// It reports a kill.
func (w *World) mercMeleeHitLocked(ms *mercState, t *NPC, now time.Time) bool {
	b := ms.body
	n := w.meleeDealtLocked(t, b, playerShotHit(b, t, meleeIntn), meleeIntn) // balance.go
	return w.mercDamageLocked(ms, t, n, numberMelee, now)
}

// mercDamageLocked is n of the merc's damage on t, the balance's already: its
// owner told the figure as its merc's (numbertags.go: its pet's to a client of
// layout 1), t's hate for the merc,
// a root broken; on a kill, its owner's experience, factions and quests as a
// pet's kill. A training dummy takes nothing, its hate the merc's (dummy.go).
// It reports a kill. Must be called with the lock held.
func (w *World) mercDamageLocked(ms *mercState, t *NPC, n int, kind numberKind, now time.Time) bool {
	owner, m := ms.owner, ms.npc
	sess := w.findSessionByEntityIDLocked(owner.ServerID)
	tell := func() {
		if sess == nil {
			return
		}
		if tid := w.objectIDForEntityLocked(sess, t.ServerID); tid != 0 {
			w.sendDamageIndication(sess, sess.PlayerObjectID(), -n, tid, kind.byMerc())
		}
	}
	m.LastCombatTime = now
	if t.Dummy != NotDummy {
		tell()
		w.dummyEngagedLocked(t, m.ServerID, n, now)
		return false
	}
	n = w.npcTakesLocked(t, n, now)             // nothing while held at 1 (deathhold.go)
	w.tallyDamageLocked(t, owner, min(n, t.HP)) // killcredit.go: the hirer's
	t.HP = max(t.HP-n, 0)
	tell()
	w.updateAggro(t, m.ServerID, n)
	w.rootBreakLocked(t, m.Level) // root.go
	log.Printf("Merc: %s's %q hit %s for %d (HP %d/%d)", owner.Name, m.Name, t.Name, n, t.HP, t.MaxHP)
	if t.HP > 0 {
		w.hitLandedLocked(t, n, sess, m.ServerID) // its heal at every hit that lands, its ward's answer (hitheal.go, ward.go)
		return false
	}
	t.Effects = nil
	w.killNPC(t)
	cs, cp := w.killCreditLocked(t, sess, owner) // killcredit.go: the most damage's group
	w.awardKillXPLocked(cs, cp, t)               // killexp.go: the players' split, the merc in no share
	w.factionKillLocked(cs, cp, t)               // faction.go, and the quests
	if owner.AttackTarget == t.ServerID {
		owner.disengage()
	}
	if ms.target == t.ServerID {
		ms.target = 0
		m.endRound()
	}
	if ms.ordered == t.ServerID {
		ms.ordered = 0
	}
	return true
}

// mercTickLocked is a tick of a merc's damage over time on the creature
// (cast.go's tickNPCEffectLocked): its figure less the share the creature's
// template resists, through the balance as a player's spell (dealtLocked),
// one of the merc's hits. A tick whose merc is gone (dead, dismissed, its
// owner's merc another since) does nothing: what is left of it runs out. Must
// be called with the lock held.
func (w *World) mercTickLocked(npc *NPC, ae *ActiveEffect, now time.Time) {
	ae.NextTick = ae.NextTick.Add(effectTick)
	m := w.NPCs[ae.merc]
	if m == nil || !m.isMerc() || m.Dead || npc.Dead || ae.HPTick >= 0 {
		return
	}
	ms := m.merc
	if !w.mercLiveLocked(ms) {
		return
	}
	hit := -creatureResists(ae.Rules.Resist, npc, arcaneDebuffed(ae.Rules.Resist, npc, ae.HPTick, now), now)
	ae.ticking = true // its own tick is no contact for itself (contactNPCLocked)
	defer func() { ae.ticking = false }()
	log.Printf("Merc: %s's %q's %q ticks on %s for %d before the balance", ms.owner.Name, m.Name, ae.Name, npc.Name, hit)
	w.mercDamageLocked(ms, npc, w.dealtLocked(npc, ms.body, hit), numberDoT, now)
}

// mercHitOn is a creature's melee hit on a merc: its roll on a player of the
// merc's class with the merc's AC and AGI (defence.go's hitOn), its buffs'
// among them; npcFightPetLocked takes it through the balance and the
// creature's template as a pet's, and petTakenLocked the share a player of
// its class takes.
func mercHitOn(npc, m *NPC, intn func(int) int) int {
	b := m.merc.body
	return npcMeleeRollFor(npc, b).hitOn(b, intn)
}
