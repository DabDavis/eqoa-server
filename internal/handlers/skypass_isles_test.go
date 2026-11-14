package handlers

import (
	"strings"
	"testing"

	"github.com/eqoa/server/internal/skyisles"
)

// skyIsleOfTo is the isle a passage's To names, as skyisles names it: "the
// second isle of rain" "Rain Chain 2", "the Isle of Wonder" "Isle of Wonder".
func skyIsleOfTo(to string) string {
	if to == "the Isle of Wonder" {
		return "Isle of Wonder"
	}
	nth := map[string]string{"first": "1", "second": "2", "third": "3", "fourth": "4", "fifth": "5"}
	chain := map[string]string{"rain": "Rain", "day": "Day", "night": "Night", "snow": "Snow", "desolation": "Desolation"}
	f := strings.Fields(to) // the second isle of rain
	if len(f) != 5 || f[0] != "the" || f[2] != "isle" || f[3] != "of" {
		return ""
	}
	return chain[f[4]] + " Chain " + nth[f[1]]
}

// skyLandsOff are the passages whose landing is on another isle than their
// To names (2026-11-14, sky-isle-names): the spots players gave, kept as the
// owner allowed, which the isles' outlines put elsewhere. A landing moved
// back onto its isle fails this test until it leaves the list.
var skyLandsOff = map[string]string{
	"Rain Key 1": "Day Chain 2",   // petition 2049, Hoofbite's spot: Imerzan's castle isle
	"Rain Key 2": "Day Chain 3",   // petition 2053, Hoofbite's spot: the winds' isle
	"sun key":    "Watrya's Isle", // petition 2048, Thorinoak's spot: Madam Watrya's isle, not the brightstar fairies'
}

// Each passage lands on the isle its To names (skyisles' outlines), but the
// known exceptions, which land where the list says.
func TestSkyPassesLandOnTheirIsles(t *testing.T) {
	for _, p := range skyPasses {
		want := skyIsleOfTo(p.To)
		if want == "" {
			t.Errorf("%s's passage: To %q names no isle", p.Key, p.To)
			continue
		}
		got := skyisles.Isle(p.At.X, p.At.Y, p.At.Z)
		if off, ok := skyLandsOff[p.Key]; ok {
			if got != off {
				t.Errorf("%s to %s lands on %q now, not %q: take it off skyLandsOff", p.Key, p.To, got, off)
			}
			continue
		}
		if got != want {
			t.Errorf("%s to %s lands at (%.1f, %.1f, %.1f), on %q", p.Key, p.To, p.At.X, p.At.Y, p.At.Z, got)
		}
	}
}

// The GM's lines name the isle: Where Is's, from placeName.
func TestPlaceNameNamesTheIsle(t *testing.T) {
	if got := placeName(4, 4160.7, 613.3, 6404.1); got != "Plane of Sky (Night Chain 1)" {
		t.Errorf("night 1's essence: %q", got)
	}
	if got := placeName(4, 4824, 250, 5497); got != "Plane of Sky (Main Isle)" {
		t.Errorf("the Island of Wonder portal: %q", got)
	}
	if got := placeName(0, 4160.7, 613.3, 6404.1); got != worldName(0) || got == "" {
		t.Errorf("Tunaria: %q", got)
	}
}
