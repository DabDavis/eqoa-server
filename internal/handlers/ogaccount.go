package handlers

import (
	"log"
	"sync/atomic"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// ogaccount.go — [OG] accounts on a shard (world.og_accounts; the owner,
// 2026-11-18, for the Vanilla Test server: "make a tag for accounts to be
// [OG] Accounts. characters [OG] Meshuggah for example. this [OG] tag shows
// players you opt-out of the QoL changes to EQOA and only use the most basic
// upgrades like FPS, widescreen, no touchscreen, no mouse"; opt-in per
// account, chosen once when the account is made, permanent).
//
// The choice is accountinfo.og (database/ogaccount.go), set at creation on
// the account pages' Register and STATION's Create Account
// (tcp.AuthServer/AccountServer with auth.og_choice), changed only by a GM
// (the panel's set_account_og). Where world.og_accounts is set, at each
// character's entry (and a keep restart's resume) the session of an [OG]
// account:
//
//   - gets world.vanilla_ui's treatment for itself alone: none of the
//     server's quality-of-life offers (QoLOnFor), an answer to any taken as
//     off, /con the game's unknown command (ConLevelOnFor), a resume's kept
//     ones ended (stopKeptQoL);
//   - is never offered item links (itemlinks.go): its lines and the lines it
//     hears are plain names;
//   - is told, a vanilla client (one that answered the bug report offer; a PS2
//     never does, and has none of it anyway), QoLOGOff: every addition off,
//     the map, minimap and item links LOCKED off (QoLLocked, where
//     world.vanilla_ui's are off by default), no mouse (QoLNoMouse) and no
//     touch (QoLNoTouch) in the world. What stays is the client's own frame
//     rate, its window's shape, its graphics and its keyboard and controllers;
//   - has its characters drawn "[OG] Name" to everyone on the shard
//     (entity.OGTag, Player.ShownName), its name its own everywhere else.
//
// Unset (Heirloom's), an [OG] account is any other: nothing read, nothing
// drawn. world.vanilla_ui (every session's) is apart from it and may stay unset.

// ogAccounts is world.og_accounts: off unless set.
var ogAccounts atomic.Bool

// SetOGAccounts turns [OG] accounts on or off for this shard (world.og_accounts).
func SetOGAccounts(on bool) {
	ogAccounts.Store(on)
	entity.SetOGTags(on)
}

// OGAccounts reports world.og_accounts.
func OGAccounts() bool { return ogAccounts.Load() }

// ogStore is where an account's choice is read (database.SQLiteDB).
type ogStore interface {
	AccountOG(accountID int) (bool, error)
}

// accountIsOG is whether the session's account is an [OG] one on this shard:
// false where world.og_accounts is unset, with no account or no store.
func accountIsOG(st ogStore, sess *session.Session) bool {
	if !OGAccounts() || st == nil || sess.AccountID() <= 0 {
		return false
	}
	og, err := st.AccountOG(sess.AccountID())
	if err != nil {
		log.Printf("OG: account %d: %v", sess.AccountID(), err)
		return false
	}
	return og
}

// applyOG reads the session's account's choice at a character's entry and
// gives it to the session (the game's own UI) and its player (the tag).
func (h *CharacterHandler) applyOG(sess *session.Session, player *entity.Player) {
	var st ogStore
	if h.store != nil {
		if db := h.store.DB(); db != nil {
			st = db
		}
	}
	og := accountIsOG(st, sess)
	sess.SetOG(og)
	if player != nil && h.gameWorld != nil {
		h.gameWorld.SetPlayerOG(sess, og)
	}
	if og {
		log.Printf("OG: account %d's '%s' enters with the game's own UI, drawn %q", sess.AccountID(),
			playerName(player), entity.OGTag+playerName(player))
	}
}

func playerName(p *entity.Player) string {
	if p == nil {
		return ""
	}
	return p.Name
}
