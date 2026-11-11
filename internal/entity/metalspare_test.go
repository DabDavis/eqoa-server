package entity

import "testing"

// A challenger's areas, and its pet's, leave Mr. Moogie out while it duels his Metal Moogie (the GM
// Jibrish's petition 1619: a pet's splash landed on Mr. Moogie beside his twin, and he killed it);
// before and after the duel, and for anyone else, he is a creature like any other.
func TestMrMoogieIsSparedByAMetalDuelistsAreas(t *testing.T) {
	w := &World{}
	moogie := &NPC{ServerID: 50, Name: MoogieName}
	other := &NPC{ServerID: 51, Name: "a rat"}
	const duelist, bystander = EntityID(7), EntityID(8)
	if w.metalSparesLocked(moogie, duelist) {
		t.Fatal("no duel: Mr. Moogie is not spared")
	}
	w.metal = &metalDuel{npc: 999999, name: MetalMoogieName, c: &npcChallenge{player: duelist}}
	if !w.metalSparesLocked(moogie, duelist) {
		t.Error("the duelist's area hits Mr. Moogie")
	}
	if !w.metalSparesLocked(&NPC{Name: " mr. moogie "}, duelist) {
		t.Error("the name is matched as Mr. Moogie's is elsewhere: any case, trimmed")
	}
	if w.metalSparesLocked(moogie, bystander) || w.metalSparesLocked(other, duelist) {
		t.Error("only Mr. Moogie, only for the duelist")
	}
}
