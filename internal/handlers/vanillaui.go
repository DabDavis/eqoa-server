package handlers

import (
	"log"
	"sync/atomic"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// vanillaui.go — the game's own UI on a shard (world.vanilla_ui; the owner,
// 2026-11-18: "in vanilla test shard lets disable all quality of life settings
// like map, minimap, level display, dps meter, custom colors, the character
// stats showing 350/500", and "also no quest helper").
//
// Set, the server:
//
//   - offers none of its quality-of-life data at world entry: others' power
//     (powerbars.go), the pet's effects (peteffects.go), the buff window
//     (buffwindow.go), the number tags (numbertags.go), the DPS meter
//     (dpsmeter.go), a monster's name and level in the details box
//     (mobinfo.go: a vanilla client is answered with the dialogue box a PS2
//     gets), a target's level in the target box (conlevel.go), the quest
//     marks, the quest tracker and the marks over quest NPCs' heads
//     (questmarks.go, questprogress.go, questheadmarks.go); an answer to any of them is taken as "off", and a keep restart into it
//     stops what a client had asked for;
//   - answers /con (and a PS2's "/t con") as the game does, an unknown
//     command, for every client, whatever world.con_level says (the owner,
//     2026-11-18: "/con level" off on Vanilla Test);
//   - keeps offering item links (itemlinks.go) and relaying them to a client
//     that takes them: vanilla's Item Links, the map and the minimap are off
//     by DEFAULT there (QoLItemLinks, QoLMap), the player's to turn on (the
//     owner, 2026-11-18: "minimap and map can work but make it toggled off by
//     default", "item links ... can be on in vanilla test but they are
//     toggled off by default");
//   - tells a vanilla client which of its own additions are off for the
//     session (protocol.OpcodeVanillaUI, the QoL* bits), right after it
//     answers the bug report offer (which a PS2 never does: a PS2 is never
//     sent it), at every world entry and at a keep restart's resume.
//
// Unset (the default, Heirloom's), nothing changes and nothing is sent.

// vanillaUI is world.vanilla_ui: off unless set.
var vanillaUI atomic.Bool

// SetVanillaUI turns the game's own UI on or off (world.vanilla_ui).
func SetVanillaUI(on bool) { vanillaUI.Store(on) }

// VanillaUI reports world.vanilla_ui.
func VanillaUI() bool { return vanillaUI.Load() }

// QoLOn is whether this server gives its quality-of-life additions: not
// world.vanilla_ui.
func QoLOn() bool { return !vanillaUI.Load() }

// QoLOnFor is QoLOn for the session: off too for an [OG] character's on a
// shard with world.og_accounts (ogaccount.go).
func QoLOnFor(sess *session.Session) bool { return QoLOn() && (sess == nil || !sess.OG()) }

// qolAnswerFor is a client's answer to one of the quality-of-life offers as
// taken: on only where the server gives them to its session.
func qolAnswerFor(sess *session.Session, on bool) bool { return on && QoLOnFor(sess) }

// The client's additions OpcodeVanillaUI turns off, a bit each (vanilla's
// `settings::qol`, the same bits).
const (
	QoLMap         uint32 = 1 << 0  // off by default: the map (M) and minimap (N), the player's to turn on
	QoLQuestHelper uint32 = 1 << 1  // the quest guide, the quest tracker, the map's quest marks, the head marks
	QoLTargetLevel uint32 = 1 << 2  // a target's level in the target box
	QoLDPSMeter    uint32 = 1 << 3  // the DPS meter
	QoLNumberColor uint32 = 1 << 4  // Number Colors and Merc Colors: every number the game's colour
	QoLStatCaps    uint32 = 1 << 5  // Character > Summary's "346/435": the value alone, the game's
	QoLPowerBars   uint32 = 1 << 6  // others' power under their health
	QoLBuffWindow  uint32 = 1 << 7  // the buff window: the game's effects strip
	QoLPetEffects  uint32 = 1 << 8  // the pet's effects under its bar, and the group members' beside theirs
	QoLAutoCast    uint32 = 1 << 9  // auto-cast on the quick bars and auto targeting
	QoLItemLinks   uint32 = 1 << 10 // off by default: item links in chat, the player's to turn on
	QoLSpellBars   uint32 = 1 << 11 // Ability Bars 3 and 4 and Stacked Spell Bars: the game's two bars
	// An [OG] character's (ogaccount.go), never world.vanilla_ui's:
	QoLLocked  uint32 = 1 << 12 // QoLMap and QoLItemLinks LOCKED off, not off by default: no row turns them on
	QoLNoMouse uint32 = 1 << 13 // the mouse does nothing in the world (keyboard and controllers)
	QoLNoTouch uint32 = 1 << 14 // no touch pad, the touch screen does nothing in the world
	// world.follow_any_player false's, on any session, never part of QoLAllOff:
	QoLFollowGroupOnly uint32 = 1 << 15 // follow takes group members only (CLIENT's ToggleFollow, word 1195)
)

// followAnyPlayer is world.follow_any_player: on unless set false.
var followAnyPlayer atomic.Bool

func init() { followAnyPlayer.Store(true) }

// SetFollowAnyPlayer sets world.follow_any_player: on, any player can follow
// any player (the owner, 2026-11-11, Heirloom's); off, a vanilla client is
// told QoLFollowGroupOnly and follows group members only, as CLIENT's
// ToggleFollow does (Live petition 5400, Innoruuk: "Auto follow currently
// allowing to auto follow people not in group"; the owner 2026-11-21: group
// only on Live). A client never told the bit follows any player.
func SetFollowAnyPlayer(on bool) { followAnyPlayer.Store(on) }

// FollowAnyPlayer reports world.follow_any_player.
func FollowAnyPlayer() bool { return followAnyPlayer.Load() }

// QoLOGOff is what an [OG] character's session is sent: every addition off, the
// map and links locked, no mouse, no touch.
const QoLOGOff = QoLAllOff | QoLLocked | QoLNoMouse | QoLNoTouch

// QoLAllOff is every bit above: what world.vanilla_ui turns off.
const QoLAllOff = QoLMap | QoLQuestHelper | QoLTargetLevel | QoLDPSMeter | QoLNumberColor | QoLStatCaps |
	QoLPowerBars | QoLBuffWindow | QoLPetEffects | QoLAutoCast | QoLItemLinks | QoLSpellBars

// vanillaUILayout is the message's version byte.
const vanillaUILayout = 1

// buildVanillaUI is OpcodeVanillaUI: a u8 version, a u32 of the bits off.
func buildVanillaUI(off uint32) []byte {
	return build(protocol.OpcodeVanillaUI, 7, func(w *buffer.Writer) {
		w.WriteByte(vanillaUILayout)
		w.WriteUint32(off)
	})
}

// SendVanillaUI tells a vanilla client (one that answered the bug report
// offer) what is off on this server, where world.vanilla_ui is set, for an
// [OG] character, or where world.follow_any_player is false; elsewhere, and
// to any other client, nothing goes.
func SendVanillaUI(sess *session.Session) {
	off := vanillaUIBits(sess)
	if off == 0 || !sess.BugReports() {
		return
	}
	queueReliable(sess, protocol.OpcodeVanillaUI, buildVanillaUI(off))
}

// vanillaUIBits is what the session's client is told is off: an [OG]
// character's QoLOGOff (ogaccount.go), else world.vanilla_ui's QoLAllOff, else
// nothing; and QoLFollowGroupOnly with world.follow_any_player false.
func vanillaUIBits(sess *session.Session) uint32 {
	var off uint32
	switch {
	case sess.OG():
		off = QoLOGOff
	case VanillaUI():
		off = QoLAllOff
	}
	if !FollowAnyPlayer() {
		off |= QoLFollowGroupOnly
	}
	return off
}

// stopKeptQoL ends, at a keep restart's resume into world.vanilla_ui, what the
// client had asked for before it: others' power, the pet's effects, the buff
// window, the meter, the offence and defence figures, the number tags and the
// monster's details. (The quest marks, tracker and head marks are not resumed:
// keep.go.)
func stopKeptQoL(w *entity.World, sess *session.Session) {
	if QoLOnFor(sess) {
		return
	}
	if w != nil {
		w.SetPowerBars(sess, false)
		w.SetPetEffects(sess, false)
		w.SetBuffWindow(sess, false)
		w.SetDPSMeter(sess, false)
		w.SetStatFigures(sess, false) // statfigures.go
	}
	sess.SetNumberTagsLayout(0)
	sess.SetMobInfo(false)
	if sess.OG() {
		sess.SetItemLinks(false) // an [OG] character's lines plain (ogaccount.go)
	}
	log.Printf("VanillaUI: %s resumed with the game's own UI", sess.RemoteAddr())
}
