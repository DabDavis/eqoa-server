package entity

import (
	"time"

	"github.com/eqoa/server/internal/database"
)

// kinheal.go — a creature's heal of its kin (a GM's petition, 2026-11-20: "All
// cleric NPC's and Mystic ... should cast a small non tickheal (instant, 1
// shot) level appropriate heal ... They will heal other monsters they are
// social with, so freezeblood clerics will heal other freezeblood clerics").
//
// A templated creature's heal whose Target is kin (database.AbilityOnKin) lands
// on itself or on one of its kin near it, the worst off: of itself and the
// creatures of its kin within its social radius (npcsocial's row for its name,
// else DefaultSocialRadius), alive, no pet, no training dummy, no NPC bit, in
// its world (its instance) and its sight (npcSeesLocked), the one with the
// smallest share of its hit points below the heal's share (BelowOr), itself
// first of equals. Its kin: the creatures of the faction it helps
// (socialFactionLocked: its npcsocial row's, else the one its kill lowers most,
// factionhits), every one of that faction (ofFactionLocked); with none, the
// creatures of its own name (NPCClassKey). None hurt enough: it waits, due, as a
// heal on itself does.
//
// A heal of its share (HealPct, no spell pinned: instant) is that share of the
// healer's own most hit points ("about 15% of an @ level mobs health": the
// healer's, at its level), at most what the one healed lacks; a spell's heal is
// its figure. The one it heals is told the cast's target in the cast's 0x00B1,
// and the player fighting the one healed sees the figure (0x00DB from the
// healer). Anything else of the cast is a heal on itself's (templateCastLocked,
// templateHealLocked).
//
// In its own fight it is one of its turns (templateTurnLocked). Out of one (no
// enemy: the creatures of a camp are not social unless npcsocial says so, and
// a healer standing by would heal no one) it heals too, every kinHealIdleTicks,
// on the ability's own clock (every, from the first look): only one of its kin
// that is fighting (an enemy of its own), not itself (its regen is out of a
// fight's, npcregen.go), its chance and its cooldown as in a fight. It does not
// join the fight by it.

// kinHealRangeLocked is how near a creature's kin must stand for its heal: its
// npcsocial row's radius, else DefaultSocialRadius. Must be called with the
// lock held.
func (w *World) kinHealRangeLocked(npc *NPC) float32 {
	if r, ok := w.npcSocial.ruleFor(npc.Name); ok && r.Radius > 0 {
		return r.Radius
	}
	return DefaultSocialRadius
}

// kinOfLocked reports o of npc's kin (the file's comment). Must be called with
// the lock held.
func (w *World) kinOfLocked(npc, o *NPC, faction string) bool {
	if faction != "" {
		return w.ofFactionLocked(o, faction)
	}
	return database.NPCClassKey(o.Name) == database.NPCClassKey(npc.Name)
}

// kinHealTargetLocked is who npc's heal of its kin lands on now (the file's
// comment), nil for none hurt enough: out of its own fight (no enemy) only one
// of its kin that is fighting, never itself. Must be called with the lock held.
func (w *World) kinHealTargetLocked(npc *NPC, a *templateAbility) *NPC {
	below := a.BelowOr()
	idle := npc.CurrentEnemy == 0
	var best *NPC
	bestShare := 0.0
	consider := func(o *NPC) {
		if o.Dead || o.MaxHP <= 0 || o.HP >= o.MaxHP || o.healBarred() || o.HP*100 >= o.MaxHP*below {
			return
		}
		share := float64(o.HP) / float64(o.MaxHP)
		if best == nil || share < bestShare {
			best, bestShare = o, share
		}
	}
	if !idle {
		consider(npc)
	}
	if w.SpatialIndex == nil {
		return best
	}
	faction := w.socialFactionLocked(npc)
	r := w.kinHealRangeLocked(npc)
	for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, r) {
		o := w.NPCs[id]
		if o == nil || o == npc || o.Owner != 0 || o.Dummy != NotDummy || o.NPCType&0x80 != 0 || o.SpawnOnly ||
			idle && o.CurrentEnemy == 0 {
			continue
		}
		if !w.npcsShareLocked(npc, o) || dist3(npc.X, npc.Y, npc.Z, o.X, o.Y, o.Z) > r || !w.kinOfLocked(npc, o, faction) {
			continue
		}
		if !w.npcSeesLocked(npc, o.World, o.X, o.Y, o.Z) {
			continue
		}
		consider(o)
	}
	return best
}

// kinHealIdleTicks: a healer out of its own fight looks for kin to heal every
// this many ticks (a second at the world's 100 ms).
const kinHealIdleTicks = 10

// kinHealWatchLocked keeps the creatures whose template t heals their kin
// (the file's comment): npc among them while it fights by one. Must be called
// with the lock held.
func (w *World) kinHealWatchLocked(npc *NPC, t *ClassTemplate) {
	if t == nil || !t.kinHeals {
		delete(w.kinHealers, npc.ServerID)
		return
	}
	if w.kinHealers == nil {
		w.kinHealers = make(map[EntityID]*NPC)
	}
	w.kinHealers[npc.ServerID] = npc
}

// kinHealSystem casts each healer's heal of its kin out of its own fight (the
// file's comment). Every tick (it looks every kinHealIdleTicks); must be called
// with the lock held.
func (w *World) kinHealSystem(now time.Time) {
	if len(w.kinHealers) == 0 || w.TickCount%kinHealIdleTicks != 0 {
		return
	}
	for id, npc := range w.kinHealers {
		if w.NPCs[id] != npc || npc.tmpl == nil || !npc.tmpl.kinHeals {
			delete(w.kinHealers, id) // gone from the world, made again, or its template changed
			continue
		}
		if npc.Dead || npc.Owner != 0 || npc.SpawnOnly || npc.CurrentEnemy != 0 || npc.castHeld(now) ||
			now.Before(npc.kinIdleNext) {
			continue
		}
		t := npc.tmpl
		for i := range t.abilities {
			a := &t.abilities[i]
			if a.Kind != database.AbilityHeal || a.Target != database.AbilityOnKin || npc.Level < a.From ||
				npc.coolingDown(a, now) {
				continue
			}
			on := w.kinHealTargetLocked(npc, a) // one of its kin fighting, never itself
			if on == nil {
				continue // none hurt: it waits
			}
			npc.kinIdleNext = now.Add(a.every)
			if c := a.ChanceOr(); c < 100 && templateIntn(100) >= c {
				break
			}
			if w.templateCastLocked(npc, i, on.ServerID, now) {
				// its cast made it a template's fight (castHeld): the ai set's
				// next visit ends it, as any fight's with no enemy (npcsets.go)
				w.wakeNPCLocked(npc)
				break
			}
		}
	}
}
