package entity

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// roamrest_test.go — roamStepLocked's way past a creature resting and
// standing (roamRestSkip, 2026-11-18) changes nothing: two worlds seeded
// alike, one with it and one without, through fights, roots, a dummy's
// target and creatures taken out of the world and put back, stay the same
// tick for tick.

// roamRestWorld is a seeded world with n creatures round a player.
func roamRestWorld(t testing.TB, n int) (*World, []*NPC) {
	t.Helper()
	w := NewWorld()
	w.roamRnd = rand.New(rand.NewSource(11))
	w.SetPlayerModels(map[uint32]bool{humanModel: true})
	p := &Player{ServerID: 1, Name: "Viewer", X: 1000, Z: 1000}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	var npcs []*NPC
	for i := 0; i < n; i++ {
		x, z := 1000+float32(i%40)*12-240, 1000+float32(i/40)*12-240
		npcs = append(npcs, addNPC(w, EntityID(100+i), "beetle", x, z, creatureModel, 0))
	}
	return w, npcs
}

// roamRestState is what the wander leaves of a creature.
func roamRestState(n *NPC) string {
	r := n.roam
	return fmt.Sprintf("%v %v %v %v %v %v %v %v %v %v %v %v %v", n.X, n.Y, n.Z, n.VelX, n.VelY, n.VelZ, n.Facing,
		r.walking, r.toX, r.toZ, r.restUntil.UnixNano(), r.last.UnixNano(), r.nearPlayer)
}

func TestRoamRestSkipChangesNothing(t *testing.T) {
	defer func(v bool) { roamRestSkip = v }(roamRestSkip)
	const n = 300
	wa, na := roamRestWorld(t, n)
	wb, nb := roamRestWorld(t, n)
	start := time.Unix(1_800_000_000, 0)
	// the same events in both, by tick
	event := func(w *World, npcs []*NPC, tick int, now time.Time) {
		switch tick {
		case 50: // fights: some walking, some resting
			for i := 0; i < n; i += 7 {
				npcs[i].CurrentEnemy = 1
			}
		case 120:
			for i := 0; i < n; i += 7 {
				npcs[i].CurrentEnemy = 0
			}
		case 150: // roots
			for i := 3; i < n; i += 11 {
				npcs[i].Effects = append(npcs[i].Effects, &ActiveEffect{Root: true, Until: now.Add(9 * time.Second)})
			}
		case 200: // a dummy's fight
			for i := 5; i < n; i += 13 {
				npcs[i].DummyTarget = 1
			}
		case 260:
			for i := 5; i < n; i += 13 {
				npcs[i].DummyTarget = 0
			}
		case 300: // taken out of the world (the roamers keep them until found again)
			for i := 2; i < n; i += 17 {
				delete(w.NPCs, npcs[i].ServerID)
			}
		case 304: // and put back before the creatures are found again
			for i := 2; i < n; i += 17 {
				w.NPCs[npcs[i].ServerID] = npcs[i]
			}
		case 400: // dead
			for i := 9; i < n; i += 19 {
				npcs[i].Dead = true
			}
		}
	}
	now := start
	moved := 0
	for tick := 0; tick < 900; tick++ {
		for _, r := range []struct {
			w    *World
			npcs []*NPC
			skip bool
		}{{wa, na, true}, {wb, nb, false}} {
			roamRestSkip = r.skip
			event(r.w, r.npcs, tick, now)
			r.w.TickCount++
			r.w.roamSystem(now)
		}
		for i := range na {
			a, b := roamRestState(na[i]), roamRestState(nb[i])
			if a != b {
				t.Fatalf("tick %d, creature %d: with the skip %s, without %s", tick, i, a, b)
			}
			if na[i].moving() {
				moved++
			}
		}
		now = now.Add(100 * time.Millisecond)
	}
	if moved == 0 {
		t.Fatal("nothing walked")
	}
	t.Logf("%d creature-ticks walking of %d", moved, 900*n)
}

// BenchmarkRoamSystem is a tick of the wander over 2,000 creatures round a
// player, with the skip and without.
func BenchmarkRoamSystem(b *testing.B) {
	defer func(v bool) { roamRestSkip = v }(roamRestSkip)
	for _, skip := range []bool{false, true} {
		b.Run(fmt.Sprintf("skip=%v", skip), func(b *testing.B) {
			roamRestSkip = skip
			w, _ := roamRestWorld(b, 2000)
			now := time.Unix(1_800_000_000, 0)
			for i := 0; i < 300; i++ { // past the first rests
				w.TickCount++
				w.roamSystem(now)
				now = now.Add(100 * time.Millisecond)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.TickCount++
				w.roamSystem(now)
				now = now.Add(100 * time.Millisecond)
			}
		})
	}
}
