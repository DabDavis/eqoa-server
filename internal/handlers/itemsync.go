package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/session"
)

// itemsync.go — the server's copies of a client's item lists (itemlist.go's
// invList) kept in step with the client's, and set right when they are not.
//
// The client applies the server's messages in the order they come; the server's
// copy of each list must change in that same order, each change with its own
// message, or a key, a list index or a record's place means one thing to the
// client and another here. A client's packets are handled each in a goroutine of
// its own and the World's item hooks in theirs, so every one of them holds the
// session's item lists (session.LockItems) while it works: the UDP listener's
// dispatch for every message, withItems for what runs outside it.
//
// Live petition 5401 (2026-11-21, Smoke, a vampire Shadow Knight at the kappa
// camps): "the lvl 5 cloth pants you cant delete or sell them out of your
// inventory". His client asked for keys 88 to 99 the server's copy did not hold
// (DestroyItem from 07:52:11, MerchantSell at 08:00), after half an hour in which
// his pack changed every few seconds from three sides at once: Feed's Unit of
// Blood every three seconds (entity/potion.go's summon hook, a goroutine of its
// own), seven Units spent at each Vampiric Strike (the cast's packet), a Take
// all on a kappa every fifteen seconds and bursts of Destroys a second apart
// (each its own packet). Two of them interleaving was enough: a Destroy's list
// index looked up, a record before it taken by another, the record after it
// taken; or a new record filed here before a Take all's but queued after its
// 0x001A. The same misses were in the logs before (Live 11-20, Smoke; Heirloom
// 11-18 to 11-20, four players), so build386 (2026-11-21) did not bring it.

// withItems runs fn under the session's item lists' hold: for what changes them
// outside a message's handler (the World's hooks, the GM panel), never inside
// one (the hold is not reentrant).
func withItems(sess *session.Session, fn func()) {
	if sess == nil {
		fn()
		return
	}
	sess.LockItems()
	defer sess.UnlockItems()
	fn()
}

// itemsResyncEvery is how often a session's item lists may be sent again
// (resyncItems): a client out of step whose requests keep missing is not sent a
// dump at each one.
const itemsResyncEvery = 30 * time.Second

// itemsResyncLine is what the player whose lists were sent again is told.
const itemsResyncLine = "Your inventory was out of step with the server and has been sent again. Try that once more."

// resyncItems sets a session's item lists right after a request named a key
// the server's copy does not hold: the client holds a record this server
// cannot name, so no request of it can reach it, and the only cure CLIENT has is
// a MemoryDump, which fills both lists afresh, its own and the server's copy
// (key = position, from the database: SendMemoryDump). It is sent where the
// character stands, as a restart's resume sends one (ResumeDump), and the
// player told. Not while a trade is open (its window would stand on a list it
// no longer has: the next miss after it ends); a corpse's window is closed
// first, as the resume closes it; a dead character is left to its respawn's
// dump. At most once every itemsResyncEvery. Called
// under the session's item lists' hold (a handler's). It reports whether the
// lists were sent.
func resyncItems(sess *session.Session, who, why string) bool {
	if sess == nil || !sess.CharacterInWorld() {
		return false
	}
	// a dead character's respawn sends it its dump
	if h := keepChars.Load(); h == nil || h.gameWorld == nil {
		return false
	} else if p := h.gameWorld.SessionPlayer(sess); p == nil {
		return false
	} else if snap, ok := h.gameWorld.PlayerSnapshot(p.ServerID); !ok || snap.IsDead() {
		return false
	}
	if h := keepTrades.Load(); h != nil {
		h.mu.Lock()
		trading := h.trades[sess] != nil
		h.mu.Unlock()
		if trading {
			log.Printf("ItemLists: %s's lists are out of step (%s), not sent again during a trade", who, why)
			return false
		}
	}
	if !sess.TakeItemsResync(time.Now(), itemsResyncEvery) {
		return false
	}
	if h := keepWindows.Load(); h != nil {
		ls := h.loots()
		ls.mu.Lock()
		_, open := ls.open[sess]
		ls.mu.Unlock()
		if open {
			h.closeLoot(sess)
		}
	}
	if !dumpWhereItStands(sess, false) {
		return false
	}
	SendClientMessage(sess, itemsResyncLine)
	log.Printf("ItemLists: %s's lists sent again where it stands: %s", who, why)
	return true
}
