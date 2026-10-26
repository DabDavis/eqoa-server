package handlers

import (
	"encoding/binary"
	"log"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Voice chat's messages (entity/voice.go, internal/voice; protocol's
// OpcodeVoiceOffer .. OpcodeVoiceRoomState).
//
// Once the player is in the world a server with voice offers it (0x7E10, a
// u32 1); a client whose player turned voice on answers with its flags (bit
// 0 on; 0 off), and from then on is sent its ICE servers (0x7E11) and its
// roster (0x7E12), signals the forwarding server (0x7E13) and uses the
// lobby's rooms (0x7E14, answered 0x7E15 and 0x7E16). A PS2 ignores the
// offer, as it ignores OpcodeEntityView's, and is never sent anything else.

// voiceOffered is whether this server offers voice (main.go: config.yaml's
// voice, enabled with a URL and the secret).
var voiceOffered atomic.Bool

// SetVoiceOffered makes the world entry offer voice, or not.
func SetVoiceOffered(on bool) { voiceOffered.Store(on) }

// buildVoiceOffer is OpcodeVoiceOffer from the server: a u32, 1.
func buildVoiceOffer() []byte {
	return build(protocol.OpcodeVoiceOffer, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendVoiceOffer offers voice to the session's client when this server has
// it.
func SendVoiceOffer(sess *session.Session) {
	if !voiceOffered.Load() {
		return
	}
	queueReliable(sess, protocol.OpcodeVoiceOffer, buildVoiceOffer())
}

// VoiceHandler takes a client's voice flags, its signals and its lobby ops.
type VoiceHandler struct {
	world *entity.World
	now   func() time.Time
}

// NewVoiceHandler creates the voice handler.
func NewVoiceHandler(w *entity.World) *VoiceHandler { return &VoiceHandler{world: w, now: time.Now} }

// RegisterHandlers registers the voice handlers.
func (h *VoiceHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeVoiceOffer, h.HandleVoiceOn)
	r.Register(protocol.OpcodeVoiceSignal, h.HandleVoiceSignal)
	r.Register(protocol.OpcodeVoiceRoom, h.HandleVoiceRoom)
}

// HandleVoiceOn makes the client's answer its player's: a u32 of flags, bit 0
// voice on, 0 off. Its ICE servers and roster follow (World.SetVoiceFlags).
func (h *VoiceHandler) HandleVoiceOn(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("Voice: short on/off (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	flags := binary.LittleEndian.Uint32(data[:4])
	if !h.world.VoiceEnabled() {
		return nil
	}
	if !h.world.SetVoiceFlags(sess, flags, h.now()) {
		log.Printf("Voice: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	log.Printf("Voice: %s %s", sess.RemoteAddr(), map[bool]string{true: "on", false: "off"}[flags&entity.VoiceFlagOn != 0])
	return nil
}

// HandleVoiceSignal hands a client's signal part to the forwarding server
// (World.VoiceSignalIn), or drops it.
func (h *VoiceHandler) HandleVoiceSignal(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil {
		return nil
	}
	h.world.VoiceSignalIn(sess, msg.Data(), h.now())
	return nil
}

// HandleVoiceRoom is a client's lobby op: join, leave or list
// (World.VoiceRoomOp).
func (h *VoiceHandler) HandleVoiceRoom(sess *session.Session, msg *protocol.Message) error {
	if h.world == nil {
		return nil
	}
	h.world.VoiceRoomOp(sess, msg.Data(), h.now())
	return nil
}
