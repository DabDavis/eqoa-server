package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// readycheck.go — the ready check's messages (the GM Jibrish's petition 1711; the rules
// are the World's, entity/readycheck.go): the offer, the client's answer to it and its
// window's reply (protocol.OpcodeReadyCheckOffer..OpcodeReadyCheckReply). The typed
// /readycheck, /ready and /notready (and a PS2's "/t ..." spellings) are raidcmds.go's.

// buildReadyCheckOffer is protocol.OpcodeReadyCheckOffer: a u8, 1.
func buildReadyCheckOffer() []byte {
	return build(protocol.OpcodeReadyCheckOffer, 3, func(w *buffer.Writer) { w.WriteByte(1) })
}

// SendReadyCheckOffer offers the session's client the ready check's window: one that
// shows it answers 0x7E4A, a PS2 ignores it and is told in a line.
func SendReadyCheckOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeReadyCheckOffer, buildReadyCheckOffer())
}

// handleReadyAnswer is 0x7E4A: a u8, nonzero for the window. Nothing is answered.
func (h *RaidHandler) handleReadyAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.gameWorld == nil {
		return nil
	}
	if !h.gameWorld.SetReadyWindows(sess, data[0] != 0) {
		log.Printf("ReadyCheck: %s has no player in the world", sess.RemoteAddr())
	}
	return nil
}

// handleReadyReply is 0x7E56: u32 check id, u8 1 ready, 0 not.
func (h *RaidHandler) handleReadyReply(sess *session.Session, msg *protocol.Message) error {
	r := buffer.NewReader(msg.Data())
	id, e1 := r.ReadUint32()
	yes, e2 := r.ReadByte()
	if e1 != nil || e2 != nil || id == 0 || h.gameWorld == nil {
		return nil
	}
	h.gameWorld.ReadyAnswer(sess, id, yes != 0, false)
	return nil
}
