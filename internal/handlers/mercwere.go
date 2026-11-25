package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercwere.go — a merc's were form (petition 3069, 2026-11-25: "it would be cool if we
// could pick were-forms for our mercs like we do their path and spells, it would be cool
// to have a pack of lv 60 wolves"; the owner approved). The World's half, the form on the
// merc, is entity/mercwere.go.
//
//   - /merc [name or 1-3] were: what form each merc takes and the forms it may take;
//     /merc [name or 1-3] were <form> gives it one (its name, or a part of it only one form
//     holds), in place of any it took; /merc [name or 1-3] were none takes it away. With no
//     merc named, every hire takes it: a pack.
//   - The forms it may take are the were forms (entity.IsWereForm: the five lines' own and
//     the Feral masteries') its hirer's own book holds: a merc takes only what its hirer
//     earned on the were line, so the line and its quests stay the way to a form, and a
//     form a player could not show a merc cannot either.
//   - Kept in its row (charmerc.were, the form's name), so at every login and through a
//     keep restart (its kit is made from its row), and taken at once by a merc out (rekit).
//     A row whose form its hirer's book no longer holds (a cure that took the line, a GM's
//     edit) is a merc in its own body until its hirer holds it again.
//   - Its menu's "Were form" (shown only to a hirer whose book holds one): the forms and
//     None.

// The were form's lines (ours). A question is 127 characters at most.
const (
	mercWereNoneHeld = "Your book holds no were form: a mercenary takes only a form you have earned yourself."
	mercWereIs       = "%s takes the were form %s."
	mercWereIsNot    = "%s takes no were form."
	mercWereLost     = "%s's were form %s is not in your book now: it keeps its own body until it is."
	mercWereMay      = "Your mercenaries may take: %s. /merc [name] were <form> gives one, /merc [name] were none takes it away."
	mercWereWhich    = "No were form of yours is called %q. Yours: %s."
	mercWereSet      = "%s takes the form of %s."
	mercWereGone     = "%s leaves its were form and is itself again."
	mercWereAsk      = "%s's were form: %s. Which?"
)

// mercWereForm is a were form its hirer's book holds: its spell and name.
type mercWereForm struct {
	id   int
	name string
}

// wereForms is the were forms the character's book holds, in its book's order, each name
// once (its first record).
func (m *Mercs) wereForms(serverID uint32) []mercWereForm {
	if m.db == nil || serverID == 0 {
		return nil
	}
	book, err := database.NewSpellbookStore(m.db).GetCharacterSpells(serverID)
	if err != nil {
		log.Printf("Merc: character %d's book: %v", serverID, err)
		return nil
	}
	var out []mercWereForm
	seen := map[string]bool{}
	for _, e := range book {
		name := e.Pattern.SpellName
		if !entity.IsWereForm(name) || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, mercWereForm{id: e.Pattern.SpellID, name: name})
	}
	return out
}

// mercWereNamed is the form of forms a word names: its name whole, else the one form whose
// name holds the word.
func mercWereNamed(forms []mercWereForm, word string) (mercWereForm, bool) {
	w := strings.ToLower(strings.TrimSpace(word))
	var hit []mercWereForm
	for _, f := range forms {
		n := strings.ToLower(f.name)
		if n == w {
			return f, true
		}
		if strings.Contains(n, w) {
			hit = append(hit, f)
		}
	}
	if len(hit) == 1 {
		return hit[0], true
	}
	return mercWereForm{}, false
}

// mercWereSaid is the forms' names, for a line.
func mercWereSaid(forms []mercWereForm) string {
	var names []string
	for _, f := range forms {
		names = append(names, f.name)
	}
	return strings.Join(names, ", ")
}

// mercWereSpell is the row's were form as its kit carries it: the spell of its hirer's
// book of the row's name, nil for none (or one its hirer's book no longer holds).
func (m *Mercs) mercWereSpell(row database.MercRow) *entity.CastSpell {
	if row.Were == "" || m.db == nil {
		return nil
	}
	for _, f := range m.wereForms(row.ServerID) {
		if strings.EqualFold(f.name, row.Were) {
			if book := entity.CastSpellsByID(m.db, []int{f.id}); len(book) == 1 {
				return &book[0]
			}
			return nil
		}
	}
	return nil
}

// were is /merc [name or 1-3] were [form|none] (the file's doc): slot 0 is every hire.
func (m *Mercs) were(sess *session.Session, p *entity.Player, rows []database.MercRow, slot int, word string) {
	if m.db == nil || len(rows) == 0 {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	var picked []database.MercRow
	for _, r := range rows {
		if slot == 0 || r.Slot == slot {
			picked = append(picked, r)
		}
	}
	if len(picked) == 0 {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	forms := m.wereForms(p.CharServerID)
	held := func(name string) bool {
		for _, f := range forms {
			if strings.EqualFold(f.name, name) {
				return true
			}
		}
		return false
	}
	word = strings.TrimSpace(word)
	switch {
	case word == "":
		for _, r := range picked {
			switch {
			case r.Were == "":
				SendClientMessage(sess, fmt.Sprintf(mercWereIsNot, r.Name))
			case held(r.Were):
				SendClientMessage(sess, fmt.Sprintf(mercWereIs, r.Name, r.Were))
			default:
				SendClientMessage(sess, fmt.Sprintf(mercWereLost, r.Name, r.Were))
			}
		}
		if len(forms) == 0 {
			SendClientMessage(sess, mercWereNoneHeld)
			return
		}
		SendClientMessage(sess, fmt.Sprintf(mercWereMay, mercWereSaid(forms)))
		return
	case mercMasterNoneWords[strings.ToLower(word)]:
		for _, r := range picked {
			if r.Were == "" {
				SendClientMessage(sess, fmt.Sprintf(mercWereIsNot, r.Name))
				continue
			}
			if m.setWere(sess, p, r, "") {
				SendClientMessage(sess, fmt.Sprintf(mercWereGone, r.Name))
			}
		}
		return
	}
	if len(forms) == 0 {
		SendErrorMessage(sess, mercWereNoneHeld)
		return
	}
	f, ok := mercWereNamed(forms, word)
	if !ok {
		SendErrorMessage(sess, fmt.Sprintf(mercWereWhich, word, mercWereSaid(forms)))
		return
	}
	for _, r := range picked {
		if m.setWere(sess, p, r, f.name) {
			SendClientMessage(sess, fmt.Sprintf(mercWereSet, r.Name, f.name))
		}
	}
}

// setWere keeps the row's were form and gives the merc out its kit: false when it could
// not be kept (said).
func (m *Mercs) setWere(sess *session.Session, p *entity.Player, row database.MercRow, name string) bool {
	if err := m.db.SetMercWere(p.CharServerID, row.Slot, name); err != nil {
		log.Printf("Merc: %s's %q's were form not kept: %v", p.Name, row.Name, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	m.rekitSlot(p, row.Slot) // the merc out takes it now
	log.Printf("Merc: %s's %q's were form: %q (was %q)", p.Name, row.Name, name, row.Were)
	return true
}

// menuWere asks the merc's were form (its menu's Were form): the forms its hirer's book
// holds, as many as a box takes, and None.
func (m *Mercs) menuWere(sess *session.Session, p *entity.Player, slot int, info entity.MercInfo) {
	forms := m.wereForms(p.CharServerID)
	if len(forms) == 0 {
		mercSay(sess, mercWereNoneHeld)
		return
	}
	row, ok, _ := m.db.Merc(p.CharServerID, slot)
	if !ok {
		return
	}
	forms = forms[:min(len(forms), dialogRoom(sess)-1)]
	var keys, answers []string
	for _, f := range forms {
		label := f.name
		if strings.EqualFold(f.name, row.Were) {
			label += " (now)"
		}
		keys, answers = append(keys, f.name), append(answers, label)
	}
	keys, answers = append(keys, "none"), append(answers, "None")
	held := "none"
	if row.Were != "" {
		held = row.Were
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageWere, slot: slot, keys: keys}, fmt.Sprintf(mercWereAsk, info.Name, held), answers)
	m.mu.Unlock()
}

// wereAnswer is a choice in the were form's box (menuAnswer's).
func (m *Mercs) wereAnswer(sess *session.Session, p *entity.Player, a mercAsk, n int) {
	if n < 0 || n >= len(a.keys) {
		return
	}
	m.Command(sess, p, fmt.Sprintf("%d were %s", a.slot, a.keys[n]))
}
