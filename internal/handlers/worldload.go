package handlers

import (
	"log"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// worldLoadHoldTime bounds how long state channels stay held after a teleport
// when no 0x0049 arrives. Retail's loads measured 3.5 s and 4.4 s; a world
// streamed off the memory card or the HDD is slower, so this is generous. It
// only matters for a teleport the client services without a reload, and it
// sends no 0x0049 at all — there the cost is entities freezing for this long.
const worldLoadHoldTime = 15 * time.Second

// The hold / clear / resend layers are switchable at run time so each can be
// removed on its own; see session.WorldLoadLayers for what the switches are and
// why they live in that package.

// beginWorldLoad is called immediately after queuing a Teleport (0x00B2). It
// holds state channel output until the client reports back with 0x0049, and
// drops every entity the client can see, so that the channels are free for
// whatever stands at the destination.
func beginWorldLoad(sess *session.Session, worldChanged bool) {
	hold, clear, _ := session.WorldLoadLayers()
	if hold {
		sess.HoldStateChannels(worldLoadHoldTime)
	}
	if worldChanged && clear {
		clearVisibilityChannels(sess)
	}
}

// clearVisibilityChannels clears every occupied NPC visibility channel so the client
// drops the old world's entities and the visibility system is free to re-fill
// the channels at the destination.
func clearVisibilityChannels(sess *session.Session) {
	conn := sess.Connection()
	if conn == nil {
		return
	}

	vis := entity.GetOrCreateVisibility(sess)

	cleared := 0
	for i := 0; i < entity.MaxVisibleEntities; i++ {
		if vis.ChannelEntity[i] == 0 {
			continue
		}
		// The entity's own id and type, flag cleared: CLIENT deletes it and
		// makes nothing. Zeros would be type 0, an assert on an empty slot.
		if state := conn.GetServerObject(i + 1 + entity.NPCChannelOffset); state != nil {
			state.MarkRemoved()
		}
		vis.ChannelEntity[i] = 0
		vis.ChannelObjectID[i] = 0
		cleared++
	}

	log.Printf("World change: cleared %d occupied visibility channels for %s", cleared, sess.RemoteAddr())
}

// WorldLoadHandler answers the client's "my scene is new, describe it again"
// message, opcode 0x0049.
//
// The PS2 sends this reliable one-byte message after it has finished loading a
// world, in the same packet as the first position update (0x40) from the new
// position. Retail replies within ~50 ms with every active state channel at
// once — 1,088 bytes across three packets in
// pcaps/sandstorm-worldchange-20260807.pcap, 1,119 in pcaps/ps2_sandstorm.pcap
// — and that reply is the only reason a player and the NPCs around them exist
// after a teleport.
//
// We were logging it as "Unhandled opcode: 0x0049 ... len=1" and dropping it,
// which is why a world change produced a correctly loaded world with no player,
// no NPCs and a camera pointing at nothing while movement still worked: steering
// is local, everything you can SEE comes from these channels.
//
// The payload byte is not decoded. Retail sends 0x00; our own PS2 sends 0x00 and
// sometimes 0x09 for the same destination world, so it is not the world index
// and nothing here depends on it.
type WorldLoadHandler struct{}

// NewWorldLoadHandler creates a world-load handler.
func NewWorldLoadHandler() *WorldLoadHandler {
	return &WorldLoadHandler{}
}

// RegisterHandlers registers the world-load handlers.
func (h *WorldLoadHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeEnableChannel, h.HandleEnableChannel)
}

// HandleEnableChannel handles opcode 0x0049.
func (h *WorldLoadHandler) HandleEnableChannel(sess *session.Session, msg *protocol.Message) error {
	// Lift the hold first: a client that is asking for its scene back has
	// finished loading, whatever the hold deadline says.
	sess.ReleaseStateChannels()

	conn := sess.Connection()
	if conn == nil {
		return nil
	}

	if _, _, resend := session.WorldLoadLayers(); !resend {
		log.Printf("EnableChannel (0x0049) from %s: resend layer disabled, not re-arming",
			sess.RemoteAddr())
		return nil
	}

	n := conn.ResendAll()
	log.Printf("EnableChannel (0x0049) from %s: client finished loading, re-arming %d state channels",
		sess.RemoteAddr(), n)
	return nil
}
