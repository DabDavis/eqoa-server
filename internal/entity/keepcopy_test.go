package entity

import (
	"encoding/json"
	"testing"
	"time"
)

// keepCopiesFile is kept through JSON, as the file holds it.
func keepCopiesFile(t *testing.T, kept []KeptCopy) []KeptCopy {
	t.Helper()
	b, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	var back []KeptCopy
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	return back
}

// killCopy lays npc dead at died.
func killCopy(w *World, npc *NPC, died time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	npc.HP, npc.Dead, npc.DeathTime = 0, true, died
	w.wakeNPCLocked(npc)
}

// The owner, 2026-11-12 ("the rolling reset is deleting loot on mobs"): a copy's
// corpse (a quest step's spawn, Graal the Leviathan; a trigger's; a GM's) comes
// through a keep restart. Written beside the rows' creatures and read back in a
// new process (the same database), it lies dead where it fell, under its own id,
// with its death's time, level and flags, and goes when its corpse would have;
// one whose id something already holds is made under another, and the restore
// says which. Not carried: a living copy, a copy in an instance, a pet. The
// control: the new process given nothing has no corpse there (what a restart did).
func TestACopysCorpseComesThroughARestart(t *testing.T) {
	db := keepNPCDB(t)
	old := keepNPCProcess(t, db)
	died := time.Now().Add(-10 * time.Second)
	spawn := func(x float32) *NPC {
		id, _, ok := old.SpawnNPCCopy("Lord Keeper", x, 10, 2005, 1, 0)
		if !ok {
			t.Fatal("no spawn")
		}
		return old.NPCs[id]
	}
	dead := spawn(2210)
	old.mu.Lock()
	dead.NPCType &^= 0x80 // a hostile quest spawn
	dead.QuestSpawn = true
	dead.MaxHP = 4321
	old.mu.Unlock()
	killCopy(old, dead, died)
	alive := spawn(2220)
	inInstance := spawn(2230)
	old.mu.Lock()
	inInstance.Instance = 3
	old.mu.Unlock()
	killCopy(old, inInstance, died)
	add := spawn(2240) // a creature's summoned add carries nothing (masterdeath.go)
	old.mu.Lock()
	add.withMaster = true
	old.mu.Unlock()
	killCopy(old, add, died)
	pet := &NPC{ServerID: dead.ServerID + 100, Name: "a skeleton", Temporary: true, Owner: 1, CopyOf: keepBoss, Level: 20, MaxHP: 50}
	old.mu.Lock()
	old.putNPCLocked(pet)
	old.mu.Unlock()
	killCopy(old, pet, died)

	kept := keepCopiesFile(t, old.KeepCopies())
	if len(kept) != 1 || EntityID(kept[0].ID) != dead.ServerID || EntityID(kept[0].Row) != dead.CopyOf || !kept[0].Died.Equal(died) {
		t.Fatalf("written %+v, want the dead copy %d alone", kept, dead.ServerID)
	}

	// the control: a new process given nothing has no corpse on the id
	c := keepNPCProcess(t, db)
	if c.NPCs[dead.ServerID] != nil {
		t.Fatal("control: the corpse is there with nothing restored")
	}

	w := keepNPCProcess(t, db)
	ids, lost := w.RestoreCopies(kept)
	w.SetCorpseLinger(-1) // the creature's own corpse time, no linger (corpselinger.go)
	got := w.NPCs[dead.ServerID]
	if lost != 0 || ids[dead.ServerID] != dead.ServerID || got == nil {
		t.Fatalf("restored %v (%d lost), %+v", ids, lost, got)
	}
	if !got.Dead || got.HP != 0 || !got.DeathTime.Equal(died) || !got.Temporary || got.CopyOf != dead.CopyOf ||
		got.X != dead.X || got.Z != dead.Z || got.MaxHP != 4321 || got.NPCType != dead.NPCType || !got.QuestSpawn || got.Name != dead.Name {
		t.Fatalf("the corpse after the restart: %+v", got)
	}
	if lies, ok := w.CorpseLies(got.ServerID); !ok || !lies.Equal(died) {
		t.Fatal("its corpse does not lie")
	}
	if w.NPCs[alive.ServerID] != nil || w.NPCs[inInstance.ServerID] != nil || w.NPCs[pet.ServerID] != nil {
		t.Fatal("a living copy, an instance's or a pet came back")
	}
	// it goes when its corpse would have: a copy is taken away, not raised
	w.mu.Lock()
	w.corpseSystem(got.corpseEnds(w.corpseDecay).Add(time.Second))
	w.mu.Unlock()
	if w.NPCs[dead.ServerID] != nil {
		t.Fatal("the copy's corpse outlived its decay")
	}

	// its id taken: made under another, and said so
	w2 := keepNPCProcess(t, db)
	w2.mu.Lock()
	w2.putNPCLocked(&NPC{ServerID: dead.ServerID, Name: "squatter", Temporary: true})
	w2.mu.Unlock()
	ids, _ = w2.RestoreCopies(kept)
	if to := ids[dead.ServerID]; to == 0 || to == dead.ServerID || w2.NPCs[to] == nil || !w2.NPCs[to].Dead {
		t.Fatalf("over a taken id: %v", ids)
	}
	// and a row gone or another's makes nothing
	k := kept[0]
	k.Name = "someone else"
	if ids, lost := keepNPCProcess(t, db).RestoreCopies([]KeptCopy{k}); len(ids) != 0 || lost != 1 {
		t.Fatalf("another's row: %v, %d lost", ids, lost)
	}
}
