package entity

import (
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A real server's database with tools/construct_quad_20261117.py applied
// (EQOA_REAL_DB, copied; skipped when its pettemplate row is not there): the
// Tinker's Construct and its two forms, summoned by a level 60 ranger, cleric
// and enchanter (whose pets swung once a round), each swing Attack4, four hits
// a 6 s round for 60 s at a creature; Tinker Kick at 0 s and then every 12 s
// (its recast) but when another cast holds it; arcane resist 315, the cap.
func TestRealConstructQuadKickAR(t *testing.T) {
	db := simRealDB(t)
	rows, _, err := db.PetTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if rows[database.PetTemplateKey{Spell: "construct"}].Template != "pet construct" {
		t.Skip("tools/construct_quad_20261117.py not applied to this database")
	}
	w := NewWorld()
	w.SessionManager = session.NewManager()
	w.LoadNPCs(database.NewNPCStore(db))
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadBalance(db); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	for _, id := range []int{901835, 901836, 901837} {
		for _, class := range []int{1, 9, 12} {
			p := &Player{ServerID: EntityID(9000 + class), Name: "Zerkziee", Race: 3, Class: class, Level: 60, HP: 5000, MaxHP: 5000,
				X: 5000, Y: 50, Z: 5000}
			w.Players[p.ServerID] = p
			ts, ok, err := db.TemplateSpellByID(id)
			if err != nil || !ok {
				t.Fatalf("spell %d: %v %v", id, ok, err)
			}
			sp := SpellOfRow(&ts.Pattern, 0, ts.Line, ts.Effects, ts.FX)
			w.mu.Lock()
			ps := w.makePetLocked(&sp, p, p, nil, now)
			w.mu.Unlock()
			if ps == nil {
				t.Fatalf("%d: no pet", id)
			}
			pet := ps.npc
			pet.X, pet.Y, pet.Z = 5000, 50, 5000
			foe := &NPC{ServerID: 77000, Name: "a training bag", Level: 55, HP: 50_000_000, MaxHP: 50_000_000, ModelID: 0x55432169,
				Size: 1, X: 5002, Y: 50, Z: 5000}
			w.mu.Lock()
			w.putNPCLocked(pet)
			w.putNPCLocked(foe)
			w.mu.Unlock()
			ps.target = foe.ServerID
			rounds, swings := 0, 0
			var kicks []float64
			var last time.Time
			for i := 0; i < 600; i++ {
				at := now.Add(time.Duration(i) * 100 * time.Millisecond)
				n0, h0 := len(w.petCasts), pet.RoundHits
				w.mu.Lock()
				w.petFightLocked(ps, foe, 0.1, at)
				w.petCastSystemLocked(at)
				w.mu.Unlock()
				if pet.RoundStart != last {
					rounds, last, h0 = rounds+1, pet.RoundStart, 0
				}
				swings += pet.RoundHits - h0
				for _, c := range w.petCasts[min(n0, len(w.petCasts)):] {
					if c.spell.SpellID == 902052 {
						kicks = append(kicks, float64(i)/10)
					}
				}
			}
			if k := npcSwingKind(pet); k != 5 || len(swingHits(k)) != 4 || rounds != 10 {
				t.Errorf("%d, owner class %d: swing %d, %d rounds in 60 s; want Attack4 (5), four hits, 10 rounds", id, class, k, rounds)
			}
			if id != 901836 && swings != 40 { // the drachnid's casts hold its melee a little (castBusy)
				t.Errorf("%d, owner class %d: %d swings in 10 rounds, want 40", id, class, swings)
			}
			if len(kicks) != 5 || kicks[0] != 0 {
				t.Errorf("%d, owner class %d: Tinker Kick at %v, want 0, 12, 24, 36, 48", id, class, kicks)
			}
			for j := 1; j < len(kicks); j++ {
				if gap := kicks[j] - kicks[j-1]; gap < 12 || gap > 14 {
					t.Errorf("%d, owner class %d: kicks %v: a gap of %.1f s, want its 12 s recast", id, class, kicks, gap)
				}
			}
			if ar := pet.petStat(StatAR); ar != ResistCap {
				t.Errorf("%d: AR %d, want %d", id, ar, ResistCap)
			}
			w.mu.Lock()
			delete(w.NPCs, foe.ServerID)
			delete(w.NPCs, pet.ServerID)
			w.mu.Unlock()
		}
	}
}
