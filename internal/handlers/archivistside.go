package handlers

import (
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// archivistside.go — the quests' side spells in the Spell Archivist's "Grant
// basic spells" (the GM Jibrish's petition 1361, 2026-11-09: "Add to the basic
// spell option spell grant the quested side spells for example the level 20
// spells, the level 5 spells. Do this for each class.").
//
// A class's side spells are what its quests teach a character of it besides
// what the merchants sell: of every enabled quest whose classes take it (or
// that names none), at each step its own spells and the class's (RewardFor),
// and the scrolls among those rewards' items, each scroll's spell the record
// the scribe would write for the class (scrollSpell, the class's classspells
// first), granted once the character is of the spell's level and the quest's
// (capped at 60), but:
//
//   - never a step's choice. Outside the 30/40/49 reward quests the only
//     choices that teach a spell are the level 20 class quests' (1-20/<city>/
//     <class>/L20): two scrolls, three for the paladin and the cleric, each
//     the first spell of one of the class's paths (Spacious Mind is the
//     enchanter's self power, Expansive Mind's line; Power Boon its power for
//     others, Power Jet's), every class's alike. A path is one at a time
//     (petition 1325), and a rehabilitator counts the level 20 pick a path's
//     (rehabpaths.go), so those follow the path rule: "Grant path spells"
//     gives the level 20 spell of the path chosen (archivistpaths.go, offered
//     from level 20 now), and no spell of the class's paths is a side spell,
//     wherever a quest teaches it (a city whose level 20 quest kept one path
//     teaches its spell with no choice: the warrior's Bellow);
//   - nor the quests whose spells are another system's: the 30/40/49 path
//     reward quests (the paths), the were, werehunter and vampire quests
//     (sideSkipped: any class's lines of their own, whose later forms are
//     masteries), and the Tank Exemplars' (two paths of their own at 55, the
//     guide's Offensive and Defensive: the warrior's Dropkick against
//     Serenity);
//   - nor a were's ability (IsWereAbility), nor a spell the class's
//     classspells row has as a mastery's, an epic's or a master class's
//     (grantedTo), as the merchants' part leaves them: the 60 epics' choices,
//     the ranger's Nimble Grace, the Plane of Disease reward that is the
//     Animator's (Stormlord's Touch) or the Assassin's.
//
// Like the merchants' part, the grant only writes what the book lacks, by
// record or by exact name: nothing is taken out.

// sideSkipped are the import key families whose quests teach no side spell
// (the notes at the top), as EQOA-Data's folders name them.
var sideSkipped = map[string]bool{"30,40,49": true, "Weres": true, "TankExemplar": true}

// sideSkip is whether a quest's spells are no side spell's by its family.
func sideSkip(qu *database.Quest) bool {
	family, _, _ := strings.Cut(importKey(qu.Source), "/")
	return sideSkipped[family]
}

// questTeaching is a spell a quest step teaches a class, by its record or by
// a scroll it hands over (spell or item), and the quest's level.
type questTeaching struct {
	quest, step int
	spell, item int
	level       int
}

// classTeachings is what the enabled quests teach a character of the class
// at their steps, choices left out (the notes at the top), in quest and step
// order.
func (q *QuestEngine) classTeachings(class int) []questTeaching {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	qs := make([]*database.Quest, 0, len(q.cat.all))
	for _, qu := range q.cat.all {
		if qu.Enabled && !sideSkip(qu) && (len(qu.Classes) == 0 || containsInt(qu.Classes, class)) {
			qs = append(qs, qu)
		}
	}
	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	var out []questTeaching
	for _, qu := range qs {
		for i := range qu.Steps {
			st := &qu.Steps[i]
			rw := st.RewardFor(class)
			for _, id := range rw.Spells {
				out = append(out, questTeaching{quest: qu.ID, step: st.Step, spell: id, level: qu.MinLevel})
			}
			for _, it := range rw.Items {
				out = append(out, questTeaching{quest: qu.ID, step: st.Step, item: it.PatternID, level: qu.MinLevel})
			}
		}
	}
	return out
}

// sideSpells is the class's side spells to the level (capped at 60), in
// level, then name order, one a name (the notes at the top). The paths are
// the class's as the running quests give them (classPaths).
func sideSpells(qe *QuestEngine, arch *spellArchive, class, level int) []database.SpellName {
	if qe == nil || arch == nil {
		return nil
	}
	level = min(level, archivistLevelCap)
	paths, _ := classPaths(qe, arch, class)
	pathIDs, pathNames := map[int]bool{}, map[string]bool{}
	for _, pa := range paths {
		for _, s := range pa.spells {
			pathIDs[s.id], pathNames[exactKey(s.name)] = true, true
		}
	}
	has := func(id int) bool { _, ok := arch.sources[id][class]; return ok }
	seen := map[int]bool{}
	var out []database.SpellName
	for _, t := range qe.classTeachings(class) {
		var s database.SpellName
		ok := false
		if t.spell > 0 {
			s, ok = arch.names[t.spell]
		} else if sc, scroll := arch.scrollRecs[t.item]; scroll {
			s, ok = arch.spells.scrollSpell(sc.Name, sc.LevelReq, has)
		}
		switch {
		case !ok, seen[s.SpellID], max(s.Level, t.level) > level:
			continue
		case pathIDs[s.SpellID] || pathNames[exactKey(s.Name)]:
			continue
		case database.IsWereAbility(s.Name, arch.descs[s.SpellID]) || !grantedTo(arch.sources[s.SpellID], class):
			continue
		}
		seen[s.SpellID] = true
		out = append(out, s)
	}
	sortSpells(out)
	return lacking(nil, out)
}

// basicSpells is the basic grant's spells for a character of the class and
// race at the level: the merchants' (spellArchive.basic) and the quests' side
// spells (sideSpells), but those he leaves to the masteries (masteryOwn,
// petition 1381), in level, then name order, one a name.
func basicSpells(qe *QuestEngine, arch *spellArchive, class, race, level int) []database.SpellName {
	all := append(arch.basic(class, race, level), sideSpells(qe, arch, class, level)...)
	own := masteryOwn(qe, arch, class, race)
	kept := make([]database.SpellName, 0, len(all))
	for _, s := range all {
		if !own[s.SpellID] {
			kept = append(kept, s)
		}
	}
	sortSpells(kept)
	return lacking(nil, kept)
}

// sortSpells sorts spells by level, then name, then record.
func sortSpells(v []database.SpellName) {
	sort.SliceStable(v, func(i, j int) bool {
		if v[i].Level != v[j].Level {
			return v[i].Level < v[j].Level
		}
		if v[i].Name != v[j].Name {
			return v[i].Name < v[j].Name
		}
		return v[i].SpellID < v[j].SpellID
	})
}
