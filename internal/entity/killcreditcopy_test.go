package entity

import (
	"slices"
	"testing"
	"time"
)

// Heirloom petition 2574 (Nephycelium, Tomb of Kings): copies of one row (two dungeon instances') each count only
// the damage made on them. A copy shared its original's damage map (`c := *t`), so once the original held one, every
// copy's damage went into that one map and a kill in Nephycelium's instance went to Skywalker, who had hit the same
// creature in his own. The control: a copy's own damage still decides its own kill.
func TestCopiesOfARowCountTheirOwnDamage(t *testing.T) {
	w := NewWorld()
	mk := func(id EntityID, name string) *Player {
		p := &Player{ServerID: id, Name: name, Level: 60, HP: 100, MaxHP: 100, World: 14, CharServerID: uint32(id)}
		w.Players[id] = p
		return p
	}
	neph, sky := mk(1, "Nephycelium"), mk(2, "Skywalker")
	w.mu.Lock()
	defer w.mu.Unlock()
	row := &NPC{ServerID: 9100, Name: "Excavation Team", World: 14, HP: 14860, MaxHP: 14860}
	w.NPCs[row.ServerID] = row
	w.tallyDamageLocked(row, sky, 1300) // a splash on the original: it holds a map now
	mine := w.spawnCopyLocked(row, 10, 0, 10, 0, 14)
	his := w.spawnCopyLocked(row, 10, 0, 10, 0, 14)
	w.tallyDamageLocked(his, sky, 20000)
	w.tallyDamageLocked(mine, neph, 14860)
	if n := mine.damageBy[sky.ServerID]; n != 0 {
		t.Fatalf("Skywalker's damage on another copy counted on Nephycelium's: %d", n)
	}
	if _, got := w.killCreditLocked(mine, nil, neph); got != neph {
		t.Fatalf("Nephycelium's kill in his own instance went to %v", got)
	}
	if _, got := w.killCreditLocked(his, nil, neph); got != sky {
		t.Fatalf("the control: Skywalker's copy, his damage: the kill went to %v", got)
	}
	if n := row.damageBy[sky.ServerID]; n != 1300 {
		t.Fatalf("the original's own tally changed: %d", n)
	}
}

// Petition 2574 too: a pet's area in a dungeon instance takes no creature of another instance standing at the same
// spot (the control: its own instance's, beside it, is taken).
func TestAPetsAreaStaysInItsInstance(t *testing.T) {
	r := newPetRig(t)
	pet, _ := r.summon(t, fireElemental(), 0)
	SetInstancedWorld(pet.World, true)
	t.Cleanup(func() { SetInstancedWorld(pet.World, false) })
	r.p.Instance = 7
	mk := func(id EntityID, inst uint32, dx float32) *NPC {
		n := &NPC{ServerID: id, Name: "Excavation Team", Level: 40, HP: 5000, MaxHP: 5000, X: pet.X + 3 + dx, Y: pet.Y, Z: pet.Z,
			World: pet.World, Instance: inst, Temporary: true}
		r.w.NPCs[id] = n
		r.w.SpatialIndex.Insert(id, n.X, n.Z)
		return n
	}
	target, beside, other := mk(7201, 7, 0), mk(7202, 7, 2), mk(7203, 8, 2)
	r.w.mu.Lock()
	defer r.w.mu.Unlock()
	foes := r.w.petAreaFoesLocked(pet, target, SpellArea{Centre: AreaOnTarget, Radius: 30, Max: 10}, r.at(time.Second))
	if !slices.Contains(foes, beside) || slices.Contains(foes, other) {
		t.Errorf("its own instance's taken %v, another instance's %v", slices.Contains(foes, beside), slices.Contains(foes, other))
	}
}
