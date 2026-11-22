package handlers

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A spiritmaster binds: talked to, it asks its question (npcwords.go: the live
// game's "Would you like to be bound?", or its own), and its one answer binds
// the player to the spot right in front of it (F3 20261025-172313), kept in
// charbind; the client's Exit binds no one. Return Home and a death take the player there (homeOf); a
// character that never bound goes where every login starts, as before.

// boundLine is a spiritmaster's word once the player is bound, where it has
// none of its own (npcwords.go).
const boundLine = "You are now bound to this place."

// bindReach is how far in front of a spiritmaster its bind point lies.
const bindReach = 3.0

// bindVisual and bindCast are the bind's 0x00B1, the spiritmaster's onto the
// player, as the reference server's Spiritmaster Zole cast it (questrec16, the user's
// "spiritmasters supposed to castingcombat 0x08F3B955 when binding players").
const (
	bindVisual uint32 = 0x08F3B955
	bindCast          = 3 * time.Second
)

// bindStore keeps bind points (database.SQLiteCharacterStore).
type bindStore interface {
	SaveBind(serverID uint32, b database.BindRow) error
}

// IsSpiritmaster is whether the NPC of that name binds: a "Spiritmaster ",
// or a "Spiritbinder " (petition 2389, 2026-11-19: Spiritbinder Pluto
// in Freeport, a spiritmaster in all but his name).
func IsSpiritmaster(name string) bool {
	return strings.HasPrefix(name, "Spiritmaster ") || strings.HasPrefix(name, "Spiritbinder ")
}

// bindSpot is the spot right in front of a spiritmaster (inFrontOf).
func bindSpot(npc *entity.NPC) entity.Bind {
	x, z, face := inFrontOf(npc.SpawnX, npc.SpawnZ, npc.SpawnFacing)
	return entity.Bind{Set: true, World: npc.World, X: x, Y: npc.SpawnY, Z: z, Facing: face, By: npc.Name, NPC: npc.ServerID}
}

// inFrontOf is the spot bindReach in front of someone standing at (x, z)
// with heading h (CLIENT's: the direction (sin h, cos h)), and the heading
// that faces them from it.
func inFrontOf(x, z, h float32) (fx, fz, face float32) {
	a := float64(h)
	return x + float32(bindReach*math.Sin(a)), z + float32(bindReach*math.Cos(a)), float32(math.Mod(a+math.Pi, 2*math.Pi))
}

// bindOfRow is a kept bind point.
func bindOfRow(b database.BindRow) entity.Bind {
	return entity.Bind{Set: true, World: uint8(b.World), X: b.X, Y: b.Y, Z: b.Z, Facing: b.Facing, By: b.Spiritmaster}
}

// rowOfBind is a bind point to keep.
func rowOfBind(b entity.Bind) database.BindRow {
	return database.BindRow{World: int(b.World), X: b.X, Y: b.Y, Z: b.Z, Facing: b.Facing, Spiritmaster: b.By}
}

// homeOf is where Return Home and a death take player id: its bind point when
// it has one its game can load and that is a place to stand, else where
// every login starts (loginSpawnDest, loginSpawnOffset off it); via names it
// for the log.
func homeOf(gw *entity.World, sess *session.Session, id entity.EntityID) (x, y, z float32, world uint8, via string, ok bool) {
	if b, bound := gw.BindOf(id); bound {
		if w, err := ValidateTeleport(b.X, b.Y, b.Z, b.World); err != nil {
			log.Printf("Home: the bind point by %s is not a place to stand (%v); the login spawn instead", b.By, err)
		} else if !CanLoadWorld(sess, w) {
			log.Printf("Home: the bind point by %s is in world %d, which this game cannot load; the login spawn instead", b.By, w)
		} else {
			return b.X, b.Y, b.Z, w, "the bind point by " + b.By, true
		}
	}
	dx, dy, dz, dw, via, found := ResolveTeleDest(gw, loginSpawnDest)
	if !found {
		log.Printf("Home: no %q destination", loginSpawnDest)
		return 0, 0, 0, 0, "", false
	}
	x, z = dx+loginSpawnOffset, dz+loginSpawnOffset
	w, err := ValidateTeleport(x, dy, z, dw)
	if err != nil {
		log.Printf("Home: %s is not a placeable spawn (%v)", via, err)
		return 0, 0, 0, 0, "", false
	}
	return x, dy, z, w, via, true
}

// askBind opens a spiritmaster's question.
func (h *InteractHandler) askBind(sess *session.Session, player *entity.Player, npc *entity.NPC) {
	player.PendingBind = bindSpot(npc)
	w := bindWordsOf(npc.Name)
	h.sendOptionBox(sess, w.Ask, []string{w.Answer})
	log.Printf("Bind: %s asks %s", npc.Name, player.Name)
}

// answerBind takes the answer to the open question: its one answer (option 0)
// binds the player there, kept, then does the player's quest step at that
// spiritmaster (a "bind your spirit" step) in the same go; anything else (the
// client's Exit) closes it.
// The reference server's Zole (questrec16) answered the step only at the next talk: two
// talks a new player did not know to make (F3 20261029-200655, 20261030-062009).
func (h *InteractHandler) answerBind(sess *session.Session, player *entity.Player, option byte) {
	b := player.PendingBind
	player.PendingBind = entity.Bind{}
	if option != 0 {
		return
	}
	h.gameWorld.SetBind(player.ServerID, b)
	if h.binds != nil {
		if err := h.binds.SaveBind(player.CharServerID, rowOfBind(b)); err != nil {
			log.Printf("Bind: saving %s's bind point: %v", player.Name, err)
		}
	}
	for _, line := range bindWordsOf(b.By).Bound {
		sess.Queue().AddWithSequence(buildClientMessage(b.By + ": " + line))
	}
	h.gameWorld.NPCVisual(b.NPC, player.ServerID, bindCast, bindVisual)
	log.Printf("Bind: %s bound by %s at (%.1f, %.1f, %.1f) world=%d", player.Name, b.By, b.X, b.Y, b.Z, b.World)
	if Quests().TalkStep(sess, player, b.By, b.NPC) {
		log.Printf("Bind: %s's quest step at %s done with the bind", player.Name, b.By)
	}
}
