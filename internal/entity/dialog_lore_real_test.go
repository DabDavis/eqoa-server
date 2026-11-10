package entity

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// How often a town's people and the guards speak of the world's lore, over a
// real server's NPCs (EQOA_REAL_DB, a database that holds the lore set:
// tools/seed_town_dialog.py --set lore --apply; a copy is opened). The lore
// lines are npcdialog's rows of source "lore" (the user, 2026-11-10: "Let's
// add more lines, the NPCs talk about random eqoa lore"); they are weighed
// against every other line a speaker has for a neutral listener, of each of
// the ten races in turn, as TownLine picks them. Held: every speaker has lore
// to say (a town's own, its people's, or the common lines'), no speaker says
// lore in more than 60% of talks, and the median speaker in 25% to 50% (with
// the 549 lines of 2026-11-10: 44%, 22% to 57%; a small town's folk and a
// guard on a road 49-51%, a troll or an ogre at home 29%), so lore is heard
// often and the everyday talk still is. The shares are logged by role and by
// town: run it after adding lines to see what the new ones did to the mix.
// Skipped without a database, or with no lore line in it.
func TestRealTownDialogLore(t *testing.T) {
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
	lore := map[int]bool{}
	rows, err := db.Query(`SELECT id FROM npcdialog WHERE source = 'lore'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		lore[id] = true
	}
	rows.Close()
	if len(lore) == 0 {
		t.Skip("no line of source lore: tools/seed_town_dialog.py --set lore --apply first")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	type tally struct {
		shares []float64
		kinds  []int // how many different lore lines a speaker of it has
	}
	add := func(m map[string]*tally, key string, share float64, kinds int) {
		if m[key] == nil {
			m[key] = &tally{}
		}
		m[key].shares = append(m[key].shares, share)
		m[key].kinds = append(m[key].kinds, kinds)
	}
	median := func(v []float64) float64 {
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return s[len(s)/2]
	}
	lowHigh := func(v []float64) (float64, float64) {
		lo, hi := v[0], v[0]
		for _, x := range v {
			lo, hi = min(lo, x), max(hi, x)
		}
		return lo, hi
	}
	all, byRole, byTown, byKind := &tally{}, map[string]*tally{}, map[string]*tally{}, map[string]*tally{}
	for _, npc := range w.NPCs {
		town, role, ok := w.dialogSpeakerLocked(npc)
		if !ok || ownLines(lines, npc.Name) {
			continue // not a speaker, or one with a pool of its own (it says those alone)
		}
		voice, sex := w.dialogVoiceLocked(npc, town), w.dialogSexLocked(npc)
		loreW, allW := 0, 0
		said := map[int]bool{}
		for race := 0; race <= 9; race++ {
			for _, c := range DialogCandidates(lines, town, role, npc.Name, MoodNeutral, sex, race, voice) {
				allW += c.Weight
				if lore[c.Line.ID] {
					loreW += c.Weight
					said[c.Line.ID] = true
				}
			}
		}
		if allW == 0 {
			t.Errorf("%s (%s, %s) has nothing to say", npc.Name, town, role)
			continue
		}
		share := float64(loreW) / float64(allW)
		kind := "a town's own folk"
		switch {
		case town == "":
			kind = "a guard on a road"
		case !voice.Resident:
			kind = "a guest in a town"
		case BrokenSpeech(voice.Race):
			kind = "a troll or an ogre at home"
		}
		name := town
		if name == "" {
			name = "(no town: guards on the roads)"
		}
		all.shares, all.kinds = append(all.shares, share), append(all.kinds, len(said))
		add(byRole, role, share, len(said))
		add(byTown, name, share, len(said))
		add(byKind, kind, share, len(said))
		if len(said) == 0 {
			t.Errorf("%s (%s, %s, race %d) has no lore to say", npc.Name, town, role, voice.Race)
		}
		if share > 0.6 {
			t.Errorf("%s (%s, %s, race %d): lore is %.0f%% of what it says, more than 60%%", npc.Name, town, role, voice.Race, 100*share)
		}
	}
	if len(all.shares) == 0 {
		t.Fatal("no NPC speaks")
	}
	report := func(title string, m map[string]*tally) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			lo, hi := lowHigh(m[k].shares)
			kinds := append([]int(nil), m[k].kinds...)
			sort.Ints(kinds)
			t.Logf("%s %-32s %4d speakers: a lore line %2.0f%% of talks (%2.0f%% to %2.0f%%), %3d lore lines each (%d to %d)",
				title, k, len(m[k].shares), 100*median(m[k].shares), 100*lo, 100*hi, kinds[len(kinds)/2], kinds[0], kinds[len(kinds)-1])
		}
	}
	report("by kind", byKind)
	report("by role", byRole)
	report("by town", byTown)
	lo, hi := lowHigh(all.shares)
	med := median(all.shares)
	t.Logf("%d lore lines of %d; %d speakers: a lore line in %.0f%% of talks with a neutral listener at the median speaker (%.0f%% to %.0f%%)",
		len(lore), len(lines), len(all.shares), 100*med, 100*lo, 100*hi)
	if med < 0.25 || med > 0.5 {
		t.Errorf("the median speaker speaks of lore in %.0f%% of talks: outside 25%% to 50%%", 100*med)
	}
}
