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

// A monster's name and level when talked to (world.mob_info; the GM Jibrish's
// petition 1459, 2026-11-10: "For the Heirloom server: Add the ability to press
// the mob information button on monster (Q by default on keyboard). This should
// ONLY show the monsters name and level, nothing else.").
//
// The button is □, vanilla's Q. CLIENT's InteractWithTarget (0x00BD5B68) opens
// a merchant's, a bank's or a blacksmith's window by the target's npc flags,
// opens its own details box for a player or a pet (VIWndGame::m_p678, drawn by
// DrawTargetInfo 0x00C51F10 while □ is held, OnKeyUp 133 closing it: a
// player's name, guild, race, class or title and level; a pet's name and
// "Level N" alone), and talks to anything else with 0x0004, which for a
// creature with nothing to say answered nothing (F3 20261025-171528). With the
// switch on, a monster's 0x0004 is answered with the pet's two lines and
// nothing else (no class, hit points, faction or con):
//
//   - to a client that answered OpcodeMobInfoOffer (vanilla) as OpcodeMobInfo,
//     which it shows in that details box as a pet's, while □ is held;
//   - to any other (a PS2, an older vanilla) as a dialogue box (0x0046, the
//     case at 0x00BD1A4C: ShowDialog queues it, "\n" a line, a '%' doubled
//     for its text expander), the same lines.
//
// A monster is an NPC without the NPC bit (0x80: CLIENT's ToggleAutoAttack
// refuses to attack a target that has it), alive (a corpse's talk is its
// loot). Whatever makes a talk do something keeps doing it: in reach this is
// HandleInteract's last answer, after every other. Out of reach, where a talk
// does nothing, a monster is answered too: it is looked at, not talked to, as
// CLIENT's details box shows a player at any distance. Nothing else changes:
// no menu's question is forgotten, nothing is said aloud.

// mobInfoOn is world.mob_info: off unless set.
var mobInfoOn atomic.Bool

// SetMobInfo turns the monsters' name and level on or off (world.mob_info).
func SetMobInfo(on bool) { mobInfoOn.Store(on) }

// MobInfoOn reports world.mob_info.
func MobInfoOn() bool { return mobInfoOn.Load() }

// mobInfoLayout is the layout of OpcodeMobInfo the offer names.
const mobInfoLayout = 1

// mobInfoNameMost is the most characters of a name shown, what OpcodeMobInfo's
// count allows (no NPC's name is near it; a PS2's box holds far more).
const mobInfoNameMost = 63

// buildMobInfoOffer is OpcodeMobInfoOffer from the server: a u8, the layout.
func buildMobInfoOffer() []byte {
	return build(protocol.OpcodeMobInfoOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(mobInfoLayout)
	})
}

// SendMobInfoOffer offers the session's client a monster's name and level in
// its own details box, where world.mob_info is on; elsewhere nothing goes.
func SendMobInfoOffer(sess *session.Session) {
	if !MobInfoOn() {
		return
	}
	queueReliable(sess, protocol.OpcodeMobInfoOffer, buildMobInfoOffer())
}

// mobInfoOf is what a monster shows: its name as players see it (a source's
// trailing '$' off, as its town line's speaker has it, and only what a PS2 can
// type: consoletext.go) and its level, the one the con colour reads
// (target.go). ok is false for an NPC that is no monster: one with the NPC
// bit, or dead.
func mobInfoOf(npc *entity.NPC) (name string, level int, ok bool) {
	if npc == nil || npc.Dead || npc.NPCType&0x80 != 0 {
		return "", 0, false
	}
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(npc.Name), "$"))
	return consoleLine(name, mobInfoNameMost), npc.Level, true
}

// mobInfoLines is the two lines as CLIENT's details box draws a pet's: the
// name, then "Level" (word 503) and the level.
func mobInfoLines(name string, level int) string {
	return name + "\nLevel " + strconv.Itoa(level)
}

// buildMobInfo is OpcodeMobInfo: the layout, the id the client knows the
// monster by, its level (a byte: none is past 255) and its name (mobInfoOf's,
// ASCII, so its bytes are its characters).
func buildMobInfo(objectID uint32, name string, level int) []byte {
	if len(name) > mobInfoNameMost {
		name = name[:mobInfoNameMost]
	}
	level = max(0, min(level, 255))
	return build(protocol.OpcodeMobInfo, 2+1+4+1+1+len(name), func(w *buffer.Writer) {
		w.WriteByte(mobInfoLayout)
		w.WriteUint32(objectID)
		w.WriteByte(byte(level))
		w.WriteByte(byte(len(name)))
		w.WriteBytes([]byte(name))
	})
}

// showMobInfo answers player's talk to npc, which the session's client knows
// by objectID, with the monster's name and level where world.mob_info is on:
// in the client's own details box when it said it has one (OpcodeMobInfo),
// else in a dialogue box. It reports whether it answered; false (the switch
// off, no monster) leaves the talk as it was.
func showMobInfo(sess *session.Session, player *entity.Player, npc *entity.NPC, objectID uint32) bool {
	if !MobInfoOn() {
		return false
	}
	name, level, ok := mobInfoOf(npc)
	if !ok {
		return false
	}
	how := "a dialogue box"
	if sess.MobInfo() {
		queueReliable(sess, protocol.OpcodeMobInfo, buildMobInfo(objectID, name, level))
		how = "its details box"
	} else {
		// a '%' doubled, which CLIENT's text expander (VIClientText::Expand)
		// shows as one (alone it would read the next character as a token: %p
		// the player's name), as a sign's words are sent (signs.go)
		SendDialogueBox(sess, 0, strings.ReplaceAll(mobInfoLines(name, level), "%", "%%"))
	}
	who := "?"
	if player != nil {
		who = player.Name
	}
	log.Printf("MobInfo: %s shown %q, level %d, in %s", who, name, level, how)
	return true
}

// MobInfoHandler takes a client's OpcodeMobInfoAnswer.
type MobInfoHandler struct{}

// NewMobInfoHandler creates the mob info's handler.
func NewMobInfoHandler() *MobInfoHandler { return &MobInfoHandler{} }

// RegisterHandlers registers the mob info's answer.
func (h *MobInfoHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeMobInfoAnswer, h.HandleMobInfoAnswer)
}

// HandleMobInfoAnswer makes the client's answer its session's: a u8, nonzero to
// be sent OpcodeMobInfo in place of the dialogue box, 0 for the box (a wider
// answer is read by its first byte). Nothing is answered.
func (h *MobInfoHandler) HandleMobInfoAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("MobInfo: short answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	sess.SetMobInfo(on)
	log.Printf("MobInfo: %s %s", sess.RemoteAddr(), map[bool]string{true: "shows a monster's in its details box", false: "takes the dialogue box"}[on])
	return nil
}
