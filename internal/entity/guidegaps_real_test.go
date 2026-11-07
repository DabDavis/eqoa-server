package entity

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The guides' raid bosses on a database with tools/seed_guide_gaps.py's sets in
// (EQOA_REAL_DB; skipped without one, none is in this repository): each fights by its
// template at the guide's figures, is no one's charm, attacks on sight once a trigger has
// spawned it however player-like its body, and is reached at its size (EQOA_MODELS names a
// models.txt). A hit is measured as the game makes it on a warrior of the guide's 6,850 AC.
func TestRealGuideBosses(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database with the guide-gap sets")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	w := NewWorld()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadCharmRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if m := os.Getenv("EQOA_MODELS"); m != "" {
		if radii, err := LoadModelRadii(m); err == nil && len(radii) > 0 {
			SetModelRadii(radii)
			t.Cleanup(func() { SetModelRadii(nil) })
		}
	}
	bosses := []struct {
		name           string
		minHP, maxHP   int
		minHit, maxHit int
		hits           int
		minReach       float32
	}{
		{"Talon's Veeshan", 290000, 310000, 2100, 2900, 4, 0},
		{"Veeshan's Bulwark", 950000, 1050000, 900, 1200, 4, 0},
		{"Construct of Radiance", 190000, 210000, 200, 400, 4, 0},
		{"Rallos Zek", 480000, 520000, 1900, 2500, 4, 3},
		{"Rallos Zek the Warlord", 480000, 520000, 1900, 2500, 4, 3},
		{"Rallos Zek Unbound", 480000, 520000, 3100, 4100, 2, 3},
		{"Razeal Moloch", 380000, 420000, 1500, 2600, 4, 0},
	}
	for _, b := range bosses {
		var npc *NPC
		for _, n := range w.NPCs {
			if n.Name == b.name {
				npc = n
				break
			}
		}
		if npc == nil {
			t.Errorf("%s: no row", b.name)
			continue
		}
		if npc.TemplateName() == "" {
			t.Errorf("%s: fights by no template", b.name)
		}
		if npc.MaxHP < b.minHP || npc.MaxHP > b.maxHP {
			t.Errorf("%s: %d HP, want %d-%d", b.name, npc.MaxHP, b.minHP, b.maxHP)
		}
		p := &Player{Name: "tank", Level: 60, Class: 0, AC: 6850, MaxHP: 13000, HP: 13000}
		w.mu.Lock()
		lo, hi := 1<<30, 0
		for i := 0; i < 200; i++ {
			d := w.npcHitPlayerLocked(npc, p, npc.meleeOut(npcMeleeRollFor(npc, p).hitOn(p, meleeIntn)))
			lo, hi = min(lo, d), max(hi, d)
		}
		_, _, _, hits := npcRound(npc)
		// a trigger's spawn: no NPC bit, attacks on sight whatever its body
		c := *npc
		c.NPCType &^= 0x80
		c.EventAggro = true
		threatens := w.threatensLocked(&c, p)
		r, ruled := w.charmRuleLocked(npc)
		refused := w.charmStartLocked(&CastSpell{}, []EntityID{npc.ServerID})
		w.mu.Unlock()
		if refused != charmCannot {
			t.Errorf("%s: a charm's start says %q, want %q", b.name, refused, charmCannot)
		}
		fmt.Printf("%s: %d HP, %d-%d on a 6,850 AC warrior, %d hits a round, reach +%.1f, charmable %v\n", b.name, npc.MaxHP, lo, hi, len(hits), npcReachExtra(npc), !ruled || r.Charmable)
		if lo < b.minHit || hi > b.maxHit {
			t.Errorf("%s: hits %d-%d on 6,850 AC, want within %d-%d", b.name, lo, hi, b.minHit, b.maxHit)
		}
		if len(hits) != b.hits {
			t.Errorf("%s: %d hits a round, want %d", b.name, len(hits), b.hits)
		}
		if !threatens {
			t.Errorf("%s: a trigger's spawn of it does not attack on sight", b.name)
		}
		if !ruled || r.Charmable {
			t.Errorf("%s: may be charmed", b.name)
		}
		if os.Getenv("EQOA_MODELS") != "" && npcReachExtra(npc) < b.minReach {
			t.Errorf("%s: reach +%.1f, want at least +%.1f at size %.2f", b.name, npcReachExtra(npc), b.minReach, npc.Size)
		}
	}
}
