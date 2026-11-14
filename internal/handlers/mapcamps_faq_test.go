package handlers

import "testing"

// The FAQ's camps on the maps (the owner, 2026-11-14: "also add those new
// camps to the map"): tools/faq_camps_20261114.py's new camps are map camps
// (tools/gen_camp_teles.py's FAQ_CAMPS) and the camps it re-levelled show its
// levels (FAQ_LEVELS), in campDests and mapCamps alike (the GM panel's map,
// vanilla's, the Teleporter's boxes).
func TestFAQCampsOnTheMap(t *testing.T) {
	newCamps := map[string]struct {
		world  uint8
		levels string
	}{
		"frosteyeexcavation":        {0, "54-57"},
		"taeewaggressors":           {0, "33-37"},
		"desertbanditstakxiv":       {0, "60-62"},
		"frogloksburialmounds":      {0, "60-63"},
		"mossmouthgoblins":          {0, "21-30"},
		"hobgoblinscyclopsfortress": {1, "46-50"},
		"quintashislair":            {2, "58-66"},
		"thehuntbeasts":             {2, "48-55"},
	}
	for key, want := range newCamps {
		c, ok := mapCamps[key]
		if !ok {
			t.Errorf("no map camp %s", key)
			continue
		}
		if c.World != want.world || c.Levels != want.levels || len(c.Creatures) == 0 || c.Name == "" {
			t.Errorf("%s: world %d levels %q creatures %v name %q, want world %d levels %q", key, c.World, c.Levels,
				c.Creatures, c.Name, want.world, want.levels)
		}
	}
	tele := map[string]string{"cracktooth": "21-30", "gargoyles": "41-50", "tendora": "46-50"}
	for key, want := range tele {
		if got := campDests[key].Levels; got != want {
			t.Errorf("#tele %s levels %q, want the FAQ's %q", key, got, want)
		}
	}
	if got := campDests["tendora"].NPC; got != "Tendora-Buki spahi" {
		t.Errorf("tendora's anchor %q: the village's own", got)
	}
	maps := map[string]string{"taeewbrokenskullrock": "41-47", "clankolbokthewarrenssouth": "46-53", "ratmanthevastlydeep": "48-62"}
	for key, want := range maps {
		if got := mapCamps[key].Levels; got != want {
			t.Errorf("map camp %s levels %q, want %q", key, got, want)
		}
	}
	// a map camp of every world drawn: Rathe's first one (the hobgoblins)
	found := false
	for _, m := range MapCampMarkers(nil, 1) {
		if m.Key == "hobgoblinscyclopsfortress" && m.Levels == "46-50" {
			found = true
		}
	}
	if !found {
		t.Errorf("Rathe's map camps have no Cyclops' Fortress hobgoblins")
	}
}
