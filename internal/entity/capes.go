package entity

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// capes.go — a cape, an equip slot of this server's own (the user, 2026-11-08: "lets add a new
// equip slot for heirloom server only. capes for all. there are many capes we can borrow from for
// various styles."; world.capes, handlers/capes.go).
//
// The game has 22 equip slots and no cape: CLIENT's fit check (handlers/equip.go fitsEquip) knows
// no wear slot past 27, and neither its look nor its entity channel has a place for one. A cape is
// an item of wear slot 28 worn in equip slot 22; its record's model is the style a client draws
// (vanilla cuts each style out of a body on the disc) and its colour a tint (0xRRGGBBAA, 0 the
// art's own). A client that draws capes asks for them (protocol.OpcodeCapes) and is told, in the
// world, which of the players in its view wear which (OpcodeCapesInView, here), and at character
// select which of its characters do (OpcodeCapesList, handlers/capes.go). The entity channels
// carry none of it: every client reads them, a PS2 by the disc's 162 bytes. A session that never
// asked (every PS2) is sent nothing here.

// capesOn is world.capes: whether this server has the cape slot at all.
var capesOn atomic.Bool

// SetCapes turns the cape slot on or off: at the start, from config.
func SetCapes(on bool) { capesOn.Store(on) }

// CapesOn reports whether this server has the cape slot (world.capes).
func CapesOn() bool { return capesOn.Load() }

// CapeTint is the colour the player's cape is drawn in: its dye where it has
// one (an Armor Dyer's, #dye's: chardyes' cape slot, database.DyeCape), else
// its record's own colour (CapeColor, 0 the art's own).
func (p *Player) CapeTint() uint32 {
	if c := p.Dyes[database.DyeCape]; c != 0 {
		return c
	}
	return p.CapeColor
}

// capeWorn is one entry of OpcodeCapesInView: the id the client knows the
// wearer by, the style and the colour.
type capeWorn struct {
	id    uint32
	style uint8
	color uint32
}

// ResendCapes makes the next pass send the session's client its capes in view
// whatever it was sent before: its scene is new (a MemoryDump, a world
// change), or it has just asked for them. Safe without the world lock.
func (vs *VisibilityState) ResendCapes() {
	if vs != nil {
		vs.capesDue.Store(true)
	}
}

// capesInViewLocked is the capes me's client sees: its own first (by its own
// object id), then each player on one of its entity slots, by the slot's id,
// in slot order. An invisible player's is left out of another's view (its
// body is hidden there, and a cape drawn alone would show where it stands),
// not of its own; but a Wisp's in its own body (its own form, its Raiment),
// which shows as what hangs on an invisible body whether invisible or not:
// the wisp in its helm shows where it stands anyway, and its cape with it
// (the owner, 2026-11-11: "let the capes be visible on wisp players"; an
// invisible Wisp had shown its wisp alone). Must be called with the lock held.
func (w *World) capesInViewLocked(sess *session.Session, me *Player) []capeWorn {
	var out []capeWorn
	if me.CapeStyle != 0 {
		out = append(out, capeWorn{sess.PlayerObjectID(), me.CapeStyle, me.CapeTint()})
	}
	vs, _ := sess.VisibilityState().(*VisibilityState)
	if vs == nil {
		return out
	}
	for i, id := range vs.ChannelEntity {
		if id == 0 || i >= len(vs.ChannelObjectID) {
			continue
		}
		p := w.Players[id]
		if p == nil || p.CapeStyle == 0 || p.Invisible() && !p.wispOwnBody() {
			continue
		}
		out = append(out, capeWorn{vs.ChannelObjectID[i], p.CapeStyle, p.CapeTint()})
	}
	return out
}

// encodeCapesInView is OpcodeCapesInView's body: a u8 count, then each
// entry's u32 id, u8 style and u32 colour, little-endian (the first 255).
func encodeCapesInView(capes []capeWorn) []byte {
	if len(capes) > 255 {
		capes = capes[:255]
	}
	out := make([]byte, 1, 1+9*len(capes))
	out[0] = byte(len(capes))
	for _, c := range capes {
		out = binary.LittleEndian.AppendUint32(out, c.id)
		out = append(out, c.style)
		out = binary.LittleEndian.AppendUint32(out, c.color)
	}
	return out
}

// capesMessage is op with body, a message of its own size (a full view's set
// is past a default message's 1,300 bytes, and comes fragmented).
func capesMessage(op protocol.GameOpcode, body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(op)), body...))
	return msg
}

// capesSyncSystem sends each client that draws capes the set its view shows
// (capesInViewLocked) when it changed, or when it is due again (ResendCapes).
// Every tick, after the visibility system has put this tick's entities on
// their slots, so a wearer's channel and its cape go out together. A session
// that never asked for capes (a PS2's) is skipped, and on a server without
// them nothing runs. Must be called with the lock held.
func (w *World) capesSyncSystem() {
	if !CapesOn() || w.SessionManager == nil {
		return
	}
	w.SessionManager.ForEach(func(s *session.Session) bool {
		if !s.Capes() || !s.CharacterInWorld() {
			return true
		}
		p := w.sessionPlayerLocked(s)
		if p == nil {
			return true
		}
		vs := GetOrCreateVisibility(s)
		body := encodeCapesInView(w.capesInViewLocked(s, p))
		if !vs.capesDue.Swap(false) && string(body) == vs.capesSent {
			return true
		}
		vs.capesSent = string(body)
		s.Queue().AddWithSequence(capesMessage(protocol.OpcodeCapesInView, body))
		return true
	})
}
