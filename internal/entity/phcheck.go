package entity

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

// phcheck.go — what a GM's or a sub GM's #phcheck reads (the owner,
// 2026-11-26: "GMs and sub GMs ... a command to check placeholders"): the
// placeholder spawns that can roll a named, read off the world as it stands.
// Read-only: it spawns, kills and changes nothing.
//
// A spot rolls a named two ways, both read here:
//
//   - table npcrare (npcrare.go): the spot comes back as its rare at the rule's
//     chance, while no creature of the rare's name stands alive anywhere;
//   - a GM's /ph (campspawns, campspawn.go): the spot pops one of its kinds at
//     its odds. A binding's named is the kind (or kinds) of the lowest odds,
//     when they are below the highest: "desert marauder 31, desert pillager 33,
//     desert henchman 33, Sothimus the Brigand 3" rolls Sothimus at 3%; even
//     odds (a camp of child and pariah of Xrag) have no named.
//
// A spot under both for one named rolls it at r + (1-r)*k (npcrare's roll
// first, then the kind it just popped: respawnNPCLocked).

// PHSpot is one placeholder spot of a named, as it stands now.
type PHSpot struct {
	ID EntityID
	// Name is the kind it stands as now; Own its row's.
	Name, Own string
	// World and the spawn point: where it comes back.
	World   uint8
	X, Y, Z float32
	// Dead: no creature on the spot now. Back is when it comes back (zero: a
	// GM holds it, or no death time); Corpse: its corpse still lies (Back then
	// counts the decay too).
	Dead, Corpse, Held bool
	Back               time.Time
	// AsNamed: it stands as the named now.
	AsNamed bool
	// Chance is its percent of rolling the named at its next return.
	Chance float64
	// Odds is its roll as a GM reads it: "desert marauder 31%, ..." or the
	// npcrare rule's "Lord Koreshaa 10%".
	Odds string
}

// PHUp is a creature of a named's name alive in the world.
type PHUp struct {
	ID      EntityID
	World   uint8
	X, Y, Z float32
	// OnSpot: it stands on one of the named's placeholder spots.
	OnSpot bool
}

// PHGroup is a named and its placeholder spots.
type PHGroup struct {
	Named string
	Key   string // database.NPCClassKey of Named
	Spots []PHSpot
	Up    []PHUp
	// OneAtATime: an npcrare rule of the group rolls only while none of the
	// name is up.
	OneAtATime bool
}

// phRoll is one named one spot rolls: its name and chance, and whether it is
// npcrare's (one at a time).
type phRoll struct {
	name   string
	chance float64
	rare   bool
}

// phOddsText is kinds at weights (nil: even) as #ph and #phcheck read them.
func phOddsText(names []string, weights []int) string {
	parts := make([]string, len(names))
	for i, n := range names {
		if weights != nil {
			parts[i] = n + " " + pctText(float64(weights[i]))
		} else {
			parts[i] = n + " " + pctText(100/float64(len(names)))
		}
	}
	return strings.Join(parts, ", ")
}

// pctText is a percent as a GM reads it, to a tenth: "3%", "14.5%".
func pctText(p float64) string {
	return strconv.FormatFloat(math.Round(p*10)/10, 'f', -1, 64) + "%"
}

// phRollsLocked is each spot's rolls of a named, and its odds text. Must be
// called with the lock held.
func (w *World) phRollsLocked() (map[EntityID][]phRoll, map[EntityID]string) {
	rolls := map[EntityID][]phRoll{}
	odds := map[EntityID]string{}
	for id, cs := range w.camps.spawns {
		if len(cs.kinds) < 2 {
			continue
		}
		names := make([]string, 0, len(cs.kinds))
		for _, k := range cs.kinds {
			if r := w.camps.recs[k]; r != nil {
				names = append(names, r.rec.NPCName)
			} else {
				names = append(names, "?")
			}
		}
		weights := cs.weights
		if weights != nil && len(weights) != len(names) {
			weights = nil
		}
		odds[id] = phOddsText(names, weights)
		if weights == nil {
			continue // even odds: no named
		}
		lo, hi, sum := weights[0], weights[0], 0
		for _, v := range weights {
			lo, hi, sum = min(lo, v), max(hi, v), sum+max(v, 0)
		}
		if lo >= hi || sum <= 0 {
			continue
		}
		for i, v := range weights {
			if v == lo && names[i] != "?" {
				rolls[id] = append(rolls[id], phRoll{name: names[i], chance: 100 * float64(v) / float64(sum)})
			}
		}
	}
	for id, r := range w.rares {
		name := r.rare.rec.NPCName
		key := database.NPCClassKey(name)
		merged := false
		for i, x := range rolls[id] {
			if database.NPCClassKey(x.name) == key { // both rules: npcrare first, then the kind
				rolls[id][i].chance = r.chance + (100-r.chance)*x.chance/100
				rolls[id][i].rare = true
				merged = true
			}
		}
		if !merged {
			rolls[id] = append(rolls[id], phRoll{name: name, chance: r.chance, rare: true})
		}
		t := "rare " + name + " " + pctText(r.chance)
		if odds[id] != "" {
			t += ", else " + odds[id]
		}
		odds[id] = t
	}
	return rolls, odds
}

// PHGroups is every named a placeholder spot can roll, with its spots and
// the creatures of its name up now, by name. Read-only.
func (w *World) PHGroups(now time.Time) []PHGroup {
	w.mu.RLock()
	defer w.mu.RUnlock()
	rolls, odds := w.phRollsLocked()
	groups := map[string]*PHGroup{}
	for id, rs := range rolls {
		npc := w.NPCs[id]
		if npc == nil {
			continue
		}
		own := npc.Name
		if r := w.camps.recs[int(id)]; r != nil {
			own = r.rec.NPCName
		} else if rr := w.rares[id]; rr != nil {
			own = rr.own.rec.NPCName
		}
		for _, r := range rs {
			key := database.NPCClassKey(r.name)
			g := groups[key]
			if g == nil {
				g = &PHGroup{Named: r.name, Key: key}
				groups[key] = g
			}
			g.OneAtATime = g.OneAtATime || r.rare
			s := PHSpot{
				ID: id, Name: npc.Name, Own: own, World: npc.World,
				X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ,
				Dead: npc.Dead, Held: npc.GMHeld,
				AsNamed: !npc.Dead && database.NPCClassKey(npc.Name) == key,
				Chance:  r.chance, Odds: odds[id],
			}
			if npc.Dead && !npc.DeathTime.IsZero() {
				gone := npc.CorpseGone
				if gone.IsZero() {
					s.Corpse = true
					gone = npc.corpseEnds(w.corpseDecay)
				}
				if !npc.GMHeld {
					s.Back = gone.Add(npc.RespawnTime)
					if s.Back.Before(now) {
						s.Back = now // due at the next tick
					}
				}
			}
			g.Spots = append(g.Spots, s)
		}
	}
	if len(groups) == 0 {
		return nil
	}
	spots := map[EntityID]bool{}
	for _, g := range groups {
		for _, s := range g.Spots {
			spots[s.ID] = true
		}
	}
	for id, n := range w.NPCs {
		if n.Dead || n.SpawnOnly {
			continue
		}
		g := groups[database.NPCClassKey(n.Name)]
		if g == nil {
			continue
		}
		g.Up = append(g.Up, PHUp{ID: id, World: n.World, X: n.X, Y: n.Y, Z: n.Z, OnSpot: spots[id]})
	}
	out := make([]PHGroup, 0, len(groups))
	for _, g := range groups {
		sort.Slice(g.Spots, func(i, j int) bool { return g.Spots[i].ID < g.Spots[j].ID })
		sort.Slice(g.Up, func(i, j int) bool { return g.Up[i].ID < g.Up[j].ID })
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
