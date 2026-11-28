package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// petrename.go — players name their pets and mercs (the owner, 2026-11-25:
// "let players rename pets and mercs but apply the name filter to catch bad
// actors"; petition 3019, a player asking a GM to rename his mercs, which only
// a GM could).
//
//   - /merc [name or 1-3] rename <name> (#merc, a PS2's "/t merc ..."; also
//     "/merc rename <old name> <new name>"): the merc of the slot, out, on
//     standby or resting, named anew in its row (charmerc.name) and where it
//     stands;
//   - /pet rename <name> (#pet, a PS2's "/t pet rename <name>"): the player's
//     summoned pet out, named at once and kept for its summoning spell (table
//     petnames: every later summoning of that spell comes by it, through a
//     logout or a restart). A charmed creature keeps its own name (it is the
//     world's: entity/petrename.go).
//
// A name is held to a character's rules (CharacterHandler.isValidName:
// letters only, 3 to 15 of them, no word the name filter refuses
// (profanity_names.go), not a reserved name) and cased as a GM's Change Name
// cases one (its first letter a capital, the rest small), and more, as a name
// another player sees over a creature: not one of the staff words
// (petStaffNames), not a character's (any one's: a GM's is the filter's), not
// another character's merc's nor one of the player's own others (a merc's:
// petition 2409, a merc's name is the world's), not a named individual's of
// the world (entity.World.NPCIndividualNamed). It costs nothing and has no
// wait: each is logged, and nothing in it is a resource to farm.
//
// Every rename asked is logged (table petnamelog, and the server's log under
// "PetRename:"): the accepted ones' row changes in gm_audit as well, the
// player named, so the GM panel lists and reverts them (as the Pet Groomer's
// dyes are). A refusal by the filter (a refused word, a reserved or staff
// name, a GM's name) is the outcome "filtered" and a "PetRename: FILTERED"
// line, which a GM finds in the panel's server log (its filter "FILTERED")
// and its Pet renames table; the player is told only that the name is not
// allowed and that refused names are logged.

// petRenameMin and petRenameMax are a pet's or a merc's name's length in
// letters: a character's (CharNameMin and CharNameMax, isValidName's).
// validCharName is asked after them, so a character's rules hold here
// whatever they become.
const (
	petRenameMin = CharNameMin
	petRenameMax = CharNameMax
)

// petStaffNames are names no pet or merc takes besides a character's
// reserved ones (isValidName: admin, gm, moderator, server, system): the
// staff's and the makers', which over a creature read as theirs.
var petStaffNames = map[string]bool{
	"admin": true, "administrator": true, "gm": true, "gamemaster": true, "moderator": true, "mod": true,
	"server": true, "system": true, "guide": true, "staff": true, "support": true, "owner": true,
	"developer": true, "dev": true, "sony": true, "soe": true, "verant": true, "daybreak": true,
}

// The rename's lines that name the length: a character's name's
// (CharNameMin to CharNameMax).
var (
	mercRenameUsage = fmt.Sprintf("/merc [name or 1-3] rename <new name>: one word of %d to %d letters, as a character's name.", CharNameMin, CharNameMax)
	petRenameUsage  = fmt.Sprintf("/pet rename <new name>: names the pet you summoned, one word of %d to %d letters, as a character's name.", CharNameMin, CharNameMax)
	renameLength    = fmt.Sprintf("%%q is not a name: %d to %d letters.", CharNameMin, CharNameMax)
)

// The rename's other lines.
const (
	renameNotWord     = "%q is not a name: one word of letters only."
	renameFiltered    = "That name is not allowed. Refused names are logged for the GMs."
	renameNotName     = "%q is not a name a character may have, nor a pet or a mercenary."
	renameCharacter   = "%s is a character's name: choose another."
	renameMercTaken   = "%s is another mercenary's name: choose another."
	renameIndividual  = "%s is the name of someone in the world: choose another."
	renameSame        = "It is named %s already."
	mercRenamedLine   = "Your mercenary %s is now named %s."
	petRenamedLine    = "Your %s is now named %s. It keeps the name each time you summon it."
	renameLedgerLine  = "The name could not be kept. Try again."
	mercRenameNoSlot  = "You have no mercenary %d."
	mercRenameNoNamed = "You have no mercenary named %s. Yours: %s."
)

// petVerdict is what the name rules make of a name asked: the name as it is
// kept (cased), or the refusal told and its reason logged, filtered whether
// the name filter refused it.
type petVerdict struct {
	name, refusal, reason string
	filtered              bool
}

// petNameCase is a name cased as a character's is by a GM's Change Name: its
// first letter a capital, the rest small.
func petNameCase(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + strings.ToLower(name[1:])
}

// petNameShape holds a name asked to the rules that need nothing but the
// name: one word, letters, its length, the filter's words, the reserved and
// staff names, the character's rules. ok is whether it passed.
func petNameShape(asked string) (v petVerdict, ok bool) {
	asked = strings.TrimSpace(asked)
	if strings.ContainsAny(asked, " \t") {
		return petVerdict{refusal: fmt.Sprintf(renameNotWord, asked), reason: "more than one word"}, false
	}
	for _, c := range asked {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return petVerdict{refusal: fmt.Sprintf(renameNotWord, asked), reason: "not letters only"}, false
		}
	}
	if len(asked) < petRenameMin || len(asked) > petRenameMax {
		return petVerdict{refusal: fmt.Sprintf(renameLength, asked), reason: fmt.Sprintf("%d letters", len(asked))}, false
	}
	if refusedName(asked) {
		return petVerdict{refusal: renameFiltered, reason: "a word the name filter refuses", filtered: true}, false
	}
	if petStaffNames[strings.ToLower(asked)] {
		return petVerdict{refusal: renameFiltered, reason: "a reserved or staff name", filtered: true}, false
	}
	if !validCharName(asked) {
		return petVerdict{refusal: fmt.Sprintf(renameNotName, asked), reason: "a character's name rules"}, false
	}
	return petVerdict{name: petNameCase(asked)}, true
}

// checkName holds a name asked for one of the player's mercs (merc, its slot
// exceptSlot among rows) or its pet (current its name now) to every rule (the
// file's doc).
func (m *Mercs) checkName(p *entity.Player, asked string, merc bool, rows []database.MercRow, exceptSlot int, current string) petVerdict {
	v, ok := petNameShape(asked)
	if !ok {
		return v
	}
	name := v.name
	if strings.EqualFold(name, current) {
		return petVerdict{refusal: fmt.Sprintf(renameSame, current), reason: "its name already"}
	}
	if strings.EqualFold(name, p.Name) {
		return petVerdict{refusal: fmt.Sprintf(renameCharacter, name), reason: "its owner's name"}
	}
	for _, r := range rows {
		if r.Slot != exceptSlot && strings.EqualFold(r.Name, name) {
			return petVerdict{refusal: fmt.Sprintf(renameMercTaken, name), reason: "the player's other mercenary's name"}
		}
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
		if merc {
			elsewhere, err := m.db.MercNamesElsewhere(p.CharServerID)
			if err != nil {
				log.Printf("PetRename: the names held besides %s's: %v", p.Name, err)
				return petVerdict{refusal: renameLedgerLine, reason: "the mercenaries' names could not be read"}
			}
			if _, held := elsewhere[strings.ToLower(name)]; held {
				return petVerdict{refusal: fmt.Sprintf(renameMercTaken, name), reason: "another character's mercenary's name"}
			}
		}
	}
	if m.gw != nil && m.gw.NPCIndividualNamed(name) {
		return petVerdict{refusal: fmt.Sprintf(renameIndividual, name), reason: "a named individual's name"}
	}
	return petVerdict{name: name}
}

// logRename writes a rename asked to petnamelog and the server's log (the
// file's doc).
func (m *Mercs) logRename(sess *session.Session, p *entity.Player, kind, old, asked string, v petVerdict) {
	outcome := database.PetRenamed
	switch {
	case v.filtered:
		outcome = database.PetFiltered
	case v.refusal != "":
		outcome = database.PetRefused
	}
	acct := 0
	if sess != nil {
		acct = sess.AccountID()
	}
	switch outcome {
	case database.PetFiltered:
		log.Printf("PetRename: FILTERED %s (character %d, account %d) asked to name its %s %q %q: %s", p.Name, p.CharServerID, acct, kind, old, asked, v.reason)
	case database.PetRefused:
		log.Printf("PetRename: refused %s (character %d, account %d) naming its %s %q %q: %s", p.Name, p.CharServerID, acct, kind, old, asked, v.reason)
	default:
		log.Printf("PetRename: %s (character %d, account %d) named its %s %q %q", p.Name, p.CharServerID, acct, kind, old, v.name)
	}
	if m.db == nil {
		return
	}
	if err := m.db.LogPetRename(database.PetRenameEntry{ServerID: p.CharServerID, CharName: p.Name, AccountID: acct, Kind: kind,
		OldName: old, Asked: strings.TrimSpace(asked), Outcome: outcome, Reason: v.reason}); err != nil {
		log.Printf("PetRename: %s's rename not logged: %v", p.Name, err)
	}
}

// renameWho is the player as a gm_audit line names a row change of its own
// (the Pet Groomer's form).
func renameWho(sess *session.Session) database.GMEdit {
	who := database.GMEdit{Via: "game"}
	if sess != nil {
		who.AccountID = sess.AccountID()
		if addr := sess.RemoteAddr(); addr != nil {
			who.IP = addr.IP.String()
		}
	}
	return who
}

// mercRenameLine reads a /merc line that renames: the merc named before
// "rename" (its slot's number or its name), or after it when two words follow
// ("rename Jormonis Rennis"), and the name asked. ok false: not a rename.
func mercRenameLine(args string) (pick, asked string, ok bool) {
	f := strings.Fields(args)
	at := -1
	for i, w := range f {
		if strings.EqualFold(w, "rename") {
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
	rest := f[at+1:]
	if pick == "" && len(rest) == 2 {
		pick, rest = rest[0], rest[1:]
	}
	return pick, strings.Join(rest, " "), true
}

// rename is /merc rename: the merc picked (pick its slot or its name, "" the
// one hired) named asked, if the rules let it (the file's doc).
func (m *Mercs) rename(sess *session.Session, p *entity.Player, rows []database.MercRow, pick, asked string) {
	slot := 0
	if pick != "" {
		s, ok := mercPicked(rows, pick)
		if !ok {
			if len(rows) == 0 {
				SendErrorMessage(sess, mercNoneLine)
			} else {
				SendErrorMessage(sess, fmt.Sprintf(mercRenameNoNamed, pick, mercRowNames(rows)))
			}
			return
		}
		slot = s
	}
	if len(rows) == 0 {
		SendErrorMessage(sess, mercNoneLine)
		return
	}
	if slot = m.oneSlot(sess, rows, slot, "rename <new name>"); slot == 0 {
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
	if strings.TrimSpace(asked) == "" {
		SendErrorMessage(sess, mercRenameUsage)
		return
	}
	v := m.checkName(p, asked, true, rows, slot, row.Name)
	if v.refusal == "" && m.db != nil {
		if err := m.db.SetMercNameAudited(p.CharServerID, slot, v.name, renameWho(sess), "/merc rename: "+row.Name+" to "+v.name); err != nil {
			log.Printf("PetRename: %s's merc %q (slot %d) not renamed: %v", p.Name, row.Name, slot, err)
			v = petVerdict{refusal: renameLedgerLine, reason: "its row could not be written: " + err.Error()}
		}
	}
	m.logRename(sess, p, "merc", row.Name, asked, v)
	if v.refusal != "" {
		SendErrorMessage(sess, v.refusal)
		return
	}
	m.gw.RenameMerc(p.CharServerID, slot, v.name) // out or on standby; resting, its row alone
	// its surname follows its new name (mercsurname.go)
	if row.Surname != "" {
		if s := surnamesH.Load(); s != nil {
			s.tell(surnamePair{row.Name, ""}, surnamePair{v.name, row.Surname})
		}
	}
	SendClientMessage(sess, fmt.Sprintf(mercRenamedLine, row.Name, v.name))
}

// petRenameLine reads a /pet line: "/pet ..." or "#pet ...", or a PS2's
// "/t pet rename ..." (a tell to "pet" otherwise), the words after "pet".
func petRenameLine(message string) (rest string, ok bool) {
	word, rest, _ := strings.Cut(strings.TrimSpace(message), " ")
	if strings.EqualFold(word, "/pet") || strings.EqualFold(word, "#pet") {
		return strings.TrimSpace(rest), true
	}
	name, body, tell := parseTell(message)
	if !tell || !strings.EqualFold(name, "pet") {
		return "", false
	}
	if first, _, _ := strings.Cut(strings.TrimSpace(body), " "); !strings.EqualFold(first, "rename") {
		return "", false
	}
	return strings.TrimSpace(body), true
}

// PetCommand is /pet: "rename <name>" names the player's summoned pet (the
// file's doc); anything else, its usage.
func (m *Mercs) PetCommand(sess *session.Session, p *entity.Player, args string) {
	if m == nil || m.gw == nil || p == nil {
		return
	}
	first, asked, _ := strings.Cut(strings.TrimSpace(args), " ")
	if !strings.EqualFold(first, "rename") || strings.TrimSpace(asked) == "" {
		SendClientMessage(sess, petRenameUsage)
		return
	}
	current, summoned := m.gw.PetOutName(sess)
	switch {
	case current == "":
		SendErrorMessage(sess, entity.PetRenameNoPet)
		return
	case !summoned:
		SendErrorMessage(sess, entity.PetRenameCharmed)
		return
	}
	var rows []database.MercRow
	if m.db != nil {
		rows, _ = m.db.Mercs(p.CharServerID)
	}
	v := m.checkName(p, asked, false, rows, 0, current)
	if v.refusal != "" {
		m.logRename(sess, p, "pet", current, asked, v)
		SendErrorMessage(sess, v.refusal)
		return
	}
	old, spell, why := m.gw.RenamePet(sess, v.name, m.now())
	if why != "" {
		SendErrorMessage(sess, why)
		return
	}
	if m.db != nil {
		if err := m.db.SetPetNameAudited(p.CharServerID, spell, v.name, renameWho(sess), fmt.Sprintf("/pet rename: %s (spell %d) to %s", old, spell, v.name)); err != nil {
			log.Printf("PetRename: %s's pet %q (spell %d) named %q for now only: %v", p.Name, old, spell, v.name, err)
		}
	}
	m.logRename(sess, p, "pet", old, asked, v)
	SendClientMessage(sess, fmt.Sprintf(petRenamedLine, old, v.name))
}

// loadPetNames puts a character's pet names (table petnames) on its player
// at a login (and a keep restart's, before its pet is summoned again).
func (h *CharacterHandler) loadPetNames(serverID uint32) {
	if h.gameWorld == nil || h.store == nil || h.store.DB() == nil {
		return
	}
	got, err := h.store.DB().PetNamesOf(serverID)
	if err != nil {
		log.Printf("SelectedChar: character %d's pet names: %v", serverID, err)
		return
	}
	h.gameWorld.SetPetNames(serverID, got)
}
