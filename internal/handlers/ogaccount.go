package handlers

import (
	"log"
	"sync/atomic"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// ogaccount.go — [OG] characters on a shard (world.og_accounts; the owner,
// 2026-11-18, for the Vanilla Test server: "this [OG] tag shows players you
// opt-out of the QoL changes to EQOA and only use the most basic upgrades like
// FPS, widescreen, no touchscreen, no mouse"; then 2026-11-19: "lets make [OG]
// status a char tag like [HC] the account status is confusing new players").
//
// The choice is the CHARACTER's, characters.og (database/ogchar.go), made once
// while it is new, permanent, as Hardcore's is: on vanilla's create page (the
// 0x7EB0 offer's bit 2, its 'O' block: hardcore.go), else in CLIENT's question
// box at its first world entry (hardcoreask.go, after Hardcore's), changed only
// by a GM (the panel's set_character_og). The account's flag (accountinfo.og)
// was moved onto its characters once (database.MigrateAccountOG, at startup)
// and is not read here. The config key keeps its name, world.og_accounts.
// Where it is set, at each character's entry (and a takeover's, and a keep
// restart's resume) the session of an [OG] character:
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
//   - is drawn "[OG] Name" to everyone on the shard (entity.OGTag,
//     Player.ShownName), its name its own everywhere else.
//
// Unset (Heirloom's), an [OG] character is any other: nothing read, nothing
// drawn, nothing asked. world.vanilla_ui (every session's) is apart from it.

// ogAccounts is world.og_accounts: off unless set.
var ogAccounts atomic.Bool

// SetOGAccounts turns [OG] characters on or off for this shard (world.og_accounts).
func SetOGAccounts(on bool) {
	ogAccounts.Store(on)
	entity.SetOGTags(on)
}

// OGAccounts reports world.og_accounts.
func OGAccounts() bool { return ogAccounts.Load() }

// ogStore is where a character's choice is read (database.SQLiteDB).
type ogStore interface {
	CharacterOG(serverID uint32) (bool, error)
}

// characterIsOG is whether the character is an [OG] one on this shard: false
// where world.og_accounts is unset, with no character or no store.
func characterIsOG(st ogStore, serverID uint32) bool {
	if !OGAccounts() || st == nil || serverID == 0 {
		return false
	}
	og, err := st.CharacterOG(serverID)
	if err != nil {
		log.Printf("OG: character %d: %v", serverID, err)
		return false
	}
	return og
}

func (h *CharacterHandler) ogStore() ogStore {
	if h.store != nil {
		if db := h.store.DB(); db != nil {
			return db
		}
	}
	return nil
}

// readOG reads the entering character's choice into the session alone, before
// its entry fills its lists (its gifts and transfer, by its [OG] sizes:
// itemlists.go's roomCaps); applyOG reads it again with the player.
func (h *CharacterHandler) readOG(sess *session.Session, serverID uint32) {
	sess.SetOG(characterIsOG(h.ogStore(), serverID))
}

// applyOG reads the session's character's choice at its entry and gives it to
// the session (the game's own UI) and its player (the tag).
func (h *CharacterHandler) applyOG(sess *session.Session, player *entity.Player) {
	var id uint32
	if player != nil {
		id = player.CharServerID
	}
	og := characterIsOG(h.ogStore(), id)
	sess.SetOG(og)
	if player != nil && h.gameWorld != nil {
		h.gameWorld.SetPlayerOG(sess, og)
	}
	if og {
		log.Printf("OG: '%s' (account %d) enters with the game's own UI, drawn %q", playerName(player),
			sess.AccountID(), entity.OGTag+playerName(player))
	}
}

// madeOG is a character just made [OG] in the game (the question box): its
// session, its player and its client told at once, as an entry would, and what
// the client had asked for before it ended (stopKeptQoL).
func madeOG(w *entity.World, sess *session.Session) {
	sess.SetOG(OGAccounts())
	if w != nil {
		w.SetPlayerOG(sess, OGAccounts())
	}
	stopKeptQoL(w, sess)
	SendVanillaUI(sess)
}

func playerName(p *entity.Player) string {
	if p == nil {
		return ""
	}
	return p.Name
}
