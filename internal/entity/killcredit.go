package entity

import (
	"log"

	"github.com/eqoa/server/internal/session"
)

// killcredit.go — a kill is the group's that did the most damage (the owner, 2026-11-12, "yes most damaage", on the
// GM Jibrish's petition 1859: "Figure out why the queen qy'tha im targeting wasnt lootable by the raid").
//
// A kill went to whoever landed its last blow, with that one's group: the Queen Qy'tha a raid had brought down from
// 400,056 to 309 died to the Primal Spirit of a player not yet in the raid, so its loot rights, its XP and its quest
// credit were that player's alone and the raid was told "That corpse belongs to another player or group".
//
// Now each creature keeps the damage made on it this life by each player (damageBy: a player's own hits and spells, its
// pets' and its mercenaries' counted to it), tallied where its hit points are taken (tallyDamageLocked: a melee hit,
// a spell's, a pet's, a merc's; at most what it had left, so an overkill counts no more than it took). At its death the
// damage is summed by group (a raid's groups are one group here; a player alone is its own), and the kill is the
// group's that made the most (killCreditLocked): the last blow's own when its group made the most, else given to that
// group's member who made the most damage, in the creature's world. XP, faction, quests and loot rights all follow that
// player as they followed the killer. A tie stays with the last blow's group. A creature no player's damage reached
// (a GM's #kill, a guard's) keeps its killer as before. Its fight given up (giveUpLocked: it went home and healed) or a
// new life starts the count anew.

// tallyDamageLocked counts n of damage on npc as maker's (a pet's or a merc's owner). Must be called with the lock held.
func (w *World) tallyDamageLocked(npc *NPC, maker *Player, n int) {
	if npc == nil || maker == nil || n <= 0 {
		return
	}
	if npc.damageBy == nil {
		npc.damageBy = map[EntityID]int64{}
	}
	npc.damageBy[maker.ServerID] += int64(n)
}

// killCreditLocked is the player (and its session) a kill of npc is credited to: killer's when its group made the most
// damage on it, else the most damaging member of the group that did (the file's doc). Must be called with the lock
// held.
func (w *World) killCreditLocked(npc *NPC, sess *session.Session, killer *Player) (*session.Session, *Player) {
	if npc == nil || killer == nil || len(npc.damageBy) == 0 {
		return sess, killer
	}
	key := func(p *Player) int64 {
		if p.GroupID != 0 && w.groups[p.GroupID] != nil {
			return int64(p.GroupID)
		}
		return -int64(p.ServerID)
	}
	sums := map[int64]int64{}
	for id, n := range npc.damageBy {
		if p := w.Players[id]; p != nil {
			sums[key(p)] += n
		}
	}
	mine := key(killer)
	best, bestSum := mine, sums[mine]
	for k, n := range sums {
		if n > bestSum {
			best, bestSum = k, n
		}
	}
	if best == mine {
		return sess, killer
	}
	var to *Player
	for id, n := range npc.damageBy {
		p := w.Players[id]
		if p == nil || key(p) != best || p.World != npc.World {
			continue
		}
		if to == nil || n > npc.damageBy[to.ServerID] || n == npc.damageBy[to.ServerID] && p.ServerID < to.ServerID {
			to = p
		}
	}
	if to == nil {
		return sess, killer
	}
	log.Printf("KillCredit: %s's kill of %q goes to %s's group, the most damage (%d of it; the last blow's group %d)",
		killer.Name, npc.Name, to.Name, bestSum, sums[mine])
	return w.sessionOfLocked(to), to
}
