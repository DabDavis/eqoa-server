package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// The tradeskills' tables (handlers/craft.go; the client's half is
// claude-notes/tradeskill-protocol-20261029.md):
//
//   - craftrecipes: every recipe. id is the number the client echoes; skill
//     0 Weapon Crafting, 1 Armor Crafting, 2 Tailoring, 3 Jewel Crafting, 4
//     Carpentry, 5 Alchemy; comp the index in the skill's component list
//     (the Recipe Book's material tab, CraftComponents); level the trade
//     level 1-10 (the book's Lvl column, the level the skill must reach);
//     rarity 0 common, 1 uncommon, 2 rare, 3 ultra rare; name the book's
//     (63 characters at most); product and yield what a success makes;
//     ingredients a JSON list [{"item": patternid, "count": n, "keep":
//     false}] (keep true: not lost on a failure, a gem or an alloy; false:
//     consumed on a failure, the metal or the mold); difficulty the skill
//     points the roll opposes; xp what a success gives at the recipe's own
//     level; discover 1 if an Experiment can find it; scroll the
//     itempattern of a scroll that teaches it (0 none).
//   - charcraftskills: a character's skill, level 1-10 and rank 0-9, and the
//     XP into the rank. No row: skills 0-3 at level 1 rank 0, skills 4 and 5
//     never live (handlers/craftrules.go).
//   - charrecipes: a character's recipe book, ord the client's order.
//
// The recipes are the owner's to fill (a content tool writes them, a GM
// edits them in the panel); none are made here.
const craftSchema = `
CREATE TABLE IF NOT EXISTS craftrecipes (
	id          INTEGER PRIMARY KEY,
	skill       INTEGER NOT NULL,
	comp        INTEGER NOT NULL,
	level       INTEGER NOT NULL,
	rarity      INTEGER NOT NULL,
	name        TEXT NOT NULL,
	product     INTEGER NOT NULL,
	yield       INTEGER NOT NULL DEFAULT 1,
	ingredients TEXT NOT NULL,
	difficulty  INTEGER NOT NULL,
	xp          INTEGER NOT NULL,
	discover    INTEGER NOT NULL DEFAULT 1,
	scroll      INTEGER NOT NULL DEFAULT 0,
	description TEXT NOT NULL DEFAULT '',
	source      TEXT NOT NULL DEFAULT 'gm'
);
CREATE INDEX IF NOT EXISTS idx_craftrecipes_scroll ON craftrecipes(scroll);
CREATE TABLE IF NOT EXISTS charcraftskills (
	charid INTEGER NOT NULL,
	skill  INTEGER NOT NULL,
	level  INTEGER NOT NULL DEFAULT 1,
	rank   INTEGER NOT NULL DEFAULT 0,
	xp     INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (charid, skill)
);
CREATE TABLE IF NOT EXISTS charrecipes (
	charid INTEGER NOT NULL,
	recipe INTEGER NOT NULL,
	ord    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (charid, recipe)
);`

// ensureCraft makes the tradeskills' tables (initSchema).
func (db *SQLiteDB) ensureCraft() error {
	if _, err := db.Exec(craftSchema); err != nil {
		return fmt.Errorf("craft: %w", err)
	}
	return nil
}

// CraftSkills is how many tradeskills a character has: CLIENT's six slots
// (VIClientPart3834C +0x10).
const CraftSkills = 6

// CraftComponents is how many components each skill's list has
// (VIWndRecipe::ComponentName, the tables at 0x00CEF118): a recipe's comp
// indexes it, and one outside it is never shown in the Recipe Book.
var CraftComponents = [CraftSkills]int{11, 11, 19, 11, 11, 13}

// The bounds the client and the rules put on a recipe.
const (
	CraftLevelMin, CraftLevelMax = 1, 10
	CraftRarityMax               = 3
	craftNameUnits               = 63  // wstr(64): the book's name, VIRec144_C017C0 +0x10
	craftDescUnits               = 255 // wstr(256): VICraftDetail's description
	craftYieldMax                = 65535
)

// CraftIngredient is one entry of a recipe's ingredients: an item, how many,
// and whether it is kept when the attempt fails (a gem, an alloy) or lost
// (the metal, the mold).
type CraftIngredient struct {
	Item  int  `json:"item"`
	Count int  `json:"count"`
	Keep  bool `json:"keep"`
}

// CraftRecipe is one row of craftrecipes.
type CraftRecipe struct {
	ID          int
	Skill       int
	Comp        int
	Level       int
	Rarity      int
	Name        string
	Product     int
	Yield       int
	Ingredients []CraftIngredient
	Difficulty  int
	XP          int
	Discover    bool
	Scroll      int
	Description string
	Source      string
}

// Units is the recipe's ingredients as a multiset: each item's count, its
// entries added together.
func (r *CraftRecipe) Units() map[int]int {
	u := map[int]int{}
	for _, in := range r.Ingredients {
		u[in.Item] += in.Count
	}
	return u
}

// CombineKey is a multiset of items as one string, the items in ascending
// order with their counts ("12*2,40*1"): two combinations are the same
// exactly when their keys are.
func CombineKey(units map[int]int) string {
	ids := make([]int, 0, len(units))
	for id, n := range units {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(id))
		b.WriteByte('*')
		b.WriteString(strconv.Itoa(units[id]))
	}
	return b.String()
}

// ParseCraftIngredients reads a recipe's ingredients column.
func ParseCraftIngredients(s string) ([]CraftIngredient, error) {
	var out []CraftIngredient
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, fmt.Errorf("ingredients is not a JSON list of {\"item\", \"count\", \"keep\"}: %v", err)
	}
	return out, nil
}

// units is a string's length in UTF-16 units, the client's measure.
func units(s string) int { return len(utf16.Encode([]rune(s))) }

// CraftRecipeProblems is what is wrong with a recipe for the game to make or
// show it, none when nothing is: what keeps it from being made (the loader
// leaves such a recipe out), then what only spoils how the Recipe Book shows
// it (a component outside its skill's list, a rarity past 3, a name or
// description too long for the client: the loader keeps it and says so). The
// GM panel refuses either.
func CraftRecipeProblems(r CraftRecipe) []string {
	return append(craftMakeProblems(r), craftShownProblems(r)...)
}

// craftMakeProblems are the problems that keep a recipe from being made.
func craftMakeProblems(r CraftRecipe) []string {
	var p []string
	if r.Skill < 0 || r.Skill >= CraftSkills {
		p = append(p, fmt.Sprintf("skill %d is not 0-5 (0 Weapon, 1 Armor, 2 Tailoring, 3 Jewel, 4 Carpentry, 5 Alchemy)", r.Skill))
	}
	if r.Level < CraftLevelMin || r.Level > CraftLevelMax {
		p = append(p, fmt.Sprintf("level %d is not a trade level, 1-10", r.Level))
	}
	if r.Product <= 0 {
		p = append(p, "product is no item")
	}
	if r.Yield < 1 || r.Yield > craftYieldMax {
		p = append(p, fmt.Sprintf("yield %d is not 1-%d", r.Yield, craftYieldMax))
	}
	if len(r.Ingredients) == 0 {
		p = append(p, "it has no ingredients")
	}
	for i, in := range r.Ingredients {
		if in.Item <= 0 || in.Count < 1 {
			p = append(p, fmt.Sprintf("ingredient %d (item %d, count %d) needs an item and a count of 1 or more", i+1, in.Item, in.Count))
		}
	}
	if r.Difficulty < 0 || r.XP < 0 {
		p = append(p, "difficulty and xp are 0 or more")
	}
	if r.Scroll < 0 {
		p = append(p, "scroll is an item, or 0")
	}
	return p
}

// craftShownProblems are the problems in how the book shows a recipe.
func craftShownProblems(r CraftRecipe) []string {
	var p []string
	if r.Skill >= 0 && r.Skill < CraftSkills && (r.Comp < 0 || r.Comp >= CraftComponents[r.Skill]) {
		p = append(p, fmt.Sprintf("comp %d is outside skill %d's component list (0-%d): the Recipe Book never shows it",
			r.Comp, r.Skill, CraftComponents[r.Skill]-1))
	}
	if r.Rarity < 0 || r.Rarity > CraftRarityMax {
		p = append(p, fmt.Sprintf("rarity %d is not 0-3 (common, uncommon, rare, ultra rare)", r.Rarity))
	}
	if strings.TrimSpace(r.Name) == "" || units(r.Name) > craftNameUnits {
		p = append(p, fmt.Sprintf("name %q is not 1-%d characters", r.Name, craftNameUnits))
	}
	if units(r.Description) > craftDescUnits {
		p = append(p, fmt.Sprintf("description is longer than %d characters", craftDescUnits))
	}
	return p
}

// CraftRecipes is every recipe the game can make, read at once, with what
// the handlers look one up by.
type CraftRecipes struct {
	list     []*CraftRecipe // by id
	byID     map[int]*CraftRecipe
	byScroll map[int][]*CraftRecipe
	byUnits  map[string][]*CraftRecipe
	consumed map[int]bool // the items some recipe loses on a failure
}

// newCraftRecipes indexes recipes (in id order).
func newCraftRecipes(recipes []CraftRecipe) *CraftRecipes {
	c := &CraftRecipes{byID: map[int]*CraftRecipe{}, byScroll: map[int][]*CraftRecipe{},
		byUnits: map[string][]*CraftRecipe{}, consumed: map[int]bool{}}
	for i := range recipes {
		r := &recipes[i]
		c.list = append(c.list, r)
		c.byID[r.ID] = r
		if r.Scroll > 0 {
			c.byScroll[r.Scroll] = append(c.byScroll[r.Scroll], r)
		}
		key := CombineKey(r.Units())
		c.byUnits[key] = append(c.byUnits[key], r)
		for _, in := range r.Ingredients {
			if !in.Keep {
				c.consumed[in.Item] = true
			}
		}
	}
	return c
}

// NewCraftRecipes is a set of recipes as the loader makes one (tests).
func NewCraftRecipes(recipes []CraftRecipe) *CraftRecipes {
	sorted := append([]CraftRecipe(nil), recipes...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].ID < sorted[b].ID })
	return newCraftRecipes(sorted)
}

// Recipe is the recipe of that id.
func (c *CraftRecipes) Recipe(id int) (*CraftRecipe, bool) {
	if c == nil {
		return nil, false
	}
	r, ok := c.byID[id]
	return r, ok
}

// All is every recipe, in id order.
func (c *CraftRecipes) All() []*CraftRecipe {
	if c == nil {
		return nil
	}
	return c.list
}

// Scroll is the recipes a scroll of that item teaches, in id order.
func (c *CraftRecipes) Scroll(item int) []*CraftRecipe {
	if c == nil || item <= 0 {
		return nil
	}
	return c.byScroll[item]
}

// Combine is the recipes whose ingredients are exactly that multiset, in id
// order.
func (c *CraftRecipes) Combine(units map[int]int) []*CraftRecipe {
	if c == nil {
		return nil
	}
	return c.byUnits[CombineKey(units)]
}

// ConsumedOnFail is whether some recipe loses the item on a failure: such an
// item is lost in an Experiment that makes nothing too (the strategy guide's
// "Consume on Fail": "lost on an unsuccessful attempt at combining, whether
// the item combination is a valid recipe or not").
func (c *CraftRecipes) ConsumedOnFail(item int) bool {
	return c != nil && c.consumed[item]
}

// craftRecipesFor is how long a read of craftrecipes is kept: a row a tool
// writes is used within it, a GM's edit at once (InvalidateCraftRecipes).
const craftRecipesFor = 30 * time.Second

var craftRecipesCache struct {
	sync.Mutex
	recipes *CraftRecipes
	at      time.Time
	db      *SQLiteDB
	logged  map[string]bool // problems said once a run
}

// InvalidateCraftRecipes makes the next CraftRecipes read the table again.
func InvalidateCraftRecipes() {
	craftRecipesCache.Lock()
	craftRecipesCache.at = time.Time{}
	craftRecipesCache.Unlock()
}

// CraftRecipes is every recipe of craftrecipes the game can make, read at
// most every craftRecipesFor; a row it cannot make is left out and said in
// the log once. Never call it holding the World's lock or another query's
// rows: it may query (the database has one connection).
func (db *SQLiteDB) CraftRecipes() (*CraftRecipes, error) {
	craftRecipesCache.Lock()
	defer craftRecipesCache.Unlock()
	if craftRecipesCache.db == db && time.Since(craftRecipesCache.at) < craftRecipesFor {
		return craftRecipesCache.recipes, nil
	}
	rows, err := db.Query(`SELECT id, skill, comp, level, rarity, name, product, yield, ingredients,
		difficulty, xp, discover, scroll, description, source FROM craftrecipes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CraftRecipe
	var problems []string
	for rows.Next() {
		var r CraftRecipe
		var ingredients string
		var discover int
		if err := rows.Scan(&r.ID, &r.Skill, &r.Comp, &r.Level, &r.Rarity, &r.Name, &r.Product, &r.Yield,
			&ingredients, &r.Difficulty, &r.XP, &discover, &r.Scroll, &r.Description, &r.Source); err != nil {
			return nil, err
		}
		r.Discover = discover != 0
		if r.Ingredients, err = ParseCraftIngredients(ingredients); err != nil {
			problems = append(problems, fmt.Sprintf("recipe %d %q left out: %v", r.ID, r.Name, err))
			continue
		}
		if p := craftMakeProblems(r); len(p) > 0 {
			problems = append(problems, fmt.Sprintf("recipe %d %q left out: %s", r.ID, r.Name, strings.Join(p, "; ")))
			continue
		}
		if p := craftShownProblems(r); len(p) > 0 {
			problems = append(problems, fmt.Sprintf("recipe %d %q: %s", r.ID, r.Name, strings.Join(p, "; ")))
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	c := newCraftRecipes(list)
	for key, same := range c.byUnits {
		if len(same) > 1 {
			problems = append(problems, fmt.Sprintf("recipes %s have the same ingredients (%s): an Experiment makes the first it may",
				craftIDs(same), key))
		}
	}
	if craftRecipesCache.logged == nil || craftRecipesCache.db != db {
		craftRecipesCache.logged = map[string]bool{}
	}
	sort.Strings(problems)
	for _, p := range problems {
		if !craftRecipesCache.logged[p] {
			craftRecipesCache.logged[p] = true
			log.Printf("Craft: %s", p)
		}
	}
	craftRecipesCache.recipes, craftRecipesCache.at, craftRecipesCache.db = c, time.Now(), db
	return c, nil
}

func craftIDs(rs []*CraftRecipe) string {
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = strconv.Itoa(r.ID)
	}
	return strings.Join(ids, ", ")
}

// CraftSkill is a character's charcraftskills row for one skill; Row is
// false where it has none (the handlers' defaults apply).
type CraftSkill struct {
	Level, Rank, XP int
	Row             bool
}

// CharCraftSkills is a character's six skills as its rows hold them.
func (db *SQLiteDB) CharCraftSkills(charID uint32) ([CraftSkills]CraftSkill, error) {
	var out [CraftSkills]CraftSkill
	rows, err := db.Query(`SELECT skill, level, rank, xp FROM charcraftskills WHERE charid = ?`, charID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var skill int
		var s CraftSkill
		if err := rows.Scan(&skill, &s.Level, &s.Rank, &s.XP); err != nil {
			return out, err
		}
		if skill >= 0 && skill < CraftSkills {
			s.Row = true
			out[skill] = s
		}
	}
	return out, rows.Err()
}

// SetCharCraftSkill writes a character's skill.
func (db *SQLiteDB) SetCharCraftSkill(charID uint32, skill int, s CraftSkill) error {
	if skill < 0 || skill >= CraftSkills {
		return fmt.Errorf("skill %d is not 0-5", skill)
	}
	_, err := db.Exec(`INSERT INTO charcraftskills (charid, skill, level, rank, xp) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (charid, skill) DO UPDATE SET level = excluded.level, rank = excluded.rank, xp = excluded.xp`,
		charID, skill, s.Level, s.Rank, s.XP)
	return err
}

// CharRecipe is a recipe in a character's book and its place in the book's
// order.
type CharRecipe struct {
	Recipe, Ord int
}

// CharRecipeBook is a character's book in its order (ties by recipe id).
func (db *SQLiteDB) CharRecipeBook(charID uint32) ([]CharRecipe, error) {
	rows, err := db.Query(`SELECT recipe, ord FROM charrecipes WHERE charid = ? ORDER BY ord, recipe`, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharRecipe
	for rows.Next() {
		var r CharRecipe
		if err := rows.Scan(&r.Recipe, &r.Ord); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddCharRecipes puts recipes at the end of a character's book, in the order
// given (each at the book's last order + 1), and returns those it did not
// hold before.
func (db *SQLiteDB) AddCharRecipes(charID uint32, recipes ...int) ([]int, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var last sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(ord) FROM charrecipes WHERE charid = ?`, charID).Scan(&last); err != nil {
		return nil, err
	}
	next := 0
	if last.Valid {
		next = int(last.Int64) + 1
	}
	var added []int
	for _, id := range recipes {
		res, err := tx.Exec(`INSERT OR IGNORE INTO charrecipes (charid, recipe, ord) VALUES (?, ?, ?)`, charID, id, next)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			added = append(added, id)
			next++
		}
	}
	return added, tx.Commit()
}

// RemoveCharRecipe takes a recipe out of a character's book, and says whether
// it was there.
func (db *SQLiteDB) RemoveCharRecipe(charID uint32, recipe int) (bool, error) {
	res, err := db.Exec(`DELETE FROM charrecipes WHERE charid = ? AND recipe = ?`, charID, recipe)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// SetCharRecipeOrders writes the orders given (recipe -> ord) of a character's
// book in one transaction.
func (db *SQLiteDB) SetCharRecipeOrders(charID uint32, ords map[int]int) error {
	if len(ords) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for recipe, ord := range ords {
		res, err := tx.Exec(`UPDATE charrecipes SET ord = ? WHERE charid = ? AND recipe = ?`, ord, charID, recipe)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return errors.New("the book changed meanwhile")
		}
	}
	return tx.Commit()
}

// deleteCharCraft takes a deleted character's skills and book with it
// (SQLiteCharacterStore.Delete).
func (db *SQLiteDB) deleteCharCraft(charID uint32) error {
	if _, err := db.Exec(`DELETE FROM charcraftskills WHERE charid = ?`, charID); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM charrecipes WHERE charid = ?`, charID)
	return err
}
