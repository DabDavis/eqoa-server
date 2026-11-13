package entity

import (
	"testing"
	"time"
)

// A template's taken_resist (petition 1944, the Plane of Disease's raid bosses: "should take 15% more
// damage from lightning resistance based damage, procs and nukes"): a lightning hit or tick on an
// "LR:115" creature lands at 115%, after its strong resist's cut; another resist's, none and a heal
// come as they were.
func TestATemplateTakesMoreOfAResist(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tm := &ClassTemplate{}
	tm.TakenResist = "LR:115"
	npc := &NPC{ServerID: 9, Level: 63, tmpl: tm}
	for _, c := range []struct {
		resist Stat
		n      int
		want   int
	}{
		{StatLR, -1000, -1150},
		{StatLR, -1, -1},
		{StatFR, -1000, -1000},
		{StatNone, -1000, -1000},
		{StatLR, 500, 500}, // a heal
	} {
		if got := creatureResists(c.resist, npc, c.n, now); got != c.want {
			t.Errorf("%v %d: %d, want %d", c.resist, c.n, got, c.want)
		}
	}
	// strong in it too (Queen Qy'tha's LR at 50): half, then 115% of that
	tm.ResistKind, tm.ResistPct = "DR,PR,AR,LR", 50
	if got := creatureResists(StatLR, npc, -1000, now); got != -575 {
		t.Errorf("strong in LR at 50 and taking 115%%: %d, want -575", got)
	}
	// several, and less than all
	tm.ResistKind, tm.ResistPct, tm.TakenResist = "", 0, "fr:90, LR:120"
	if a, b := creatureResists(StatFR, npc, -1000, now), creatureResists(StatLR, npc, -1000, now); a != -900 || b != -1200 {
		t.Errorf("FR:90, LR:120: %d and %d", a, b)
	}
}
