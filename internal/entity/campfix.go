package entity

import (
	"sort"
	"strings"
	"time"

	"github.com/eqoa/server/internal/session"
)

// A camp report's view of the world (handlers/campfix.go; board entry 513, the
// GM Jibrish's ask of 2026-11-09): the NPCs of rows a player may name, where
// they spawn and how they face, and what the report's flags ask of a creature
// and a place. Nothing here changes an NPC: a report is a suggestion, applied
// (or not) by a GM's review.

// CampfixNPC is an NPC of a row as a camp report sees it: its spawn and its
// spawn's facing (the row's place and byte, as the loader made them), its
// level, whether it is dead now.
type CampfixNPC struct {
	ID      EntityID
	Name    string
	Level   int
	World   uint8
	X, Y, Z float32 // its spawn
	Facing  float32 // its spawn's facing in radians: the row's byte, as the loader turned it
	Dead    bool
	// Movement is NPCState.Movement's words (CampfixNear's alone)
	Movement string
	// Dist is how far its spawn is from the place asked about
	Dist float32
}

// campfixRow reports an NPC a player may name in a report: of a row (not a
// GM's, a quest's or a trigger's spawn: those are Temporary, as a summoned pet,
// a genie and a portal are), in the world (not spawn-only), and nobody's pet
// now, genie or portal (a charmed creature is its master's while it lasts).
func campfixRow(npc *NPC) bool {
	return !npc.Temporary && !npc.QuestSpawn && !npc.SpawnOnly && npc.Owner == 0 && npc.GenieOf == 0 && npc.PortalOf == 0
}

func campfixOf(npc *NPC, d float32) CampfixNPC {
	return CampfixNPC{ID: npc.ServerID, Name: npc.Name, Level: npc.Level, World: npc.World,
		X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ, Facing: npc.SpawnFacing, Dead: npc.Dead, Dist: d}
}

// CampfixPlayer is a session's player as a report's mark takes it: where it
// stands, the way it faces (Player.Facing: its latest 0x40's heading), whether
// it may mark (dead or about to rise, flying), and its target when that is an
// NPC a report may name (its spawn's distance from the player in Dist).
type CampfixPlayer struct {
	World           uint8
	X, Y, Z, Facing float32
	Dead, Flying    bool
	Target          CampfixNPC
	HasTarget       bool
}

// CampfixPlayerOf is sess's player now, false with none.
func (w *World) CampfixPlayerOf(sess *session.Session, now time.Time) (CampfixPlayer, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CampfixPlayer{}, false
	}
	me := CampfixPlayer{World: p.World, X: p.X, Y: p.Y, Z: p.Z, Facing: p.Facing,
		Dead: p.IsDead() || p.respawnPending(), Flying: p.flyingNow(now)}
	if p.TargetObjectID != 0 && p.TargetObjectID != 0xFFFFFFFF {
		if e := w.entitySeenByLocked(sess, p.TargetObjectID); e != nil {
			if npc, ok := e.(*NPC); ok && campfixRow(npc) {
				me.Target = campfixOf(npc, dist3(npc.SpawnX, npc.SpawnY, npc.SpawnZ, p.X, p.Y, p.Z))
				me.HasTarget = true
			}
		}
	}
	return me, true
}

// CampfixName is what a world holds of one name: how many NPCs of rows bear it
// (without case), and the one of them whose spawn is nearest the place asked
// about (its spelling is the name's as shown).
type CampfixName struct {
	Count   int
	Nearest CampfixNPC
}

// CampfixScan is the NPCs of rows in world whose spawn lies within reach of
// (x, y, z), nearest first (none when reach is not above 0), and what the
// world holds of each of names (CampfixName; compared without case, trimmed).
func (w *World) CampfixScan(world uint8, x, y, z, reach float32, names []string) ([]CampfixNPC, []CampfixName) {
	keys := make([]string, len(names))
	for i, n := range names {
		keys[i] = strings.TrimSpace(n)
	}
	of := make([]CampfixName, len(names))
	w.mu.RLock()
	defer w.mu.RUnlock()
	var near []CampfixNPC
	for _, npc := range w.NPCs {
		if npc.World != world || !campfixRow(npc) {
			continue
		}
		d := dist3(npc.SpawnX, npc.SpawnY, npc.SpawnZ, x, y, z)
		if reach > 0 && d <= reach {
			near = append(near, campfixOf(npc, d))
		}
		for i, k := range keys {
			if k == "" || !strings.EqualFold(npc.Name, k) {
				continue
			}
			o := &of[i]
			o.Count++
			if o.Count == 1 || d < o.Nearest.Dist || d == o.Nearest.Dist && npc.ServerID < o.Nearest.ID {
				o.Nearest = campfixOf(npc, d)
			}
		}
	}
	sortCampfix(near)
	return near, of
}

// sortCampfix puts the nearest first, the lower id of two as near (the map's
// order is random).
func sortCampfix(l []CampfixNPC) {
	sort.Slice(l, func(i, j int) bool {
		if l[i].Dist != l[j].Dist {
			return l[i].Dist < l[j].Dist
		}
		return l[i].ID < l[j].ID
	})
}

// CampfixNear is the NPCs of rows in world whose spawn lies within reach of any
// of pts, nearest (to its nearest point) first, at most most of them (0: all),
// each with its movement in NPCState's words.
func (w *World) CampfixNear(world uint8, pts [][3]float32, reach float32, most int) []CampfixNPC {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var near []CampfixNPC
	var of []*NPC
	for _, npc := range w.NPCs {
		if npc.World != world || !campfixRow(npc) {
			continue
		}
		best := float32(-1)
		for _, p := range pts {
			if d := dist3(npc.SpawnX, npc.SpawnY, npc.SpawnZ, p[0], p[1], p[2]); d <= reach && (best < 0 || d < best) {
				best = d
			}
		}
		if best >= 0 {
			near = append(near, campfixOf(npc, best))
			of = append(of, npc)
		}
	}
	byID := make(map[EntityID]*NPC, len(of))
	for _, npc := range of {
		byID[npc.ServerID] = npc
	}
	sortCampfix(near)
	if most > 0 && len(near) > most {
		near = near[:most]
	}
	for i := range near {
		near[i].Movement = w.npcStateLocked(byID[near[i].ID]).Movement
	}
	return near
}

// CampfixRow is the NPC of row id as a report sees it, false when there is
// none a report may name.
func (w *World) CampfixRow(id EntityID) (CampfixNPC, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc := w.NPCs[id]
	if npc == nil || !campfixRow(npc) {
		return CampfixNPC{}, false
	}
	return campfixOf(npc, 0), true
}

// CampfixNewcomerThreat reports whether the NPC of row id would attack on
// sight a level 1 character of one of combos (race, class, home city: a new
// character's, as at its start spot), as aggroSystem decides (threatensLocked,
// and not green to it but by a trigger), alive whether or not it is dead now.
// StartThreats asks the same of the start spots.
func (w *World) CampfixNewcomerThreat(id EntityID, combos [][3]int) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc := w.NPCs[id]
	if npc == nil {
		return false
	}
	c := *npc // its nature as it would be alive: the copy keeps what is looked at
	c.Dead, c.Owner, c.challenge, c.nature = false, 0, nil, 0
	for _, k := range combos {
		p := &Player{Name: "a new character", Level: 1, Race: k[0], Class: k[1], HumType: k[2],
			World: c.World, X: c.SpawnX, Y: c.SpawnY, Z: c.SpawnZ}
		if w.threatensLocked(&c, p) && (c.EventAggro || !greenTo(&c, p)) {
			return true
		}
	}
	return false
}

// NearTown reports a place within reach of a town's own ground (towns.go: a
// town is within townMargin of its people), across.
func (w *World) NearTown(world uint8, x, z, reach float32) bool {
	w.mu.Lock() // the towns are found at their first use
	defer w.mu.Unlock()
	if w.towns == nil {
		w.buildTownsLocked()
	}
	r := reach + townMargin
	lo, hi := townCellOf(world, x-r, z-r), townCellOf(world, x+r, z+r)
	for cx := lo.cx; cx <= hi.cx; cx++ {
		for cz := lo.cz; cz <= hi.cz; cz++ {
			for _, p := range w.towns[townKey{world, cx, cz}] {
				if distXZ(x, z, p[0], p[1]) <= r {
					return true
				}
			}
		}
	}
	return false
}

// CampfixGround is what the ground and the water files say of a place: the
// floor under it (groundnpc.go's Floor: the highest terrain no more than a
// step above it), and whether a body standing there is in the water (water.go:
// bodyAbove over its feet). Known is false for each without its files, or
// where they say nothing.
type CampfixGround struct {
	Floor      float32
	FloorKnown bool
	Wet        bool
	WetKnown   bool
}

// CampfixGroundAt is CampfixGround at (x, y, z) in world.
func (w *World) CampfixGroundAt(world uint8, x, y, z float32) CampfixGround {
	w.mu.RLock()
	g, e := w.ground, w.env
	w.mu.RUnlock()
	var out CampfixGround
	if g != nil {
		out.Floor, out.FloorKnown = g.Floor(world, x, y, z)
	}
	if e != nil {
		if ek, ok := e.(envKnows); !ok || ek.Knows(world) {
			f, ok := e.Flags(world, x, y+bodyAbove, z)
			out.Wet, out.WetKnown = ok && f&envWater != 0, ok
		}
	}
	return out
}

// CampfixKeepsToLand reports a creature of the land (water.go's keepsTo:
// npcmove's word, else where it spawned), false for one of the water or of
// both, or no NPC of the row.
func (w *World) CampfixKeepsToLand(id EntityID) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	npc := w.NPCs[id]
	return npc != nil && w.keepsToLocked(npc) == keepsToLand
}
