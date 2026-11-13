package entity

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// bossmoments.go — a boss's moments on clocks of their own, casting no spell
// (database.BossMomentKind; the Cazic-Thule trials, 2026-11-11, work board
// 616's code asks 2, 4 and 5, the Trial of Contention's Watchman from the t196
// walkthrough): "Casts a MASSIVE DoT on himself when he spawns" (a self dot:
// its damage off its own hit points each time, never below 1, hate to no
// one), "Sometimes casts a 10,000 heal on your raid party members" (a raid
// heal: its damage onto count players of its hate list, at random, alive and
// within bossHealReach), "Casts a 0 Power Debuff on Target who has agro when
// the Watchman is engaged" (power to 0: the player it fights, its power set
// to 0). Each comes every Every seconds of its fight, the first First into it,
// its clocks begun with the fight; its name (none: its kind's) in the lines.
//
// A sethp (the owner, 2026-11-13: "dreadnaughts are supposed to have an AoE
// which reduces hp to 700 for players and mobs. Cazic Thule sets to 10hp for
// his debuff"; QuestMasters' Dreadnaught and Siliskor guides, 2006: "debuffed
// by the dreadnaught to 700hps", "Siliskor attacks with an AE debuff ...
// regardless of class, level or resists"): the hit points of whoever it
// reaches taken down to SetHP, the same moment for all of them. Never raised
// (one at or below it is left as it is) and never to death; no resist. With
// no Radius it is the one its creature fights (a player, or a pet or a
// creature in a scripted fight); with one, everyone round the creature within
// it that the creature has a clear line to (sight.go: the guides' boulders and
// the elephant statue) and stands where it does: the players a creature
// notices (as its area spells', templateAreaLocked) and every creature alive,
// pets, mercenaries and its own side alike, never the creature itself, never
// one no one may attack (the NPC bit), a training dummy or one out of view.
// A player sees the difference as a hit: its 0x00DB from the creature, its
// bar from the stat channel, a line. Chance is the share of its due times it
// goes off (0: every one), any moment's: Cazic Thule's random death touch.

// bossHealReach is how near its creature a raid heal's players stand.
const bossHealReach float32 = 60

// bossMomentIntn is the raid heal's pick (a test's own).
var bossMomentIntn = rand.Intn

// bossMomentsLocked runs npc's moments due in its fight f. Must be called with
// the lock held.
func (w *World) bossMomentsLocked(npc *NPC, f *templateFight, now time.Time) {
	ms := npc.tmpl.moments
	if len(f.momentNext) != len(ms) {
		f.momentNext = make([]time.Time, len(ms))
		for i, m := range ms {
			f.momentNext[i] = now.Add(time.Duration(m.First * float64(time.Second)))
		}
	}
	for i := range ms {
		if now.Before(f.momentNext[i]) {
			continue
		}
		f.momentNext[i] = now.Add(time.Duration(ms[i].Every * float64(time.Second)))
		if c := ms[i].Chance; c > 0 && c < 100 && bossMomentIntn(100) >= c {
			continue
		}
		w.bossMomentLocked(npc, &ms[i], now)
	}
}

// momentName is what a moment's lines call it.
func momentName(m *database.TemplateAbility) string {
	if m.Name != "" {
		return m.Name
	}
	switch m.Kind {
	case database.AbilitySelfDoT:
		return "Wracking Blood"
	case database.AbilityRaidHeal:
		return "Gift of Contention"
	case database.AbilitySetHP:
		return "Wither"
	}
	return "Mind Drain"
}

// bossMomentLocked is one moment of npc's. Must be called with the lock held.
func (w *World) bossMomentLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	switch m.Kind {
	case database.AbilitySelfDoT:
		if npc.HP > 1 {
			npc.HP = max(1, npc.HP-m.Damage)
		}
	case database.AbilityRaidHeal:
		var ps []*Player
		for id, th := range npc.Threats {
			p := w.Players[id]
			if p == nil || th <= 0 || p.IsDead() || !w.npcSharesLocked(npc, p) || DistanceBetween(p, npc) > bossHealReach {
				continue
			}
			ps = append(ps, p)
		}
		// the map's order is random: a stable order, then the pick
		sortPlayersByID(ps)
		for k := 0; k < m.Count && len(ps) > 0; k++ {
			j := bossMomentIntn(len(ps))
			p := ps[j]
			ps = append(ps[:j], ps[j+1:]...)
			top := p.MaxHPNow()
			if p.HP >= top {
				continue
			}
			p.HP = min(p.HP+m.Damage, top)
			p.StatsDirty = true
			if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
				w.sendChatToSession(sess, fmt.Sprintf("%s's %s heals you for %d.", npc.Name, momentName(m), m.Damage))
			}
		}
	case database.AbilityPowerZero:
		p := w.Players[npc.CurrentEnemy]
		if p == nil || p.IsDead() || p.Power == 0 {
			return
		}
		p.Power = 0
		p.StatsDirty = true
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s takes all your power.", npc.Name, momentName(m)))
		}
		log.Printf("Template: %s's %s: %s's power to 0", npc.Name, momentName(m), p.Name)
	case database.AbilitySetHP:
		w.setHPMomentLocked(npc, m, now)
	}
}

// setHPMomentLocked is a sethp of npc's (the file's doc): the players and the
// creatures it reaches above m.SetHP taken down to it. Must be called with the
// lock held.
func (w *World) setHPMomentLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	to := m.SetHP
	if to < 1 {
		return
	}
	ps, ns := w.setHPReachLocked(npc, m, now)
	if len(ps) == 0 && len(ns) == 0 {
		return
	}
	name := momentName(m)
	others := make([]EntityID, 0, len(ps)+len(ns))
	for _, p := range ps {
		others = append(others, p.ServerID)
	}
	for _, n := range ns {
		others = append(others, n.ServerID)
	}
	fx := m.FX
	if fx == 0 {
		fx = templateSpecialFX
	}
	if m.Radius > 0 {
		w.broadcastNPCCastLocked(npc, npc.ServerID, 0, fx, others...)
	} else {
		w.broadcastNPCSwingLocked(npc, others[0], 0, fx)
	}
	for _, p := range ps {
		taken := p.HP - to
		p.HP = to
		p.StatsDirty = true
		p.LastCombatTime = now
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			from := w.objectIDForEntityLocked(sess, npc.ServerID)
			if from == 0 {
				from = sess.PlayerObjectID() // the creature out of its view: the figure still shown
			}
			w.sendDamageIndication(sess, from, -taken, sess.PlayerObjectID(), 0) // the creature's on the player: untagged
			w.sendChatToSession(sess, fmt.Sprintf("%s's %s takes %d from you, leaving you %d hit points.", npc.Name, name, taken, to))
		}
		log.Printf("Template: %s's %s takes %s from %d to %d (HP %d/%d)", npc.Name, name, p.Name, p.HP+taken, to, p.HP, p.MaxHPNow())
	}
	for _, n := range ns {
		log.Printf("Template: %s's %s takes the creature %q from %d to %d (HP %d/%d)", npc.Name, name, n.Name, n.HP, to, to, n.MaxHP)
		n.HP = to
		n.LastCombatTime = now
	}
}

// setHPReachLocked is whom npc's sethp m reaches now above its hit points (the
// file's doc), each list in id order. Must be called with the lock held.
func (w *World) setHPReachLocked(npc *NPC, m *database.TemplateAbility, now time.Time) ([]*Player, []*NPC) {
	to := m.SetHP
	var ps []*Player
	var ns []*NPC
	creature := func(n *NPC) bool {
		return n != nil && n != npc && !n.Dead && n.HP > to && n.NPCType&0x80 == 0 && n.Dummy == NotDummy &&
			!n.hiddenAt(now) && w.npcsShareLocked(n, npc)
	}
	if m.Radius <= 0 {
		if p := w.Players[npc.CurrentEnemy]; p != nil {
			if !p.IsDead() && p.HP > to && w.npcSharesLocked(npc, p) {
				ps = append(ps, p)
			}
		} else if n := w.NPCs[npc.CurrentEnemy]; creature(n) {
			ns = append(ns, n)
		}
		return ps, ns
	}
	if w.SpatialIndex == nil {
		return nil, nil
	}
	r := float32(m.Radius)
	place := areaPlace{world: npc.World, caster: [3]float32{npc.X, npc.Y, npc.Z}, centre: [3]float32{npc.X, npc.Y, npc.Z}}
	for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, r) {
		if p := w.Players[id]; p != nil {
			if p.IsDead() || p.HP <= to || !w.npcSharesLocked(npc, p) || !w.noticeableByLocked(npc, p) ||
				(w.safeLocked(p) && !inFightLocked(npc, p)) || w.apartLocked(p, npc) {
				continue
			}
			if dist3(p.X, p.Y, p.Z, npc.X, npc.Y, npc.Z) <= r && w.reachesLocked(place, p.X, p.Y, p.Z) {
				ps = append(ps, p)
			}
			continue
		}
		if n := w.NPCs[id]; creature(n) {
			if dist3(n.X, n.Y, n.Z, npc.X, npc.Y, npc.Z) <= r && w.reachesLocked(place, n.X, n.Y, n.Z) {
				ns = append(ns, n)
			}
		}
	}
	sortPlayersByID(ps)
	sort.Slice(ns, func(i, j int) bool { return ns[i].ServerID < ns[j].ServerID })
	return ps, ns
}

// sortPlayersByID sorts players by their ids.
func sortPlayersByID(ps []*Player) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].ServerID < ps[j-1].ServerID; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}
