package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

// Petition 1133, Jibmage: "Crocs on crocodile island are repopping way too fast. Up their
// pop time buy double." A row of npcrespawn is that NPC's time and no other's; taking the row
// away puts it back on the world's figure. The rule holds however the order of the start's
// calls falls: rules first and NPC after, or the other way round.
func TestARespawnRuleIsItsRowsAlone(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/respawn.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO npcrespawn (npc_row, seconds) VALUES (3216, 120), (77, 0)`); err != nil {
		t.Fatal(err)
	}
	w := NewWorld()
	w.SetCorpseTimes(0, 60*time.Second)
	croc, other, zero := &NPC{ServerID: 3216, Name: "crocodile"}, &NPC{ServerID: 3217, Name: "crocodile"}, &NPC{ServerID: 77, Name: "rat"}
	w.NPCs[croc.ServerID], w.NPCs[zero.ServerID] = croc, zero
	if n, err := w.LoadRespawnRules(db); err != nil || n != 1 {
		t.Fatalf("read %d rows (%v), want the one that counts", n, err)
	}
	if croc.RespawnTime != 120*time.Second {
		t.Errorf("an NPC already in the world: %v, want 2m", croc.RespawnTime)
	}
	w.mu.Lock()
	w.applyNPCSettingsLocked(other)
	late := &NPC{ServerID: 3216, Name: "crocodile"}
	w.applyNPCSettingsLocked(late)
	w.mu.Unlock()
	if other.RespawnTime != 60*time.Second {
		t.Errorf("the control, another row of the name: %v, want 1m", other.RespawnTime)
	}
	if late.RespawnTime != 120*time.Second {
		t.Errorf("made after the rules: %v, want 2m", late.RespawnTime)
	}
	w.SetCorpseTimes(0, 90*time.Second)
	if croc.RespawnTime != 120*time.Second {
		t.Errorf("after the world's figure changed: %v, want its own 2m", croc.RespawnTime)
	}
	if _, err := db.Exec(`DELETE FROM npcrespawn`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadRespawnRules(db); err != nil {
		t.Fatal(err)
	}
	if croc.RespawnTime != 90*time.Second {
		t.Errorf("the rule taken away: %v, want the world's 1m30s", croc.RespawnTime)
	}

	// and the corpse system waits for it
	w2 := NewWorld()
	w2.SetCorpseTimes(0, 60*time.Second)
	w2.SetRespawnRules(map[EntityID]time.Duration{5: 2 * time.Minute})
	n := &NPC{ServerID: 5, Name: "crocodile", Dead: true, HP: 0, MaxHP: 10}
	w2.NPCs[5] = n
	w2.mu.Lock()
	w2.applyNPCSettingsLocked(n)
	gone := time.Unix(1_800_000_000, 0)
	n.DeathTime, n.CorpseGone = gone.Add(-time.Minute), gone
	w2.sets.corpse.add(n)
	w2.corpseSystem(gone.Add(90 * time.Second))
	back1 := !n.Dead
	w2.corpseSystem(gone.Add(121 * time.Second))
	back2 := !n.Dead
	w2.mu.Unlock()
	if back1 || !back2 {
		t.Errorf("back at 90 s: %v, want false; at 121 s: %v, want true", back1, back2)
	}
}
