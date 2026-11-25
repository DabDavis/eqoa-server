package entity

import (
	"fmt"
	"log"
	"time"
)

// dungeonsummon.go — a player summoned to someone standing in a dungeon instance (petition 2972, Heirloom,
// 2026-11-25: "Summoning keeps booting the player when they accept"). A summon (the Summoning Portal, Call of the
// Hero, a GM's #summon and #grpsummon, /grpsummon) lands its player at the summoner's spot in the dungeons' world,
// and until now left its Player.Instance as it was: 0 for one coming from outside, so dungeonsSystem found it in no
// instance and sent it out after its grace, every time it was called.
//
// Now a summon joins its player to the SUMMONER's instance (GateTravel.Via, JoinInstanceOf), by the door's own rules
// (DungeonEnter): the instance must be open and the summoner in it; the player must be one it holds (its opener, of
// its party's group or raid, or added with /addinstance; never one taken out of it) and not locked out of its
// dungeon. The summon's callers ask first (InstanceJoinCheck) and refuse the call up front, the words to both
// players, rather than moving a player the dungeon would put out again. A GM's summon (force) may bring anyone: the
// player is added to the instance as /addinstance adds one (a GM
// is put in it alone: dungeonsSystem lets a GM visit). #gotoplayer joins its GM to the player's instance so. A summon landing anywhere but the dungeons' world
// changes nothing.

// The lines a summon into an instance its player may not stand in says to both players (ours).
const (
	dungeonJoinNotParty = "%s cannot be summoned into %s's instance of %s: they are not in its party. Group them, or its leader can /addinstance them first."
	dungeonJoinRemoved  = "%s cannot be summoned into %s's instance of %s: they were taken out of it."
	dungeonJoinNoInst   = "%s cannot be summoned to %s: %s stands in no open dungeon instance."
	dungeonJoinLocked   = "%s cannot be summoned into %s's instance of %s: they are locked out of new instances of it for %d more minutes."
)

// dungeonJoinInstLocked is the instance a summon from via takes its player into: nil, "" and true when via is not in
// the dungeons' world (the summon is none of the dungeons' business). Must be called with the lock held.
func (w *World) dungeonJoinInstLocked(via *Player) (*dungeonInst, bool) {
	if via == nil || !w.inDungeonsLocked(via) {
		return nil, false
	}
	in := w.dungeons.insts[via.Instance]
	if in == nil || (!via.GM && !w.dungeonAllowsLocked(in, via)) {
		return nil, true
	}
	return in, true
}

// instanceJoinCheckLocked is why p may not be summoned to via ("" and true: it may). Must be called with the lock
// held.
func (w *World) instanceJoinCheckLocked(p, via *Player, force bool, now time.Time) (string, bool) {
	if p == nil || via == nil {
		return "", true
	}
	in, dungeon := w.dungeonJoinInstLocked(via)
	if !dungeon {
		return "", true
	}
	if in == nil {
		return fmt.Sprintf(dungeonJoinNoInst, p.Name, via.Name, via.Name), false
	}
	if w.inDungeonsLocked(p) && p.Instance == in.id {
		return "", true // there already
	}
	if force {
		return "", true
	}
	name := w.dungeons.defs[in.def].Name
	if _, out := in.removed[p.CharServerID]; out {
		return fmt.Sprintf(dungeonJoinRemoved, p.Name, in.ownerName, name), false
	}
	if !w.dungeonAllowsLocked(in, p) {
		return fmt.Sprintf(dungeonJoinNotParty, p.Name, in.ownerName, name), false
	}
	if _, locked := w.dungeonLockedOutLocked(p.CharServerID, in.def, in, now); locked {
		left, _ := w.dungeonLockLeftLocked(p.CharServerID, in.def, now)
		return fmt.Sprintf(dungeonJoinLocked, p.Name, in.ownerName, name, dungeonLockMinutes(left)), false
	}
	return "", true
}

// InstanceJoinCheck is whether the player may be summoned to via now, and why not: the summon's callers ask it
// before they call (and before a GM's move), so a refusal is said up front. force: a GM's summon, which may bring
// anyone into an open instance.
func (w *World) InstanceJoinCheck(player, via EntityID, force bool) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.instanceJoinCheckLocked(w.Players[player], w.Players[via], force, w.dungeonNowLocked())
}

// JoinInstanceOf puts the player in via's instance as it is summoned there: the check again (things change while a
// call is answered), then its instance, its way out (where it stands now, when outside the dungeons), the instance's
// stood list and the stray check's grace, as the door does. Call it just before the move; it changes nothing for a
// summon that lands outside the dungeons. A forced join (a GM's) adds the player to the instance.
func (w *World) JoinInstanceOf(player, via EntityID, force bool) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, v := w.Players[player], w.Players[via]
	now := w.dungeonNowLocked()
	if why, ok := w.instanceJoinCheckLocked(p, v, force, now); !ok {
		return why, false
	}
	in, dungeon := w.dungeonJoinInstLocked(v)
	if p == nil || !dungeon || in == nil {
		return "", true
	}
	if force && !p.GM && !w.dungeonAllowsLocked(in, p) { // a GM visits any instance it is put in (dungeonsSystem)
		if in.invited == nil {
			in.invited = map[uint32]string{}
		}
		in.invited[p.CharServerID] = p.Name
		delete(in.removed, p.CharServerID)
	}
	if !w.inDungeonsLocked(p) {
		w.dungeons.stays[p.CharServerID] = dungeonStay{From: GateSpot{World: p.World, X: p.X, Y: p.Y, Z: p.Z,
			Place: "where you were"}, FromFacing: p.Facing}
	}
	p.Instance = in.id
	in.lastIn = now
	in.markStood(p.CharServerID)
	w.dungeons.movedAt[p.ServerID] = now
	delete(w.dungeons.badAt, p.ServerID)
	log.Printf("Dungeons: %s is summoned into %s (instance %d, %s's) by %s", p.Name, w.dungeons.defs[in.def].Name,
		in.id, in.ownerName, v.Name)
	return "", true
}
