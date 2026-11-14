package entity

import (
	"testing"
	"time"
)

// Petition 5035: a corpse lies 20 minutes from the death while its creature comes back as
// before. At the creature's own corpse time (3 minutes) its corpse is left where it fell as a
// copy of it (dead, the same death, its row's), told to the hook; the creature goes and comes
// back its respawn time later, as before; the copy is no one's to open until its loot has
// moved (CorpseMoved), lies on, says it decays at 20 minutes, and goes then. The control: the
// linger off, the corpse goes at 3 minutes as before.
func TestACorpseLiesTwentyMinutesItsCreatureBackAsBefore(t *testing.T) {
	died := time.Now().Add(-time.Second)
	w, npc := corpseWorld(t, died)
	var left []CorpseLeft
	w.OnCorpseLeft(func(l []CorpseLeft) { left = append(left, l...) })
	if end, ok := w.CorpseDecaysAt(npc.ServerID); !ok || !end.Equal(died.Add(20*time.Minute)) {
		t.Fatalf("the creature's corpse decays at %v, want 20m", end.Sub(died))
	}

	at := died.Add(DefaultCorpseDecay)
	w.mu.Lock()
	gone := w.corpseSystem(at)
	w.mu.Unlock()
	if len(gone) != 0 || !npc.CorpseGone.Equal(at) {
		t.Fatalf("at 3 minutes: gone %v, its corpse went %v", gone, npc.CorpseGone)
	}
	w.runCorpsesGone(gone)
	if len(left) != 1 || left[0].From != npc.ServerID || !left[0].Died.Equal(died) {
		t.Fatalf("the hook heard %+v", left)
	}
	c := w.NPCs[left[0].To]
	if c == nil || !c.Dead || !c.Temporary || c.CopyOf != npc.ServerID || !c.DeathTime.Equal(died) || c.X != 1003 || c.Z != 1000 {
		t.Fatalf("the corpse left: %+v", c)
	}
	if _, ok := w.CorpseLies(c.ServerID); ok {
		t.Fatal("the left corpse opens before its loot moved")
	}
	w.CorpseMoved(c.ServerID)
	if d, ok := w.CorpseLies(c.ServerID); !ok || !d.Equal(died) {
		t.Fatal("the left corpse does not lie once moved")
	}
	if end, ok := w.CorpseDecaysAt(c.ServerID); !ok || !end.Equal(died.Add(20*time.Minute)) {
		t.Fatalf("the left corpse decays at %v, want 20m", end.Sub(died))
	}

	// the creature back as before: its respawn time after 3 minutes
	w.mu.Lock()
	w.corpseSystem(at.Add(DefaultRespawnAfter))
	w.mu.Unlock()
	if npc.Dead || npc.X != npc.SpawnX {
		t.Fatalf("the creature not back at 3m + its respawn: dead %v", npc.Dead)
	}
	w.mu.Lock()
	gone = w.corpseSystem(died.Add(20*time.Minute - time.Second))
	w.mu.Unlock()
	if len(gone) != 0 || w.NPCs[c.ServerID] == nil {
		t.Fatal("the left corpse went before 20 minutes")
	}
	w.mu.Lock()
	gone = w.corpseSystem(died.Add(20 * time.Minute))
	w.mu.Unlock()
	if len(gone) != 1 || gone[0] != c.ServerID || w.NPCs[c.ServerID] != nil {
		t.Fatalf("at 20 minutes: gone %v, still in the world %v", gone, w.NPCs[c.ServerID] != nil)
	}

	// the control: no linger
	w2, npc2 := corpseWorld(t, died)
	w2.SetCorpseLinger(-1)
	w2.mu.Lock()
	gone = w2.corpseSystem(at)
	w2.mu.Unlock()
	if len(gone) != 1 || gone[0] != npc2.ServerID {
		t.Fatalf("the linger off: gone %v at 3 minutes", gone)
	}
}

// A copy's own corpse (a GM's /spawn) lies on in place to 20 minutes; a pet's goes at its time.
func TestACopysCorpseLiesOnInPlace(t *testing.T) {
	died := time.Now().Add(-time.Second)
	w, npc := corpseWorld(t, died)
	npc.Temporary, npc.CopyOf = true, 1
	w.mu.Lock()
	gone := w.corpseSystem(died.Add(DefaultCorpseDecay))
	w.mu.Unlock()
	if len(gone) != 0 || w.NPCs[npc.ServerID] == nil || npc.CorpseDecay != 20*time.Minute {
		t.Fatalf("a copy's corpse at 3 minutes: gone %v, decay %v", gone, npc.CorpseDecay)
	}
	w2, pet := corpseWorld(t, died)
	pet.Owner = 1
	w2.mu.Lock()
	gone = w2.corpseSystem(died.Add(DefaultCorpseDecay))
	w2.mu.Unlock()
	if len(gone) != 1 {
		t.Fatalf("a pet's corpse at 3 minutes: gone %v", gone)
	}
}

// A left corpse's 20 minutes come through a keep restart (KeptCopy.Lies).
func TestALeftCorpsesTimeComesThroughARestart(t *testing.T) {
	died := time.Now().Add(-4 * time.Minute)
	w, npc := corpseWorld(t, died)
	w.mu.Lock()
	w.corpseSystem(died.Add(DefaultCorpseDecay))
	w.mu.Unlock()
	w.runCorpsesGone(nil)
	kept := w.KeepCopies()
	if len(kept) != 1 || kept[0].Row != uint32(npc.ServerID) || kept[0].Lies != 20*time.Minute {
		t.Fatalf("kept %+v", kept)
	}
	w2, _ := corpseWorld(t, died)
	w2.NPCs[npc.ServerID].Dead, w2.NPCs[npc.ServerID].DeathTime = false, time.Time{}
	ids, lost := w2.RestoreCopies(kept)
	c := w2.NPCs[ids[EntityID(kept[0].ID)]]
	if lost != 0 || c == nil || c.CorpseDecay != 20*time.Minute {
		t.Fatalf("restored %v (%d lost): %+v", ids, lost, c)
	}
	if _, ok := w2.CorpseLies(c.ServerID); !ok {
		t.Fatal("the restored corpse does not lie 4 minutes after the death")
	}
}
