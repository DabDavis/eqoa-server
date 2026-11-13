package entity

import (
	"fmt"
	"log"
	"time"
)

// deathhold.go — a creature's hold at 1 hit point (database.AbilityDeathHold; the GM Jibrish's petition 1894:
// "If mr moogie hits 1 hp, he freezes and is immune to damage, unable to take damage or die for 30 seconds.
// When this happens he should /say ...").
//
// Once a fight, the first hit that would take it to 0 leaves it at 1 instead, and it holds for Hold seconds
// (30): no hit lands on it (every hit that takes a creature's hit points goes through npcTakesLocked: a
// player's melee, bow, spells and their ticks, procs and damage shields; a pet's, a mercenary's, an NPC's,
// a scripted fight's casts, a boss's tick), no heal at a hit, ward or regeneration raises it, and it does
// nothing of its own (no chase, melee round, cast, kick or moment: aiSystem passes it by), its line in /say
// (Say) as it holds. Its hate list is kept: when the time is up its fight goes on from 1 hit point, its own
// moments and casts on their clocks again, and the next hit that lands kills it, as any creature (the brief:
// after the hold it is killable). A fight's end ends a hold with it; the next fight may hold again. Never a
// pet's, an add's or a dummy's. A GM's kill is a kill.

// holdOn is a hold under way: when it began and when it ends.
type holdOn struct {
	at, until time.Time
}

// heldNow reports npc held at 1 hit point at now (the file's doc).
func (n *NPC) heldNow(now time.Time) bool {
	return n != nil && n.tfight != nil && n.tfight.held != nil && now.Before(n.tfight.held.until)
}

// holding reports npc's hold begun this fight with it still at 1 hit point or under: no heal at a hit raises
// it (hitheal.go). Past its time a hit on 1 hit point kills, so a heal there is never due.
func (n *NPC) holding() bool {
	return n != nil && n.tfight != nil && n.tfight.held != nil && n.HP <= 1
}

// npcTakesLocked is what of a hit of n lands on npc at now: nothing while it is held, its hit points less 1
// when the hit would kill it and its hold is due (which begins it), else n. Every hit that takes an NPC's hit
// points asks it first. Must be called with the lock held.
func (w *World) npcTakesLocked(npc *NPC, n int, now time.Time) int {
	if npc == nil || n <= 0 {
		return n
	}
	if npc.heldNow(now) {
		return 0
	}
	t := npc.tmpl
	if t == nil || t.deathHold == nil || npc.Dead || n < npc.HP || npc.Owner != 0 || npc.add != nil || npc.Dummy != NotDummy {
		return n
	}
	f := npc.fight()
	if f.held != nil {
		return n // once a fight
	}
	a := t.deathHold
	f.held = &holdOn{at: now, until: now.Add(time.Duration(a.HoldOr() * float64(time.Second)))}
	npc.stopMoving()
	if a.Say != "" {
		w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, a.Say) // abilitytimes.go
	}
	log.Printf("Template: %s holds at 1 hit point for %gs (a hit of %d on HP %d/%d)", npc.Name, a.HoldOr(), n, npc.HP, npc.MaxHP)
	return max(npc.HP-1, 0)
}

// deathHoldWords is #stats' words for a template's hold, "" for none.
func deathHoldWords(npc *NPC) string {
	a := npc.tmpl.deathHold
	if a == nil {
		return ""
	}
	s := fmt.Sprintf("holds at 1 hit point for %gs once a fight, frozen and taking nothing", a.HoldOr())
	if f := npc.tfight; f != nil && f.held != nil {
		if left := time.Until(f.held.until); left > 0 {
			s += fmt.Sprintf(" (held now, %.0fs left)", left.Seconds())
		} else {
			s += " (held this fight)"
		}
	}
	return s
}
