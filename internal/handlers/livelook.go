package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// SOE's live game's look (protocol.OpcodeLiveLook, entity/livelook.go): the
// live CLIENT's 201-byte entity channel and 0x002C record, which carry a
// colour per armour slot, held item and the robe, for a client that reads
// them (eqoa-rust-client-vanilla); every other client is sent the disc's.
//
// After a session's first character list the server offers it (0x7E0C, a
// u32 1). A client that reads the live look answers 1 (0 to go back); the
// server says what is in force (0x7E0D, a u32) and, while the client is
// still choosing a character, sends its list again in that look. A PS2
// ignores the offer (CLIENT's dispatcher has no default case), and a server
// that never offers it (Sandstorm) is never asked. The offer goes after the
// list so a PS2's list keeps its sequence number.

// buildLiveLookOffer is OpcodeLiveLook from the server: a u32, 1.
func buildLiveLookOffer() []byte {
	return build(protocol.OpcodeLiveLook, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// buildLiveLookInForce is OpcodeLiveLookInForce: 1 the live game's look, 0
// the disc's.
func buildLiveLookInForce(on bool) []byte {
	return build(protocol.OpcodeLiveLookInForce, 6, func(w *buffer.Writer) {
		v := uint32(0)
		if on {
			v = 1
		}
		w.WriteUint32(v)
	})
}

// SendLiveLookOffer offers the session's client the live game's look, once
// a session.
func SendLiveLookOffer(sess *session.Session) {
	if sess.OfferLiveLookOnce() {
		queueReliable(sess, protocol.OpcodeLiveLook, buildLiveLookOffer())
	}
}

// LiveLookHandler takes a client's answer to the offer.
type LiveLookHandler struct {
	chars *CharacterHandler
}

// NewLiveLookHandler creates the live look's handler; chars sends a list
// again.
func NewLiveLookHandler(chars *CharacterHandler) *LiveLookHandler {
	return &LiveLookHandler{chars: chars}
}

// RegisterHandlers registers the live look's handler.
func (h *LiveLookHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeLiveLook, h.HandleLiveLook)
}

// HandleLiveLook makes the client's answer its session's look: a u32, bit 0
// the live game's. It says what is now in force, and sends the list again in
// it to a client still choosing a character.
func (h *LiveLookHandler) HandleLiveLook(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("LiveLook: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4])&1 != 0
	was := sess.LiveLook()
	sess.SetLiveLook(on)
	queueReliable(sess, protocol.OpcodeLiveLookInForce, buildLiveLookInForce(on))
	log.Printf("LiveLook: %s reads %s look", sess.RemoteAddr(), map[bool]string{true: "the live game's", false: "the disc's"}[on])
	if on != was && h.chars != nil && sess.State() == session.StateCharacterSelect && !sess.CharacterInWorld() {
		h.chars.SendCharacterSelectEQOA(sess)
	}
	return nil
}

// listGear is a character's 0x002C gear as sess's client reads it: the
// disc's 32 bytes, or the live game's 71 with its dyes.
func (h *CharacterHandler) listGear(sess *session.Session, serverID uint32, disc []byte) []byte {
	if !sess.LiveLook() {
		return disc
	}
	var d entity.Dyes
	if h.store != nil && h.store.DB() != nil {
		if got, err := h.store.DB().CharDyes(serverID); err != nil {
			log.Printf("SendCharacterSelectEQOA: character %d's dyes: %v", serverID, err)
		} else {
			d = entity.Dyes(got)
		}
	}
	return entity.LiveGear(disc, &d)
}

// loadDyes puts a character's dyes (table chardyes) on its player at a
// login.
func (h *CharacterHandler) loadDyes(player *entity.Player, serverID uint32) {
	if player == nil || h.store == nil || h.store.DB() == nil {
		return
	}
	got, err := h.store.DB().CharDyes(serverID)
	if err != nil {
		log.Printf("SelectedChar: character %d's dyes: %v", serverID, err)
		return
	}
	player.Dyes = entity.Dyes(got)
	n := 0
	for _, c := range got {
		if c != 0 {
			n++
		}
	}
	if n > 0 {
		log.Printf("SelectedChar: '%s' wears %d dyes", player.Name, n)
	}
}

// dyeStore is what #dye writes through (database.SQLiteDB).
type dyeStore interface {
	CharDyes(serverID uint32) ([database.DyeSlots]uint32, error)
	SetCharDye(serverID uint32, slot int, color uint32, who database.GMEdit, detail string) (uint32, int64, error)
}
