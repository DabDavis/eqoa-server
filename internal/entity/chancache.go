package entity

import (
	"encoding/binary"
	"time"

	"github.com/eqoa/server/internal/session"
)

// chancache.go — an entity's channel made once a pass, not once a viewer (the
// owner 2026-11-26: "cache the other items like mercs"; pprof of the Heirloom
// shard, ~68 players: entity channel sync 13% of its CPU, serializeEntityDiscIn
// 8.6% of it, putNPCUpdate 4%).
//
// entityChannelSyncSystem puts every occupied channel of every session each
// tick, and a creature in a town is on the channels of everyone near it: the
// same NPC's 162 bytes were made from scratch once per viewer, its dyes looked
// up twice per viewer, and a player's target found once per viewer by a walk
// over every session (targetOfLocked's sessionOfLocked). What a channel holds
// splits in two:
//
//   - the entity's own, the same for everyone in one pass: putNPCUpdate's or
//     putPlayerUpdate's bytes (all but the object id, bytes 1-4, which is the
//     viewer's table's, and a player's battle-music byte 155, which reads the
//     clock), a corpse's bytes 26/27/58, the NPC's dyes and class key, the
//     wearer's dyes and piece auras, and whom the entity targets;
//   - the viewer's: the object id, every *ViewLocked (duel, PvP, CTF, a
//     challenge, decorations' and pets' art for a client that draws them),
//     the target's id in the viewer's table, and the look the viewer's client
//     asked for (forViewerAuras).
//
// fillChanEntryLocked makes the first part and layChannelLocked lays the
// second over a copy of it, in the order the serializer always ran them.
// chanCache holds the first part for one pass (entityChannelSyncSystem resets
// it at its start and nothing outside the pass reads it); every other caller
// (serializeEntityForChannelIn: the view's pass, a pet's, a corpse's) makes it
// for its one call, so there is one code path, cached or not. A channel's bytes
// are the old serializer's (chancache_test.go holds both to a verbatim copy
// of it, chanlegacy_test.go, over the same scenes; TestTickRigChannels on a
// real database each channel at every tick's end).
//
// Nothing in the pass changes an entity (it only reads the world and writes
// the sessions' channels), so the first viewer's figures are every viewer's.
// What the old path read per viewer off something another goroutine may swap
// mid-pass (the itemaura and charaura tables' atomic pointers, a wearer's
// piece auras) is now read once an entity a pass, as a pass that ran a little
// faster would have; the clock (byte 155) is still read per viewer, as
// before.

// chanEntry is one entity's part of its channels this pass (the doc above).
type chanEntry struct {
	kind   chanKind
	board  *NPC    // chanPlate: the board whose plate it is
	npc    *NPC    // chanNPC: the NPC; chanPlate: w.NPCs[id], nil when none
	p      *Player // chanPlayer
	base   [EntityStateSize]byte
	dyes   *Dyes
	auras  *PieceAuras
	key    string // chanNPC: its classKey (putNPCDyeRobeKey)
	target Entity // targetOfLocked's
}

type chanKind uint8

const (
	chanNone   chanKind = iota // neither a plate, an NPC nor a player: no channel
	chanPlate                  // a board's name plate (boardplate.go)
	chanNPC                    // an NPC
	chanPlayer                 // a player
)

// chanCache is a pass's chanEntry by entity (the doc above). Its map and slice
// keep their room from pass to pass.
type chanCache struct {
	at      map[EntityID]int32
	entries []chanEntry
}

// reset empties the cache for a new pass.
func (c *chanCache) reset() {
	if c.at == nil {
		c.at = make(map[EntityID]int32)
	}
	clear(c.at)
	c.entries = c.entries[:0]
}

// entryLocked is id's entry, made on its first viewer this pass. Must be
// called with the lock held.
func (c *chanCache) entryLocked(w *World, id EntityID) *chanEntry {
	if i, ok := c.at[id]; ok {
		return &c.entries[i]
	}
	c.entries = append(c.entries, chanEntry{})
	e := &c.entries[len(c.entries)-1]
	c.at[id] = int32(len(c.entries) - 1)
	w.fillChanEntryLocked(e, id)
	return e
}

// fillChanEntryLocked makes id's entry: every read of the entity alone its
// channel takes (the file's doc), as the serializer made them before the
// cache (ca939510's serializeEntityDiscIn and serializeEntityForChannelIn,
// kept as chanlegacy_test.go's oracle). Must be called with the lock held.
func (w *World) fillChanEntryLocked(e *chanEntry, id EntityID) {
	if board := w.plateBoardLocked(id); board != nil {
		e.kind, e.board = chanPlate, board
		if n, ok := w.NPCs[id]; ok { // the old path looked for an NPC of any id, a plate's too
			e.npc = n
			if n.WornMask != 0 {
				e.auras = w.NPCPieceAurasOf(n)
			}
			e.dyes = w.npcDyesLocked(n)
		}
		return
	}
	if npc, ok := w.NPCs[id]; ok {
		e.kind, e.npc = chanNPC, npc
		buf := putNPCUpdate(e.base[:], 0, npc)
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
		e.dyes = w.npcDyesLocked(npc)
		e.key = npc.classKey()
		e.target = w.targetOfLocked(npc)
		if npc.WornMask != 0 {
			e.auras = w.NPCPieceAurasOf(npc)
		}
		return
	}
	if p, ok := w.Players[id]; ok {
		e.kind, e.p = chanPlayer, p
		putPlayerUpdate(e.base[:], 0, p)
		e.base[155] = 0 // the battle music's, read per viewer as before (layDiscLocked)
		e.target = w.targetOfLocked(p)
		e.dyes, e.auras = p.LookDyes(), w.PieceAurasOf(p)
	}
}

// serializeEntityChannelCached is serializeEntityForChannelIn(s, sess, id,
// objectID) with the entity's part out of c (the file's doc). Must be called
// with the lock held.
func (w *World) serializeEntityChannelCached(c *chanCache, s *chanScratch, sess *session.Session, id EntityID, objectID uint32) []byte {
	return w.layChannelLocked(c.entryLocked(w, id), s, sess, objectID)
}

// layChannelLocked is the channel of e's entity as sess sees it under
// objectID, made in s's buffers (nil: new ones; what it returns is s's until
// the next call): the disc's look (layDiscLocked) in the look sess's client
// asked for (forViewerAuras). Must be called with the lock held.
func (w *World) layChannelLocked(e *chanEntry, s *chanScratch, sess *session.Session, objectID uint32) []byte {
	var disc, live []byte
	if s != nil {
		if s.disc == nil {
			s.disc, s.live = make([]byte, EntityStateSize), make([]byte, 0, chanScratchSize)
		}
		disc, live = s.disc, s.live
	}
	buf := w.layDiscLocked(e, disc, sess, objectID)
	if buf == nil {
		return nil
	}
	return forViewerAuras(live, sess, buf, e.dyes, e.auras)
}

// layDiscLocked is the channel in the disc's look of e's entity as sess sees
// it under objectID, made in to (EntityStateSize bytes; nil: a new buffer;
// a plate's is a new one), nil for no entity: e's part with every viewer's
// write laid over it, in the order they always ran. Must be called with the
// lock held.
func (w *World) layDiscLocked(e *chanEntry, to []byte, sess *session.Session, objectID uint32) []byte {
	switch e.kind {
	case chanNone:
		return nil
	case chanPlate:
		return serializePlate(objectID, e.board) // a board's name plate, from its board as it stands (boardplate.go)
	}
	if to == nil {
		to = make([]byte, EntityStateSize)
	}
	buf := to[:EntityStateSize]
	copy(buf, e.base[:])
	binary.LittleEndian.PutUint32(buf[1:5], objectID)
	if e.kind == chanNPC {
		npc := e.npc
		w.duelViewPetLocked(sess, npc, buf)   // a duel opponent's pet as a creature (duel.go)
		w.pvpViewPetLocked(sess, npc, buf)    // a foe's pet, to be swung at (pvp.go)
		w.challengeViewLocked(sess, npc, buf) // a Metal Moogie not one's own to fight (metalmoogie.go)
		w.decorArtViewLocked(sess, npc, buf)  // a decoration's art, to a client that draws decorations (decorlights.go)
		w.petArtViewLocked(sess, npc, buf)    // a pet's art (a rank's PetArt), to a client that draws it (decorlights.go)
		putNPCDyeTints(e.dyes, buf)           // its dyes' nearest tints (npcdyes.go, mercdyes.go)
		putNPCDyeRobeKey(e.key, buf)          // and the robe its dyes put on
		putChannelTargetOfLocked(w, sess, false, e.target, buf)
		if npc.NoSelect && !npc.Dead {
			buf[26] &^= 1 // not selectable: no Tab, no click, no target box (npctitle.go's noselect)
		}
		return buf
	}
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
	p := e.p
	if p.inCombatForMusic(time.Now()) { // putPlayerUpdate's byte 155, its clock read for this viewer as before
		buf[155] = 1
	}
	w.duelViewLocked(sess, p.ServerID, buf) // a duel opponent as a creature (duel.go)
	w.pvpViewLocked(sess, p, buf)           // a foe, to be swung at (pvp.go)
	w.ctfViewLocked(sess, p, buf)           // the arena's teams by their names' colours (ctf.go)
	putChannelTargetOfLocked(w, sess, true, e.target, buf)
	return buf
}
