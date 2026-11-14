package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// lootrights.go — who may open a corpse, and how many at once (2026-11-11).
//
// The kill group's alone (petition 1537, Hoofbite: "Should not allow players from
// outside of kill group to loot mobs"): a corpse whose death a player was credited
// with (entity's KillCredit: a melee kill, a spell's, a pet's, a GM's #kill) opens
// only for those the kill counted for (the killer and its group's members near it,
// KillCredit.Players) and for whoever is in the killer's group (a raid's too) when
// it opens, a member who was farther off included. Anyone else is told CLIENT's
// own word 675, "That corpse belongs to another player or group" (0x0085 3). A
// corpse no player was credited with (the town guards', a dummy's, one from before
// a restart) is anyone's, as every corpse was.
// From the kill's OpenAt (five minutes after the death, entity/corpseopen.go) a
// corpse killed outside a raid is anyone's too, whatever is left on it; a raid's
// keeps its kill group for as long as it lies.
//
// One looter at a time (petition 1540, Hoofbite: "should only allow one person to
// loot corpse at a time"): while a corpse's window is open for one player, another
// opening it is told word 673, "That corpse is already being looted by another
// player" (0x0085 1). The window is let go when its looter closes it (0x0016),
// empties the corpse, or the corpse goes; and a holder no longer looting is passed
// over and its window closed: one out of the world, one that walked off
// (lootHoldReach beyond the corpse's reach), or one that has done nothing in it
// for lootHoldIdle.

// lootHoldIdle is how long an open window holds its corpse with nothing taken.
const lootHoldIdle = 60 * time.Second

// lootHoldReach is how far past a corpse's opening reach its looter may stand
// and still hold it.
const lootHoldReach float32 = 5

// lootRight is a corpse's kill group: the death it is of, the characters the
// kill counted for and the killer's group.
type lootRight struct {
	died  time.Time
	chars map[uint32]bool
	group uint32
	// openAt is when it is anyone's (KillCredit.OpenAt), zero never
	openAt time.Time
}

// heardKillRight takes a kill from the World (its lock held: never wait here)
// and keeps its kill group for the corpse.
func (h *NPCWindowHandler) heardKillRight(k entity.KillCredit) {
	if len(k.Players) == 0 {
		return
	}
	r := lootRight{died: k.Died, chars: map[uint32]bool{}, group: k.Group, openAt: k.OpenAt}
	for _, p := range k.Players {
		r.chars[p.ServerID] = true
	}
	h.killRights.Store(k.NPCID, r)
}

// mayLoot reports whether the player may open the corpse of that death (the
// file's doc).
func (h *NPCWindowHandler) mayLoot(player *entity.Player, id entity.EntityID, died time.Time) bool {
	v, ok := h.killRights.Load(id)
	if !ok {
		return true
	}
	r := v.(lootRight)
	if !r.died.Equal(died) || player == nil || r.chars[player.CharServerID] {
		return true
	}
	if r.group != 0 && h.groupIDOf != nil && h.groupIDOf(player) == r.group {
		return true
	}
	if !r.openAt.IsZero() && !h.lootClock().Before(r.openAt) {
		log.Printf("Loot: %s opens another's kill: anyone's %v after its death (not a raid's)", player.Name, r.openAt.Sub(r.died))
		return true
	}
	return false
}

// heldByOtherLocked reports whether another player's window holds the corpse
// (the file's doc); a holder no longer looting is let go of here, its window
// closed. Must be called with ls.mu held.
func (h *NPCWindowHandler) heldByOtherLocked(ls *lootState, sess *session.Session, npc *entity.NPC, now time.Time) (string, bool) {
	for s, open := range ls.open {
		if s == sess || open != npc.ServerID {
			continue
		}
		var holder *entity.Player
		if h.playerOf != nil {
			holder = h.playerOf(s)
		}
		reach := entity.InteractRange + npc.ReachExtra() + lootHoldReach
		switch {
		case holder == nil:
		case !entity.InRange(holder, npc, reach):
		case now.Sub(ls.since[s]) >= lootHoldIdle:
		default:
			return holder.Name, true
		}
		name := "a player gone"
		if holder != nil {
			name = holder.Name
		}
		log.Printf("Loot: %s's window on %q let go of (no longer looting)", name, npc.Name)
		delete(ls.open, s)
		delete(ls.since, s)
		if holder != nil {
			queueReliable(s, protocol.OpcodeClientCloseLoot, closeLootPayload())
		}
	}
	return "", false
}

// lootClock is the time for the loot's holds (a test's stand-in, else now).
func (h *NPCWindowHandler) lootClock() time.Time {
	if h.lootNow != nil {
		return h.lootNow()
	}
	return time.Now()
}
