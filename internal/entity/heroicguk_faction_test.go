package entity

import (
	"testing"

	"github.com/eqoa/server/internal/database"
)

func TestFroglokFactionInHeroicGuk(t *testing.T) {
	const guk = "The Frogloks of Guk"
	const rivals = "The Dark Ones"
	const friendly = 750
	for _, tc := range []struct {
		name, dungeon, npc string
		protected          bool
	}{
		{name: "Heroic Guk", dungeon: "guk", npc: "froglok tal wizard", protected: true},
		{name: "regular Guk", npc: "froglok tal wizard"},
		{name: "another heroic dungeon", dungeon: "bb", npc: "gnoll scout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newDungeons(t, 2)
			w := r.w
			frog, human := r.ps[0], r.ps[1]
			frog.Race = raceFroglok
			r.group(frog, human)
			if kept, unknown := w.SetNPCFactions([]database.NPCFactionRow{{Name: tc.npc, World: -1, Faction: guk}}); kept != 1 || unknown != 0 {
				t.Fatalf("NPC faction rows: kept %d, unknown %d", kept, unknown)
			}
			if kept, unknown := w.SetFactionHits([]database.FactionHitRow{
				{NPCName: tc.npc, Faction: guk, Delta: -10},
				{NPCName: tc.npc, Faction: rivals, Delta: 5},
			}); kept != 2 || unknown != 0 {
				t.Fatalf("kill faction rows: kept %d, unknown %d", kept, unknown)
			}

			var npc *NPC
			if tc.dungeon != "" {
				for _, sess := range r.ss {
					if why, ok := w.DungeonEnter(sess, tc.dungeon, r.now); !ok {
						t.Fatalf("entering dungeon: %s", why)
					}
				}
				r.land()
				npc = r.copies(frog.InstanceKey())[tc.npc]
				if npc == nil {
					t.Fatal("dungeon creature missing")
				}
			} else {
				npc = &NPC{ServerID: 920010, Name: tc.npc, X: 1010, Y: 50, Z: 1000, Level: 50, HP: 50, MaxHP: 50}
			}

			w.mu.Lock()
			defer w.mu.Unlock()
			if tc.dungeon == "" {
				w.putNPCLocked(npc)
				w.SpatialIndex.Insert(npc.ServerID, npc.X, npc.Z)
			}
			for _, p := range r.ps {
				w.SpatialIndex.Remove(p.ServerID, p.X, p.Z)
				p.X, p.Y, p.Z = npc.X+5, npc.Y, npc.Z
				w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
				p.Factions = map[string]int{guk: friendly, rivals: 0}
			}
			if got := w.targetFaceLocked(frog, npc) == FaceHostile; got != tc.protected {
				t.Errorf("hostile target face for friendly Froglok: %t, want %t", got, tc.protected)
			}
			w.drawNearLocked(frog, r.now, true)
			if got := npc.CurrentEnemy == frog.ServerID; got != tc.protected {
				t.Errorf("aggro on friendly Froglok: %t, want %t", got, tc.protected)
			}
			if w.threatensLocked(npc, human) {
				t.Error("an allied human draws aggro")
			}
			if !tc.protected {
				frog.Factions[guk] = FactionKOSAt
				if !w.threatensLocked(npc, frog) {
					t.Error("the faction should attack a kill-on-sight Froglok")
				}
				frog.Factions[guk] = friendly
			}

			// Both races get the kill as the killer and as a group member.
			for i, killer := range r.ps {
				w.factionKillLocked(r.ss[i], killer, npc)
			}
			for i, p := range r.ps {
				wantGuk, wantRivals, wantLines := friendly-20, 10, 4
				if p == frog && tc.protected {
					wantGuk, wantRivals, wantLines = friendly, 0, 0
				}
				if gotGuk, gotRivals := p.Factions[guk], p.Factions[rivals]; gotGuk != wantGuk || gotRivals != wantRivals {
					t.Errorf("race %d standings: %d/%d, want %d/%d", p.Race, gotGuk, gotRivals, wantGuk, wantRivals)
				}
				if n := len(factionLines(r.ss[i])); n != wantLines {
					t.Errorf("race %d faction messages: %d, want %d", p.Race, n, wantLines)
				}
			}
		})
	}
}
