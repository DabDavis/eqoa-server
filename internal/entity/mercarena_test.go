package entity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Heirloom petition 3100 (Yevkassim): "Mercenaries followed into an instanced
// duel? Not sure if that should be." A duel is its two alone (duel.go: a
// merc is no duel's), so while its owner is in a Neriak arena duel the merc
// waits where it was, in its own world, fighting nothing, and its owner is
// told so once; back out, it follows again. The control: a teleport to
// another world with no duel, which it follows at its next turn as ever.
func TestMercWaitsOutsideAnArenaDuel(t *testing.T) {
	r := newMercRig(t)
	ms := r.hire(r.kit(0, 0, 0), 0)
	r.tick(time.Second)
	n := ms.npc
	at := [3]float32{n.X, n.Y, n.Z}
	r.sent()

	r.w.mu.Lock()
	r.p.arenaStay = &arenaStay{from: GateSpot{World: 0, X: r.p.X, Y: r.p.Y, Z: r.p.Z}, match: 7}
	r.p.World, r.p.Instance, r.p.X, r.p.Z = DuelArenaWorld, 7, arenaX, arenaZ
	r.w.mu.Unlock()
	for i := 2; i <= 5; i++ {
		r.tick(time.Duration(i) * time.Second)
	}
	if n.World != 0 || [3]float32{n.X, n.Y, n.Z} != at {
		t.Fatalf("in the duel: the merc at world %d (%.1f, %.1f, %.1f), want where it was in world 0 %v", n.World, n.X, n.Y, n.Z, at)
	}
	want := fmt.Sprintf(mercArenaWaitLine, n.Name)
	if got := strings.Count(strings.Join(ownerLines(r.sess), "\n"), want); got != 1 {
		t.Errorf("told %q %d times, want once", want, got)
	}

	// back out of the arena, then a teleport elsewhere: it follows
	r.w.mu.Lock()
	r.p.arenaStay = nil
	r.p.World, r.p.Instance, r.p.X, r.p.Z = 2, 0, 5000, 5000
	r.w.mu.Unlock()
	r.tick(6 * time.Second)
	if n.World != 2 || distXZ(n.X, n.Z, r.p.X, r.p.Z) > 20 {
		t.Errorf("out of the duel: the merc at world %d (%.1f, %.1f), want beside its owner in world 2", n.World, n.X, n.Z)
	}
}
