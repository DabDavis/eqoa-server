package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// guenhwyvar.go — the Stone of Guenhwyvar (the user, 2026-11-07: "give drizzt a
// stone of guenhwyvar summons level 60 panther pet named Guenhwyvar, mean pet").
//
// The stone is an item record of this name, CLIENT's gem type as the fang is
// (wolffang.go: the item menu's Use and the HUD's gem quick slot), never spent.
// Its Use brings the pet of the spell GuenhwyvarSpell, with no cast: a summoning
// spell's pet (entity/pet.go) out of its petstats row (entity/petstats.go: her
// body, her numbers, her spells), so she is any pet to everything after that.
// While she is out the stone does nothing, and after a call she needs
// GuenhwyvarRest before another, so a stone is no heal in a fight. Ours, all of
// it: the live game's records hold no such item.

// The stone's record's name, the spell it calls by, and her rest (ours).
const (
	GuenhwyvarStoneName = "Stone of Guenhwyvar"
	GuenhwyvarSpell     = 760113
	GuenhwyvarRest      = time.Minute
)

// IsGuenhwyvarStone is whether an item record is the stone: worn nowhere and so
// named.
func IsGuenhwyvarStone(p *database.ItemPatternRecord) bool {
	return p != nil && p.EquipSlot == -1 && strings.EqualFold(strings.TrimSpace(p.ItemName), GuenhwyvarStoneName)
}

// useGuenhwyvarStone is the stone's Use: she comes unless she is out or resting,
// and the player is told which.
func (h *EquipHandler) useGuenhwyvarStone(sess *session.Session, player *entity.Player) {
	if h.gameWorld == nil || h.spells == nil {
		return
	}
	sp, err := h.spells.SpellPattern(GuenhwyvarSpell)
	if err != nil || sp == nil {
		log.Printf("Guenhwyvar: the stone's spell %d: %v", GuenhwyvarSpell, err)
		return
	}
	row, _, err := h.spells.SpellEffectRow(GuenhwyvarSpell)
	if err != nil {
		log.Printf("Guenhwyvar: the stone's spell's effect: %v", err)
		return
	}
	spell := castSpellRow(&database.CharSpellEntry{Pattern: *sp}, row)
	now := time.Now()
	h.stoneMu.Lock()
	last := h.stoneUsed[player.CharServerID]
	h.stoneMu.Unlock()
	switch h.gameWorld.SummonByItem(sess, spell, now, last.Add(GuenhwyvarRest)) {
	case entity.ItemSummoned:
		h.stoneMu.Lock()
		if h.stoneUsed == nil {
			h.stoneUsed = make(map[uint32]time.Time)
		}
		h.stoneUsed[player.CharServerID] = now
		h.stoneMu.Unlock()
		SendClientMessage(sess, "Guenhwyvar answers the stone and comes to your side.")
	case entity.ItemPetOut:
		SendClientMessage(sess, "Guenhwyvar is already at your side.")
	case entity.ItemPetResting:
		SendClientMessage(sess, "Guenhwyvar is resting. Call her again in a moment.")
	}
}
