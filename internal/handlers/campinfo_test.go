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
