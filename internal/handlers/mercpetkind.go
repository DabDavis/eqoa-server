package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercpetkind.go — the kind of pet a merc summons (petition 1564, a player:
// "is there a way to make an option to mercenary liaison when choosing
// magician, to pick which mag pet he will use?").
//
// A merc summons the best summoning of its book (entity/mercpet.go's
// mercPetSpell). A class whose book holds several kinds of pet has them here,
// each by the words its summonings' names hold: a magician's earth, water,
// fire and air elementals (Elementaling, Elementalkin, Elemental), its Greater
// Efreet and its Ghilan; a necromancer's bones, flesh and death; an
// enchanter's animations, golems and blades; a shaman's spirits (Kindred to
// Primal) and its Spiritual ones (Guide to Warrior). /merc pet names the kinds
// and the merc's; /merc pet <word> picks one by its name (a part of it: "air",
// "fire"), "best" goes back to the best of all. The pick is the hire's
// (charmerc.pet, kept over logins and restarts), the merc out takes it at
// once (its pet out of another summoning goes and the picked one comes), and
// a kind its book has nothing of yet at its level (an air elemental before
// 12) leaves it the best of all until it has. The Mercenary Liaison's words
// are unchanged (board #680): the pick is /merc's.

// mercPetKind is a class's kind of pet: its name and the words of its
// summonings' names.
type mercPetKind struct {
	name  string
	words []string
}

// mercPetKinds are the classes' kinds of pet (the file's doc); a class with
// one kind (a wizard's familiars) or none is not here.
var mercPetKinds = map[int][]mercPetKind{
	8: {{"Spirit", []string{"kindred", "feral", "companion", "vigilant", "frenzied", "primal"}},
		{"Spiritual", []string{"spiritual"}}},
	10: {{"Earth", []string{"earth", "badi"}}, {"Water", []string{"water", "ghilan"}}, {"Fire", []string{"fire", "efreet"}},
		{"Air", []string{"air", "djinn"}}, {"Efreet", []string{"efreet"}}, {"Ghilan", []string{"ghilan"}}},
	11: {{"Bones", []string{"bones"}}, {"Flesh", []string{"flesh"}}, {"Death", []string{"death"}}},
	12: {{"Animation", []string{"animation"}}, {"Golem", []string{"golem"}}, {"Blades", []string{"blade", "tempest"}}},
}

// mercPetBest is the pick of the best of all, as /merc pet takes it (and as
// a row keeps it for a class with a usual kind: mercPetUsual).
const mercPetBest = "best"

// mercPetUsual is a class's kind for a merc whose hirer picked none (the
// owner, 2026-11-12: "magician merc at lvl 60 is not summoning Badi, which is
// the earth tier equivalent to Efreet, Ghilan, Djinn"): the magician's earth
// line, its tank (Elementaling to Elemental, then Badi to Badi Elder).
var mercPetUsual = map[int]string{10: "earth"}

// The masteries' pets a merc of the class summons as well as its book's
// (mercMasteryPets): the magician's, whose book (the levels' spells) ends its
// elementals at the Greater Efreet (46) and the Ghilan (47) and has no earth
// or air of that tier, every one past them (Summon Badi, Djinn and Efreet at
// 49, their 55s, Badi Elder, Efreeti Lord, Ghilan Noble and Djinni Vizier at
// 60) a class mastery's. The necromancer's (Swarming Death), the enchanter's
// (Storm of Blades), the shaman's (Primal Spirit) and the wizard's (Arch
// Familiar) books reach 60 of their own; a druid has none.
var mercMasteryPetClasses = map[int]bool{10: true}

// mercMasteryPetMostClasses is the most classes a mastery's summoning is
// taught to for a merc to take it: past it the masteries every class may buy
// (Invention, the Swamp Friends, the Constructs).
const mercMasteryPetMostClasses = 2

// mercPetKindNamed is the class's kind a word names (its name holding it),
// false for none or more than one.
func mercPetKindNamed(class int, word string) (mercPetKind, bool) {
	word = strings.ToLower(strings.TrimSpace(word))
	var found []mercPetKind
	for _, k := range mercPetKinds[class] {
		if word != "" && strings.Contains(strings.ToLower(k.name), word) {
			if strings.EqualFold(k.name, word) {
				return k, true
			}
			found = append(found, k)
		}
	}
	if len(found) != 1 {
		return mercPetKind{}, false
	}
	return found[0], true
}

// mercPetPick is the summonings' words a hire's pick (charmerc.pet) holds for
// its class, none for its best.
func mercPetPick(class int, pick string) []string {
	if pick == "" {
		pick = mercPetUsual[class] // its usual, else its best
	}
	if pick == "" || strings.EqualFold(pick, mercPetBest) {
		return nil
	}
	if k, ok := mercPetKindNamed(class, pick); ok {
		return k.words
	}
	return nil
}

// The pet kinds' lines (ours).
const (
	mercPetNone  = "A %s has no choice of pet."
	mercPetList  = "%s summons: %s. Pets of a %s: %s, or best. /merc pet <name> picks one."
	mercPetSet   = "%s will summon %s."
	mercPetWhich = "Which pet? A %s's are %s, or best."
	mercPetAll   = "the best it has"
)

// mercPetKindsOf is a class's kinds' names, joined.
func mercPetKindsOf(class int) string {
	var n []string
	for _, k := range mercPetKinds[class] {
		n = append(n, k.name)
	}
	return strings.Join(n, ", ")
}

// mercPetSaid is a hire's pick as its lines say it.
func mercPetSaid(class int, pick string) string {
	if pick == "" && mercPetUsual[class] != "" {
		if k, ok := mercPetKindNamed(class, mercPetUsual[class]); ok {
			return k.name + " (its usual)"
		}
	}
	if k, ok := mercPetKindNamed(class, pick); ok && pick != "" && !strings.EqualFold(pick, mercPetBest) {
		return k.name
	}
	return mercPetAll
}

// mercMasteryPets is the masteries' summonings of the class up to the level
// a merc of it takes beside its book (mercMasteryPetClasses), none its book
// has: a class mastery's (database.GotBy), taught to no more than
// mercMasteryPetMostClasses classes.
func (m *Mercs) mercMasteryPets(class, level int, book []entity.CastSpell) ([]entity.CastSpell, error) {
	if !mercMasteryPetClasses[class] || m.db == nil {
		return nil, nil
	}
	rows, err := m.db.Query(`SELECT c.spellid, c.source, (SELECT count(DISTINCT class) FROM classspells x WHERE x.spellid = c.spellid)
		FROM classspells c WHERE c.class = ?`, class)
	if err != nil {
		return nil, err
	}
	take := map[int]bool{}
	for rows.Next() {
		var id, n int
		var source string
		if rows.Scan(&id, &source, &n) == nil && database.GotBy(source) == "mastery" && n <= mercMasteryPetMostClasses {
			take[id] = true
		}
	}
	rows.Close()
	if len(take) == 0 {
		return nil, nil
	}
	all, err := entity.SimBookFromDB(m.db, class, level, true)
	if err != nil {
		return nil, err
	}
	have := map[int]bool{}
	for _, s := range book {
		have[s.SpellID] = true
	}
	var out []entity.CastSpell
	for _, s := range all {
		if take[s.SpellID] && !have[s.SpellID] && s.Def.Summons() && !s.Def.Charms() && s.Familiar == nil && s.Travel == nil {
			s.DisplayIndex = len(book) + len(out) + 1
			out = append(out, s)
		}
	}
	return out, nil
}

// pet is /merc pet [word]: the kinds said, or one picked (the file's doc).
func (m *Mercs) pet(sess *session.Session, p *entity.Player, slot int, word string) {
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
	if len(mercPetKinds[row.Class]) == 0 {
		SendClientMessage(sess, fmt.Sprintf(mercPetNone, class))
		return
	}
	if word == "" {
		SendClientMessage(sess, fmt.Sprintf(mercPetList, row.Name, mercPetSaid(row.Class, row.Pet), class, mercPetKindsOf(row.Class)))
		return
	}
	pick := ""
	if strings.EqualFold(strings.TrimSpace(word), mercPetBest) {
		if mercPetUsual[row.Class] != "" {
			pick = mercPetBest // the best of all, not its usual
		}
	} else {
		k, ok := mercPetKindNamed(row.Class, word)
		if !ok {
			SendErrorMessage(sess, fmt.Sprintf(mercPetWhich, class, mercPetKindsOf(row.Class)))
			return
		}
		pick = strings.ToLower(k.name)
	}
	if err := m.db.SetMercPet(p.CharServerID, slot, pick); err != nil {
		log.Printf("Merc: %s's pet pick not kept: %v", p.Name, err)
		return
	}
	m.rekit(p.CharServerID, p.Level) // the merc out takes it now
	log.Printf("Merc: %s's %q will summon %s", p.Name, row.Name, mercPetSaid(row.Class, pick))
	SendClientMessage(sess, fmt.Sprintf(mercPetSet, row.Name, mercPetSaid(row.Class, pick)))
}
