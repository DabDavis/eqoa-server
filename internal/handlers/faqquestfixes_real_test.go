package handlers

import (
	"os"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// tools/faq_quest_fixes_20261114.py on a real server's database (EQOA_REAL_DB,
// one it has been applied to): the drake lord's gem crafting is three
// Experiments any jewelcrafting kit makes (4 rubilent gems a refined, 4
// refined a polished, 2 polished the pristine gem 101262 asks to be shown),
// each found by its combine alone, discoverable and kept on a failure; the
// drake gems hold ten to a stack and are no longer Lore (a loot step of four,
// five, six or 32 could hold one); and the steps the tool put in read back
// as the engine reads them (Shrixiv's floor with its height, Gedwin's four
// rings). The chains themselves are walked by TestRealQuestChainsWalk with
// EQOA_WALK_QUESTS.
func TestRealFAQQuestFixes(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var n int
	w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'guide_set_log'`).Scan(&n)
	if n > 0 {
		w.db.QueryRow(`SELECT count(*) FROM guide_set_log WHERE setname = 'faq-quest-fixes-1114'`).Scan(&n)
	}
	if n == 0 {
		t.Skip("tools/faq_quest_fixes_20261114.py has not been applied to this database")
	}
	item := func(name string) int {
		var id int
		if err := w.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ?`, name).Scan(&id); err != nil {
			t.Fatalf("item %q: %v", name, err)
		}
		return id
	}
	recipes, err := w.db.CraftRecipes()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		from    string
		count   int
		product string
	}{{"Rubilent Gem", 4, "Refined Rubilent Gem"}, {"Refined Rubilent Gem", 4, "Polished Rubilent Gem"},
		{"Polished Rubilent Gem", 2, "Pristine Rubilent Gem"}} {
		got := recipes.Combine(map[int]int{item(c.from): c.count})
		if len(got) != 1 {
			t.Errorf("%d %s make %d recipes, want 1", c.count, c.from, len(got))
			continue
		}
		r := got[0]
		if r.Product != item(c.product) || r.Skill != 3 || r.Level != 1 || !r.Discover || !r.Ingredients[0].Keep {
			t.Errorf("%d %s: %+v, want %s, jewelry level 1, discoverable, kept on a failure", c.count, c.from, *r, c.product)
		}
		if p := database.CraftRecipeProblems(*r); len(p) > 0 {
			t.Errorf("recipe %q: %v", r.Name, p)
		}
	}
	for _, g := range []string{"Emeraldine Gem", "Sandstone Gem", "Pearlescent Gem", "Rubilent Gem", "Refined Rubilent Gem", "Polished Rubilent Gem"} {
		var lore, stack int
		w.db.QueryRow(`SELECT lore, maxstack FROM itempattern WHERE patternid = ?`, item(g)).Scan(&lore, &stack)
		if lore != 0 || stack < 10 {
			t.Errorf("%s: lore %d, stack %d; want no Lore, ten to a stack", g, lore, stack)
		}
	}
	shrixiv := w.quest(101026)
	if st := shrixiv.Step(11); st == nil || st.Kind != database.StepReach || st.Height != 5 || st.Y != 84 || st.World != 2 {
		t.Errorf("101026 step 11: %+v, want the fortress's fourth floor, height 5", st)
	}
	if st := shrixiv.Step(12); st == nil || len(database.EventsOn(st.Events, database.EventStart)) != 1 {
		t.Errorf("101026 step 12: %+v, want Shrixiv spawned at its start", st)
	}
	if st := w.quest(101263).Step(11); st == nil || st.Target != "Gedwin" || len(st.Choices.Options) != 4 {
		t.Errorf("101263 step 11: %+v, want Gedwin's four rings", st)
	}
	if st := w.quest(101308).Step(3); st == nil || st.Target != "Mark of Desolation" ||
		len(database.EventsOn(st.Events, database.EventDone)) != 1 {
		t.Errorf("101308 step 3: %+v, want the stones to the Mark of Desolation, who sends the player", st)
	}
}
