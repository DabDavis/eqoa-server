package entity

import (
	"fmt"
	"log"
	"math/rand"
	"slices"
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
// Its damage over time (Tick: the owner, 2026-11-13, "dreadnaught's aoe that
// debuffs to 700hp also has a damage over time ticks 650 with no CR, with max
// CR is ticking around 80") lands on everyone it reaches, at or below its
// figure too, after the fall: Tick a tick at no resist, TickAtCap at a resist
// at the cap (ResistCap, 315), in proportion between, on the creature's
// template's dice as its spells' ticks are (npcTickLocked), every effectTick
// (6 s) from its landing for TickFor (0: tickDuration, 24 s); a second
// landing replaces the first. On a creature its template's resists cut it and
// it brings no hate (its own side's); it may kill, the fall never does.

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
// creatures it reaches above m.SetHP taken down to it, and its damage over
// time (m.Tick) on everyone it reaches. Must be called with the lock held.
func (w *World) setHPMomentLocked(npc *NPC, m *database.TemplateAbility, now time.Time) {
	to := m.SetHP
	if to < 1 {
		return
	}
	ps, ns := w.setHPReachLocked(npc, m, now)
	if m.Tick <= 0 {
		ps = slices.DeleteFunc(ps, func(p *Player) bool { return p.HP <= to })
		ns = slices.DeleteFunc(ns, func(n *NPC) bool { return n.HP <= to })
	}
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
		p.LastCombatTime = now
		if p.HP <= to {
			continue
		}
		taken := p.HP - to
		p.HP = to
		p.StatsDirty = true
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
		n.LastCombatTime = now
		if n.HP <= to {
			continue
		}
		log.Printf("Template: %s's %s takes the creature %q from %d to %d (HP %d/%d)", npc.Name, name, n.Name, n.HP, to, to, n.MaxHP)
		n.HP = to
	}
	if m.Tick > 0 {
		for _, p := range ps {
			w.bossDoTOnPlayerLocked(npc, m, p, now)
		}
		for _, n := range ns {
			w.bossDoTOnCreatureStartLocked(npc, m, n, now)
		}
	}
}

// setHPDoTSpellID is the spell id a sethp's damage over time is kept under on
// whom it lands (ours; no record: a merc's own are its spells' ids negated,
// all above -1,000,000), so a second landing replaces the first.
const setHPDoTSpellID = -1_990_001

// bossDoTFor is a sethp's damage over time's time: its own, else a tick line's
// with no time.
func bossDoTFor(m *database.TemplateAbility) time.Duration {
	if m.TickFor > 0 {
		return time.Duration(m.TickFor * float64(time.Second))
	}
	return tickDuration
}

// bossDoTOnPlayerLocked puts a sethp's damage over time on p: m.Tick a tick,
// its resist (m.Resist) taking its share as a creature's spell's does
// (npcTickLocked, on npc's template's dice), but at the cap its own share
// (1 - TickAtCap/Tick), its first tick now. Must be called with the lock held.
func (w *World) bossDoTOnPlayerLocked(npc *NPC, m *database.TemplateAbility, p *Player, now time.Time) {
	ae := &ActiveEffect{SpellID: setHPDoTSpellID, Name: momentName(m), Caster: npc.ServerID, Until: now.Add(bossDoTFor(m)),
		NextTick: now, HPTick: -m.Tick, npcFoe: npc.ServerID, npcResist: shieldResistStat(m.Resist), npcAtCap: bossDoTAtCap(m)}
	ae.Rules.Harm = true
	w.breakInvisLocked(p, "a hit") // invis.go
	w.addEffectLocked(p, ae, now)  // its first tick now (npcTickLocked)
}

// bossDoTAtCap is the share of a sethp's tick a resist at the cap takes: one
// less its tick there over its tick, else the server's (0).
func bossDoTAtCap(m *database.TemplateAbility) float64 {
	if m.Tick <= 0 || m.TickAtCap <= 0 {
		return 0
	}
	return 1 - float64(m.TickAtCap)/float64(m.Tick)
}

// bossDoTOnCreatureStartLocked puts a sethp's damage over time on the creature
// n, in place of one of the same before, cut by n's template resists of its
// element (npcDoTTickLocked), its first tick now. Must be called with the lock
// held.
func (w *World) bossDoTOnCreatureStartLocked(npc *NPC, m *database.TemplateAbility, n *NPC, now time.Time) {
	n.Effects = slices.DeleteFunc(n.Effects, func(e *ActiveEffect) bool { return e.SpellID == setHPDoTSpellID })
	ae := &ActiveEffect{SpellID: setHPDoTSpellID, Name: momentName(m), Caster: npc.ServerID, Until: now.Add(bossDoTFor(m)),
		NextTick: now, HPTick: -m.Tick, npcCaster: npc.ServerID, bossDoT: true,
		Rules: EffectRules{Resist: shieldResistStat(m.Resist), Harm: true}}
	n.Effects = append(n.Effects, ae)
	if w.affected == nil {
		w.affected = make(map[EntityID]*NPC)
	}
	w.affected[n.ServerID] = n
	w.tickNPCEffectLocked(n, ae, now)
}

// bossDoTOnCreatureLocked is a tick of a sethp's damage over time on the
// creature t: n off its hit points, no hate (its own side's as much as a
// pet's), its death at nothing. Must be called with the lock held.
func (w *World) bossDoTOnCreatureLocked(src, t *NPC, n int, name string) {
	t.HP = max(t.HP-n, 0)
	log.Printf("Template: %s's %q ticks on %s for %d (HP %d/%d)", src.Name, name, t.Name, n, t.HP, t.MaxHP)
	if t.HP > 0 {
		return
	}
	t.Effects = nil
	w.killNPC(t)
	w.dropAggroLocked(t.ServerID)
	log.Printf("Combat: %s killed by %s's %q", t.Name, src.Name, name)
}

// setHPReachLocked is whom npc's sethp m reaches now, whatever their hit
// points (the file's doc), each list in id order. Must be called with the
// lock held.
func (w *World) setHPReachLocked(npc *NPC, m *database.TemplateAbility, now time.Time) ([]*Player, []*NPC) {
	var ps []*Player
	var ns []*NPC
	creature := func(n *NPC) bool {
		return n != nil && n != npc && !n.Dead && n.HP > 0 && n.NPCType&0x80 == 0 && n.Dummy == NotDummy &&
			!n.hiddenAt(now) && w.npcsShareLocked(n, npc)
	}
	if m.Radius <= 0 {
		if p := w.Players[npc.CurrentEnemy]; p != nil {
			if !p.IsDead() && !p.respawnPending() && w.npcSharesLocked(npc, p) {
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
			if p.IsDead() || !w.npcSharesLocked(npc, p) || !w.noticeableByLocked(npc, p) ||
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
