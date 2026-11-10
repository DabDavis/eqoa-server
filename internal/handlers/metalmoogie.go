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
	// the tribute's second question while Metal Moogie V2 is on the server (the GM Jibrish's petition
	// 1441: "if a player has a token from defeating metal moogie that can instead choose to use it to
	// summon Metal Moogie V2"): its uses, and what he says when it cannot be taken
	moogieTributeQuestion = "Mr. Moogie: 'A tribute! Shall I give you my favor for it, or will you spend it to face my metal twin remade?'"
	moogieFavorOpt        = "I would have your favor"
	moogieV2Opt           = "Summon Metal Moogie V2"
	moogieV2Untaken       = "Mr. Moogie says, 'Your tribute slipped away, %s. My twin waits for another.'"
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

// v2Offered reports whether his box offers p Metal Moogie V2's duel for a tribute: p holds one and V2's
// original is on the server (entity's MetalMoogieV2Here).
func (h *InteractHandler) v2Offered(p *entity.Player) bool {
	if h.gameWorld == nil || !h.gameWorld.MetalMoogieV2Here() {
		return false
	}
	_, ok := h.favor.tribute(p)
	return ok
}

// moogieTributeAnswers is the tribute's second question's answers for p: his favor while the server has
// it, V2's duel, nothing.
func (h *InteractHandler) moogieTributeAnswers(p *entity.Player) moogieChoices {
	var c moogieChoices
	if _, ok := h.favor.favor(); ok {
		c.labels, c.does = append(c.labels, moogieFavorOpt), append(c.does, moogieDoFavor)
	}
	c.labels, c.does = append(c.labels, moogieV2Opt, moogieNoOpt), append(c.does, moogieDoV2, moogieDoNothing)
	return c
}

// moogieV2 is V2's duel asked for a tribute (petition 1441): Metal Moogie V2 rises as Metal Moogie does
// (entity's MetalMoogieV2Challenge, every rule of the duel his), and once he stands one tribute is taken
// from the pack; refused (another's duel, his original gone, no tribute) nothing is taken. A tribute that
// cannot be taken after he rose ends his duel at once, so none is had for nothing.
func (h *InteractHandler) moogieV2(sess *session.Session, p *entity.Player, moogie entity.EntityID) {
	f := h.favor
	r, ok := f.tribute(p)
	if !ok {
		SendClientMessage(sess, moogieNoTribute)
		return
	}
	line, rose := h.gameWorld.MetalMoogieV2Challenge(sess, moogie, time.Now())
	SendClientMessage(sess, line)
	if !rose {
		return
	}
	if !f.take(sess, p, r) {
		h.gameWorld.EndMetalDuelOf(sess, "the tribute could not be taken")
		SendClientMessage(sess, fmt.Sprintf(moogieV2Untaken, p.Name))
		return
	}
	log.Printf("Moogie: %s spends a %s on %s's duel", p.Name, r.Pattern.ItemName, entity.MetalMoogieV2Name)
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
