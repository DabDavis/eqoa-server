package entity

import "testing"

// The owner, 2026-11-12: Freeport's Dungeon Warden became the papered "Instanced Raids" board against the well
// (tools/raidboard_20261112.py); it opens the same dungeons, the entrance twins stay wardens, and it has a plate.
func TestTheInstancedRaidsBoardOpensTheDungeons(t *testing.T) {
	for _, name := range []string{"Dungeon Warden", "Instanced Raids", " instanced raids "} {
		if !IsDungeonWarden(name) {
			t.Errorf("%q does not open the dungeons", name)
		}
	}
	if IsDungeonWarden("PvP Events") || IsDungeonRaidBoard("Dungeon Warden") || !IsDungeonRaidBoard("Instanced Raids") {
		t.Error("the board's name test is wrong")
	}
	board := &NPC{Name: DungeonRaidBoardName, ModelID: int64(PaperedBoardBody), Size: 1}
	if !board.hasPlate() {
		t.Error("the Instanced Raids board shows no name plate")
	}
	w := NewWorld()
	w.dungeons.on, w.dungeons.world = true, 14
	w.NPCs[10] = &NPC{ServerID: 10, Name: DungeonRaidBoardName, World: 0, X: 100, Z: 200, Facing: 0}
	spot, _, ok := w.dungeonWardenSpotLocked()
	if !ok || spot.Place != "in front of the Instanced Raids" || spot.Z <= 200 {
		t.Errorf("the way out: %+v %v", spot, ok)
	}
}
