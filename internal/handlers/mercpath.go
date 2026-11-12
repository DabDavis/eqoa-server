package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercpath.go — a merc's class path (the owner, 2026-11-11: "lets add path
// choice to each merc, so blood nec or power nec and they both use their path
// spells. same with all classes").
//
// A class's paths are the Spell Archivist's (archivistpaths.go's
// archivistPathLines: the path reward quests' lines at 20, 30, 40 and 49, a
// player's one path at a time). A merc's book (Mercs.kit: what its levels give
// a player of its class) holds every path's spells; with a path it keeps the
// chosen path's and drops the others' (mercPathBook), so a blood necromancer
// casts Blood Squall to Blood Gale and none of the Power line, and every spell
// no path names stays. The path is the hire's (charmerc.path, kept over
// logins and restarts): -1, until the hirer picks, is its class's default
// (mercPathDefault: what its role casts best: a healer's heal path, the
// shaman's pet, else the first line). /merc path names the paths and the
// merc's; /merc path <word> picks one by its name or any of its spells' (a
// part of it: "blood", "power", "boon"), and the merc out takes it at once.
// The hire itself is unchanged: its line says the path taken and how to
// change it.

// mercPathNames are each class's paths' short names, in archivistPathLines'
// order.
var mercPathNames = map[int][]string{
	0:  {"Roar", "Pillar"},
	1:  {"Eye", "Blade"},
	2:  {"Bastion", "Secure", "Barrier"},
	3:  {"Blood", "Tower"},
	4:  {"Stance", "Strike"},
	5:  {"Power", "Combat"},
	6:  {"Evasion", "Wound"},
	7:  {"Ember", "Health"},
	8:  {"Wind", "Spirit"},
	9:  {"Ward", "Healing", "Elemental"},
	10: {"Mark", "Missile"},
	11: {"Blood", "Power"},
	12: {"Mind", "Boon"},
	13: {"Clash", "Mental"},
	14: {"Knack", "Cocktail"},
}

// mercPathDefault is a class's path for a merc whose hirer picked none, where
// it is not the first: the druid's and the cleric's heals (a healer merc's
// part), the shaman's pet (a merc casts no debuff that only weakens).
var mercPathDefault = map[int]int{7: 1, 8: 1, 9: 1}

// mercPathOf is the path a merc of class takes for path (charmerc.path): it,
// when the class has it, else the class's default; -1 for a class with none.
func mercPathOf(class, path int) int {
	lines := archivistPathLines[class]
	if len(lines) == 0 {
		return -1
	}
	if path >= 0 && path < len(lines) {
		return path
	}
	return mercPathDefault[class]
}

// mercPathName is a class's path's name.
func mercPathName(class, path int) string {
	if n := mercPathNames[class]; path >= 0 && path < len(n) {
		return n[path]
	}
	if l := archivistPathLines[class]; path >= 0 && path < len(l) && len(l[path]) > 0 {
		return l[path][0]
	}
	return ""
}

// mercPathNamed is the class's path a hirer's word names: its name, or a
// spell of its line, holding the word; false for none or more than one.
func mercPathNamed(class int, word string) (int, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return 0, false
	}
	found := -1
	for i, line := range archivistPathLines[class] {
		hit := strings.Contains(strings.ToLower(mercPathName(class, i)), word)
		for _, s := range line {
			hit = hit || strings.Contains(strings.ToLower(s), word)
		}
		if !hit {
			continue
		}
		if found >= 0 {
			return 0, false
		}
		found = i
	}
	return found, found >= 0
}

// mercPathBook is the book with the path's spells and no other path's (the
// file's doc).
func mercPathBook(class, path int, book []entity.CastSpell) []entity.CastSpell {
	path = mercPathOf(class, path)
	if path < 0 {
		return book
	}
	other := map[string]bool{}
	for i, line := range archivistPathLines[class] {
		for _, s := range line {
			if i != path {
				other[strings.ToLower(s)] = true
			}
		}
	}
	for _, s := range archivistPathLines[class][path] {
		delete(other, strings.ToLower(s)) // a spell of two lines is its path's
	}
	out := book[:0:0]
	for _, s := range book {
		if !other[strings.ToLower(s.Name)] {
			out = append(out, s)
		}
	}
	return out
}

// The paths' lines (ours).
const (
	mercPathNone   = "A %s has no paths."
	mercPathList   = "%s's path: %s. Paths of a %s: %s. /merc path <name> picks one."
	mercPathIsLine = "%s takes the path of %s: /merc path names the others."
	mercPathSet    = "%s takes the path of %s."
	mercPathWhich  = "Which path? A %s's are %s."
)

// mercPathAll is a class's paths' names, joined.
func mercPathAll(class int) string {
	var n []string
	for i := range archivistPathLines[class] {
		n = append(n, mercPathName(class, i))
	}
	return strings.Join(n, ", ")
}

// path is /merc path [word]: the paths said, or one picked (the file's doc).
func (m *Mercs) path(sess *session.Session, p *entity.Player, slot int, word string) {
	if m.db == nil {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID, slot)
	if err != nil || !ok {
		SendClientMessage(sess, mercNoneLine)
		return
	}
	class := entity.ClassName(row.Class)
	if mercPathOf(row.Class, row.Path) < 0 {
		SendClientMessage(sess, fmt.Sprintf(mercPathNone, class))
		return
	}
	if word == "" {
		SendClientMessage(sess, fmt.Sprintf(mercPathList, row.Name, mercPathName(row.Class, mercPathOf(row.Class, row.Path)), class,
			mercPathAll(row.Class)))
		return
	}
	k, ok := mercPathNamed(row.Class, word)
	if !ok {
		SendErrorMessage(sess, fmt.Sprintf(mercPathWhich, class, mercPathAll(row.Class)))
		return
	}
	if err := m.db.SetMercPath(p.CharServerID, slot, k); err != nil {
		log.Printf("Merc: %s's path not kept: %v", p.Name, err)
		return
	}
	m.rekit(p.CharServerID, p.Level) // the merc out takes it now
	log.Printf("Merc: %s's %q takes the path of %s", p.Name, row.Name, mercPathName(row.Class, k))
	SendClientMessage(sess, fmt.Sprintf(mercPathSet, row.Name, mercPathName(row.Class, k)))
}
