package entity

import "testing"

// BeginWarp is the travel NPCs' state change made atomic, because the account
// pages perform it from an HTTP goroutine that races the tick loop.
func TestBeginWarpIncrementsTheCounter(t *testing.T) {
	w := NewWorld()
	p := &Player{ServerID: 1, World: 0, Facing: 2.5}
	w.Players[p.ServerID] = p

	facing, first, changed, ok := w.BeginWarp(1, 9)
	if !ok {
		t.Fatal("BeginWarp did not find the player")
	}
	if first == 0 {
		t.Error("the teleport counter must advance; the travel NPCs send an " +
			"incrementing one and #goto's constant 0 is the path that does not warp live")
	}
	if !changed {
		t.Error("world 0 -> 9 is a world change")
	}
	if facing != 2.5 {
		t.Errorf("facing came back as %v, want the player's own 2.5", facing)
	}

	_, second, changed, _ := w.BeginWarp(1, 9)
	if second <= first {
		t.Errorf("counter went %d -> %d; each warp needs its own", first, second)
	}
	if changed {
		t.Error("9 -> 9 is not a world change; holding the channels for a load " +
			"that is not happening strands the client")
	}
	if p.World != 9 {
		t.Errorf("destination world not committed: %d", p.World)
	}
}

func TestBeginWarpReportsAnAbsentPlayer(t *testing.T) {
	w := NewWorld()
	if _, _, _, ok := w.BeginWarp(404, 0); ok {
		t.Error("BeginWarp claimed to warp a player who is not in the world")
	}
}
