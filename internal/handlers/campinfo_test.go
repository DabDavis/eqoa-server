package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/entity"
)

// The map's XP camps: every #tele camp, where its anchor stands (the NPC of
// the anchor's name nearest the written spawn, within campAnchorReach) or as
// written when none is loaded, with what campNotes says of it. A namesake
// across the world is not the camp's anchor.
func TestCampMarkersFollowTheirAnchors(t *testing.T) {
	all := CampMarkers(nil, -1)
	if len(all) != len(campDests) {
		t.Fatalf("%d camps, want every one of campDests' %d", len(all), len(campDests))
	}
	for _, m := range all {
		if !m.Fallback {
			t.Fatalf("%s: no world, yet not as written", m.Key)
		}
		if _, ok := campNotes[m.Key]; !ok {
			t.Errorf("%s has no campNotes entry: run tools/gen_camp_teles.py", m.Key)
		}
	}
	d := campDests["dfc"]
	w := entity.NewWorld()
	// a namesake 2000 off: not the camp's
	w.NPCs[1] = &entity.NPC{ServerID: 1, Name: d.NPC, World: d.World, SpawnX: d.X + 2000, SpawnZ: d.Z, SpawnY: 54}
	if m, ok := CampMarkerOf(w, "deathfist"); !ok || m.Key != "dfc" || !m.Fallback || m.X != d.X {
		t.Fatalf("with only a far namesake: %+v (%v), want dfc as written", m, ok)
	}
	// moved 50 by a GM: the marker goes with it
	w.NPCs[2] = &entity.NPC{ServerID: 2, Name: d.NPC, World: d.World, SpawnX: d.X + 50, SpawnZ: d.Z - 20, SpawnY: 60}
	m, ok := CampMarkerOf(w, "dfc")
	if !ok || m.Fallback || m.X != d.X+50 || m.Z != d.Z-20 || m.Y != 60 {
		t.Fatalf("with the anchor moved: %+v, want at (%v, %v)", m, d.X+50, d.Z-20)
	}
	if !strings.Contains(m.Note, "XP camps") || m.Guide != "29-34" || len(m.Creatures) == 0 || m.Aliases[0] != "deathfist" {
		t.Errorf("dfc's notes %+v, want EQOA-Data's note, its guide levels, its creatures and the alias", m)
	}
	if _, ok := CampMarkerOf(w, "freeport"); ok {
		t.Error("freeport is a #tele place, not a camp")
	}
	if xr, _ := CampMarkerOf(nil, "xrag"); len(xr.Rare) == 0 || xr.Rare[0].Name != "Overlord Xrag" {
		t.Errorf("xrag's rare mobs %+v, want Overlord Xrag", xr.Rare)
	}
	if n := len(CampMarkers(nil, 2)); n == 0 || n >= len(all) {
		t.Errorf("Odus has %d camps of %d", n, len(all))
	}
}

// The map's other camps (mapcamps.go, tools/gen_camp_teles.py --map-out):
// drawn on the maps beside the #tele camps, never a #tele name (so neither
// #tele, #goto nor the Teleporter offers one: those read campDests alone),
// each named apart, in its world, its anchor among its creatures; a map camp
// follows its anchor as a #tele camp does. The controls: CampMarkers (what
// the camp reports name) holds the #tele camps alone, and a #tele camp is
// still found as one.
func TestMapCampsAreTheMapsAlone(t *testing.T) {
	if len(mapCamps) < 100 {
		t.Fatalf("%d map camps; the tool wrote 154", len(mapCamps))
	}
	names := map[string]string{}
	for k, c := range mapCamps {
		if _, _, ok := findTeleDest(k); ok {
			t.Errorf("map camp %s is a #tele name", k)
		}
		if _, ok := campDests[k]; ok {
			t.Errorf("map camp %s is a #tele camp's key", k)
		}
		if o, ok := names[strings.ToLower(c.Name)]; ok {
			t.Errorf("%s and %s are both named %q", k, o, c.Name)
		}
		names[strings.ToLower(c.Name)] = k
		if c.World > 2 || c.NPC == "" || c.Levels == "" || c.Label == "" || c.Zone == "" {
			t.Errorf("%s: %+v", k, c)
		}
		own := false
		for _, n := range c.Creatures {
			own = own || strings.EqualFold(strings.TrimSpace(n), strings.TrimSpace(c.NPC))
		}
		if !own {
			t.Errorf("%s: its anchor %q is not among its creatures %v", k, c.NPC, c.Creatures)
		}
	}
	if n := len(CampMarkers(nil, -1)); n != len(campDests) {
		t.Errorf("CampMarkers holds %d, want the %d #tele camps alone", n, len(campDests))
	}
	all := MapCampMarkers(nil, -1)
	if len(all) != len(campDests)+len(mapCamps) {
		t.Fatalf("MapCampMarkers holds %d, want %d #tele and %d map camps", len(all), len(campDests), len(mapCamps))
	}
	tele := 0
	for _, m := range all {
		if m.Tele {
			tele++
			if _, ok := campDests[m.Key]; !ok {
				t.Errorf("%s is drawn as a #tele camp", m.Key)
			}
		} else if m.Name == "" {
			t.Errorf("map camp %s has no name", m.Key)
		}
	}
	if tele != len(campDests) {
		t.Errorf("%d #tele camps drawn, want %d", tele, len(campDests))
	}
	if n := len(MapCampMarkers(nil, 2)); n <= len(CampMarkers(nil, 2)) {
		t.Errorf("Odus draws %d camps, no more than its #tele ones", n)
	}
	var key string
	for k := range mapCamps {
		if key == "" || k < key {
			key = k
		}
	}
	c := mapCamps[key]
	w := entity.NewWorld()
	w.NPCs[1] = &entity.NPC{ServerID: 1, Name: c.NPC, World: c.World, SpawnX: c.X + 40, SpawnZ: c.Z, SpawnY: 70}
	m, ok := CampMarkerOf(w, strings.ToUpper(key))
	if !ok || m.Tele || m.Fallback || m.X != c.X+40 || m.Y != 70 || m.Name != c.Name {
		t.Fatalf("map camp %s: %+v (%v), want it at its moved anchor", key, m, ok)
	}
	if m, ok := CampMarkerOf(nil, "deathfist"); !ok || !m.Tele || m.Key != "dfc" {
		t.Errorf("the control: deathfist is %+v (%v), want the #tele camp dfc", m, ok)
	}
}
