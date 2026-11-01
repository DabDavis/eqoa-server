package entity

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The town dialog over a real server's NPCs (EQOA_REAL_DB, a database
// tools/seed_town_dialog.py has seeded; a copy is opened): every speaker
// (dialogSpeakerLocked) has a line for a neutral listener of every race, no
// wildlife or undead speaks, and the speakers are counted by town and role.
// Skipped without a database, or with npcdialog empty.
func TestRealTownDialog(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := realWorld(t)
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "dialog.db")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := w.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.LoadUndeadTags(db); err != nil {
		t.Fatal(err)
	}
	lines, err := db.NPCDialog()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Skip("npcdialog is empty: tools/seed_town_dialog.py --apply first")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	byTown, byRole := map[string]int{}, map[string]int{}
	guests := map[int]int{} // the speakers who are guests, by race
	speakers, silentFolk := 0, 0
	for _, npc := range w.NPCs {
		town, role, ok := w.dialogSpeakerLocked(npc)
		if !ok {
			if DialogTown(w.npcFactionLocked(npc)) != "" && w.playerModels[uint32(npc.ModelID)] {
				silentFolk++ // a town's person who says nothing: wildlife, undead, a dummy
			}
			continue
		}
		speakers++
		byTown[town]++
		byRole[role]++
		if _, wild := w.wildlifeLocked(npc); wild || w.undeadLocked(npc) {
			t.Errorf("%s speaks: wildlife or undead", npc.Name)
		}
		voice := w.dialogVoiceLocked(npc, town)
		if !voice.Resident && voice.Race >= 0 {
			guests[voice.Race]++
		}
		for race := 0; race <= 10; race++ {
			picks := DialogCandidates(lines, town, role, npc.Name, MoodNeutral, w.dialogSexLocked(npc), race, voice)
			if len(picks) == 0 {
				t.Errorf("%s (%s, %s) has nothing to say to race %d", npc.Name, town, role, race)
				break
			}
			// each in its own people's voice: a guest never speaks for its host
			// town, a troll or an ogre never says a common line
			for _, c := range picks {
				l := c.Line
				if l.Voiced || l.Name != "" {
					continue
				}
				if l.Town != "" && !voice.Resident {
					t.Errorf("%s, a guest in %s (race %d), would say the town's %q", npc.Name, town, voice.Race, l.Line)
				}
				if l.Town == "" && BrokenSpeech(voice.Race) {
					t.Errorf("%s (race %d) would say the common %q", npc.Name, voice.Race, l.Line)
				}
			}
		}
	}
	var towns []string
	for town := range byTown {
		towns = append(towns, town)
	}
	sort.Strings(towns)
	for _, town := range towns {
		name := town
		if name == "" {
			name = "(no town: guards on the roads)"
		}
		t.Logf("%-32s %d speakers", name, byTown[town])
	}
	t.Logf("guests (a race not their town's own) by race: %v", guests)
	t.Logf("%d speakers of %d NPCs (by role %v); %d of a town's people say nothing (wildlife, undead, dummies)",
		speakers, len(w.NPCs), byRole, silentFolk)
	if speakers == 0 {
		t.Error("no NPC speaks")
	}
}
