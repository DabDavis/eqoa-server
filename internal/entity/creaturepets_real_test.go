package entity

import (
	"os"
	"testing"
	"time"
)

// TestRealCreaturePets is the pets creatures summon over a real database (EQOA_REAL_DB, copied; one after
// tools/creature_pets_20261125.py): every owner alive in the world gets its pets at the first sweep, beside it and
// paired with it, the template rows stay out of the world, and a second sweep makes none.
func TestRealCreaturePets(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realCompanionWorld(t)
	db := simRealDB(t)
	owners, pets, err := w.LoadCreaturePets(db)
	if err != nil {
		t.Fatal(err)
	}
	if owners == 0 {
		t.Skip("no npcpets rows: run tools/creature_pets_20261125.py on the copy first")
	}
	now := time.Now()
	w.sweepCreaturePets(now)
	w.mu.Lock()
	w.linkCompanionsLocked()
	made, paired, far, withPets, wanting := 0, 0, 0, 0, 0
	for _, n := range w.NPCs {
		if n.petOf != 0 {
			made++
			if w.companionOf[n.ServerID] == n.petOf {
				paired++
			}
			if o := w.NPCs[n.petOf]; o == nil || distXZ(n.X, n.Z, o.X, o.Z) > 2 {
				far++
			}
			continue
		}
		if w.cpets.templates[n.ServerID] && !n.SpawnOnly {
			t.Errorf("template row %d %q stands in the world", n.ServerID, n.Name)
		}
		if rules := w.creaturePetRulesLocked(n); len(rules) > 0 && !n.Dead && !n.SpawnOnly && !n.hiddenAt(now) {
			wanting++
			if len(w.cpets.owners[n.ServerID].pets) > 0 {
				withPets++
			}
		}
	}
	w.mu.Unlock()
	t.Logf("npcpets: %d owners, %d pets; in the world: %d creatures want pets, %d have them; %d pets made, %d paired, %d not beside their owner",
		owners, pets, wanting, withPets, made, paired, far)
	if withPets != wanting || paired != made || far != 0 {
		t.Errorf("owners %d of %d with pets, pets %d of %d paired, %d not beside", withPets, wanting, paired, made, far)
	}
	w.sweepCreaturePets(now.Add(2 * time.Second))
	w.mu.Lock()
	again := 0
	for _, n := range w.NPCs {
		if n.petOf != 0 {
			again++
		}
	}
	w.mu.Unlock()
	if again != made {
		t.Errorf("a second sweep: %d pets, was %d", again, made)
	}
}
