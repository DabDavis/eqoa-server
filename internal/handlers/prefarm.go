package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// prefarm.go — /prefarm on and /prefarm off, everyone's (the owner, 2026-11-25:
// "/prefarm off option for players that removes the quest loot drops from mobs, rule
// applies to group if player is group leader"; petition 2975: Freezing Candles on
// every corpse of a player who did not want them, destroyed to clear the corpses).
//
// A quest's loot step item drops on a kill's corpse "ahead" when no player of the
// kill is on the step (questpreloot.go: for whoever may loot it, so players can farm
// it early). With /prefarm off, a kill the choice holds for has no such drop; a
// player of the kill on the step still has its own copy, as ever. Whose choice holds:
//
//   - a kill of a group or raid (entity.KillCredit.Group): its leader's at the death
//     (KillCredit.Leader), whoever of it killed; a member's own is not read while it
//     is in a group (the leader decides for the group, the owner's words); a group
//     whose leader is not in the world drops as ever;
//   - a kill of no group: its killer's.
//
// On is the default (a character with no row in charprefarm). The choice is kept
// in table charprefarm, so it outlives a logout and every restart; it is read from
// there at each kill that has an item to drop ahead (no copy held in memory).
//
// Its spellings: "/prefarm", "/prefarm on", "/prefarm off", the same with "#"
// (chat.go), and a PS2's "/t prefarm", "/t prefarm on", "/t prefarm off" (it keeps
// an unknown /command to itself and cannot type a #). "/prefarm" alone says the
// setting, and whose holds in the player's group.

// prefarmWord is /prefarm's word.
const prefarmWord = "prefarm"

// The lines it answers with.
const (
	prefarmOnLine       = "Prefarm is on: quest items drop on your kills even for a quest step no one of the kill is on (/prefarm off to stop them)."
	prefarmOffLine      = "Prefarm is off: a quest item drops on your kills only for a player of the kill on its step (/prefarm on to have them all)."
	prefarmSoloLine     = "Out of a group it holds for your own kills; as a group or raid leader, for all your group's kills."
	prefarmLeaderLine   = "You lead your group: your setting holds for all its kills."
	prefarmMemberLine   = "In a group its leader's setting holds for its kills: %s has prefarm %s."
	prefarmNoLeaderLine = "In a group its leader's setting holds for its kills; your leader is not in the world, so quest items drop as usual."
	prefarmUsage        = "Usage: /prefarm, /prefarm on or /prefarm off (on a PS2 /t prefarm off): off stops quest items dropping ahead on your kills."
)

// prefarmStore is where /prefarm is kept (database.SQLiteDB).
type prefarmStore interface {
	PrefarmOff(serverID uint32) (bool, error)
	SetPrefarmOff(serverID uint32, off bool) error
}

// SetPrefarmStore sets where /prefarm is kept.
func (h *ChatHandler) SetPrefarmStore(s prefarmStore) { h.prefarm = s }

// prefarmLine reads a typed line as /prefarm: the command word ("/prefarm",
// "#prefarm", any case) and its rest. ok false for any other word.
func prefarmLine(word, rest string) (arg string, ok bool) {
	switch strings.ToLower(word) {
	case "/" + prefarmWord, "#" + prefarmWord:
		return strings.ToLower(strings.TrimSpace(rest)), true
	}
	return "", false
}

// prefarmTellWords is whether a PS2's "/t prefarm <body>" is /prefarm: the body
// empty, "on" or "off"; anything else to a "prefarm" is a tell.
func prefarmTellWords(body string) bool {
	switch strings.ToLower(strings.TrimSpace(body)) {
	case "", "on", "off":
		return true
	}
	return false
}

// prefarmCommand is /prefarm: arg "" says the setting, "on" or "off" sets it.
func (h *ChatHandler) prefarmCommand(sess *session.Session, player *entity.Player, arg string) {
	if player == nil {
		return
	}
	if h.prefarm == nil {
		SendErrorMessage(sess, "Prefarm cannot be changed on this server.")
		return
	}
	switch arg {
	case "":
		off, err := h.prefarm.PrefarmOff(player.CharServerID)
		if err != nil {
			log.Printf("Prefarm: '%s' read: %v", player.Name, err)
			SendErrorMessage(sess, "Prefarm could not be read just now: try again.")
			return
		}
		h.sayPrefarm(sess, player, off)
	case "on", "off":
		off := arg == "off"
		if err := h.prefarm.SetPrefarmOff(player.CharServerID, off); err != nil {
			log.Printf("Prefarm: '%s' not kept: %v", player.Name, err)
			SendErrorMessage(sess, "Prefarm could not be changed just now: try again.")
			return
		}
		h.sayPrefarm(sess, player, off)
		log.Printf("Prefarm: '%s' turned prefarm %s", player.Name, arg)
	default:
		SendErrorMessage(sess, prefarmUsage)
	}
}

// sayPrefarm tells a player its setting and whose holds for its kills.
func (h *ChatHandler) sayPrefarm(sess *session.Session, player *entity.Player, off bool) {
	if off {
		SendColoredChat(sess, prefarmOffLine, 255, 255, 0)
	} else {
		SendColoredChat(sess, prefarmOnLine, 255, 255, 0)
	}
	SendColoredChat(sess, h.prefarmWhoseLine(player), 255, 255, 0)
}

// prefarmWhoseLine says whose /prefarm holds for the player's kills now.
func (h *ChatHandler) prefarmWhoseLine(player *entity.Player) string {
	if h.gameWorld == nil {
		return prefarmSoloLine
	}
	gid := h.gameWorld.GroupIDOf(player)
	if gid == 0 {
		return prefarmSoloLine
	}
	leader := h.gameWorld.RaidLeaderOf(gid)
	switch {
	case leader == 0:
		return prefarmNoLeaderLine
	case leader == player.CharServerID:
		return prefarmLeaderLine
	}
	name := "your leader"
	for _, m := range h.gameWorld.GroupMembers(player) {
		if m.CharServerID == leader {
			name = m.Name
			break
		}
	}
	off, err := h.prefarm.PrefarmOff(leader)
	if err != nil {
		log.Printf("Prefarm: leader %d read: %v", leader, err)
		return prefarmNoLeaderLine
	}
	return fmt.Sprintf(prefarmMemberLine, name, onOff(!off))
}

// onOff is "on" or "off".
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// prefarmKill is /prefarm's verdict on the kill the quest engine is counting
// (QuestEngine.killed), worked out at its first need and kept for the rest of it.
type prefarmKill struct {
	k       entity.KillCredit
	decided bool
	off     bool
	whose   uint32 // the character whose choice held, for the log
}

// aheadOffLocked is whether /prefarm off holds for the kill being counted (the
// file's doc): its items for no player of the kill on their step are not dropped.
// False outside a kill.
func (q *QuestEngine) aheadOffLocked() bool {
	pk := q.prefarm
	if pk == nil {
		return false
	}
	if pk.decided {
		return pk.off
	}
	pk.decided = true
	var id uint32
	switch {
	case pk.k.Group != 0:
		id = pk.k.Leader // 0: no leader in the world, as ever
	case len(pk.k.Players) > 0:
		id = pk.k.Players[0].ServerID // the killer
	}
	if id == 0 || q.db == nil {
		return false
	}
	off, err := q.db.PrefarmOff(id)
	if err != nil {
		log.Printf("Quests: /prefarm of character %d: %v", id, err)
		return false
	}
	pk.off, pk.whose = off, id
	return off
}
