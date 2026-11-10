package entity

import (
	"os"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/ground"
)

// A real server's database with tools/fix_pets_1110.py applied (EQOA_REAL_DB,
// copied; skipped when its spell 905495 is not there): the Efreeti Lord's kit
// ends with Efreeti's Fire on its owner (petition 1478), the Ghilan Noble's
// with its tide, a heal of its group (petition 1495), both rows read whole;
// and each of Permafrost's ice goblin shamans, hit by a player, has a Vigilant
// Spirit come to its help against that player (petition 1493), on the real
// ground when EQOA_GROUND_DIR names one (their line of sight in the caves).
func TestRealPets1110(t *testing.T) {
	db := simRealDB(t)
	if _, ok, err := db.TemplateSpellByID(905495); err != nil || !ok {
		t.Skipf("spell 905495: there %v, %v: tools/fix_pets_1110.py not applied to this database", ok, err)
	}
	t.Run("the kits", func(t *testing.T) {
		r := newPetRig(t)
		if _, err := r.w.LoadClassTemplates(db); err != nil {
			t.Fatal(err)
		}
		r.p.Class, r.p.Level = 10, 60
		for id, want := range map[int]string{701087: "buff owner Efreeti's Fire", 701088: "heal group Ghilan's Healing Tide"} {
			s := r.w.templates.sheets[id]
			if s == nil || len(s.abilities) == 0 {
				t.Fatalf("petstats %d: no workbook pet (its row unreadable?): %v", id, r.w.ClassTemplateProblems())
			}
			a := s.abilities[len(s.abilities)-1]
			got := a.Kind + " " + a.TargetOr()
			if a.pinned != nil {
				got += " " + a.pinned.Name
			}
			if got != want {
				t.Errorf("petstats %d (%s): its last ability %q, want %q", id, s.Name, got, want)
			}
		}
	})
	t.Run("Permafrost's shamans", func(t *testing.T) {
		w := NewWorld()
		w.LoadNPCs(database.NewNPCStore(db))
		if _, err := w.LoadSocialRules(db); err != nil {
			t.Fatal(err)
		}
		if dir := os.Getenv("EQOA_GROUND_DIR"); dir != "" {
			g, err := ground.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Close()
			w.SetGround(g)
		}
		p := &Player{ServerID: 9001, Name: "Waldorf", Level: 60, HP: 5000, MaxHP: 5000, World: 0}
		w.Players[p.ServerID] = p
		shamans := 0
		for _, n := range w.NPCs {
			if !strings.EqualFold(strings.TrimSpace(n.Name), "ice goblin shaman") || n.World != 0 ||
				n.X < 4600 || n.X > 5400 || n.Z < 4000 || n.Z > 4800 {
				continue
			}
			shamans++
			p.X, p.Y, p.Z = n.X+1, n.Y, n.Z
			w.mu.Lock()
			w.updateAggro(n, p.ServerID, 100)
			var helper *NPC
			for _, id := range w.SpatialIndex.QueryRadius(n.X, n.Z, 25) {
				if h := w.NPCs[id]; h != nil && h.Name == "Vigilant Spirit" && h.CurrentEnemy == p.ServerID {
					helper = h
				}
			}
			w.mu.Unlock()
			if helper == nil {
				t.Errorf("the ice goblin shaman at (%.0f, %.0f, %.0f) hit: no Vigilant Spirit came to its help", n.X, n.Y, n.Z)
				continue
			}
			t.Logf("the shaman at (%.0f, %.0f, %.0f): its spirit %.1f off comes", n.X, n.Y, n.Z,
				dist3(n.X, n.Y, n.Z, helper.X, helper.Y, helper.Z))
		}
		if shamans != 7 {
			t.Errorf("%d ice goblin shamans in Permafrost, want its 7", shamans)
		}
	})
}
