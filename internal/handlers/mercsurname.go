package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercsurname.go — a merc's surname (Heirloom petition 3430: "I would like to
// request surnames for mercs"; the owner, 2026-11-28: yes).
//
//   - /merc [name or 1-3] surname <surname> (a PS2's "/t merc ..."): one word
//     held to a character's surname's rules (surname.go: petNameShape's
//     letters, 3 to 15, the name filter, the staff words; not its owner's name,
//     a character's, nor a named individual's), cased as one, kept in
//     charmerc.surname with its gm_audit row and logged as a rename is
//     (petnamelog, kind "merc surname"). No level and no wait: a merc's name
//     takes none either (petrename.go).
//   - /merc [name or 1-3] surname alone says its surname and the rules;
//     "surname clear" (none, off) takes it off.
//
// Where it shows: where a character's does, after its name on vanilla's name
// plates and its details box (OpcodeSurnames, by the merc's name: a merc's
// name is the world's, petition 2409); a PS2 draws no surname on a plate
// (surname.go's doc).

var mercSurnameUsage = fmt.Sprintf("/merc [name or 1-3] surname <surname>: a surname after your mercenary's name, one word of %d to %d letters, as a character's surname. /merc [name or 1-3] surname clear takes it off.", CharNameMin, CharNameMax)

const (
	mercSurnameTaken   = "Your mercenary is now %s %s."
	mercSurnameIs      = "%s's surname is %s."
	mercSurnameNone    = "%s has no surname."
	mercSurnameCleared = "%s's surname %s is cleared."
	mercSurnameSame    = "%s's surname is %s already."
	mercSurnameOwnName = "%s is its own name: choose another."
)

// mercSurnameLine reads a /merc line that names a surname: the merc named
// before "surname" (its slot's number or its name; "" the one hired), and the
// words after it. ok false: not a surname's line.
func mercSurnameLine(args string) (pick, rest string, ok bool) {
	f := strings.Fields(args)
	at := -1
	for i, w := range f {
		if strings.EqualFold(w, "surname") {
			at = i
			break
		}
	}
	if at < 0 || at > 1 {
		return "", "", false
	}
	if at == 1 {
		pick = f[0]
	}
	return pick, strings.Join(f[at+1:], " "), true
}

// surname is /merc surname (the file's doc).
func (m *Mercs) surname(sess *session.Session, p *entity.Player, rows []database.MercRow, pick, rest string) {
	if len(rows) == 0 {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	slot := 0
	if pick != "" {
		s, ok := mercPicked(rows, pick)
		if !ok {
			SendErrorMessage(sess, fmt.Sprintf(mercRenameNoNamed, pick, mercRowNames(rows)))
			return
		}
		slot = s
	}
	if slot = m.oneSlot(sess, rows, slot, "surname <surname>"); slot == 0 {
		return
	}
	var row database.MercRow
	found := false
	for _, r := range rows {
		if r.Slot == slot {
			row, found = r, true
		}
	}
	if !found {
		SendErrorMessage(sess, fmt.Sprintf(mercRenameNoSlot, slot))
		return
	}
	rest = strings.TrimSpace(rest)
	switch {
	case rest == "":
		if row.Surname != "" {
			SendClientMessage(sess, fmt.Sprintf(mercSurnameIs, row.Name, row.Surname))
		} else {
			SendClientMessage(sess, fmt.Sprintf(mercSurnameNone, row.Name))
		}
		SendClientMessage(sess, mercSurnameUsage)
		return
	case strings.EqualFold(rest, "clear") || strings.EqualFold(rest, "none") || strings.EqualFold(rest, "off"):
		if row.Surname == "" {
			SendClientMessage(sess, fmt.Sprintf(mercSurnameNone, row.Name))
			return
		}
		if !m.keepMercSurname(sess, p, row, "") {
			return
		}
		m.logRename(sess, p, "merc surname", row.Name+" "+row.Surname, "clear", petVerdict{name: ""})
		SendClientMessage(sess, fmt.Sprintf(mercSurnameCleared, row.Name, row.Surname))
		return
	}
	v := m.checkSurname(p, row, rest)
	if v.refusal == "" && strings.EqualFold(v.name, row.Surname) {
		SendClientMessage(sess, fmt.Sprintf(mercSurnameSame, row.Name, row.Surname))
		return
	}
	if v.refusal == "" && !m.keepMercSurname(sess, p, row, v.name) {
		v = petVerdict{refusal: renameLedgerLine, reason: "its row could not be written"}
	}
	m.logRename(sess, p, "merc surname", row.Name+" "+row.Surname, rest, v)
	if v.refusal != "" {
		SendErrorMessage(sess, v.refusal)
		return
	}
	SendClientMessage(sess, fmt.Sprintf(mercSurnameTaken, row.Name, v.name))
}

// checkSurname holds a merc's surname asked to a character's surname's rules
// (surname.go's take and checkName): its shape, and not its own name, its
// owner's, a character's nor a named individual's.
func (m *Mercs) checkSurname(p *entity.Player, row database.MercRow, asked string) petVerdict {
	v, ok := petNameShape(asked)
	if !ok {
		return v
	}
	name := v.name
	switch {
	case strings.EqualFold(name, row.Name):
		return petVerdict{refusal: fmt.Sprintf(mercSurnameOwnName, name), reason: "its own name"}
	case strings.EqualFold(name, p.Name):
		return petVerdict{refusal: fmt.Sprintf(renameCharacter, name), reason: "its owner's name"}
	}
	if m.db != nil {
		named, gm, err := m.db.NamedCharacter(name)
		if err != nil {
			log.Printf("PetRename: the characters named %q: %v", name, err)
			return petVerdict{refusal: renameLedgerLine, reason: "the characters could not be read"}
		}
		if gm {
			return petVerdict{refusal: renameFiltered, reason: "a GM's character's name", filtered: true}
		}
		if named {
			return petVerdict{refusal: fmt.Sprintf(renameCharacter, name), reason: "a character's name"}
		}
	}
	if m.gw != nil && m.gw.NPCIndividualNamed(name) {
		return petVerdict{refusal: fmt.Sprintf(renameIndividual, name), reason: "a named individual's name"}
	}
	return v
}

// keepMercSurname writes the merc's surname ("" none) and tells the clients
// that draw surnames; false (the player told) when it could not be written.
func (m *Mercs) keepMercSurname(sess *session.Session, p *entity.Player, row database.MercRow, surname string) bool {
	if m.db == nil {
		SendErrorMessage(sess, renameLedgerLine)
		return false
	}
	detail := fmt.Sprintf("/merc surname: %s's surname %q to %q", row.Name, row.Surname, surname)
	if err := m.db.SetMercSurnameAudited(p.CharServerID, row.Slot, surname, renameWho(sess), detail); err != nil {
		log.Printf("PetRename: %s's merc %q (slot %d) surname not kept: %v", p.Name, row.Name, row.Slot, err)
		SendErrorMessage(sess, renameLedgerLine)
		return false
	}
	if s := surnamesH.Load(); s != nil {
		s.tell(surnamePair{row.Name, surname})
	}
	return true
}
