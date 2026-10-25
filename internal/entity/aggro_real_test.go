package entity

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The aggro rule over a real server's NPCs (EQOA_REAL_DB names its database; a
// copy is opened): the creatures Sandstorm's captures showed (aggro.go's doc)
// take the side they took there, Tunaria's wildlife by Freeport hostile and
// Neriak's town creatures not, and it says how many of each kind there are and
// which named guards a rule would set on players. Skipped without a database:
// none is in this repository.
func TestTheAggroRuleOverARealWorld(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realWorld(t)
	// the GMs' rules (npcaggro) as the server reads them at its start, off
	// another copy
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rules.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := w.LoadAggroRules(db)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d npcaggro rules", n)
	w.mu.Lock()
	defer w.mu.Unlock()

	var hostile, typed, townCreature, folk, bit int
	var guards []string
	for _, npc := range w.NPCs {
		switch {
		case npc.NPCType&0x80 != 0:
			bit++
		case npc.ModelID != 0 && w.playerModels[uint32(npc.ModelID)]:
			folk++
		}
		if !w.hostileLocked(npc) {
			if npc.NPCType&0x80 == 0 && npc.ModelID != 0 && !w.playerModels[uint32(npc.ModelID)] && npc.Faction != FactionHostile {
				townCreature++ // a creature's body, peaceful: its spawn is in a town
			}
			continue
		}
		hostile++
		if npc.Faction == FactionHostile {
			typed++
		}
		if strings.Contains(strings.ToLower(npc.Name), "guard") {
			guards = append(guards, npc.Name)
		}
	}
	sort.Strings(guards)
	t.Logf("%d NPCs: %d attack on sight (%d of them typed hostile), %d creature bodies peaceful (in a town, or by a rule), %d townsfolk bodies, %d with the NPC bit",
		len(w.NPCs), hostile, typed, townCreature, folk, bit)
	t.Logf("named guards a rule would set on players (%d): %v", len(guards), guards)

	// Sandstorm's: by Freeport (a level-51 player at (13612, 14935)) the
	// wildlife was hostile; in Neriak (a level-1 player at (25308, 8798)) the
	// town's creatures attacked no one.
	near := func(x, z float32, names ...string) (found []*NPC) {
		for _, npc := range w.NPCs {
			if npc.World != 0 || distXZ(npc.SpawnX, npc.SpawnZ, x, z) > 400 {
				continue
			}
			for _, n := range names {
				if npc.Name == n {
					found = append(found, npc)
				}
			}
		}
		return found
	}
	wild := near(13612, 14935, "Black Bear Cub", "Young Bear", "Lavacrawler ", "Young Lioness")
	// those that came within 30 of the player (a Fire Bat at 8.0, a Tavern
	// Rat at 23.5) or showed it the white face (a Cave Minnow); the City
	// Cats came no nearer than 41, which says nothing
	town := near(25308, 8798, "Fire Bat", "Cave Minnow", "Tavern Rat")
	for _, npc := range near(25308, 8798, "City Cat") {
		t.Logf("City Cat at (%.0f, %.0f) attacks on sight: %v", npc.SpawnX, npc.SpawnZ, w.hostileLocked(npc))
	}
	t.Logf("by Freeport %d of the captured wildlife's names, in Neriak %d of its town creatures'", len(wild), len(town))
	for _, npc := range wild {
		if !w.hostileLocked(npc) {
			t.Errorf("%q at (%.0f, %.0f) is peaceful: Sandstorm's were hostile", npc.Name, npc.SpawnX, npc.SpawnZ)
		}
	}
	for _, npc := range town {
		if w.hostileLocked(npc) {
			t.Errorf("%q at (%.0f, %.0f) attacks on sight: Neriak's attacked no one (tools/seed_npc_aggro.py --apply seeds Cave Minnow)", npc.Name, npc.SpawnX, npc.SpawnZ)
		}
	}
}
