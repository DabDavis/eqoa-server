package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A camp's social pull by its own faction (socialaggro.go): a name that lives in
// two camps is of each camp's faction where a factionhitareas row says so, for
// whom it helps (socialFactionLocked), whom it counts as kin (ofFactionLocked),
// its kin heal (kinheal.go) and the guards' look (guards.go).

// The two camps: Blackburrow's round (1000, 1000), the Gnolls of Tunaria's
// round (3000, 1000), 200 across.
const (
	burrowX, campX, campZ float32 = 1000, 3000, 1000
)

// newCampFix is socialFix with the gnolls' factions: a spiritpaw of either camp
// (its name's kill lowers Blackburrow, the Tunaria camp's area row says
// Gnolls of Tunaria and leaves Blackburrow be), a strongfang of Tunaria's, a
// Blackburrow gnoll, and a raid boss of Tunaria's with no social row. Spiritpaws
// and strongfangs are social with no faction of their own: the place decides.
func newCampFix(t *testing.T) *aggroFix {
	t.Helper()
	f := newSocialFix(t)
	f.w.factions.hits[database.NPCClassKey("gnoll spiritpaw")] = []FactionHit{{"Blackburrow Gnolls", -10}}
	f.w.factions.hits[database.NPCClassKey("gnoll strongfang")] = []FactionHit{{"Gnolls of Tunaria", -10}}
	f.w.factions.hits[database.NPCClassKey("Blackburrow gnoll")] = []FactionHit{{"Blackburrow Gnolls", -10}}
	f.w.factions.hits[database.NPCClassKey("Gnoll warlord")] = []FactionHit{{"Gnolls of Tunaria", -30}}
	if kept, unknown := f.w.SetFactionHitAreas([]database.FactionHitAreaRow{
		{NPCName: "gnoll spiritpaw", Faction: "Gnolls of Tunaria", Delta: -15, World: 0, X: float64(campX), Z: float64(campZ), Radius: 200},
		{NPCName: "gnoll spiritpaw", Faction: "Blackburrow Gnolls", Delta: 0, World: 0, X: float64(campX), Z: float64(campZ), Radius: 200},
	}); kept != 2 || unknown != 0 {
		t.Fatalf("areas kept %d, unknown %d", kept, unknown)
	}
	f.w.SetSocialRules(map[string]SocialRule{
		"gnoll spiritpaw":   {Social: true},
		"gnoll strongfang":  {Social: true},
		"Blackburrow gnoll": {Social: true},
		"Guard Brenna":      {Social: true, Faction: "The Freeport Militia"},
	})
	return f
}

// A spiritpaw in the Tunaria camp comes to its strongfang's help, not to a
// Blackburrow gnoll's beside it; one in Blackburrow comes to the Blackburrow
// gnoll's, not to a strongfang's. Each side the other way round too: a
// strongfang helps the Tunaria spiritpaw, not Blackburrow's.
func TestACampsSpiritpawHelpsItsOwnCamp(t *testing.T) {
	for _, c := range []struct {
		what   string
		x      float32 // the camp
		victim string
		helps  bool
		helper string
	}{
		{"Tunaria's spiritpaw, its strongfang hit", campX, "gnoll strongfang", true, "gnoll spiritpaw"},
		{"Tunaria's spiritpaw, a Blackburrow gnoll hit", campX, "Blackburrow gnoll", false, "gnoll spiritpaw"},
		{"Blackburrow's spiritpaw, a Blackburrow gnoll hit", burrowX, "Blackburrow gnoll", true, "gnoll spiritpaw"},
		{"Blackburrow's spiritpaw, a strongfang hit", burrowX, "gnoll strongfang", false, "gnoll spiritpaw"},
		{"a strongfang, Tunaria's spiritpaw hit", campX, "gnoll spiritpaw", true, "gnoll strongfang"},
		{"a strongfang, Blackburrow's spiritpaw hit", burrowX, "gnoll spiritpaw", false, "gnoll strongfang"},
		{"a Blackburrow gnoll, Blackburrow's spiritpaw hit", burrowX, "gnoll spiritpaw", true, "Blackburrow gnoll"},
		{"a Blackburrow gnoll, Tunaria's spiritpaw hit", campX, "gnoll spiritpaw", false, "Blackburrow gnoll"},
	} {
		t.Run(c.what, func(t *testing.T) {
			f := newCampFix(t)
			victim := f.npc(100, c.victim, 20, bearBody, c.x, campZ)
			helper := f.npc(101, c.helper, 20, bearBody, c.x+8, campZ)
			f.hit(victim, 5)
			if got := helper.CurrentEnemy == f.p.ServerID; got != c.helps {
				t.Errorf("helped %v, want %v (enemy %d)", got, c.helps, helper.CurrentEnemy)
			}
		})
	}
}

// The GMs' own words stand over the place: a row naming a faction is its
// faction anywhere, a row that is not social (a creature left solitary on
// purpose) never helps, and a raid boss with no row never does, its camp's
// faction or not.
func TestACampsGMRulesStand(t *testing.T) {
	f := newCampFix(t)
	f.w.SetSocialRules(map[string]SocialRule{
		"gnoll spiritpaw":  {Social: true, Faction: "Blackburrow Gnolls"},
		"gnoll strongfang": {Social: false},
	})
	victim := f.npc(100, "gnoll strongfang", 20, bearBody, campX, campZ)
	paw := f.npc(101, "gnoll spiritpaw", 20, bearBody, campX+10, campZ)
	solo := f.npc(102, "gnoll strongfang", 20, bearBody, campX-10, campZ)
	boss := f.npc(103, "Gnoll warlord", 50, bearBody, campX, campZ+10)
	f.hit(victim, 5)
	for _, n := range []*NPC{paw, solo, boss} {
		if n.CurrentEnemy != 0 {
			t.Errorf("%s came (enemy %d)", n.Name, n.CurrentEnemy)
		}
	}
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if got := f.w.socialFactionLocked(paw); got != "Blackburrow Gnolls" {
		t.Errorf("a row's faction in the Tunaria camp: %q, want its row's", got)
	}
}

// A wall between the camp's helper and both the one hit and its attacker: it
// does not come, as before; the control, nothing between, does.
func TestACampsHelperNeedsItsSight(t *testing.T) {
	for _, walled := range []bool{true, false} {
		f := newCampFix(t)
		f.moveTo(campX-5, campZ)
		victim := f.npc(100, "gnoll strongfang", 20, bearBody, campX, campZ)
		helper := f.npc(101, "gnoll spiritpaw", 20, bearBody, campX+8, campZ)
		if walled {
			f.w.SetGround(sightGround{wallAt(campX + 4)})
		}
		f.hit(victim, 5)
		if got := helper.CurrentEnemy == f.p.ServerID; got == walled {
			t.Errorf("walled %v: helped %v", walled, got)
		}
	}
}

// A spiritpaw's kin heal (kinheal.go) takes its camp's kin: in Tunaria the
// strongfang, not the Blackburrow gnoll; in Blackburrow the other way round.
func TestACampsKinIsItsPlacesFaction(t *testing.T) {
	f := newCampFix(t)
	tun := f.npc(100, "gnoll spiritpaw", 20, bearBody, campX, campZ)
	bur := f.npc(101, "gnoll spiritpaw", 20, bearBody, burrowX, campZ)
	fang := f.npc(102, "gnoll strongfang", 20, bearBody, campX+5, campZ)
	bbg := f.npc(103, "Blackburrow gnoll", 20, bearBody, burrowX+5, campZ)
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	for _, c := range []struct {
		healer, o *NPC
		kin       bool
	}{{tun, fang, true}, {tun, bbg, false}, {bur, bbg, true}, {bur, fang, false}} {
		faction := f.w.socialFactionLocked(c.healer)
		if got := f.w.kinOfLocked(c.healer, c.o, faction); got != c.kin {
			t.Errorf("%s at x %.0f (%s): %s at x %.0f kin %v, want %v", c.healer.Name, c.healer.SpawnX, faction,
				c.o.Name, c.o.SpawnX, got, c.kin)
		}
	}
}

// The guards' look is as it was: who is a town's guard is its row's faction,
// whatever the area rows say of other names; a camp's creature is still their
// prey wherever its faction comes from, and a guard still leaves its militia's
// own alone.
func TestTheGuardsAreAsTheyWereWithCampAreas(t *testing.T) {
	f := newCampFix(t)
	g := f.npc(201, "Guard Brenna", 50, humanBody, 2000, 2000)
	paw := f.wanderer(202, "gnoll spiritpaw", 20, bearBody, 2010, 2000)
	paw.SpawnX, paw.SpawnZ = campX, campZ // a Tunaria spiritpaw that wandered to the post
	militia := f.wanderer(203, "Guard Alinon", 30, bearBody, 2010, 2005)
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	if !f.w.townGuardLocked(g) {
		t.Error("the militia's guard is no longer a town's guard")
	}
	if f.w.townGuardLocked(paw) {
		t.Error("a spiritpaw is a town's guard")
	}
	if !f.w.guardsPreyLocked(g, paw, f.now) {
		t.Error("the guard leaves a camp's spiritpaw alone")
	}
	if f.w.guardsPreyLocked(g, militia, f.now) {
		t.Error("the guard takes on one of its militia")
	}
}

// Equal most-lowered factions go by name, whatever order the rows came in or
// whether an area named one; worstAt and lowersAt agree with hitsAt at every
// place round the camp.
func TestTheWorstFactionAtAPlace(t *testing.T) {
	f := newCampFix(t)
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	tf := &f.w.factions
	for _, order := range [][]FactionHit{
		{{"Gnolls of Tunaria", -10}, {"Blackburrow Gnolls", -10}},
		{{"Blackburrow Gnolls", -10}, {"Gnolls of Tunaria", -10}},
	} {
		tf.hits[database.NPCClassKey("tied gnoll")] = order
		if got := tf.worstAt(&NPC{Name: "tied gnoll", SpawnX: burrowX, SpawnZ: campZ}); got != "Blackburrow Gnolls" {
			t.Errorf("rows %v: %q, want Blackburrow Gnolls (by name)", order, got)
		}
	}
	// an area-only faction tied with the name's
	tf.hits[database.NPCClassKey("tied gnoll")] = []FactionHit{{"Gnolls of Tunaria", -15}}
	tf.areas[database.NPCClassKey("tied gnoll")] = []hitArea{{FactionHit{"Blackburrow Gnolls", -15}, 0, campX, campZ, 50}}
	if got := tf.worstAt(&NPC{Name: "tied gnoll", SpawnX: campX, SpawnZ: campZ}); got != "Blackburrow Gnolls" {
		t.Errorf("an area's tie: %q, want Blackburrow Gnolls", got)
	}

	for x := campX - 400; x <= campX+400; x += 25 {
		for _, world := range []uint8{0, 1} {
			n := &NPC{Name: "gnoll spiritpaw", SpawnX: x, SpawnZ: campZ, World: world}
			worst, low := 0, map[string]bool{}
			want := ""
			for _, h := range tf.hitsAt(n) {
				if h.Delta < 0 {
					low[h.Faction] = true
				}
				if h.Delta < worst || h.Delta < 0 && h.Delta == worst && h.Faction < want {
					worst, want = h.Delta, h.Faction
				}
			}
			if got := tf.worstAt(n); got != want {
				t.Errorf("x %.0f world %d: worstAt %q, hitsAt's %q", x, world, got, want)
			}
			for _, fac := range []string{"Gnolls of Tunaria", "Blackburrow Gnolls", "Citizens of Freeport"} {
				if got := tf.lowersAt(n, fac); got != low[fac] {
					t.Errorf("x %.0f world %d: lowersAt %s %v, hitsAt's %v", x, world, fac, got, low[fac])
				}
			}
		}
	}
}

// A dense camp's hit asks each NPC near of its faction and builds nothing to
// do it: 60 gnolls, each asked whom it helps and whether the one hit is of it.
func TestACampsFactionLookBuildsNothing(t *testing.T) {
	f := newCampFix(t)
	var camp []*NPC
	for i := 0; i < 60; i++ {
		name := "gnoll spiritpaw"
		if i%2 == 1 {
			name = "gnoll strongfang"
		}
		camp = append(camp, f.npc(EntityID(300+i), name, 20, bearBody, campX+float32(i%10), campZ+float32(i/10)))
	}
	victim := camp[1]
	f.w.mu.Lock()
	defer f.w.mu.Unlock()
	allocs := testing.AllocsPerRun(20, func() {
		for _, n := range camp {
			if fac := f.w.socialFactionLocked(n); fac != "" {
				_ = f.w.ofFactionLocked(victim, fac)
			}
		}
	})
	if allocs != 0 {
		t.Errorf("%.0f allocations a hit's look over the camp, want none", allocs)
	}
	if got := f.w.socialFactionLocked(camp[0]); got != "Gnolls of Tunaria" {
		t.Errorf("a camp spiritpaw helps %q", got)
	}
}
