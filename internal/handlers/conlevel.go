package handlers

import (
	"log"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// conlevel.go — a target's level, seen as its con colour is (world.con_level;
// Live petition 5130, Indulgence: "Would it be possible to con an enemy to see
// what level it is?", the owner 2026-11-15: "yes lets allow").
//
// CLIENT shows a target's con colour (the ring under it, 0x0010's con byte)
// and, held □, a player's or a pet's level, never a monster's. With the
// switch on, everyone may see it:
//
//   - a client that draws it (vanilla) is offered it at world entry
//     (protocol.OpcodeConLevelOffer) and shows the level it already has (the
//     entity channel's byte 140, which every server sends) in its target
//     box, beside the name; nothing more goes to it;
//   - anyone, a PS2 too, may type /con (or #con; a PS2's "/t con", since it
//     keeps an unknown /command to itself and cannot type a #) for one line,
//     "<name> is level N.", of its current target, player or creature.
//
// Nothing else changes: the con colour, aggro and every rule stay as they
// were. A player's level is no secret (CLIENT's own details box shows it), so
// /con on a player answers too. Off, /con is an unknown command as before
// and no offer goes.

// conLevelOn is world.con_level: off unless set.
var conLevelOn atomic.Bool

// SetConLevel turns a target's level on or off (world.con_level).
func SetConLevel(on bool) { conLevelOn.Store(on) }

// ConLevelOn reports world.con_level, which world.vanilla_ui overrides
// (vanillaui.go): there /con is an unknown command, as the game's.
func ConLevelOn() bool { return conLevelOn.Load() && QoLOn() }

// ConLevelOnFor is ConLevelOn for the session: off for an [OG] character's
// (ogaccount.go), whose /con is the game's unknown command.
func ConLevelOnFor(sess *session.Session) bool { return ConLevelOn() && QoLOnFor(sess) }

// conLevelLayout is the layout the offer names.
const conLevelLayout = 1

// The lines /con answers with.
const conNoTargetLine = "You have no target to consider."

// buildConLevelOffer is OpcodeConLevelOffer: a u8, the layout.
func buildConLevelOffer() []byte {
	return build(protocol.OpcodeConLevelOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(conLevelLayout)
	})
}

// SendConLevelOffer offers the session's client its target's level in its
// target box, where world.con_level is on; elsewhere nothing goes.
func SendConLevelOffer(sess *session.Session) {
	if !ConLevelOnFor(sess) {
		return
	}
	queueReliable(sess, protocol.OpcodeConLevelOffer, buildConLevelOffer())
}

// conLine reads a typed line as /con: "/con" or "#con" (any case) and nothing
// after it. A PS2's "/t con" is read where the tells are (chat.go).
func conLine(word, rest string) bool {
	if strings.TrimSpace(rest) != "" {
		return false
	}
	return strings.EqualFold(word, "/con") || strings.EqualFold(word, "#con")
}

// conLevelLine is /con's answer for a target named name of level.
func conLevelLine(name string, level int) string {
	return consoleLine(name, 63) + " is level " + strconv.Itoa(level) + "."
}

// conCommand is /con: the player's target's level, to the player alone.
func conCommand(w *entity.World, sess *session.Session, player *entity.Player) {
	if w == nil || player == nil {
		return
	}
	name, level, con, ok := w.Considered(sess)
	if !ok {
		SendColoredChat(sess, conNoTargetLine, 255, 255, 0)
		return
	}
	SendColoredChat(sess, conLevelLine(name, level), 255, 255, 0)
	log.Printf("Con: '%s' considers %q, level %d (con %d)", player.Name, name, level, con)
}
