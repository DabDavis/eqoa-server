package entity

import "sort"

// CoachMarker is a coach stop for a map (the GM panel's; F3 pass 18, the
// user: "add the camps and coaches to the GM panel map too"): a coachman, the
// town it serves, where it stands, the towns this server's coach menu offers
// from it (GetCoachDestinations: every other coach town of its world) and the
// towns the live game's coach ran to (liveCoachRoutes, EQOA-Data's map).
type CoachMarker struct {
	ID      EntityID
	Name    string // the coachman's
	Town    string // the town it serves (coachLabel), else its own name
	Named   bool   // Town is a town's, not the coachman's name
	World   uint8
	X, Y, Z float32  // its spawn
	Routes  []string // what its coach menu lists, as GetCoachDestinations names them
	Live    []string // the live game's from this town (EQOA-Data), none when it has none
}

// CoachMarkers are the coachmen of world (every world's when world < 0), by
// town, a GM's temporary copy left out.
func (w *World) CoachMarkers(world int) []CoachMarker {
	w.mu.RLock()
	defer w.mu.RUnlock()
	var out []CoachMarker
	labels := map[EntityID]string{}
	for _, npc := range w.NPCs {
		if !IsCoachman(npc.Name) || npc.Temporary {
			continue
		}
		label, _ := coachLabel(npc)
		labels[npc.ServerID] = label
	}
	for _, npc := range w.NPCs {
		label, ok := labels[npc.ServerID]
		if !ok || (world >= 0 && int(npc.World) != world) {
			continue
		}
		_, named := coachLabel(npc)
		m := CoachMarker{ID: npc.ServerID, Name: npc.Name, Town: label, Named: named, World: npc.World,
			X: npc.SpawnX, Y: npc.SpawnY, Z: npc.SpawnZ}
		seen := map[string]bool{}
		for id, other := range labels {
			if o := w.NPCs[id]; o == nil || id == npc.ServerID || o.World != npc.World || seen[other] {
				continue
			}
			seen[other] = true
			m.Routes = append(m.Routes, other)
		}
		sort.Strings(m.Routes)
		if named {
			m.Live = append([]string(nil), liveCoachRoutes[label]...)
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Town != out[j].Town {
			return out[i].Town < out[j].Town
		}
		return out[i].ID < out[j].ID
	})
	return out
}
