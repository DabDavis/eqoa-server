package entity

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The instanced dungeons' casters cast (petition 1900, Brother: "doing The Guk
// instance none of the NPC like wiz, knights, sha..etc... do not do any
// spells"), on a real server's database (EQOA_REAL_DB; a copy is read) with
// tools/petfix_dungeon_casters_20261113.py applied: every creature of world 14
// of a casting class (a shadow knight, a druid, a shaman, a cleric, a
// magician, a necromancer, an enchanter, a wizard: its template's class, else
// its npcclass) has an ability that finds a spell of its class at its level,
// Guk's wizards, shamans and knights among them; a warrior has none. Skipped
// without a database, or one with no dungeons.
func TestRealDungeonCastersCast(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "casters.db")
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
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	casters := map[int]bool{3: true, 7: true, 8: true, 9: true, 10: true, 11: true, 12: true, 13: true}
	type tally struct{ casting, silent int }
	byClass := map[int]*tally{}
	silent := map[string]bool{}
	named := map[string]bool{}
	w.mu.RLock()
	defer w.mu.RUnlock()
	dungeon := 0
	for _, npc := range w.NPCs {
		if npc.World != 14 || npc.tmpl == nil || !strings.HasPrefix(npc.tmpl.Name, "dungeon ") {
			continue
		}
		dungeon++
		casts := false
		for i := range npc.tmpl.abilities {
			a := &npc.tmpl.abilities[i]
			if a.Kind == database.AbilityHeal || a.Kind == database.AbilityNuke || a.Kind == database.AbilityLifetap {
				casts = casts || a.spellFor(npc.Class, npc.Level) != nil
			}
		}
		if byClass[npc.Class] == nil {
			byClass[npc.Class] = &tally{}
		}
		if casts {
			byClass[npc.Class].casting++
			named[strings.ToLower(npc.Name)] = true
		} else {
			byClass[npc.Class].silent++
			if casters[npc.Class] {
				silent[npc.Name] = true
			}
		}
	}
	if dungeon == 0 {
		t.Skip("no creature of world 14 fights by a dungeon tier here")
	}
	classes := make([]int, 0, len(byClass))
	for c := range byClass {
		classes = append(classes, c)
	}
	sort.Ints(classes)
	for _, c := range classes {
		t.Logf("class %2d: %4d cast, %4d do not", c, byClass[c].casting, byClass[c].silent)
	}
	if len(silent) > 0 {
		var names []string
		for n := range silent {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("%d names of a casting class cast nothing: %q", len(names), names)
	}
	for _, n := range []string{"froglok tal wizard", "froglok tal shaman", "froglok tal necromancer", "froglok vis knight", "ghoul tal knight"} {
		if !named[n] {
			t.Errorf("%s casts nothing", n)
		}
	}
	if b := byClass[0]; b != nil && b.casting > 0 {
		t.Errorf("%d warriors cast", b.casting)
	}
}
