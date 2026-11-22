package handlers

import "testing"

// Glowbap sends a Klick'Anon alchemist to Highbourne and Ilganis sends it back
// (work board 1933, section 5: Prima p.207, Allakhazam q829), beside Glowbap's
// own Rivervale trip. The control: Ilganis has no Rivervale trip.
func TestGlowbapAndIlganisRunToEachOther(t *testing.T) {
	if r, ok := RouteOf("Waysender Glowbap", "Highbourne"); !ok || r.QuestOnly {
		t.Fatalf("Glowbap to Highbourne: %+v %v", r, ok)
	}
	if r, ok := RouteOf("Waysender Ilganis", "Klik'Anon"); !ok || r.QuestOnly {
		t.Fatalf("Ilganis to Klik'Anon: %+v %v", r, ok)
	}
	for _, w := range wayRoutes["Waysender Glowbap"] {
		if w.To == "Highbourne" && w.Arrive != "Waysender Ilganis" {
			t.Fatalf("Glowbap's Highbourne trip lands at %q", w.Arrive)
		}
	}
	if _, ok := RouteOf("Waysender Glowbap", "Rivervale"); !ok {
		t.Fatal("Glowbap lost its Rivervale trip")
	}
	if _, ok := RouteOf("Waysender Ilganis", "Rivervale"); ok {
		t.Fatal("the control: Ilganis runs to Rivervale")
	}
}
