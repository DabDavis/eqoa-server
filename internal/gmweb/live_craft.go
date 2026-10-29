package gmweb

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/handlers"
)

// The tradeskills' tables in the panel (handlers/craft.go, database/craft.go):
// a recipe row is refused when the game could not make or show it, a
// character's skill outside the scale, and a skill changed is sent to its
// character in the world.

// craftRowInt is a whole number of the row's, def when the row has none
// (an insert's column left to its default); required columns have none.
func craftRowInt(row map[string]any, col string, def int, required bool) (int, error) {
	v, ok := row[col]
	if !ok || v == nil {
		if required {
			return 0, badRequest("%s is required", col)
		}
		return def, nil
	}
	f, fok := asFloat(v)
	if !fok || f != math.Trunc(f) || math.Abs(f) > 1<<31 {
		return 0, badRequest("%s is a whole number", col)
	}
	return int(f), nil
}

// craftRowText is a text of the row's, def when the row has none.
func craftRowText(row map[string]any, col, def string, required bool) (string, error) {
	v, ok := row[col]
	if !ok || v == nil {
		if required {
			return "", badRequest("%s is required", col)
		}
		return def, nil
	}
	return fmt.Sprint(v), nil
}

// craftRecipeOfRow is a craftrecipes row as the game reads one.
func craftRecipeOfRow(row map[string]any) (database.CraftRecipe, error) {
	var r database.CraftRecipe
	var err error
	ints := []struct {
		col      string
		to       *int
		def      int
		required bool
	}{
		{"skill", &r.Skill, 0, true}, {"comp", &r.Comp, 0, true}, {"level", &r.Level, 0, true},
		{"rarity", &r.Rarity, 0, true}, {"product", &r.Product, 0, true}, {"yield", &r.Yield, 1, false},
		{"difficulty", &r.Difficulty, 0, true}, {"xp", &r.XP, 0, true}, {"scroll", &r.Scroll, 0, false},
	}
	for _, f := range ints {
		if *f.to, err = craftRowInt(row, f.col, f.def, f.required); err != nil {
			return r, err
		}
	}
	discover, err := craftRowInt(row, "discover", 1, false)
	if err != nil {
		return r, err
	}
	r.Discover = discover != 0
	if r.Name, err = craftRowText(row, "name", "", true); err != nil {
		return r, err
	}
	if r.Description, err = craftRowText(row, "description", "", false); err != nil {
		return r, err
	}
	ingredients, err := craftRowText(row, "ingredients", "", true)
	if err != nil {
		return r, err
	}
	if r.Ingredients, err = database.ParseCraftIngredients(ingredients); err != nil {
		return r, badRequest("%v", err)
	}
	return r, nil
}

// checkCraftRecipeRow refuses a recipe the game could not make, or whose
// book the client could not show (database.CraftRecipeProblems).
func checkCraftRecipeRow(row map[string]any) error {
	r, err := craftRecipeOfRow(row)
	if err != nil {
		return err
	}
	if p := database.CraftRecipeProblems(r); len(p) > 0 {
		return badRequest("not a recipe the game can use: %s", strings.Join(p, "; "))
	}
	return nil
}

// checkCraftSkillRow refuses a character's skill outside the scale: skill
// 0-5, level 1-10, rank 0-9, XP 0 or more.
func checkCraftSkillRow(row map[string]any) error {
	skill, err := craftRowInt(row, "skill", 0, true)
	if err != nil {
		return err
	}
	level, err := craftRowInt(row, "level", 1, false)
	if err != nil {
		return err
	}
	rank, err := craftRowInt(row, "rank", 0, false)
	if err != nil {
		return err
	}
	xp, err := craftRowInt(row, "xp", 0, false)
	if err != nil {
		return err
	}
	switch {
	case skill < 0 || skill >= database.CraftSkills:
		return badRequest("skill %d is not 0-5", skill)
	case level < database.CraftLevelMin || level > database.CraftLevelMax:
		return badRequest("level %d is not 1-10", level)
	case rank < 0 || rank > 9:
		return badRequest("rank %d is not 0-9", rank)
	case xp < 0:
		return badRequest("xp is 0 or more")
	}
	return nil
}

// liveCraftSkills sends a character in the world its six skills again (the
// change is committed: the database is free).
func (s *Server) liveCraftSkills(before, after map[string]any) string {
	row := after
	if row == nil {
		row = before
	}
	id, _ := asFloat(row["charid"])
	p, online := s.onlineCharacter(uint32(id))
	if !online {
		return "at the character's next login"
	}
	sess := s.sessionFor(uint32(id))
	if sess == nil || s.d.DB == nil {
		return fmt.Sprintf("%s is in the world: at its next login", p.Name)
	}
	if err := handlers.SendCraftSlots(sess, s.d.DB, uint32(id)); err != nil {
		return "not sent to the character: " + err.Error()
	}
	return fmt.Sprintf("at once: %s was sent its tradeskills", p.Name)
}
