package handlers

import (
	"math"
	"strings"
	"testing"
)

// The camps the map lacked (the owner, 2026-11-23: "we are missing some XP
// camps on the map still", a camp of shrieks where a player stood): the shriek
// camp at (17601, 21116) in Tunaria has a marker, and the census's camps
// (tools/gen_camp_teles.py's CENSUS_CAMPS) are on the maps of all three
// worlds it found them in, each its own name and place.
func TestMissingCampsOnTheMap(t *testing.T) {
	shriek := false
	for _, m := range MapCampMarkers(nil, 0) {
		if math.Hypot(float64(m.X)-17601, float64(m.Z)-21116) > 60 {
			continue
		}
		for _, c := range m.Creatures {
			if strings.EqualFold(c, "shriek") {
				shriek = true
			}
		}
	}
	if !shriek {
		t.Errorf("no map camp of shrieks at (17601, 21116)")
	}
	census := map[string]struct {
		world  uint8
		levels string
	}{
		"aviakswidowspeak":                      {0, "38-43"},
		"griffonsserpenthills":                  {0, "48-49"},
		"tenaciousfrostgiantspermafrosteast":    {0, ""},
		"deathfistmyrmidonsdeathfistforge":      {0, ""},
		"ateshahlakerathe":                      {1, ""},
		"mithrilgargoylesgeomancerscitadeleast": {1, "51"},
		"kerranslavespaineel":                   {2, ""},
	}
	for key, want := range census {
		c, ok := mapCamps[key]
		if !ok {
			t.Errorf("no map camp %s", key)
			continue
		}
		if c.World != want.world || c.Name == "" || c.Label == "" || len(c.Creatures) == 0 ||
			(want.levels != "" && c.Levels != want.levels) {
			t.Errorf("%s: world %d levels %q name %q creatures %v", key, c.World, c.Levels, c.Name, c.Creatures)
		}
	}
	names := map[string]string{}
	for key, c := range mapCamps {
		if other, ok := names[c.Name]; ok {
			t.Errorf("map camps %s and %s are both %q", key, other, c.Name)
		}
		names[c.Name] = key
		if _, ok := campDests[key]; ok {
			t.Errorf("map camp %s is a #tele camp's key", key)
		}
	}
	if n := len(MapCampMarkers(nil, 1)); n < 30 {
		t.Errorf("the Rathe's map has %d camps", n)
	}
}
