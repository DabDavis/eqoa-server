package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// metalmoogie.go — Mr. Moogie's challenge and his favor, as his talk offers them (moogie.go; the GM
// Jibrish's petitions 1379 and 1380, by Jibcro): "I'd like to prove my worth" stands a Metal Moogie up
// to duel the player (entity/metalmoogie.go), and a Moogie Tribute, the one item Metal Moogie's corpse
// gives, handed back to Mr. Moogie lands his Duelist's Favor on its giver: half again as big for an hour
// or until death (the spell's record, "Self, Increase Size by 50%, 60min": entity's EffectSize). The
// item and the spell are the data's, found by their names (entity.MoogieTributeName,
// entity.DuelistsFavorName); without the spell his box offers no hand-in, and the tribute stays in the
// pack. The words are ours.

// The turn-in's words: the answer in his box, and what he says.
const (
	moogieProveOpt   = "I'd like to prove my worth" // Jibcro's words
	moogieTributeOpt = "I bring you a Moogie Tribute"
	moogieFavored    = "Mr. Moogie says, 'A worthy duelist at last! Wear my favor proudly, %s, and stand tall.'"
	moogieNoFavor    = "Mr. Moogie says, 'I have no favor to give today. Keep your tribute.'"
	moogieNoTribute  = "Mr. Moogie says, 'You carry no tribute of mine.'"
)

// MoogieFavor is what the turn-in needs: the pack, and the spell records.
type MoogieFavor struct {
	inv    *database.InventoryStore
	spells *database.SpellbookStore
}

// NewMoogieFavor is the turn-in over the pack and the spell records.
func NewMoogieFavor(inv *database.InventoryStore, spells *database.SpellbookStore) *MoogieFavor {
	if inv == nil || spells == nil {
		return nil
	}
	return &MoogieFavor{inv: inv, spells: spells}
}

// tribute is the player's first Moogie Tribute in the pack (not banked, not worn), ok false for none.
func (f *MoogieFavor) tribute(p *entity.Player) (database.CharInventoryEntry, bool) {
	if f == nil || p == nil {
		return database.CharInventoryEntry{}, false
	}
	rows, err := f.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Moogie: %s's inventory: %v", p.Name, err)
		return database.CharInventoryEntry{}, false
	}
	for _, r := range rows {
		if r.Location == 0 && r.EquipLoc < 0 && r.StackLeft > 0 &&
			strings.EqualFold(strings.TrimSpace(r.Pattern.ItemName), entity.MoogieTributeName) {
			return r, true
		}
	}
	return database.CharInventoryEntry{}, false
}

// favor is the Duelist's Favor's record as the World lands it, ok false when the server has none.
func (f *MoogieFavor) favor() (entity.CastSpell, bool) {
	if f == nil {
		return entity.CastSpell{}, false
	}
	id, ok, err := f.spells.SpellIDByName(entity.DuelistsFavorName)
	if err != nil || !ok {
		if err != nil {
			log.Printf("Moogie: %s: %v", entity.DuelistsFavorName, err)
		}
		return entity.CastSpell{}, false
	}
	sp, err := f.spells.SpellPattern(id)
	if err != nil || sp == nil {
		log.Printf("Moogie: %s (%d): %v", entity.DuelistsFavorName, id, err)
		return entity.CastSpell{}, false
	}
	row, found, err := f.spells.SpellEffectRow(id)
	if err != nil {
		log.Printf("Moogie: %s's effects (%d): %v", entity.DuelistsFavorName, id, err)
		return entity.CastSpell{}, false
	}
	if !found {
		row = database.SpellEffectRow{}
	}
	return entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX), true
}

// offers reports whether his box offers the hand-in: the player holds a tribute and the favor is there.
func (f *MoogieFavor) offers(p *entity.Player) bool {
	if _, ok := f.tribute(p); !ok {
		return false
	}
	_, ok := f.favor()
	return ok
}

// take takes one tribute out of the player's pack, kept and shown (0x003D, as a quest's hand-in takes
// an item). It reports whether one went.
func (f *MoogieFavor) take(sess *session.Session, p *entity.Player, r database.CharInventoryEntry) bool {
	var err error
	if r.StackLeft > 1 {
		err = f.inv.SetItemStack(p.CharServerID, r.ItemID, r.StackLeft-1)
	} else {
		err = f.inv.DeleteCharItem(p.CharServerID, r.ItemID)
	}
	if err != nil {
		log.Printf("Moogie: taking %s's %q: %v", p.Name, r.Pattern.ItemName, err)
		return false
	}
	if l := sessionInvList(sess); l != nil {
		if idx := l.indexOfItem(r.ItemID); idx >= 0 && l.took(idx, 1) {
			queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
		}
	}
	return true
}

// handIn is the hand-in answered: the tribute taken and the favor landed, else said why not.
func (h *InteractHandler) moogieHandIn(sess *session.Session, p *entity.Player) {
	f := h.favor
	r, ok := f.tribute(p)
	if !ok {
		SendClientMessage(sess, moogieNoTribute)
		return
	}
	spell, ok := f.favor()
	if !ok {
		SendClientMessage(sess, moogieNoFavor)
		return
	}
	if !f.take(sess, p, r) {
		SendClientMessage(sess, moogieNoFavor)
		return
	}
	if !h.gameWorld.GiveDuelistsFavor(sess, spell, time.Now()) {
		log.Printf("Moogie: %s's tribute taken, but %q landed nothing", p.Name, spell.Name)
	}
	log.Printf("Moogie: %s hands Mr. Moogie a %s and has his %s", p.Name, r.Pattern.ItemName, spell.Name)
	SendClientMessage(sess, fmt.Sprintf(moogieFavored, p.Name))
}

// metalTalk is a talk to a Metal Moogie standing (another's: the challenger's client fights him, never
// talks), answered in a line; false for any other NPC.
func (h *InteractHandler) metalTalk(sess *session.Session, npc *entity.NPC) bool {
	if h.gameWorld == nil {
		return false
	}
	line, ok := h.gameWorld.MetalMoogieTalk(npc)
	if !ok {
		return false
	}
	SendColoredChat(sess, line, 255, 64, 64)
	return true
}
