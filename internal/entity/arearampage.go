package entity

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/eqoa/server/internal/database"
)

// arearampage.go — a creature's rampage: its melee on everyone round it (database.AbilityRampage;
// the owner, 2026-11-17: "lets introduce rampage the area of effect melee damage ability with 10m").
// Not an enrage's rampage (enrage.go: a share of each hit on the second of its hate list), which
// keeps its own name and field.
//
// A unit of the world is a metre (a man's rig is 1.83 tall, his walk's capsule 2), so the owner's
// 10 m is a radius of 10 (database.DefaultAreaRampageRadius), from the creature's centre, as its
// area spells' radii are.
//
// When it goes, in its fight while what it fights stands in its reach (a player, or a player's pet
// or mercenary; never a creature a trigger set it on):
//
//   - who it reaches: the one it fights (in its reach, whom its rounds are at), and every player,
//     pet and mercenary within its radius that it has a clear line to from its own heights
//     (sight.go: npcAreaPlace, as its area spells and a sethp); the players a creature notices
//     (aggro.go) and one in safe mode (/testinvis or a GM's #aggro off) only when it is part of this
//     fight (areaTakesPlayerLocked, safemode.go); none across the water's edge from it, dead, or on the way back from
//     a death. No cap. Never a creature that is not a player's: its own side is not hit;
//   - each of them takes one full melee hit of its, rolled on its own as its rounds' hits are
//     (npcMeleeOnPlayerLocked: the player's AC, AGI and archetype, defence.go; its template's
//     melee_pct; the balance; the mitigation; a pet's own, npcMeleeOnPetLocked), at damage_pct
//     (default 100: the hit as it is). Melee: armour counts, no resist does. CLIENT's NPC melee has
//     no miss (defence.go: "The hits a round lands are always all of them"), so each is a hit,
//     its figure its own; its procs and the target's damage shields answer it as any melee hit;
//   - CLIENT sees it as one swing: a 0x00B1 of its round's swing (its attack animation; fx for
//     another) at the one it fights, the others named (the nearest seven: areaSeenLocked), then
//     each player's 0x00DB of the hit on it, and a line to each player it reaches: "<creature>
//     goes on a RAMPAGE!". Say is its line in /say round it as it goes.
//
// When it goes: with every, on a clock of its fight, the first first seconds in and then every
// every seconds, each due time going off at chance percent (0: always), due only while it is at
// what it fights (a due time waits for its next turn in reach, and for a cast's hold to end); with
// no every, chance percent of its melee rounds, rolled as each round begins, the rampage landing
// at the round's first hit (none in a round that is a crit, critround.go, and none owed by a round
// that went by out of reach). A creature's add (summon.go) does not rampage, as no add has its
// boss's moments.

// areaRampageIntn is the rampage's own chance (a test's own).
var areaRampageIntn = rand.Intn

// rampagePct is a melee hit n at pct percent (100: as it is), at least 1.
func rampagePct(n, pct int) int {
	if pct == 100 || pct <= 0 || n <= 0 {
		return n
	}
	return max(1, int(math.Round(float64(n)*float64(pct)/100)))
}

// areaRampageName is what its lines call it.
func areaRampageName(a *database.TemplateAbility) string {
	if a.Name != "" {
		return a.Name
	}
	return database.DefaultRampageName
}

// areaRampageTurnLocked is npc's area rampage at its turn at target, in its reach (the file's doc):
// its clock or its round's chance, and the rampage when it is due. Must be called with the lock
// held.
func (w *World) areaRampageTurnLocked(npc *NPC, target EntityID, now time.Time) {
	t := npc.tmpl
	if t == nil || t.areaRampage == nil || npc.add != nil || npc.Dead || npc.Owner != 0 || npc.Dummy != NotDummy {
		return
	}
	if p := w.Players[target]; p == nil {
		if pet := w.NPCs[target]; pet == nil || pet.Owner == 0 {
			return // a scripted fight with a creature (guards.go): no rampage
		}
	}
	a := t.areaRampage
	f := npc.fight()
	if a.Every > 0 {
		if f.rampNext.IsZero() {
			f.rampNext = now.Add(time.Duration(a.First * float64(time.Second)))
		}
		if now.Before(f.rampNext) || npc.castHeld(now) {
			return
		}
		f.rampNext = now.Add(time.Duration(a.Every * float64(time.Second)))
		if c := a.Chance; c > 0 && c < 100 && areaRampageIntn(100) >= c {
			return
		}
		w.areaRampageLocked(npc, target, now)
		return
	}
	if npc.RoundStart.IsZero() || npc.RoundCrit {
		return
	}
	if !npc.RoundStart.Equal(f.rampRound) {
		f.rampRound, f.rampAt = npc.RoundStart, time.Time{}
		if areaRampageIntn(100) < a.Chance {
			_, _, _, hits := npcRound(npc)
			f.rampAt = npc.RoundStart.Add(hits[0])
		}
	}
	if f.rampAt.IsZero() || now.Before(f.rampAt) {
		return
	}
	late := now.Sub(f.rampAt)
	f.rampAt = time.Time{}
	if late > templateSpecialAfter || npc.castHeld(now) {
		return // its round went by out of reach, or casting: the rampage with it
	}
	w.areaRampageLocked(npc, target, now)
}

// areaRampageLocked is one rampage of npc's at target, whom it fights: its swing, its line and a
// melee hit on everyone it reaches (the file's doc). It reports how many it hit. Must be called
// with the lock held.
func (w *World) areaRampageLocked(npc *NPC, target EntityID, now time.Time) int {
	a := npc.tmpl.areaRampage
	ps, pets := w.areaRampageReachLocked(npc, target, now)
	if len(ps) == 0 && len(pets) == 0 {
		return 0
	}
	others := make([]EntityID, 0, len(ps)+len(pets))
	for _, p := range ps {
		if p.ServerID != target {
			others = append(others, p.ServerID)
		}
	}
	for _, n := range pets {
		if n.ServerID != target {
			others = append(others, n.ServerID)
		}
	}
	_, delay, fx, _ := npcRound(npc)
	if a.FX != 0 {
		fx = a.FX
	}
	w.broadcastNPCSwingLocked(npc, target, uint32(delay/time.Millisecond), fx, others...)
	if a.Say != "" {
		w.sayNearLocked(npc.Name, npc.World, npc.X, npc.Z, a.Say)
	}
	name := areaRampageName(a)
	line := fmt.Sprintf("%s goes on a %s!", npc.Name, upperASCII(name))
	for _, p := range ps {
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			w.sendColoredLocked(sess, line, duelOrange)
		}
	}
	log.Printf("Template: %s's %s (radius %g): %d players, %d pets and mercenaries", npc.Name, name, a.RadiusOr(), len(ps), len(pets))
	pct := a.DamageOr()
	hit := 0
	for _, p := range ps {
		if npc.Dead {
			return hit
		}
		if p.IsDead() {
			continue
		}
		w.npcMeleeOnPlayerLocked(npc, p, pct, true, now)
		hit++
	}
	for _, n := range pets {
		if npc.Dead {
			return hit
		}
		if n.Dead {
			continue
		}
		w.npcMeleeOnPetLocked(npc, n, pct, true, now)
		hit++
	}
	return hit
}

// upperASCII is s in capitals ("Rampage": "RAMPAGE").
func upperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// areaRampageReachLocked is whom npc's rampage reaches now (the file's doc): the players and the
// pets and mercenaries, each list in id order. Must be called with the lock held.
func (w *World) areaRampageReachLocked(npc *NPC, target EntityID, now time.Time) ([]*Player, []*NPC) {
	r := float32(npc.tmpl.areaRampage.RadiusOr())
	place := npcAreaPlace(npc, npc.X, npc.Y, npc.Z, false) // its line from its own heights too (sight.go)
	var ps []*Player
	var pets []*NPC
	seen := map[EntityID]bool{}
	player := func(p *Player, main bool) {
		if seen[p.ServerID] {
			return
		}
		seen[p.ServerID] = true
		if !w.areaTakesPlayerLocked(npc, p) { // safemode.go
			return
		}
		if !main && (dist3(p.X, p.Y, p.Z, npc.X, npc.Y, npc.Z) > r || !w.reachesLocked(place, p.X, p.Y, p.Z)) {
			return
		}
		ps = append(ps, p)
	}
	pet := func(n *NPC, main bool) {
		if seen[n.ServerID] {
			return
		}
		seen[n.ServerID] = true
		if n == npc || n.Owner == 0 || n.Dead || n.HP <= 0 || n.World != npc.World || n.hiddenAt(now) || !w.npcsShareLocked(n, npc) {
			return
		}
		if !main && (dist3(n.X, n.Y, n.Z, npc.X, npc.Y, npc.Z) > r || !w.reachesLocked(place, n.X, n.Y, n.Z)) {
			return
		}
		pets = append(pets, n)
	}
	// the one it fights, in its reach whatever its radius
	if p := w.Players[target]; p != nil {
		player(p, true)
	} else if n := w.NPCs[target]; n != nil {
		pet(n, true)
	}
	if w.SpatialIndex != nil {
		for _, id := range w.SpatialIndex.QueryRadius(npc.X, npc.Z, r) {
			if p := w.Players[id]; p != nil {
				player(p, false)
			} else if n := w.NPCs[id]; n != nil {
				pet(n, false)
			}
		}
	}
	sortPlayersByID(ps)
	sort.Slice(pets, func(i, j int) bool { return pets[i].ServerID < pets[j].ServerID })
	return ps, pets
}

// areaRampageWords is #stats' words for a template's area rampage: its radius, when it goes and
// its share, and its clock's next on a fight.
func areaRampageWords(npc *NPC, now time.Time) string {
	a := npc.tmpl.areaRampage
	if a == nil || npc.add != nil {
		return ""
	}
	s := fmt.Sprintf("rampage within %g", a.RadiusOr())
	if a.Every > 0 {
		s += fmt.Sprintf(" every %gs from %gs into a fight", a.Every, a.First)
		if a.Chance > 0 && a.Chance < 100 {
			s += fmt.Sprintf(" at %d%%", a.Chance)
		}
		if f := npc.tfight; f != nil && !f.rampNext.IsZero() && now.Before(f.rampNext) {
			s += fmt.Sprintf(" (next in %.0fs)", f.rampNext.Sub(now).Seconds())
		}
	} else {
		s += fmt.Sprintf(" in %d%% of its rounds", a.Chance)
	}
	return s + fmt.Sprintf(", each a melee hit at %d%%", a.DamageOr())
}
