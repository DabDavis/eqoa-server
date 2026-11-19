package entity

import (
	"fmt"
	"log"
	"time"
)

// metalv5.go — the chain's last duel, Metal Moogie V5's script, and what a metal duel's adds are (work board
// #688, a GM's, 2026-11-10; "a GM decided all of this").
//
// V5's template heals him whole the first time he falls below half (its Self Repair: a heal of his share,
// every and cooldown of its own, so once a duel: each duel's V5 is a new NPC). The rest is this file's, on
// the world's tick while his duel is on (metalMoogieSystem, after its fight has begun):
//
//   - after that heal, the next time his hit points fall below half, once: MetalV5Hacks in /say with the
//     challenger's name, heard as Mr. Moogie's is (MetalSayHeard);
//   - the first time they fall below a tenth, once: MetalV5Complete in /say, and his hit points to his most
//     at once;
//   - MetalV5Broken after that: MetalV5Malfunction in /say, his hit points set to a hundredth of his most
//     (at least 1: no death, no hit, no hate), and from then on nothing heals him (NPC.noHeal: his hit heal,
//     his regeneration in a fight and out of one, his template's heals and taps' returns, a veil's) until he
//     dies;
//   - the duel ending inside those 10 s (a defeat, a flight, the time, a death) leaves nothing more to come:
//     the state is the duel's.
//
// His gearlings (V4's and V5's summon below a quarter: once a fight, so his climb back to whole calls none
// again) are the duel's as he is: summonLocked gives an add of a duel's creature its challenge, so every rule
// of his is theirs (another's hits, spells, pets and hate refused or passed by in the same lines, the NPC bit
// to every other client); they die with him (masterdeath.go) and go at any other end of the duel
// (duelAddsGoLocked), with every root of theirs on the challenger.

// V5's lines (a GM's words) and the malfunction's wait.
const (
	MetalV5Hacks       = "%s! HACKS! Nobody breaks my repairs that fast. I am reporting you to the GMs!"
	MetalV5Complete    = "COMPLETE HEAL ACTIVE"
	MetalV5Malfunction = "MALFUNCTION: HEAL CORE OVERLOADED"
	MetalV5Broken      = 10 * time.Second
	// metalV5Below and metalV5Last are the shares of his hit points (percent) the hacks line and the
	// complete heal go off under; metalV5Left the share the malfunction leaves him
	metalV5Below = 50
	metalV5Last  = 10
	metalV5Left  = 1
)

// metalV5 is V5's script's state in his duel: the hacks line said, the complete heal gone off and when its
// malfunction comes (zero: not yet), the malfunction come.
type metalV5 struct {
	hacked       bool
	completed    bool
	breaksAt     time.Time
	malfunctions bool
}

// metalV5Locked is V5's script's tick (the file's doc). Must be called with the lock held.
func (w *World) metalV5Locked(d *metalDuel, npc *NPC, now time.Time) {
	s := &d.v5
	if npc.Dead || npc.MaxHP <= 0 {
		return
	}
	say := func(text string) {
		n := w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, text) // abilitytimes.go
		log.Printf("Metal Moogie: %s says %q to %d players (HP %d/%d)", npc.Name, text, n, npc.HP, npc.MaxHP)
	}
	if s.completed && !s.malfunctions && !now.Before(s.breaksAt) {
		s.malfunctions = true
		npc.HP = max(1, npc.MaxHP*metalV5Left/100)
		npc.noHeal = true
		say(MetalV5Malfunction)
		return
	}
	if !s.hacked && npc.selfHealed && npc.HP*100 < npc.MaxHP*metalV5Below {
		s.hacked = true
		say(fmt.Sprintf(MetalV5Hacks, d.c.name))
	}
	if !s.completed && npc.HP*100 < npc.MaxHP*metalV5Last {
		s.completed, s.breaksAt = true, now.Add(MetalV5Broken)
		healed := npc.MaxHP - npc.HP
		npc.HP = npc.MaxHP
		say(MetalV5Complete)
		if p := w.Players[d.c.player]; p != nil {
			w.tellNPCHealLocked(w.findSessionByEntityIDLocked(p.ServerID), npc, healed)
		}
	}
}

// healBarred reports an NPC nothing may heal now: Metal Moogie V5 after his malfunction (the file's doc).
func (n *NPC) healBarred() bool { return n != nil && n.noHeal }

// duelAddsGoLocked takes the adds of a metal duel's creature out of the world, the living ones (his corpse's
// adds died with him already), and every root a creature of the duel put on its challenger: the duel is
// over. Must be called with the lock held.
func (w *World) duelAddsGoLocked(d *metalDuel) {
	for _, a := range w.adds {
		if a.challenge != d.c || a.Dead || w.NPCs[a.ServerID] != a {
			continue
		}
		w.dropAggroLocked(a.ServerID)
		w.removeNPCLocked(a)
		log.Printf("Metal Moogie: %s (%d) goes with %s's duel", a.Name, a.ServerID, d.name)
	}
	if p := w.Players[d.c.player]; p != nil && p == d.c.who {
		w.creatureRootsOffLocked(p, func(id EntityID) bool {
			n := w.NPCs[id]
			return n == nil || n.challenge == d.c
		})
	}
}
