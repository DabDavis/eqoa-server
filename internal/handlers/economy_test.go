package handlers

import (
	"testing"

	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
)

// econOf is what economy.Default holds for a character and a source, not yet
// written: what it got and what it paid.
func econOf(id uint32, src economy.Source) (got, paid int64) {
	_, chars := economy.Default.Pending()
	for _, r := range chars {
		if r.ServerID == id && r.Source == string(src) {
			got += r.Got
			paid += r.Paid
		}
	}
	return got, paid
}

// A corpse's coins count as made, the opener's part as loot and each
// member's share as a group share.
func TestCorpseCoinsAreCounted(t *testing.T) {
	c := newChatWorld(t)
	sa, pa := c.join(901, "Opal", 100, 100, 0)
	sb, pb := c.join(902, "Pell", 102, 100, 0)
	c.group(t, sa, pa, sb, pb)
	la, _ := econOf(pa.CharServerID, economy.Loot)
	lb, _ := econOf(pb.CharServerID, economy.LootSplit)
	h := &NPCWindowHandler{gameWorld: c.gw, playerOf: c.gw.SessionPlayer}
	h.giveCorpseCoins(sa, &entity.NPC{ServerID: 5900, Name: "beetle", X: 100, Z: 100, Level: 10}, 11)
	if got, _ := econOf(pa.CharServerID, economy.Loot); got-la != 6 {
		t.Errorf("the opener's loot counted %d, want 6", got-la)
	}
	if got, _ := econOf(pb.CharServerID, economy.LootSplit); got-lb != 5 {
		t.Errorf("the member's share counted %d, want 5", got-lb)
	}
}

// The auction house: the listing fee a sink, a sale between players a
// transfer from the winner to the seller, at its price.
func TestAuctionTunarIsCounted(t *testing.T) {
	f := newHouseFixture(t, "Seller", "Buyer")
	s, c := f.chars["Seller"], f.chars["Buyer"]
	_, fee0 := econOf(s.player.CharServerID, economy.AuctionFee)
	got0, _ := econOf(s.player.CharServerID, economy.AuctionSale)
	_, paid0 := econOf(c.player.CharServerID, economy.AuctionSale)
	f.give(s, f.sword, 1)
	f.sell(s, 0, 100, 900, 720)
	f.drain(s)
	f.bid(c, 1, 900)
	f.drain(c)
	f.drain(s)
	if _, fee := econOf(s.player.CharServerID, economy.AuctionFee); fee-fee0 != 1 {
		t.Errorf("the listing fee counted %d, want 1", fee-fee0)
	}
	if got, _ := econOf(s.player.CharServerID, economy.AuctionSale); got-got0 != 900 {
		t.Errorf("the seller's sale counted %d, want 900", got-got0)
	}
	if _, paid := econOf(c.player.CharServerID, economy.AuctionSale); paid-paid0 != 900 {
		t.Errorf("the buyer's payment counted %d, want 900", paid-paid0)
	}
}
