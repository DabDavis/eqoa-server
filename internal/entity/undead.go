package entity

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// undead.go — which creatures are undead, and the spells that work on nothing
// else (F3 20261025-215937, jib: "Necromancers undead nuke line should only
// work on monsters tagged Undead. For example; Ward Death should only work on
// undead creatures, such as skeletons, zombies, ghouls and so on. Otherwise it
// should give the error that the creature is not undead, fizzle, and not fire
// off."; -222603, the cleric's Smite Death the same; -220159, "Necromancer
// charm line, for example "Coerce Undead", should only work on monsters
// flagged as undead").
//
// A creature is undead when table npcundead says so for its name (a GM's word
// in the panel; tools/seed_npc_undead.py seeds it from the creatures' names
// and their bodies), and a name with no row is undead when the name itself
// says so (undeadName). A row with undead 0 keeps the creatures of an
// undead-sounding name living. No record of the live game says which
// creatures were undead: the names and the bodies are all there is.
//
// An undead-only spell is one whose line starts "Target undead" (EQOA-Data's
// lists: the cleric's and necromancer's Ward, Smite, Disrupt, Torment and
// Persecute Death, the shadow knight's Punish Death, the cleric's Weaken ...
// Death debuffs and Disable/Harm/Destroy Undead), or a charm whose name or
// description says undead (the necromancer's Charm, Command, Control and
// Coerce Undead: "CM: Charm an Undead.", their lines a plain "Target, Charm
// ; Increase 25% based on CHA"). Cast at a creature that is not undead it is
// refused at its start with castNotUndead in red (EverQuest's words: no
// recording of the live game's refusal exists), a brief cast (the fizzle a
// refused charm makes) and no power taken, no lockout. An undead-only charm
// takes undead with the charm's own roll (charm.go); every other charm still
// fails on them ("Attempt resisted!", as Sandstorm's did on Rotting Flesh).

// castNotUndead is the refusal of an undead-only spell at anything else.
const castNotUndead = "This spell only works on the undead."

// reUndeadName is the words that make a creature's name undead's, whole
// words only: "Hatebone" orcs and "Bonefish" are not, as the old substring
// list ("bone") had them. tools/seed_npc_undead.py keeps the same list.
var reUndeadName = regexp.MustCompile(`\b(?:skeletons?|skeletal|zombies?|ghouls?|ghasts?|mumm(?:y|ies|ified)|` +
	`spectres?|specters?|spectral|wraiths?|liches?|undead|revenants?|ghosts?|ghostly|phantoms?|phantasms?|` +
	`apparitions?|banshees?|wights?|unliving|unresting|reanimated|risen|raised|cadavers?|rotting|decaying|` +
	`bones|eidolons?|souls?|haunts?|dead|shades?|restless)\b`)

// undeadName reports whether a creature's name says it is undead.
func undeadName(name string) bool {
	return reUndeadName.MatchString(strings.ToLower(name))
}

// SetUndeadTags sets the GMs' undead tags (table npcundead), keyed as
// database.NPCClassKey keys a name.
func (w *World) SetUndeadTags(tags map[string]bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.undead = tags
	w.forgetWildlifeLocked() // an undead creature is not wildlife (wildlife.go)
}

// LoadUndeadTags reads table npcundead into the world: at the start and after
// a GM's edit. Call it without the database held. It reports how many names
// it read, and how many of them are undead.
func (w *World) LoadUndeadTags(db *database.SQLiteDB) (names, undead int, err error) {
	tags, err := db.NPCUndeadTags()
	if err != nil {
		return 0, 0, fmt.Errorf("npcundead: %w", err)
	}
	for _, u := range tags {
		if u {
			undead++
		}
	}
	w.SetUndeadTags(tags)
	return len(tags), undead, nil
}

// undeadLocked reports whether a creature is undead: its name's row in
// npcundead, else its name. Must be called with the lock held.
func (w *World) undeadLocked(npc *NPC) bool {
	if u, ok := w.undead[database.NPCClassKey(npc.Name)]; ok {
		return u
	}
	return undeadName(npc.Name)
}

// readUndead marks the spell undead-only (see the file's comment) from its
// name and description beside its line.
func (d *SpellDef) readUndead(name, desc string) {
	if d.Target == TargetUndead {
		d.UndeadOnly = true
		return
	}
	if d.Charms() && (strings.Contains(strings.ToLower(name), "undead") || strings.Contains(strings.ToLower(desc), "undead")) {
		d.UndeadOnly = true
	}
}

// undeadOnly reports whether the spell works on undead alone: a "Target
// undead" line, or what readUndead found.
func (d *SpellDef) undeadOnly() bool {
	return d.UndeadOnly || d.Target == TargetUndead
}

// undeadStartLocked checks an undead-only spell as it begins: the refusal's
// line, and whether it fizzles, for a target that is not undead. Must be
// called with the lock held.
func (w *World) undeadStartLocked(targets []EntityID) (refusal string, fizzle bool) {
	for _, id := range targets {
		if npc := w.NPCs[id]; npc != nil && !w.undeadLocked(npc) {
			return castNotUndead, true
		}
	}
	return "", false
}

// UndeadSummary says what makes a creature undead, for the log at the start.
func (w *World) UndeadSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	undead := 0
	for _, u := range w.undead {
		if u {
			undead++
		}
	}
	return fmt.Sprintf("undead: %d names tagged in npcundead (%d undead, %d not), any other name by its words",
		len(w.undead), undead, len(w.undead)-undead)
}
