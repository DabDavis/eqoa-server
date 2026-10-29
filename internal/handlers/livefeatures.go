package handlers

import (
	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// SOE's live client's server feature word: 0x07F5, a u32 into VIClient+0x813C
// (live CLIENT 0x012C9548), read by GetFeatures (0x012C79E0) wherever a
// feature the Frontiers disc did not have is drawn or sent. Both retail save
// states hold 0x1B. The disc has no case for 0x07F5 and ignores it; vanilla
// follows the disc today and reads it as it gains the live windows
// (~/.cache/f3pass24/livediff/report.md section 2).
const (
	// LiveFeatureAuctionMenu is the Start menu's Auction entry and the bank
	// menu's Auction option (OpenStartMenu 0x0135BE80, VIWndBankMenu::
	// OnMessage 0x01319498): the live client's reading of what the disc
	// takes from the MemoryDump's bank flag (m_p78CC), which this server
	// sends 1 (writeMemoryDumpTailWith).
	LiveFeatureAuctionMenu uint32 = 0x01
	// LiveFeatureExpansion puts race 9 (Ogre) and class 14 (Alchemist) in
	// the group search's pickers and the item text's race and class masks
	// (0x01314D98, AddRaceMask 0x0130AA80, AddClassMask 0x0130AC38): this
	// server has both.
	LiveFeatureExpansion uint32 = 0x02
	// LiveFeatureAuctionStacks is the auction's stack UI: a quantity in the
	// sell and bid forms, prices per item, CanSell letting stacks through
	// (auction.go and database/auction.go price and sell by the item).
	LiveFeatureAuctionStacks uint32 = 0x08
	// LiveFeatureDuels lets the duel part challenge and surrender
	// (entity/duel.go).
	LiveFeatureDuels uint32 = entity.DuelFeatures
)

// liveFeatures is the word this server sends: every feature it has, ORed
// here and nowhere else.
func liveFeatures() uint32 {
	return LiveFeatureAuctionMenu | LiveFeatureExpansion | LiveFeatureAuctionStacks | LiveFeatureDuels
}

// buildLiveFeatures is 0x07F5 {u32 the word}.
func buildLiveFeatures() []byte {
	return build(protocol.OpcodeLiveFeatures, 6, func(w *buffer.Writer) {
		w.WriteUint32(liveFeatures())
	})
}

// buildAuctionFormOffer is OpcodeAuctionForm from the server: a u32, 1.
func buildAuctionFormOffer() []byte {
	return build(protocol.OpcodeAuctionForm, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendLiveFeatures tells a live client what this server has (0x07F5) and
// offers the live auction forms (OpcodeAuctionForm), once the player is in
// the world. A PS2 ignores both.
func SendLiveFeatures(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeLiveFeatures, buildLiveFeatures())
	queueReliable(sess, protocol.OpcodeAuctionForm, buildAuctionFormOffer())
}
