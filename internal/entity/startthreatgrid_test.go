package entity

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// startThreatsEvery is StartThreats as it was before its grid: every NPC
// looked at for every spot.
func (w *World) startThreatsEvery(spots []database.StartSpot) []StartThreat {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []StartThreat
	for _, s := range spots {
		p := &Player{Name: "a new character", Level: 1, Race: s.Race, Class: s.Class, HumType: s.HumType,
			X: s.X, Y: s.Y, Z: s.Z, World: uint8(s.World)}
		for _, npc := range w.NPCs {
			if int(npc.World) != s.World || npc.Temporary || npc.SpawnOnly {
				continue
			}
			d := dist3(npc.SpawnX, npc.SpawnY, npc.SpawnZ, s.X, s.Y, s.Z)
			if d > startThreatReach || !w.threatensLocked(npc, p) || !npc.EventAggro && greenTo(npc, p) {
				continue
			}
			t := StartThreat{Spot: s, NPC: npc.Name, Level: npc.Level, Distance: d}
			if f, ok := w.factionAggroLocked(npc); ok {
				t.Faction, t.Standing = f, w.standingLocked(p, f)
			}
			out = append(out, t)
		}
	}
	return out
}

// The grid finds exactly the threats every NPC's look did: NPCs and spots at
// random over three worlds, round the grid's lines and on both sides of 0,
// some temporary or out of the world, some wildlife (green to a new character).
func TestStartThreatsGridIsEveryLook(t *testing.T) {
	f := newAggroFix(t, 10)
	r := rand.New(rand.NewSource(1119))
	coord := func() float32 {
		switch r.Intn(3) {
		case 0:
			return float32(r.Intn(7)-3)*startThreatReach + float32(r.Intn(5)-2)*0.25 // on and by a line
		default:
			return float32(r.Intn(1200) - 600)
		}
	}
	for i := 0; i < 3000; i++ {
		n := f.npc(EntityID(20000+i), "ghoul servant", 1+r.Intn(60), ghoulBody, coord(), coord())
		n.World = uint8(r.Intn(3))
		n.SpawnY = float32(r.Intn(80))
		n.Temporary = r.Intn(20) == 0
		n.SpawnOnly = r.Intn(20) == 0
	}
	var spots []database.StartSpot
	for i := 0; i < 200; i++ {
		spots = append(spots, database.StartSpot{Race: r.Intn(10), Class: r.Intn(15), World: r.Intn(4) - (r.Intn(30) / 29), X: coord(), Y: float32(r.Intn(80)), Z: coord()})
	}
	key := func(th []StartThreat) []string {
		var out []string
		for _, t := range th {
			out = append(out, t.NPC+"|"+t.Faction+"|"+string(rune(t.Level))+"|"+string(rune(int(t.Distance*1000)))+"|"+string(rune(t.Spot.Race))+string(rune(t.Spot.Class))+string(rune(int(t.Spot.X*4)))+string(rune(int(t.Spot.Z*4))))
		}
		sort.Strings(out)
		return out
	}
	got, want := key(f.w.StartThreats(spots)), key(f.w.startThreatsEvery(spots))
	if len(want) == 0 {
		t.Fatal("no threat at all: the test asks nothing")
	}
	if len(got) != len(want) {
		t.Fatalf("the grid found %d threats, every look %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("threat %d differs", i)
		}
	}
	t.Logf("%d threats, the same", len(want))
}
