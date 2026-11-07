package handlers

// fly.go — #fly, a GM's flight (entity/fly.go; the user, 2026-11-07: "lets add a
// gm fly command its like swimming you can look up/down to go in that direction,
// but running, standing like normal and its a on/off toggle"). The client flies
// (vanilla's walker.rs); the server turns it on and off with protocol.OpcodeFly,
// and only for a client that said it can: a PS2's CLIENT has no such walk, and
// is told so in a line and sent nothing.

import (
	"encoding/binary"
	"log"
	"strings"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What #fly answers.
const (
	flyOnLine  = "Fly on: run where you look, up and down too, no gravity (#fly off to come down)"
	flyOffLine = "Fly off: you fall and walk as normal from where you are"
	// a client that never said it flies: a PS2, an older build, or a session
	// that was no GM's when it logged in (the client says so to the GM channel)
	flyCannotLine = "Fly stays off: this client cannot fly (a PS2 or an older build; made a GM just now? log in again)"
)

// buildFly is protocol.OpcodeFly from the server: a u32, 1 on and 0 off.
func buildFly(on bool) []byte {
	return build(protocol.OpcodeFly, 6, func(w *buffer.Writer) {
		if on {
			w.WriteUint32(1)
		} else {
			w.WriteUint32(0)
		}
	})
}

// SendFlying tells sess's client whether it flies, a client that said it can
// alone. It reports whether it told it.
func SendFlying(sess *session.Session, on bool) bool {
	if !sess.FlyClient() {
		return false
	}
	queueReliable(sess, protocol.OpcodeFly, buildFly(on))
	return true
}

// flyCommand is #fly (a GM's: handleGMCommand gates it): alone it turns the
// flight over, `on` and `off` say which. The line says where it stands either
// way.
func (h *ChatHandler) flyCommand(sess *session.Session, args []string) {
	on, ok := h.gameWorld.Flying(sess)
	if !ok {
		return
	}
	want := !on
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "on":
			want = true
		case "off":
			want = false
		default:
			h.gmReply(sess, "Usage: #fly [on|off] (alone: on, or off again)")
			return
		}
	}
	if !sess.FlyClient() {
		h.gmReply(sess, flyCannotLine)
		return
	}
	if !h.gameWorld.SetFlying(sess, want, time.Now()) {
		return
	}
	SendFlying(sess, want)
	if want {
		h.gmReply(sess, flyOnLine)
	} else {
		h.gmReply(sess, flyOffLine)
	}
}

// FlyHandler takes a client's protocol.OpcodeFly: that it flies.
type FlyHandler struct{}

// NewFlyHandler creates the handler.
func NewFlyHandler() *FlyHandler { return &FlyHandler{} }

// RegisterHandlers registers the opcode.
func (h *FlyHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeFly, h.HandleFly)
}

// HandleFly keeps a client's word that it flies: what #fly may send it, and
// nothing more (only #fly turns flight on, and only a GM's).
func (h *FlyHandler) HandleFly(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	sess.SetFlyClient(on)
	log.Printf("Fly: %s %s", sess.RemoteAddr(), map[bool]string{true: "flies", false: "does not fly"}[on])
	return nil
}
