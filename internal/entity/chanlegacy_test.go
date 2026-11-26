package entity

import (
	"github.com/eqoa/server/internal/session"
)

// The channel serializer as it was before the channel cache (chancache.go),
// copied verbatim from origin/main ca939510's visibility.go (renamed only):
// the oracle chancache_test.go holds the cached and the per-viewer paths to,
// byte for byte. A change meant to change what a channel holds is made here
// too, or those tests fail, which is their point: a change to the old path's
// behaviour made in one of the new paths alone does not pass unseen.

// legacySerializeEntityForChannelIn is serializeEntityForChannelIn before the
// cache.
func (w *World) legacySerializeEntityForChannelIn(s *chanScratch, sess *session.Session, id EntityID, objectID uint32) []byte {
	var disc, live []byte
	if s != nil {
		if s.disc == nil {
			s.disc, s.live = make([]byte, EntityStateSize), make([]byte, 0, chanScratchSize)
		}
		clear(s.disc)
		disc, live = s.disc, s.live
	}
	buf, p := w.legacySerializeEntityDiscIn(disc, sess, id, objectID)
	if buf == nil {
		return nil
	}
	var dyes *Dyes
	var auras *PieceAuras
	if p != nil {
		dyes, auras = p.LookDyes(), w.PieceAurasOf(p)
	} else if n, ok := w.NPCs[id]; ok {
		if n.WornMask != 0 {
			auras = w.NPCPieceAurasOf(n) // a mercenary's heirlooms (pieceaura.go)
		}
		dyes = w.npcDyesLocked(n) // its name's dyes (npcdyes.go), a merc's its owner's (mercdyes.go)
	}
	return forViewerAuras(live, sess, buf, dyes, auras)
}

// legacySerializeEntityDiscIn is serializeEntityDiscIn before the cache.
func (w *World) legacySerializeEntityDiscIn(to []byte, sess *session.Session, id EntityID, objectID uint32) ([]byte, *Player) {
	if to == nil {
		to = make([]byte, EntityStateSize)
	}
	// a board's name plate, from its board as it stands (boardplate.go)
	if board := w.plateBoardLocked(id); board != nil {
		return serializePlate(objectID, board), nil
	}

	// Check NPC first
	if npc, ok := w.NPCs[id]; ok {
		buf := putNPCUpdate(to, objectID, npc)
		if npc.Dead {
			buf[58] = 0x0e // AnimationState.Die
			// a corpse, as the reference server turns a kill's channel: byte 26 (CLIENT's
			// VIEntity +0x18: 1 selectable, 2 a corpse) 3, health 0. The corpse
			// bit is what makes CLIENT's attack button ask for the loot
			// (ToggleAutoAttack's 0x0015, handlers/loot.go); without it a
			// creature the game shows as attackable could not be looted at all
			// (InteractWithTarget sends nothing for one: F3 20261026-042112)
			buf[26] = 3
			buf[27] = 0
		}
		w.duelViewPetLocked(sess, npc, buf)       // a duel opponent's pet as a creature (duel.go)
		w.pvpViewPetLocked(sess, npc, buf)        // a foe's pet, to be swung at (pvp.go)
		w.challengeViewLocked(sess, npc, buf)     // a Metal Moogie not one's own to fight (metalmoogie.go)
		w.decorArtViewLocked(sess, npc, buf)      // a decoration's art, to a client that draws decorations (decorlights.go)
		w.petArtViewLocked(sess, npc, buf)        // a pet's art (a rank's PetArt), to a client that draws it (decorlights.go)
		putNPCDyeTints(w.npcDyesLocked(npc), buf) // its dyes' nearest tints (npcdyes.go, mercdyes.go)
		putNPCDyeRobeKey(npc.classKey(), buf)     // and the robe its dyes put on
		w.putChannelTargetLocked(sess, npc, buf)
		if npc.NoSelect && !npc.Dead {
			buf[26] &^= 1 // not selectable: no Tab, no click, no target box (npctitle.go's noselect)
		}
		return buf, nil
	}

	// Check player (other players on visibility channels)
	if p, ok := w.Players[id]; ok {
		// NOTHING IS OVERRIDDEN HERE. SerializePlayerUpdate already writes the
		// player's NPCType — 145 = class, 146 = 0x00 — and that is what retail
		// sends for every player in pcaps/ps2_sandstorm.pcap.
		//
		// Two writes used to follow, labelled "NPCType 0x0080 (player bit
		// set)", and both were the bugs statesync.go's own comments record as
		// fixed:
		//
		//   PutUint16(buf[146:148], 0x0080)  set byte 146 to 0x80, which is the
		//     NPC bit — the exact mistake that had the client running the NPC
		//     appearance path for players and the player path for NPCs.
		//   PutUint16(buf[185:187], 0x0080)  wrote into the "C# tail" no real
		//     server sends, at offsets that are live fields in the client's own
		//     entity struct. Once the buffer shrank from 201 to its true wire
		//     size of 162 this was simply out of range, and it PANICKED the
		//     tick goroutine the first time a second player came into view:
		//     slice bounds [:187] with capacity 162.
		//
		// A player is distinguished from an NPC by the high half being ZERO,
		// which is what the serializer already does, so there is nothing for
		// the visibility path to add.
		buf := putPlayerUpdate(to, objectID, p)
		w.duelViewLocked(sess, p.ServerID, buf) // a duel opponent as a creature (duel.go)
		w.pvpViewLocked(sess, p, buf)           // a foe, to be swung at (pvp.go)
		w.ctfViewLocked(sess, p, buf)           // the arena's teams by their names' colours (ctf.go)
		w.putChannelTargetLocked(sess, p, buf)
		return buf, p
	}

	return nil, nil
}
