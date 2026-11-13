package handlers

import (
	"sort"
	"strings"

	"github.com/eqoa/server/internal/entity"
)

// The XP camps as the GM panel's map draws them (F3 pass 18, the user: "add
// the camps and coaches to the GM panel map too"): each #tele camp (camps.go)
// at its anchor as the world has it now, with what campNotes says of it; and
// the towns the map names ("add town names to the map too").

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

// mapCamp is one of EQOA-Data's map's other camps (mapcamps.go, written by
// tools/gen_camp_teles.py --map-out): for the maps alone, never a #tele
// shortcut or a Teleporter camp. Name is the one it is shown by (the map's,
// its zone added where that alone would not tell it apart), Map the map's
// name or names for it, Guide the map's level range ("" where it writes none).
type mapCamp struct {
	Name, Map, Zone, Label string
	NPC                    string
	X, Y, Z                float32
	World                  uint8
	Levels, Guide          string
	MapX, MapY             float32
	Creatures              []string
	Rare                   []campRare
}

// CampMarker is an XP camp for a map.
type CampMarker struct {
	Key       string   // what #tele takes; a map camp's own key (Tele false)
	Aliases   []string // other names #tele takes
	Tele      bool     // a #tele camp (campDests); else one of the map's (mapCamps)
	Name      string   // a map camp's name as shown (its Label says where)
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
		m := CampMarker{Key: key, Tele: true, Label: d.Label, Levels: d.Levels, World: d.World, X: d.X, Y: d.Y, Z: d.Z, Fallback: true}
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

// MapCampMarkers are CampMarkers and the map's other camps (mapCamps) of
// world (every world's when world < 0), by key: what a map draws. A map camp
// follows its anchor as a #tele camp does.
func MapCampMarkers(gw *entity.World, world int) []CampMarker {
	out := append(CampMarkers(gw, world), mapCampMarkers(gw, world)...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func mapCampMarkers(gw *entity.World, world int) []CampMarker {
	var out []CampMarker
	for key, c := range mapCamps {
		if world >= 0 && int(c.World) != world {
			continue
		}
		m := CampMarker{Key: key, Name: c.Name, Label: c.Label, Levels: c.Levels, Guide: c.Guide, World: c.World,
			X: c.X, Y: c.Y, Z: c.Z, Fallback: true, Creatures: c.Creatures}
		if gw != nil {
			if x, y, z, _, w, ok := gw.SpawnOfNPCNear(c.NPC, c.World, c.X, c.Z, campAnchorReach); ok {
				m.X, m.Y, m.Z, m.World, m.Fallback = x, y, z, w, false
			}
		}
		for _, r := range c.Rare {
			m.Rare = append(m.Rare, CampRare{Name: r.Name, Level: r.Level, Drops: r.Drops})
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// CampMarkerOf is one camp by the name #tele takes (an alias too), else a map
// camp by its key.
func CampMarkerOf(gw *entity.World, name string) (CampMarker, bool) {
	if key, _, ok := findTeleDest(name); ok {
		if _, camp := campDests[key]; camp {
			for _, m := range CampMarkers(gw, -1) {
				if m.Key == key {
					return m, true
				}
			}
		}
	}
	k := strings.ToLower(strings.TrimSpace(name))
	c, ok := mapCamps[k]
	if !ok {
		return CampMarker{}, false
	}
	for _, m := range mapCampMarkers(gw, int(c.World)) {
		if m.Key == k {
			return m, true
		}
	}
	return CampMarker{}, false
}

// TownMark is a town to name on a map.
type TownMark struct {
	Name  string
	World uint8
	X, Z  float32
}

// TownMarks are the towns of world (every world's when world < 0): Tunaria's
// coach towns as coachtowns.go places and names them, the others by
// EQOA-Data's map (mapTowns, camps.go).
func TownMarks(world int) []TownMark {
	var out []TownMark
	if world <= 0 {
		for _, t := range entity.CoachTownMarks() {
			out = append(out, TownMark{Name: t.Name, X: t.X, Z: t.Z})
		}
	}
	for _, t := range mapTowns {
		if world < 0 || int(t.World) == world {
			out = append(out, t)
		}
	}
	return out
}
