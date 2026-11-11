package handlers

import "testing"

// The Black Market and the Stone of Morathals (the owner, 2026-11-11): Waysender
// Mazaran sends to the Stone, landing on its evil waysender Iskan, and Iskan
// sends back to the Black Market, landing on Mazaran; free both ways. The
// control: the Stone's other waysender, Kurah, still has no trip.
func TestTheBlackMarketAndTheStoneSendToEachOther(t *testing.T) {
	for _, c := range []struct{ from, to, arrive string }{
		{"Waysender Mazaran", "Stone of Morathals", "Waysender Iskan"},
		{"Waysender Iskan", "the Black Market", "Waysender Mazaran"},
	} {
		if !IsWaysender(c.from) {
			t.Fatalf("%s sends nowhere", c.from)
		}
		r := wayRoutes[c.from]
		if len(r) != 1 || r[0].To != c.to || r[0].Arrive != c.arrive || r[0].Fee != 0 || r[0].MinLevel != 0 || r[0].QuestOnly {
			t.Errorf("%s's trips: %+v", c.from, r)
		}
	}
	if len(wayRoutes["Waysender Kurah"]) != 0 {
		t.Error("the control: Kurah has a trip")
	}
}
