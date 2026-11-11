package entity

import (
	"fmt"
	"log"
	"math/rand"
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
		w.bossMomentLocked(npc, &ms[i])
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
	}
	return "Mind Drain"
}

// bossMomentLocked is one moment of npc's. Must be called with the lock held.
func (w *World) bossMomentLocked(npc *NPC, m *database.TemplateAbility) {
	switch m.Kind {
	case database.AbilitySelfDoT:
		if npc.HP > 1 {
			npc.HP = max(1, npc.HP-m.Damage)
		}
	case database.AbilityRaidHeal:
		var ps []*Player
		for id, th := range npc.Threats {
			p := w.Players[id]
			if p == nil || th <= 0 || p.IsDead() || p.World != npc.World || DistanceBetween(p, npc) > bossHealReach {
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
	}
}

// sortPlayersByID sorts players by their ids.
func sortPlayersByID(ps []*Player) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].ServerID < ps[j-1].ServerID; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}
