package handlers

import (
	"errors"
	"fmt"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// hcrules.go — what a Hardcore character may not use (the owner, 2026-11-18:
// "so HC players aren't helped (can't be power-levelled, can't have high levels
// kill quest mobs for them)", "hardcore chars cant use the teleporter", "cant
// hire mercs either", "cant use the auction house either or trade at all even
// with other HC players"). Where world.hardcore is on, a Hardcore character
// (entity.Player.HardcoreBound):
//
//   - trades with no one, Hardcore or not, either way (trade.go: the ask
//     refused, both told);
//   - uses no part of the auction house: no listing, bid, buy or search
//     (auction.go's Handle refuses every request);
//   - never shares the account bank: its bank is its own, rows and tunar
//     (database/accountbank.go's hcBankSplit, turned on here); mail carries
//     no items or tunar on this server (grams are text), so nothing to refuse;
//   - is not carried by the server's own conveniences: the Teleporter, the
//     Summoning Portal (opening one, being called through one, a portal's trip
//     to Freeport), the Rock of Freeport, the players' homes (the Real Estate
//     Listings board and /home), the account pages' teleport; nor is it given
//     levels or mastery points by Dabrish. The game's own travel stays: the
//     coaches, the waysenders' routes, the gate and port spells, Return Home
//     and /stuck (home as Return Home sends it), the orbs, a raid's portals and
//     the portals GMs mark where the live game had teleporters;
//   - hires no mercenary (entity.HireMercAt, RestoreMerc: refused; one it had
//     is not brought back at its login).
//
// The words are ours.

func init() {
	database.SetHardcoreBankCheck(entity.HardcoreOn)
}

// The refusals' words (ours).
const (
	hcNoTradeWords       = "A Hardcore adventurer trades with no one."
	hcNoTradeThemWords   = "%s is a Hardcore adventurer and trades with no one."
	hcNoAuctionWords     = "A Hardcore adventurer cannot use the auction house."
	hcTeleporterWords    = "The Teleporter will not carry a Hardcore adventurer."
	hcPortalWords        = "No portal carries a Hardcore adventurer."
	hcPortalTargetWords  = "%s is a Hardcore adventurer: no portal carries them."
	hcRockWords          = "The Rock of Freeport will not carry a Hardcore adventurer."
	hcHomesWords         = "A Hardcore adventurer makes no home here: the road is their home."
	hcDabrishWords       = "I keep no ledger for a Hardcore adventurer. Every level you earn yourself."
	hcArchivistWords     = "I copy no scrolls for a Hardcore adventurer: every spell you learn, you find yourself."
	hcMercLiaisonWords   = "I hire no one out to a Hardcore adventurer: you fight without mercenaries."
	hcWebTeleportRefusal = "a Hardcore character cannot be teleported from here"
)

// hardcoreNoTrade reports a trade asked of or by a Hardcore character, refused:
// the asker told.
func hardcoreNoTrade(sess *session.Session, me, them *entity.Player) bool {
	switch {
	case me.HardcoreBound():
		SendErrorMessage(sess, hcNoTradeWords)
	case them.HardcoreBound():
		SendErrorMessage(sess, fmt.Sprintf(hcNoTradeThemWords, them.Name))
	default:
		return false
	}
	log.Printf("Hardcore: a trade of %s with %s refused", me.Name, them.Name)
	return true
}

// hardcoreNoAuction reports an auction request of a Hardcore character,
// refused and told.
func hardcoreNoAuction(sess *session.Session, p *entity.Player) bool {
	if !p.HardcoreBound() {
		return false
	}
	auctionLine(sess, hcNoAuctionWords)
	return true
}

// hcSay says a refusal in red.
func hcSay(sess *session.Session, line string) { SendErrorMessage(sess, line) }

// HardcoreWebTeleport refuses the account pages' teleport of a Hardcore
// character (on a shard with world.hardcore); nil for any other.
func HardcoreWebTeleport(db *database.SQLiteDB, serverID uint32) error {
	if db == nil || !entity.HardcoreOn() {
		return nil
	}
	st, err := db.HardcoreOf(serverID)
	if err != nil || !st.Hardcore {
		return nil
	}
	return errors.New(hcWebTeleportRefusal)
}
