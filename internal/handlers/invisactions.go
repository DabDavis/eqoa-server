package handlers

import (
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// invisActions are the client's messages that are its player's own action
// (F3 20261026-014527, jib: "when using a potion while invis'd, the invis
// should break. Effectively any action other than movement, targeting and
// chat should break invis"): the Registry ends the sender's invisibility
// before each is handled (SetInvisBreaker, World.BreakInvis), the reason in
// the log. Casting and attacking end it where they begin, as before (a cast
// refused or an attack on nothing ends nothing: entity/invis.go).
//
// Not actions: moving (the 0x40 channel), targeting (0x0010, the classic
// target info, the assists), chat and its channels, filters and modes, the
// windows of the player's own (the who list, factions, masteries, training
// points, the spell book's bar, quick slots, the quest journal, a weapon
// set's name), the group and the buddy and ignore lists, closing a window,
// and the pregame. A pet's invisibility ends when it fights (entity/pet.go),
// never by its owner's action.
var invisActions = map[protocol.GameOpcode]string{
	protocol.OpcodeInteractItem:      "an item used", // drunk, eaten, scribed, thrown, worn or taken off
	protocol.OpcodeUseWeaponSet:      "a weapon set taken up",
	protocol.OpcodeInteract:          "talking to someone", // an NPC, a corpse, a coach...
	protocol.OpcodeDialogueBoxOption: "answering someone",
	protocol.OpcodeMerchantDiag:      "a merchant",
	protocol.OpcodeMerchantBuy:       "buying",
	protocol.OpcodeMerchantSell:      "selling",
	protocol.OpcodeBankUI:            "the bank",
	protocol.OpcodeBankItem:          "the bank",
	protocol.OpcodeDepositBankTunar:  "the bank",
	protocol.OpcodeBlackSmithMenu:    "a blacksmith",
	protocol.OpcodeRequestRepair:     "a repair",
	protocol.OpcodeAuctionRequest:    "the auction",
	protocol.OpcodeTradeRequest:      "a trade",
	protocol.OpcodeTradeUI:           "a trade",
	protocol.OpcodeLootCorpse:        "looting",
	protocol.OpcodeClientLoot:        "looting",
	protocol.OpcodeLootTakeAll:       "looting",
	protocol.OpcodePetCommands:       "a pet command",
	protocol.OpcodeEmoteWave:         "an emote",
	protocol.OpcodeEmoteBow:          "an emote",
	protocol.OpcodeEmotePoint:        "an emote",
	protocol.OpcodeEmoteCheer:        "an emote",
}

// InvisBreaker ends a session's player's invisibility, the reason for the
// log (entity's World.BreakInvis).
type InvisBreaker func(sess *session.Session, why string)

// SetInvisBreaker makes every action (invisActions) end its sender's
// invisibility before it is handled.
func (r *Registry) SetInvisBreaker(fn InvisBreaker) {
	r.mu.Lock()
	r.invis = fn
	r.mu.Unlock()
}

// breakInvisFor ends the sender's invisibility when the message is an action.
func (r *Registry) breakInvisFor(sess *session.Session, op protocol.GameOpcode) {
	why, ok := invisActions[op]
	if !ok {
		return
	}
	r.mu.RLock()
	fn := r.invis
	r.mu.RUnlock()
	if fn != nil && sess != nil {
		fn(sess, why)
	}
}
