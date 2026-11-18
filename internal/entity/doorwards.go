package entity

import (
	"strings"
	"time"
)

// A door's ward: an invisible blocker (BlockerBody) of an npcs row that stands only
// while its guardian lives (the owner, 2026-11-18, of Tallon Zek's fortress: "the zek
// sentry step should place a bbox in front of the castle door until defeated", then
// "make mine the permanent": the Zek sentry a standing creature at the door). Each
// second every ward of a listed name is out of every view, collision and the move
// check (NPC.wardDown, hiddenAt) while no living guardian of its name stands within
// doorWardReach of it, and back the moment one does: its death opens the door, its
// respawn shuts it, and a keep restart needs nothing kept (the guardian alive or not
// decides).
type doorWard struct {
	Ward, Guardian string
}

var doorWards = []doorWard{
	{Ward: "Tallon's ward", Guardian: "Zek sentry"},
}

// doorWardReach is how near its ward a guardian stands; doorWardEvery how often the
// wards are looked at.
const (
	doorWardReach float32 = 120
	doorWardEvery         = 10 // ticks: a second
)

// doorWardSystem raises or lowers each door's wards by their guardians. Must be
// called with the lock held.
func (w *World) doorWardSystem(now time.Time) {
	if w.TickCount%doorWardEvery != 0 {
		return
	}
	if w.wardIDs == nil || w.TickCount%(doorWardEvery*30) == 0 {
		w.wardIDs = map[EntityID]string{}
		for id, n := range w.NPCs {
			if uint32(n.ModelID) != BlockerBody || n.Temporary {
				continue
			}
			for _, d := range doorWards {
				if strings.EqualFold(n.Name, d.Ward) {
					w.wardIDs[id] = d.Guardian
				}
			}
		}
	}
	for id, guardian := range w.wardIDs {
		n := w.NPCs[id]
		if n == nil {
			continue
		}
		n.wardDown = !w.guardianNearLocked(guardian, n, now)
	}
}

// guardianNearLocked is whether a living guardian of that name, in the world (not
// held out of it), stands within doorWardReach of the ward. Must be called with the
// lock held.
func (w *World) guardianNearLocked(name string, ward *NPC, now time.Time) bool {
	if w.SpatialIndex == nil {
		return false
	}
	for _, id := range w.SpatialIndex.QueryRadius(ward.X, ward.Z, doorWardReach) {
		g := w.NPCs[id]
		if g != nil && g != ward && !g.Dead && g.HP > 0 && g.World == ward.World && !g.hiddenAt(now) &&
			strings.EqualFold(strings.TrimSpace(g.Name), name) {
			return true
		}
	}
	return false
}
