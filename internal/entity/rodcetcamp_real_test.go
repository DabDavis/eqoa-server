package entity

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/petpass_live_1123c.py's camp reads as the server reads it (EQOA_REAL_DB
// names a database the tool was applied to; a copy is read): the paladins of
// Rodcet Nife in plate with no robe, at 130% of their hit points and their own
// class; four clerics in the robe, clerics by their template, whose heal of
// their kin finds the paladins (their npcsocial faction) and whose nuke finds a
// cleric's spell at their level; no template problem; and quest 101340's loot
// step counting the clerics at 35%. Skipped without a database.
func TestRealRodcetCamp(t *testing.T) {
	if os.Getenv("EQOA_REAL_DB") == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	data, err := os.ReadFile(os.Getenv("EQOA_REAL_DB"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "rodcet.db")
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
	if _, err := w.LoadSocialRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	for _, p := range w.ClassTemplateProblems() {
		t.Logf("problem: %s", p) // the database's own, if any; ours are checked by name below
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, name := range []string{"rodcet nife paladin", "rodcet nife cleric"} {
		tm := w.templates.byName[name]
		if tm == nil {
			t.Fatalf("no template %q", name)
		}
		if len(tm.problems) != 0 {
			t.Errorf("template %q: %q", name, tm.problems)
		}
	}
	var pals, clerics []*NPC
	for _, npc := range w.NPCs {
		switch npc.Name {
		case "paladin of Rodcet Nife":
			pals = append(pals, npc)
		case "cleric of Rodcet Nife":
			clerics = append(clerics, npc)
		}
	}
	if len(pals) != 8 || len(clerics) != 4 {
		t.Fatalf("%d paladins and %d clerics, want 8 and 4", len(pals), len(clerics))
	}
	for _, p := range pals {
		if p.Torso != 4 || p.Forearm != 4 || p.NPCGloves != 4 || p.NPCLegs != 4 || p.Feet != 4 || p.Head != 4 || p.RobeStyle != "255" {
			t.Errorf("paladin %d: armour %d %d %d %d %d %d, robe %q", p.ServerID, p.Torso, p.Forearm, p.NPCGloves, p.NPCLegs, p.Feet, p.Head, p.RobeStyle)
		}
		if p.TemplateName() != "rodcet nife paladin" || p.Class != 0 {
			t.Errorf("paladin %d fights by %q as class %d", p.ServerID, p.TemplateName(), p.Class)
		}
		if want := int(math.Round(float64(p.tbase.maxHP) * 1.3)); p.MaxHP != want || p.HP != want {
			t.Errorf("paladin %d: %d/%d hit points, want %d (130%% of %d)", p.ServerID, p.HP, p.MaxHP, want, p.tbase.maxHP)
		}
	}
	tm := w.templates.byName["rodcet nife cleric"]
	if !tm.kinHeals {
		t.Error("the cleric template heals no kin")
	}
	for i := range tm.abilities {
		a := &tm.abilities[i]
		if a.Kind == database.AbilityNuke {
			if s := a.spellFor(9, 58); s == nil {
				t.Error("the clerics' nuke finds no cleric spell at 58")
			} else {
				t.Logf("the clerics' nuke at 58: %s", s.Name)
			}
		}
	}
	for _, c := range clerics {
		if c.TemplateName() != "rodcet nife cleric" || c.Class != 9 || c.Level != 58 || c.RobeStyle != "0" || c.Torso != 0 {
			t.Errorf("cleric %d: %q class %d level %d robe %q torso %d", c.ServerID, c.TemplateName(), c.Class, c.Level, c.RobeStyle, c.Torso)
		}
		f := w.socialFactionLocked(c)
		if f != "Followers of Rodcet Nife" || !w.kinOfLocked(c, pals[0], f) {
			t.Errorf("cleric %d: of %q, the paladins its kin: %v", c.ServerID, f, w.kinOfLocked(c, pals[0], f))
		}
		if r, _ := w.npcSocial.ruleFor(c.Name); r.Social {
			t.Errorf("cleric %d comes to help in a fight", c.ServerID)
		}
		near := math.Inf(1)
		for _, p := range pals {
			near = math.Min(near, float64(dist3(c.X, c.Y, c.Z, p.X, p.Y, p.Z)))
		}
		if near > float64(DefaultSocialRadius) {
			t.Errorf("cleric %d: the nearest paladin %.1f away, past its heal's %v", c.ServerID, near, DefaultSocialRadius)
		}
	}
	qs, err := database.NewQuestStore(db).LoadQuests()
	if err != nil {
		t.Fatal(err)
	}
	for i := range qs {
		if qs[i].ID != 101340 {
			continue
		}
		st := qs[i].Step(1)
		if st == nil || database.LootChance(st.Chance) != 0.35 || len(st.Targets()) != 2 || st.Targets()[1] != "cleric of Rodcet Nife" {
			t.Fatalf("quest 101340 step 1: %+v", st)
		}
		return
	}
	t.Fatal("no quest 101340")
}
