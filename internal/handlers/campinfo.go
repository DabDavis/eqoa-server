package handlers

import (
	"sort"

	"github.com/eqoa/server/internal/entity"
)

// The XP camps as the GM panel's map draws them (F3 pass 18, the user: "add
// the camps and coaches to the GM panel map too"): each #tele camp (camps.go)
// at its anchor as the world has it now, with what campNotes says of it.

// campNote is what tools/gen_camp_teles.py writes of a camp beside its
// shortcut: EQOA-Data's note on it, the guide's levels, the creatures that make
// it here (those tools/seed_camp_loot.py binds loot to) and the rare mobs near.
type campNote struct {
	Note, Guide string
	Creatures   []string
	Rare        []campRare
}

// campRare is one of EQOA-Data's rare mobs near a camp: its level here (0: no
// NPC of the name) and the drops EQOA-Data names it the source of.
type campRare struct {
	Name  string
	Level int
	Drops []string
}

// CampMarker is an XP camp for a map.
type CampMarker struct {
	Key       string   // what #tele takes
	Aliases   []string // other names #tele takes
	Label     string
	Levels    string // the camp's creatures' here
	Guide     string // EQOA-Data's guide's
	Note      string
	World     uint8
	X, Y, Z   float32 // the anchor's spawn, or the written one (Fallback)
	Fallback  bool    // the anchor is not loaded: the place as written
	Creatures []string
	Rare      []CampRare
}

// CampRare is a rare mob near a camp (campRare).
type CampRare struct {
	Name  string
	Level int
	Drops []string
}

// CampMarkers are the camps of world (every world's when world < 0), by key.
// A camp is drawn where its anchor stands: the NPC of its anchor's name
// nearest the written spawn within campAnchorReach (so a GM's move of the
// camp moves the marker), else as written.
func CampMarkers(gw *entity.World, world int) []CampMarker {
	aliases := map[string][]string{}
	for a, k := range campAliases {
		aliases[k] = append(aliases[k], a)
	}
	var out []CampMarker
	for key, d := range campDests {
		if world >= 0 && int(d.World) != world {
			continue
		}
		m := CampMarker{Key: key, Label: d.Label, Levels: d.Levels, World: d.World, X: d.X, Y: d.Y, Z: d.Z, Fallback: true}
		if a := aliases[key]; len(a) > 0 {
			sort.Strings(a)
			m.Aliases = a
		}
		if gw != nil {
			if x, y, z, _, w, ok := gw.SpawnOfNPCNear(d.NPC, d.World, d.X, d.Z, campAnchorReach); ok {
				m.X, m.Y, m.Z, m.World, m.Fallback = x, y, z, w, false
			}
		}
		if n, ok := campNotes[key]; ok {
			m.Note, m.Guide, m.Creatures = n.Note, n.Guide, n.Creatures
			for _, r := range n.Rare {
				m.Rare = append(m.Rare, CampRare{Name: r.Name, Level: r.Level, Drops: r.Drops})
			}
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// CampMarkerOf is one camp by the name #tele takes (an alias too).
func CampMarkerOf(gw *entity.World, name string) (CampMarker, bool) {
	key, _, ok := findTeleDest(name)
	if !ok {
		return CampMarker{}, false
	}
	if _, camp := campDests[key]; !camp {
		return CampMarker{}, false
	}
	for _, m := range CampMarkers(gw, -1) {
		if m.Key == key {
			return m, true
		}
	}
	return CampMarker{}, false
}
