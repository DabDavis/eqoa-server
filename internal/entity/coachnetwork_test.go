package entity

import (
	"slices"
	"testing"
)

// The coach network is EQOA-Data's coach map read both ways: Freeport's coach
// runs to Neriak and Neriak's back (EQOA-Data gives Neriak, Bobble-by-Water
// and Hazinak (Evil) no list of their own), Rivervale's to Fayspire (listed
// only from Fayspire). The controls: Freeport's coach never ran to Rivervale
// nor to itself, and a coachman's own name is no town on it.
func TestTheCoachNetworkRunsBothWays(t *testing.T) {
	for _, pair := range [][2]string{
		{"Freeport", "Neriak"}, {"Neriak", "Freeport"}, {"Neriak", "Klik'Anon"}, {"Bobble-by-Water", "Rivervale"},
		{"Hazinak (Evil)", "Grobb"}, {"Rivervale", "Fayspire"}, {"Fayspire", "Rivervale"}, {"Dark Solace", "Oggok"},
		{"Freeport", "Highpass Hold"}, {"Highpass Hold", "Freeport"},
	} {
		if !CoachRunsTo(pair[0], pair[1]) {
			t.Errorf("no coach from %s to %s", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"Freeport", "Rivervale"}, {"Freeport", "Freeport"}, {"Qeynos", "Freeport"}, {"Freeport", "Coachman Dilga"}} {
		if CoachRunsTo(pair[0], pair[1]) {
			t.Errorf("the control: a coach from %s to %s", pair[0], pair[1])
		}
	}
	for _, town := range []string{"Neriak", "Bobble-by-Water", "Hazinak (Evil)", "Freeport", "Kerplunk Outpost"} {
		if !OnCoachNetwork(town) {
			t.Errorf("%s is not on the network", town)
		}
	}
	if OnCoachNetwork("Coachman Dilga") || OnCoachNetwork("") {
		t.Error("the control: a coachman's own name or nothing is on the network")
	}
}

// Every coachman named for a town serves one on the network, and every town a
// coachman or the network names is one a ledger may name; so only a coachman
// of no town keeps every coach town of his world (CoachMenu).
func TestEveryCoachTownIsOnTheNetwork(t *testing.T) {
	names := CoachTownNames()
	if !slices.IsSorted(names) {
		t.Errorf("CoachTownNames is not sorted: %v", names)
	}
	for coachman, town := range coachLocationNames {
		if !OnCoachNetwork(town) {
			t.Errorf("%s serves %s, which is on no coach route", coachman, town)
		}
		if !slices.Contains(names, town) {
			t.Errorf("%s's town %s is not a ledger's", coachman, town)
		}
	}
	for _, ct := range coachTowns {
		if !OnCoachNetwork(ct.name) {
			t.Errorf("the coach town %s is on no coach route", ct.name)
		}
	}
	for town, dests := range coachNetwork {
		for d := range dests {
			if !slices.Contains(names, town) || !slices.Contains(names, d) {
				t.Errorf("the route %s-%s names a town CoachTownNames lacks", town, d)
			}
		}
	}
}

// CoachMenu keeps a networked town's neighbours of the stops it is given,
// and gives a coachman of no town (or a town off the network) every stop.
func TestCoachMenu(t *testing.T) {
	all := []CoachDest{{Name: "Highpass Hold"}, {Name: "Neriak"}, {Name: "Qeynos"}, {Name: "Rivervale"}}
	runs, routed := CoachMenu("Freeport", true, all)
	var got []string
	for _, d := range runs {
		got = append(got, d.Name)
	}
	if !routed || !slices.Equal(got, []string{"Highpass Hold", "Neriak"}) {
		t.Errorf("Freeport's menu %v (routed %v), want Highpass Hold and Neriak", got, routed)
	}
	if runs, routed := CoachMenu("Coachman Dilga", false, all); routed || len(runs) != len(all) {
		t.Errorf("a coachman of no town: %v (routed %v), want every stop", runs, routed)
	}
	if runs, routed := CoachMenu("Atlantis", true, all); routed || len(runs) != len(all) {
		t.Errorf("a town off the network: %v (routed %v), want every stop", runs, routed)
	}
}
