package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A GM's travel by the map (the owner: "for gms the map lets add fast travel
// to where they click on the map"). A GM's client is offered it with the GM's
// channel at world entry (protocol.OpcodeMapTravel, SendGMChannel); vanilla
// then lets the GM Ctrl+click a spot of its map, and asks for it
// (protocol.OpcodeMapTravelTo: the world, x and z). The server takes the ask
// of a GM alone, checked on every one (acctlevel 1 and up, as every GM
// command is), and moves the character as #goto x y z does (teleportTo: the
// world's load, the client's 0x07F6), on the floor there: the ground's, the
// highest terrain at the spot (entity.World.GroundFloor, where a creature's
// spawn stands), so never on a roof or in a tree, and a client's walk takes
// the floor of anything placed over it from there. Anyone else's ask is
// logged and dropped, nothing said: no client but a GM's is offered it.

// mapTravelFrom is the height the floor is looked for under: above every
// world's terrain, so the highest is found.
const mapTravelFrom = 100000

// mapTravelFarthest is how far from a world's middle a spot may be: past
// every world's edge (Tunaria's is about 32,000 across).
const mapTravelFarthest = 1e6

// buildMapTravelOffer is protocol.OpcodeMapTravel: a u32 1.
func buildMapTravelOffer() []byte {
	return build(protocol.OpcodeMapTravel, 6, func(w *buffer.Writer) { w.WriteUint32(1) })
}

// HandleMapTravelTo is protocol.OpcodeMapTravelTo, a GM's ask to travel to a
// spot of its map: u32 world, f32 x, f32 z. A GM's character in the world is
// moved there (and told so in a [GM] line), unless the spot is no world's
// place, the GM's client cannot load its world (teleportTo says so) or the
// ground knows no floor there (said, with #goto for a spot of the GM's own
// height). Anyone else's ask, or one too short, moves no one.
func (h *ChatHandler) HandleMapTravelTo(sess *session.Session, msg *protocol.Message) error {
	d := msg.Data()
	if len(d) < 12 {
		log.Printf("Map travel: %d bytes from %s: too short, dropped", len(d), sess.RemoteAddr())
		return nil
	}
	world := binary.LittleEndian.Uint32(d)
	x := math.Float32frombits(binary.LittleEndian.Uint32(d[4:]))
	z := math.Float32frombits(binary.LittleEndian.Uint32(d[8:]))
	if !h.isGM(sess) {
		log.Printf("Map travel: account %d, not a GM, asked for (%.0f, %.0f) in world %d: refused", sess.AccountID(), x, z, world)
		return nil
	}
	if h.gameWorld == nil || !sess.CharacterInWorld() {
		return nil
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	far := func(v float32) bool {
		return math.IsNaN(float64(v)) || math.Abs(float64(v)) > mapTravelFarthest
	}
	if world >= uint32(len(worldNames)) || far(x) || far(z) {
		h.gmReply(sess, fmt.Sprintf("Map travel: (%v, %v) in world %d is no place to go.", x, z, world))
		log.Printf("Map travel: %s asked for (%v, %v) in world %d: no such place", p.Name, x, z, world)
		return nil
	}
	w := byte(world)
	y, ok := h.gameWorld.GroundFloor(w, x, mapTravelFrom, z)
	if !ok {
		h.gmReply(sess, fmt.Sprintf("Map travel: no ground is known at (%.0f, %.0f) in %s; #goto x y z goes there at a height of yours.", x, z, worldName(w)))
		log.Printf("Map travel: %s asked for (%.0f, %.0f) in world %d: no ground known there", p.Name, x, z, world)
		return nil
	}
	if !h.teleportTo(sess, sess, p, x, y, z, p.Facing, w) {
		return nil
	}
	h.gmReply(sess, fmt.Sprintf("Travelled to (%.0f, %.0f, %.0f) world=%s", x, y, z, worldName(w)))
	log.Printf("GM: %s travelled by the map to (%.0f, %.0f, %.0f) world=%d", p.Name, x, y, z, world)
	return nil
}
