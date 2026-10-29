package database

import (
	"strings"
	"testing"
)

// The tradeskills' tables (craft.go) on a fresh database: every recipe and
// item is made up.

func craftTestDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/craft.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	InvalidateCraftRecipes()
	return db
}

func addRecipe(t *testing.T, db *SQLiteDB, id, skill, comp, level int, name, ingredients string, discover, scroll int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO craftrecipes (id, skill, comp, level, rarity, name, product, yield, ingredients,
		difficulty, xp, discover, scroll) VALUES (?, ?, ?, ?, 0, ?, 900, 1, ?, 10, 40, ?, ?)`,
		id, skill, comp, level, name, ingredients, discover, scroll); err != nil {
		t.Fatal(err)
	}
}

// The loader keeps what the game can make, leaves out what it cannot (bad
// JSON, a skill past 5, a level 0, no ingredients, a count of 0), keeps a
// recipe only the book cannot show (a component past its skill's list), and
// indexes them by id, scroll and the ingredients' multiset.
func TestCraftRecipesLoad(t *testing.T) {
	db := craftTestDB(t)
	addRecipe(t, db, 1, 1, 0, 1, "Test Plate", `[{"item": 11, "count": 2, "keep": false}, {"item": 12, "count": 1, "keep": true}]`, 1, 0)
	addRecipe(t, db, 2, 1, 0, 1, "Test Plate Again", `[{"item": 12, "count": 1, "keep": true}, {"item": 11, "count": 1}, {"item": 11, "count": 1}]`, 1, 50)
	addRecipe(t, db, 3, 1, 40, 1, "Test Off The List", `[{"item": 13, "count": 1, "keep": true}]`, 1, 50)
	addRecipe(t, db, 4, 7, 0, 1, "Test No Skill", `[{"item": 11, "count": 1}]`, 1, 0)
	addRecipe(t, db, 5, 1, 0, 0, "Test No Level", `[{"item": 11, "count": 1}]`, 1, 0)
	addRecipe(t, db, 6, 1, 0, 1, "Test Bad JSON", `[{"item": 11, "count": 1`, 1, 0)
	addRecipe(t, db, 7, 1, 0, 1, "Test Nothing In", `[]`, 1, 0)
	addRecipe(t, db, 8, 1, 0, 1, "Test Zero Count", `[{"item": 11, "count": 0}]`, 1, 0)
	c, err := db.CraftRecipes()
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, r := range c.All() {
		ids = append(ids, r.ID)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("loaded %v, want 1, 2, 3", ids)
	}
	if r, ok := c.Recipe(1); !ok || len(r.Ingredients) != 2 || r.Ingredients[0].Keep || !r.Ingredients[1].Keep || !r.Discover {
		t.Fatalf("recipe 1: %+v", r)
	}
	// the same multiset however it is written: 1 and 2 both, in id order
	same := c.Combine(map[int]int{12: 1, 11: 2})
	if len(same) != 2 || same[0].ID != 1 || same[1].ID != 2 {
		t.Fatalf("the combine of two 11 and a 12: %v", same)
	}
	if got := c.Combine(map[int]int{11: 1, 12: 1}); len(got) != 0 {
		t.Fatalf("a different multiset matched: %v", got)
	}
	if s := c.Scroll(50); len(s) != 2 || s[0].ID != 2 || s[1].ID != 3 {
		t.Fatalf("scroll 50 teaches %v", s)
	}
	if !c.ConsumedOnFail(11) || c.ConsumedOnFail(12) || c.ConsumedOnFail(13) {
		t.Fatal("consumed on a failure: 11 is in recipes 1 and 2, the kept 12 and 13 in none")
	}
	// cached until told otherwise
	addRecipe(t, db, 9, 0, 0, 1, "Test Later", `[{"item": 14, "count": 1}]`, 1, 0)
	if c2, _ := db.CraftRecipes(); len(c2.All()) != 3 {
		t.Fatal("a cached read saw a new row")
	}
	InvalidateCraftRecipes()
	if c3, _ := db.CraftRecipes(); len(c3.All()) != 4 {
		t.Fatal("an invalidated read missed the new row")
	}
}

// A recipe's problems: what keeps it from being made and what spoils its
// book; none for a good one.
func TestCraftRecipeProblems(t *testing.T) {
	good := CraftRecipe{ID: 1, Skill: 2, Comp: 18, Level: 10, Rarity: 3, Name: "Test Robe", Product: 5, Yield: 1,
		Ingredients: []CraftIngredient{{Item: 7, Count: 1}}}
	if p := CraftRecipeProblems(good); len(p) != 0 {
		t.Fatalf("a good recipe: %v", p)
	}
	for name, bad := range map[string]func(r *CraftRecipe){
		"comp past Tailoring's 19":  func(r *CraftRecipe) { r.Comp = 19 },
		"a rarity of 4":             func(r *CraftRecipe) { r.Rarity = 4 },
		"a 64-character name":       func(r *CraftRecipe) { r.Name = strings.Repeat("x", 64) },
		"no product":                func(r *CraftRecipe) { r.Product = 0 },
		"a yield of 0":              func(r *CraftRecipe) { r.Yield = 0 },
		"level 11":                  func(r *CraftRecipe) { r.Level = 11 },
		"skill 6":                   func(r *CraftRecipe) { r.Skill = 6 },
		"an ingredient of no item":  func(r *CraftRecipe) { r.Ingredients = []CraftIngredient{{Item: 0, Count: 1}} },
		"a negative difficulty":     func(r *CraftRecipe) { r.Difficulty = -1 },
		"a 256-character desc":      func(r *CraftRecipe) { r.Description = strings.Repeat("y", 256) },
		"an empty ingredients list": func(r *CraftRecipe) { r.Ingredients = nil },
		"a scroll that is no item":  func(r *CraftRecipe) { r.Scroll = -2 },
	} {
		r := good
		bad(&r)
		if p := CraftRecipeProblems(r); len(p) == 0 {
			t.Errorf("%s: no problem said", name)
		}
	}
	if k := CombineKey(map[int]int{40: 1, 12: 2, 9: 0}); k != "12*2,40*1" {
		t.Fatalf("CombineKey %q", k)
	}
}

// A book: recipes added at its end in the order given, one held already left
// where it is; removed; reordered in one transaction; a character's skill
// written and read; all of it gone with the character.
func TestCraftBookAndSkills(t *testing.T) {
	db := craftTestDB(t)
	chars := NewSQLiteCharacterStore(db)
	rec := &CharacterRecord{CharName: "Smith", AccountID: 1}
	if err := chars.Create(rec); err != nil {
		t.Fatal(err)
	}
	id := rec.ServerID
	added, err := db.AddCharRecipes(id, 30, 10, 20)
	if err != nil || len(added) != 3 {
		t.Fatalf("added %v (%v)", added, err)
	}
	added, _ = db.AddCharRecipes(id, 10, 40)
	if len(added) != 1 || added[0] != 40 {
		t.Fatalf("added again %v, want 40 alone", added)
	}
	book, _ := db.CharRecipeBook(id)
	want := []CharRecipe{{30, 0}, {10, 1}, {20, 2}, {40, 3}}
	if len(book) != len(want) {
		t.Fatalf("the book %v", book)
	}
	for i := range want {
		if book[i] != want[i] {
			t.Fatalf("the book %v, want %v", book, want)
		}
	}
	if ok, _ := db.RemoveCharRecipe(id, 10); !ok {
		t.Fatal("recipe 10 was not removed")
	}
	if ok, _ := db.RemoveCharRecipe(id, 10); ok {
		t.Fatal("recipe 10 removed twice")
	}
	if err := db.SetCharRecipeOrders(id, map[int]int{40: 0, 30: 1}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCharRecipeOrders(id, map[int]int{20: 5, 99: 6}); err == nil {
		t.Fatal("an order for a recipe not in the book was written")
	}
	if book, _ = db.CharRecipeBook(id); book[0].Recipe != 40 || book[1].Recipe != 30 || book[2] != (CharRecipe{20, 2}) {
		t.Fatalf("reordered %v (the failed change's 20 kept at 2)", book)
	}

	if err := db.SetCharCraftSkill(id, 3, CraftSkill{Level: 4, Rank: 5, XP: 6}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCharCraftSkill(id, 3, CraftSkill{Level: 5, Rank: 0, XP: 1}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetCharCraftSkill(id, 6, CraftSkill{Level: 1}); err == nil {
		t.Fatal("a skill 6 was written")
	}
	skills, _ := db.CharCraftSkills(id)
	if skills[3] != (CraftSkill{Level: 5, Rank: 0, XP: 1, Row: true}) || skills[0].Row {
		t.Fatalf("skills %v", skills)
	}

	if err := chars.Delete(id); err != nil {
		t.Fatal(err)
	}
	book, _ = db.CharRecipeBook(id)
	skills, _ = db.CharCraftSkills(id)
	if len(book) != 0 || skills[3].Row {
		t.Fatalf("a deleted character's book %v and skills %v stayed", book, skills)
	}
}
