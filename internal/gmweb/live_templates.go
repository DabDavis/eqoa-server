package gmweb

import (
	"fmt"
	"math"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// The NPCs' class templates (entity/classtemplate.go, tables classtemplates
// and npctemplate): an edit of either is the world's at once, every NPC of a
// name given its template's numbers (or its own back) where it stands, and
// the next fight by it uses its abilities.

// classTemplateAbilitiesHelp is the abilities column's form, for the catalog.
const classTemplateAbilitiesHelp = `a JSON list, each {"kind": nuke | aoe | lifetap | heal | taunt | buff (the last two a pet's), ` +
	`"every": seconds from one cast's start to the next (2-3600), "first": seconds into a fight before the first, ` +
	`"chain": true to cast it as soon as the ability before it is cast (its clock then unused), ` +
	`"chance": % of its due casts (100), "damage_pct": % of its spell's figures (100), ` +
	`"max_recast" / "max_cast": the longest recast / cast time in seconds of a spell it picks (0 any), ` +
	`"line": words its spell's name must hold, "spell": a spell's name to cast whatever the level, ` +
	`"below": a heal's % of hit points under which it casts (50), "from": the NPC level from which it casts it (0 any); ` +
	`a pet's (Pet class templates): "target": owner | self (a heal's on itself, a buff's on its owner), "spells_of": the class ` +
	`0-14 whose spells it picks, "stat": the stat a buff raises (ac str sta agi dex wis int cha hp power), "element": a ` +
	`harmful spell's (fire cold lightning poison disease arcane), "heal_pct": a heal of that % of its most hit points (cast ` +
	`by size: the biggest due it is missing), "hits": a taunt's damage by level, [[level, least, most], ...] landing as it is}; ` +
	`e.g. [{"kind": "aoe", "every": 15, "first": 2}, {"kind": "nuke", "chain": true}]`

// liveTemplates reads the templates and the names given them again (the
// change is committed: the database is free) and says what the NPCs of the
// row now fight by.
func (s *Server) liveTemplates(table string, before, after map[string]any) string {
	if s.d.World == nil || s.d.DB == nil {
		return "no world is running: at the next start"
	}
	all, err := s.d.World.LoadClassTemplates(s.d.DB)
	if err != nil {
		return "not applied to the world: " + err.Error()
	}
	text := func(m map[string]any, col string) string {
		if m == nil || m[col] == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(m[col]))
	}
	row := after
	if row == nil {
		row = before
	}
	var msgs []string
	switch table {
	case "pettemplate":
		spell := database.TemplateKey(text(row, "spell"))
		if after == nil {
			msgs = append(msgs, fmt.Sprintf("from the next summoning: the pets of %q fight as before (no template; a weapon "+
				"version by its base row, if there is one); those out now keep theirs", spell))
			break
		}
		msgs = append(msgs, fmt.Sprintf("from the next summoning: the pets of %q fight by the %q template; those out now keep "+
			"theirs", spell, database.TemplateKey(text(after, "template"))))
	case "npctemplate":
		name := database.NPCClassKey(text(row, "name"))
		if after == nil {
			msgs = append(msgs, fmt.Sprintf("at once: the NPCs named %q fight as before, by no template", name))
			break
		}
		tmpl := database.TemplateKey(text(after, "template"))
		of, _ := s.d.World.NPCTemplateCount(tmpl)
		msgs = append(msgs, fmt.Sprintf("at once: the NPCs named %q fight by the %q template (%d by it in all), "+
			"its hit points theirs where they stand and its abilities their next fight's", name, tmpl, of))
		if before != nil && database.NPCClassKey(text(before, "name")) != name {
			msgs = append(msgs, fmt.Sprintf("those named %q fight as before", database.NPCClassKey(text(before, "name"))))
		}
	default:
		name := database.TemplateKey(text(row, "name"))
		of, _ := s.d.World.NPCTemplateCount(name)
		switch {
		case after == nil:
			msgs = append(msgs, fmt.Sprintf("at once: no NPC fights by %q; an npctemplate row naming it gives nothing now", name))
		case of == 0:
			msgs = append(msgs, fmt.Sprintf("ready: no NPC fights by %q (an npctemplate row gives it to the NPCs of a name)", name))
		default:
			msgs = append(msgs, fmt.Sprintf("at once: %d NPCs fight by %q, their hit points where they stand and its abilities their next fight's", of, name))
		}
	}
	for _, p := range s.d.World.ClassTemplateProblems() {
		msgs = append(msgs, p)
	}
	msgs = append(msgs, fmt.Sprintf("%d NPCs fight by a template", all))
	return strings.Join(msgs, "; ")
}

// checkClassTemplateRow holds a classtemplates row to what an NPC can fight
// by (database.CheckClassTemplate): a name, a class of the fifteen or -1,
// percents in bounds, a special that comes with a damage, abilities a JSON
// list of ones the server knows. A column an insert leaves out takes the
// table's default.
func checkClassTemplateRow(row map[string]any) error {
	name, ok := row["name"]
	if !ok || name == nil || database.TemplateKey(fmt.Sprint(name)) == "" {
		return badRequest("name is the template's name")
	}
	t := database.ClassTemplate{Name: fmt.Sprint(name), Class: -1, HPPct: 100, TakenPct: 100, MeleePct: 100}
	ints := map[string]*int{"class": &t.Class, "hp_pct": &t.HPPct, "taken_pct": &t.TakenPct, "melee_pct": &t.MeleePct,
		"special_chance": &t.SpecialChance, "special_from": &t.SpecialFrom}
	for col, dst := range ints {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || math.Abs(f) > 1e6 {
			return badRequest("%s is a whole number", col)
		}
		*dst = int(f)
	}
	floats := []struct {
		col, what string
		dst       *float64
	}{
		{"special_damage", "the special's damage a level", &t.SpecialDamage},
		{"special_every", "seconds from one timed special to the next (0 none)", &t.SpecialEvery},
		{"special_first", "seconds into a fight for the first timed special", &t.SpecialFirst},
		{"special_jitter", "seconds either way on each timed special", &t.SpecialJitter},
	}
	for _, c := range floats {
		if v, ok := row[c.col]; ok && v != nil {
			f, fok := asFloat(v)
			if !fok || math.IsNaN(f) || math.IsInf(f, 0) {
				return badRequest("%s is a number: %s", c.col, c.what)
			}
			*c.dst = f
		}
	}
	if v, ok := row["special"]; ok && v != nil {
		t.Special = strings.TrimSpace(fmt.Sprint(v))
	}
	if v, ok := row["abilities"]; ok && v != nil {
		list, err := database.ParseTemplateAbilities(fmt.Sprint(v))
		if err != nil {
			return badRequest("%v", err)
		}
		t.Abilities = list
	}
	if err := database.CheckClassTemplate(t); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

// checkPetTemplateRow holds a pettemplate row to what a pet can be summoned
// by (database.CheckPetTemplate). Whether the template is there is the
// world's to say (the answer's line).
func checkPetTemplateRow(row map[string]any) error {
	p := database.PetTemplate{SwingClass: -1, Swing: -1}
	if v, ok := row["spell"]; ok && v != nil {
		p.Spell = fmt.Sprint(v)
	}
	if v, ok := row["template"]; ok && v != nil {
		p.Template = fmt.Sprint(v)
	}
	ints := map[string]*int{"swing_class": &p.SwingClass, "swing": &p.Swing, "round_ms": &p.RoundMS}
	for col, dst := range ints {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		f, fok := asFloat(v)
		if !fok || f != math.Trunc(f) || math.Abs(f) > 1e6 {
			return badRequest("%s is a whole number", col)
		}
		*dst = int(f)
	}
	if v, ok := row["weapon"]; ok && v != nil {
		f, fok := asFloat(v)
		if !fok || (f != 0 && f != 1) {
			return badRequest("weapon is 0 or 1")
		}
	}
	if err := database.CheckPetTemplate(p); err != nil {
		return badRequest("%v", err)
	}
	return nil
}

// checkNPCTemplateRow holds an npctemplate row to a name and a template's
// name. Whether the template is there is the world's to say (the answer's
// live line): a row naming none gives its NPCs nothing.
func checkNPCTemplateRow(row map[string]any) error {
	if v, ok := row["name"]; !ok || v == nil || database.NPCClassKey(fmt.Sprint(v)) == "" {
		return badRequest("name is the NPCs' name (as NPC classes key it: lower case, no trailing $)")
	}
	if v, ok := row["template"]; !ok || v == nil || database.TemplateKey(fmt.Sprint(v)) == "" {
		return badRequest("template is a Class templates name (tank, melee, wizard, necromancer, shadowknight, or a GM's)")
	}
	return nil
}
