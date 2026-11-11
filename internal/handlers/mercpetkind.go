package handlers

import (
	"fmt"
	"log"
	"strings"

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
	10: {{"Earth", []string{"earth"}}, {"Water", []string{"water"}}, {"Fire", []string{"fire"}}, {"Air", []string{"air"}},
		{"Efreet", []string{"efreet"}}, {"Ghilan", []string{"ghilan"}}},
	11: {{"Bones", []string{"bones"}}, {"Flesh", []string{"flesh"}}, {"Death", []string{"death"}}},
	12: {{"Animation", []string{"animation"}}, {"Golem", []string{"golem"}}, {"Blades", []string{"blade", "tempest"}}},
}

// mercPetBest is the pick of the best of all, as /merc pet takes it.
const mercPetBest = "best"

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
	if k, ok := mercPetKindNamed(class, pick); ok && pick != "" {
		return k.name
	}
	return mercPetAll
}

// pet is /merc pet [word]: the kinds said, or one picked (the file's doc).
func (m *Mercs) pet(sess *session.Session, p *entity.Player, word string) {
	if m.db == nil {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID)
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
	if !strings.EqualFold(strings.TrimSpace(word), mercPetBest) {
		k, ok := mercPetKindNamed(row.Class, word)
		if !ok {
			SendErrorMessage(sess, fmt.Sprintf(mercPetWhich, class, mercPetKindsOf(row.Class)))
			return
		}
		pick = strings.ToLower(k.name)
	}
	if err := m.db.SetMercPet(p.CharServerID, pick); err != nil {
		log.Printf("Merc: %s's pet pick not kept: %v", p.Name, err)
		return
	}
	m.rekit(p.CharServerID, p.Level) // the merc out takes it now
	log.Printf("Merc: %s's %q will summon %s", p.Name, row.Name, mercPetSaid(row.Class, pick))
	SendClientMessage(sess, fmt.Sprintf(mercPetSet, row.Name, mercPetSaid(row.Class, pick)))
}
