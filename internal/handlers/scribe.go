package handlers

import (
	"log"
	"regexp"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Scribing: the item menu's Use on a spell scroll (UseItem(i, 0), the 0x003E
// equip.go takes for everything else) puts the scroll's spell in the
// character's book, and the scroll is gone.
//
// What Sandstorm does, recorded with eqoa harvest on 2026-10-21 (a druid of
// level 23 buying at two of Freeport's spell vendors, then using each scroll):
//
//   - learned: 0x003C with the scroll's list index, sound 0 and one, then
//     0x00B3 with stamp 0 and flag 1 (word 721, "You have gained a new
//     ability.") and the ability at the next display index, off the bar
//     (valid 0, slot -1, lockout 0, VIAbility+0x14 -1);
//   - refused, the scroll kept: one line of 0x0A7B in (255, 63, 63), "This
//     item can't be used by your class." for a cleric's scroll, "You are not
//     high enough level to equip that item." for one of level 24, "You
//     already know this spell." for one in the book.
//
// The checks run in VIPlayer::CanUse's order (class, race, level), so what
// the server refuses is what the client already shows as unusable. Two
// lines are not Sandstorm's: the race one (Sandstorm's scroll masks name the
// races that can be the class, so no character of the class is refused by
// race), and the one for a scroll whose spell has no record here (Sandstorm
// has one for every scroll it sells).

// scrollIcon is the art every spell scroll carries (itempattern.itemicon):
// all 508 of Sandstorm's vendor scrolls show it, and Return Home's. A scroll
// names its spell only by its own name, so the icon is what keeps a potion
// of a spell's name ("Invisibility") from being scribed.
const scrollIcon = -607649328

// The refusals, in Sandstorm's colour.
const (
	scribeWrongClass = "This item can't be used by your class."
	scribeWrongRace  = "This item can't be used by your race."
	scribeTooLow     = "You are not high enough level to equip that item."
	scribeKnown      = "You already know this spell."
	scribeNoSpell    = "The spell on this scroll cannot be learned here yet."
)

var scribeRefusalRGB = [3]uint8{255, 63, 63}

// isScroll is whether an item is a spell scroll: the scroll's art on
// something not worn.
func isScroll(p *database.ItemPatternRecord) bool {
	return int32(p.ItemIcon) == scrollIcon && p.EquipSlot == -1
}

// classTag is a scroll name's trailing class tag: "Root (SHA/DRD)",
// "Invisibility (SHA, WIZ)".
var classTag = regexp.MustCompile(`\s*\((?:[A-Za-z]{2,3}(?:\s*[,/]\s*)?)+\)\s*$`)

// spellKey is the name a scroll and its spell share, compared as
// tools/import_sandstorm_harvest.py's spell_name compares them: no '$' mark,
// no class tag, no bard '+', a backtick read as an apostrophe, any case.
func spellKey(name string) string {
	n := strings.TrimSpace(name)
	n = strings.TrimSpace(strings.TrimSuffix(n, "$"))
	n = classTag.ReplaceAllString(n, "")
	n = strings.ReplaceAll(n, "+", "")
	n = strings.ReplaceAll(n, "`", "'")
	return strings.ToLower(strings.TrimSpace(n))
}

// scrollSpell is the spell a scroll teaches, out of every pattern in id
// order: the first of its name at the scroll's level, else the first of its
// name. One name twice is mostly one spell numbered twice, Return Home's and
// Sandstorm's (Provoke is 674 and 1607, the same record).
func scrollSpell(spells []database.SpellName, scroll string, level int) (database.SpellName, bool) {
	key := spellKey(scroll)
	first := -1
	for i, s := range spells {
		if spellKey(s.Name) != key {
			continue
		}
		if s.Level == level {
			return s, true
		}
		if first < 0 {
			first = i
		}
	}
	if first < 0 {
		return database.SpellName{}, false
	}
	return spells[first], true
}

// nextDisplayIndex is where a new ability goes in the book: after the last
// one shown (display indices from 252 up are not).
func nextDisplayIndex(book []database.CharSpellEntry) int {
	next := 0
	for _, e := range book {
		if e.AddedOrder < 252 && e.AddedOrder >= next {
			next = e.AddedOrder + 1
		}
	}
	return next
}

// scribe answers a Use of the scroll at list index idx (row): refused with a
// line and the scroll kept, or the spell added and the scroll taken.
func (h *EquipHandler) scribe(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry) {
	p := &row.Pattern
	refuse := func(text string) {
		log.Printf("Scribe: %s, %q: %s", player.Name, p.ItemName, text)
		SendColoredChat(sess, text, scribeRefusalRGB[0], scribeRefusalRGB[1], scribeRefusalRGB[2])
	}
	switch {
	case p.ClassUse&(1<<uint(player.Class)) == 0:
		refuse(scribeWrongClass)
		return
	case raceUseForWire(p.RaceUse)&(1<<uint(player.Race)) == 0:
		refuse(scribeWrongRace)
		return
	case player.Level < p.LevelReq:
		refuse(scribeTooLow)
		return
	}
	if h.spells == nil {
		log.Printf("Scribe: %s, %q: no spell store", player.Name, p.ItemName)
		refuse(scribeNoSpell)
		return
	}
	names, err := h.spells.SpellNames()
	if err != nil {
		log.Printf("Scribe: %s, %q: spell names: %v", player.Name, p.ItemName, err)
		return
	}
	spell, ok := scrollSpell(names, p.ItemName, p.LevelReq)
	if !ok {
		refuse(scribeNoSpell)
		return
	}
	book, err := h.spells.GetCharacterSpells(player.CharServerID)
	if err != nil {
		log.Printf("Scribe: %s, %q: the book: %v", player.Name, p.ItemName, err)
		return
	}
	key := spellKey(spell.Name)
	for _, e := range book {
		if e.Pattern.SpellID == spell.SpellID || spellKey(e.Pattern.SpellName) == key {
			refuse(scribeKnown)
			return
		}
	}

	// The spell first: should taking the scroll then fail, the player keeps
	// a scroll of a spell known, rather than losing one for nothing.
	order := nextDisplayIndex(book)
	if err := h.spells.AddSpell(player.CharServerID, spell.SpellID, order); err != nil {
		log.Printf("Scribe: %s, %q: adding spell %d: %v", player.Name, p.ItemName, spell.SpellID, err)
		return
	}
	if err := h.takeOne(sess, player, idx, row); err != nil {
		log.Printf("Scribe: %s, %q: taking the scroll: %v (the spell stays learned)", player.Name, p.ItemName, err)
	} else {
		queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
	}
	book, err = h.spells.GetCharacterSpells(player.CharServerID)
	if err != nil {
		log.Printf("Scribe: %s, %q: the book after: %v", player.Name, p.ItemName, err)
		return
	}
	for i := range book {
		if book[i].Pattern.SpellID == spell.SpellID {
			log.Printf("Scribe: %s learned %q (spell %d, display index %d) from %q",
				player.Name, spell.Name, spell.SpellID, order, p.ItemName)
			SendAbilityAdd(sess, 0, true, &book[i])
			return
		}
	}
	log.Printf("Scribe: %s, %q: spell %d was added and is not in the book", player.Name, p.ItemName, spell.SpellID)
}

// takeOne takes one of the record at list index idx, in the database and in
// the client's list (VIItemList::Took: the record leaves when none is left).
func (h *EquipHandler) takeOne(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry) error {
	var err error
	if row.StackLeft <= 1 {
		err = h.inventoryStore.DeleteCharItem(player.CharServerID, row.ItemID)
	} else {
		err = h.inventoryStore.SetItemStack(player.CharServerID, row.ItemID, row.StackLeft-1)
	}
	if err != nil {
		return err
	}
	if l := sessionInvList(sess); l != nil {
		l.took(idx, 1)
	}
	return nil
}
