package handlers

import (
	"errors"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What the GM panel (internal/gmweb) says and does to players through the
// game, from an HTTP goroutine: every send goes through a session's queue,
// which takes its own lock, and every change to a Player through the World's
// methods, which take the World's.

// SessionOfCharacter is the in-world session playing a character (its
// serverid), or nil.
func SessionOfCharacter(gw *entity.World, sm *session.Manager, serverID uint32) *session.Session {
	if gw == nil || sm == nil {
		return nil
	}
	var found *session.Session
	sm.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() {
			return true
		}
		p := gw.SessionPlayer(sess)
		if p == nil || p.CharServerID != serverID || !gw.IsLivePlayer(p) {
			return true
		}
		found = sess
		return false
	})
	return found
}

// GMLine sends one line to a player's chat window in the colour given, as a
// GM's words to that player.
func GMLine(sess *session.Session, text string, rgb [3]byte) {
	SendColoredChat(sess, text, rgb[0], rgb[1], rgb[2])
}

// Broadcast sends one line to every player in the world, in the colour given,
// and returns how many it went to.
func Broadcast(sm *session.Manager, text string, rgb [3]byte) int {
	if sm == nil {
		return 0
	}
	var to []*session.Session
	sm.ForEach(func(sess *session.Session) bool {
		if !sess.CharacterInWorld() {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() {
			return true
		}
		to = append(to, sess)
		return true
	})
	for _, sess := range to {
		GMLine(sess, text, rgb)
	}
	log.Printf("GM broadcast to %d players: %s", len(to), text)
	return len(to)
}

// Eject ends a session as a server administrator does: 0x07A2, Disconnect(14)
// (word 716, "You have been ejected by a server administrator."), and the
// session kept session.ReplacedGrace to deliver it. The listener then saves
// its player as at a logout (position and spell effects) and lets it go. A
// session not in the game is dropped at once.
func Eject(sess *session.Session) {
	if sess == nil {
		return
	}
	if sess.InGame() || sess.CharacterInWorld() {
		SendDisconnect(sess, protocol.OpcodeAdminEjectDC)
		sess.MarkReplaced()
		return
	}
	sess.SetPendingTermination(true)
}

// EjectAccount ejects every session an account holds (Eject), a ban's or a
// kick's, and returns how many there were.
func EjectAccount(sm *session.Manager, accountID int) int {
	if sm == nil || accountID == 0 {
		return 0
	}
	var hit []*session.Session
	sm.ForEach(func(sess *session.Session) bool {
		if sess.AccountID() != accountID {
			return true
		}
		if _, replaced := sess.ReplacedFor(); replaced || sess.PendingTermination() {
			return true
		}
		hit = append(hit, sess)
		return true
	})
	for _, sess := range hit {
		Eject(sess)
	}
	if len(hit) > 0 {
		log.Printf("GM: ejected %d session(s) of account %d", len(hit), accountID)
	}
	return len(hit)
}

// SendLevel tells a client its player's level (0x0020, with no experience
// into it), as a level set by a GM is: CLIENT plays its level-up sound for a
// higher one.
func SendLevel(sess *session.Session, level int) {
	SendGrantXP(sess, level, 0)
}

// GiveItemNow gives an online player n of an item now, as a merchant's sale
// does (NPCWindowHandler.buy): onto the stack it joins or as a new record at
// the end of the inventory, in the database and in the session's mirror of
// the client's list, sent as 0x003B. It comes new, at full condition. It
// returns how many went in: fewer than n when the stack or the list is full,
// 0 when nothing could.
func GiveItemNow(sess *session.Session, gw *entity.World, inv *database.InventoryStore, pattern database.ItemPatternRecord, n int) (int, error) {
	if sess == nil || gw == nil || inv == nil || n < 1 {
		return 0, errors.New("nothing to give to")
	}
	player := gw.SessionPlayer(sess)
	l := sessionInvList(sess)
	if player == nil || l == nil {
		return 0, errors.New("the player has no inventory yet (still entering the world)")
	}
	h := &NPCWindowHandler{inv: inv}
	qty, records := n, 1
	if pattern.MaxStack < 2 {
		qty, records = 1, n
	}
	given := 0
	for r := 0; r < records; r++ {
		took := l.wouldAdd(pattern.PatternID, qty, pattern.MaxStack)
		if took == 0 {
			break // the list is full, or the stack is
		}
		rec := database.CharInventoryEntry{StackLeft: took, RemainHP: pattern.MaxHP, Pattern: pattern}
		out, key, err := h.giveItem(player, l, rec, took, 0)
		if err != nil {
			return given, err
		}
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(out, key))
		given += took
	}
	log.Printf("GM: gave %s %d of %q", player.Name, given, pattern.ItemName)
	return given, nil
}
