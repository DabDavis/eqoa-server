package entity

import (
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/session"
)

// petrename.go — a player's names for its pets and mercs, the World's half
// (handlers/petrename.go has the commands and the name rules; the owner,
// 2026-11-25: "let players rename pets and mercs but apply the name filter to
// catch bad actors").
//
// The name is drawn where every name is, the entity channel's 24 bytes at 116
// (statesync.go), made anew each tick, so everyone in view sees it at once;
// the group window is told it by the roster's next pass (group.go's
// sendRosterLocked: its key holds the names), vanilla's merc list within the
// second (uitold.go). What latches a name has to be told again:
//   - the owner's pet bar takes the pet's name once off its channel while its
//     own is empty (claude-notes/pets-charm-client-20261021.md), so it is
//     cleared (0x007F) and told the pet again (0x007E) a second after the
//     rename (petRetellAfter), when the channel already carries the new name;
//   - the non-collide list's entry (0x1003, {name, id}) is taken off and put
//     on again under the new name, the pet's by announcePetLocked, a merc's by
//     mercTellLocked, the next tick.
//
// A summoned pet's name is kept by its summoning spell (Player.PetNames, table
// petnames): every later summoning of that spell comes by it, through a
// logout (the pet itself is not kept over one: it goes when its owner leaves
// the world) and a keep restart (keeppet.go summons it again by its spell). A
// charmed creature is a creature of the world (its row's, its name what
// quests and its kin know it by) and keeps its own. A merc's name is its
// row's (charmerc.name), so its comings, its standby and a keep restart
// (its kit) all carry it.

// petRetellAfter is how long after a rename the owner's pet bar is told the
// pet again: past the channel's carrying the new name.
const petRetellAfter = time.Second

// madeAs is the name the pet was made under (petName's form), whatever its
// owner named it: a charmed creature's, its own.
func (ps *petState) madeAs() string {
	if ps.kindName != "" {
		return ps.kindName
	}
	return petName(ps.npc.Name)
}

// SetPetNames puts a character's pet names (table petnames), by summoning
// spell, on its player where it is in the world. It reports the player's
// name and whether it was in the world.
func (w *World) SetPetNames(charID uint32, names map[int]string) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	if p == nil {
		return "", false
	}
	p.PetNames = make(map[int]string, len(names))
	for k, v := range names {
		if v = strings.TrimSpace(v); v != "" {
			p.PetNames[k] = v
		}
	}
	return p.Name, true
}

// The refusals of a pet's rename, as the player is told them.
const (
	PetRenameNoPet   = "You have no pet to name."
	PetRenameCharmed = "A charmed creature keeps its own name: only a pet you summon takes one."
	PetRenameNotHere = "You must be in the world to name your pet."
)

// RenamePet names the session's player's pet, alive and summoned, at once
// (the file's doc): its name, the player's name for its spell's pets, its
// pet bar told again after petRetellAfter. It returns its name before and its
// summoning spell, or the refusal.
func (w *World) RenamePet(sess *session.Session, name string, now time.Time) (old string, spell int, refusal string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || !sess.CharacterInWorld() {
		return "", 0, PetRenameNotHere
	}
	ps := w.petStateLocked(p)
	if ps == nil || ps.npc == nil || ps.npc.Dead {
		return "", 0, PetRenameNoPet
	}
	if ps.npc.Charm != nil || ps.spell == 0 {
		return ps.npc.Name, 0, PetRenameCharmed
	}
	old = ps.npc.Name
	ps.npc.Name, ps.renamed = name, true
	if p.PetNames == nil {
		p.PetNames = map[int]string{}
	}
	p.PetNames[ps.spell] = name
	ps.retellAt = now.Add(petRetellAfter)
	log.Printf("Pet: %s's %q (spell %d) is named %q", p.Name, old, ps.spell, name)
	return old, ps.spell, ""
}

// PetOutName is the name of the session's player's pet out as it is drawn,
// and whether it may be named (a summoned one): "" for none.
func (w *World) PetOutName(sess *session.Session) (name string, summoned bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	ps := w.petStateLocked(w.sessionPlayerLocked(sess))
	if ps == nil || ps.npc == nil || ps.npc.Dead {
		return "", false
	}
	return ps.npc.Name, ps.npc.Charm == nil && ps.spell != 0
}

// retellPetLocked tells the owner's client its pet again, so its pet bar
// takes the pet's new name: 0x007F (the bar and its name cleared), then the
// pet's 0x007E and its non-collide entry under the new name from
// announcePetLocked, this tick. Must be called with the lock held.
func (w *World) retellPetLocked(ps *petState, sess *session.Session) {
	ps.retellAt = time.Time{}
	if sess == nil {
		return
	}
	sendClearPet(sess)
	ps.told = 0
	if ps.noCollide {
		sendNoCollideRemove(sess, petNoCollideID)
		ps.noCollide = false
	}
}

// RenameMerc names the character's merc of the slot anew wherever it is:
// out (its NPC, its body and its kit, so a level's new kit keeps it; its
// owner's non-collide entry put on again under it), or on standby. It
// reports its name before and whether it was found. Its row is the
// handlers' (charmerc.name).
func (w *World) RenameMerc(charID uint32, slot int, name string) (old string, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(charID)
	if p == nil {
		return "", false
	}
	if ms := w.mercAtLocked(p, slot); ms != nil {
		old = ms.npc.Name
		ms.npc.Name, ms.kit.Name = name, name
		if ms.body != nil {
			ms.body.Name = name
		}
		if ms.noCollide {
			if sess := w.sessionOfLocked(p); sess != nil {
				sendNoCollideRemove(sess, mercNoCollideIDOf(ms.slot))
			}
			ms.noCollide = false // put on again by mercSystem's mercTellLocked
		}
		ok = true
	}
	for _, b := range w.mercBench[p.ServerID] {
		if b.owner == p && b.slot == slot {
			if !ok {
				old = b.name
			}
			b.name, ok = name, true
		}
	}
	if ok {
		log.Printf("Merc: %s's %q (slot %d) is named %q", p.Name, old, slot, name)
	}
	return old, ok
}

// NPCIndividualNamed is whether a creature of the world (no one's pet, no
// merc) has the name, any case, as a person's name is written: its first
// letter a capital (claude-notes/npc-name-case-20261030.md: generic creatures
// are in lower case, individuals as SOE wrote them). A pet or a merc may not
// take a named individual's name.
func (w *World) NPCIndividualNamed(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, n := range w.NPCs {
		if n == nil || n.Owner != 0 || n.isMerc() || len(n.Name) != len(name) {
			continue
		}
		if r := []rune(n.Name); len(r) > 0 && unicode.IsUpper(r[0]) && strings.EqualFold(n.Name, name) {
			return true
		}
	}
	return false
}
