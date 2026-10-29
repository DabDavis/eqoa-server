package entity

import (
	"math"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What a duel sends (duel.go): to a client with the live window (Player.
// DuelWindow) SOE's own 0x1502, as eqoa-live-1500's CLIENT reads it; to any
// other a 0x0034 box.
//
// 0x1502 is {u8 sub, u32 duel id, ...} (the duel part's Recv, live
// 0x012E4988, sub 1..15 off the table at 0x013EC520; every sub-op reads the
// id into the part's +0x08 first). The ones a duel here sends:
//
//	1  -                                          the challenger's duel id (0x012E4FA8)
//	2  u8 race, u8 class, u8 level, name (<=23),
//	   wide guild (<=63), wide title (<=63)       an invitation, the id its own (0x012E4B08)
//	3  -                                          the invitation gone: "The duel invitation has expired.", window closed
//	4  -                                          "Your duel invitation has been declined."
//	5  name (<=23)                                "%s declined your duel invitation."
//	6  -                                          accepted: the challenger declines its own queue
//	7  u8 on, u8 set, u32 a, u32 b                the duel's state; IsDueling(id) = on && (id == a || id == b)
//	8  VIVect3 centre, u8 world                   the compass patch at the duel's centre
//
//	12 u32 mine, u32 theirs, u8 acc, u8 acc     the wager's tunar and accepts
//	13 -                                          the wager window opened (VIWndGame mode 44)
//	14 -                                          the wager window closed
//	15 u8 set, u8 acc, u8 acc                     the accepts; both set, the window closes
//
// 10 and 11 (an item staked and taken back, with its record) are not sent:
// only tunar is wagered here (duelpurse.go). A string is VIMemStream's: a u32
// count of bytes (wide: of UTF-16 units), then them.

// The 0x1502 sub-ops sent.
const (
	duelSubID           = 1
	duelSubInvite       = 2
	duelSubGone         = 3
	duelSubNoName       = 4
	duelSubDeclined     = 5
	duelSubAccepted     = 6
	duelSubState        = 7
	duelSubPlace        = 8
	duelSubWagerTunar   = 12 // u32 mine, u32 theirs, u8 my accept, u8 theirs
	duelSubWagerOpen    = 13 // the wager window opened, the inventory copied
	duelSubWagerClose   = 14 // the wager window closed
	duelSubWagerAccepts = 15 // u8 set, u8 my accept, u8 theirs (both: closed)
)

// DuelFeatures is the live client's server feature word (opcode 0x07F5 into
// VIClient+0x813C; 0x1B in both retail states) with the duel bit alone: its
// Challenge and Surrender send nothing without bit 0x10.
const DuelFeatures = 0x10

// duelBoxMark marks a duel's box (0x0034's u32, echoed by 0x0035): an
// invitation's id in the low 24 bits, as the quests' (0x51) and the
// triggers' (0x52) boxes are marked.
const duelBoxMark = 0x53000000

// DuelCancelBox is "Cancel duel?"'s box.
const DuelCancelBox = duelBoxMark | duelIDMask

// DuelBoxID is an invitation's box id.
func DuelBoxID(id uint32) uint32 { return duelBoxMark | id&duelIDMask }

// IsDuelBox reports a 0x0035's id as a duel box's.
func IsDuelBox(id uint32) bool { return id&0xFF000000 == duelBoxMark }

// duelString8 writes s as ReadString(char *, max) reads one, cut to max-1.
func duelString8(w *buffer.Writer, s string, max int) {
	b := []byte(s)
	if len(b) > max-1 {
		b = b[:max-1]
	}
	w.WriteUint32(uint32(len(b)))
	w.WriteBytes(b)
}

// duelWide writes s as ReadString(unsigned short *, maxBytes) reads one, cut
// to maxBytes/2-1 units.
func duelWide(w *buffer.Writer, s string, maxBytes int) {
	u := utf16.Encode([]rune(s))
	if n := maxBytes/2 - 1; len(u) > n {
		u = u[:n]
		if k := len(u); k > 0 && u[k-1] >= 0xD800 && u[k-1] < 0xDC00 {
			u = u[:k-1]
		}
	}
	w.WriteUint32(uint32(len(u)))
	for _, c := range u {
		w.WriteUint16(c)
	}
}

// BuildDuel is a 0x1502: the sub-op, the duel id, then fill's.
func BuildDuel(sub byte, id uint32, fill func(*buffer.Writer)) []byte {
	buf := make([]byte, 512)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeDuel))
	w.WriteByte(sub)
	w.WriteUint32(id)
	if fill != nil {
		fill(w)
	}
	return buf[:w.Position()]
}

func queueDuel(sess *session.Session, payload []byte) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeDuel)
	msg.SetData(payload)
	sess.Queue().AddWithSequence(msg)
}

func (w *World) sendDuelIDLocked(sess *session.Session, id uint32) {
	queueDuel(sess, BuildDuel(duelSubID, id, nil))
}

// sendDuelInviteLocked is the invitation to the challenged's live window:
// the challenger's race, class, level, name, guild and title (the window's
// text: "Guild:", "Race:", "Class:" the title in place of the class word,
// "Level:").
func (w *World) sendDuelInviteLocked(sess *session.Session, id uint32, from *Player, title string) {
	queueDuel(sess, BuildDuel(duelSubInvite, id, func(wr *buffer.Writer) {
		wr.WriteByte(byte(from.Race))
		wr.WriteByte(byte(from.Class))
		wr.WriteByte(byte(from.Level))
		duelString8(wr, from.Name, 24)
		duelWide(wr, from.GuildName, 128)
		duelWide(wr, title, 128)
	}))
}

func (w *World) sendDuelGoneLocked(sess *session.Session, id uint32) {
	queueDuel(sess, BuildDuel(duelSubGone, id, nil))
}

func (w *World) sendDuelNoNameLocked(sess *session.Session, id uint32) {
	queueDuel(sess, BuildDuel(duelSubNoName, id, nil))
}

func (w *World) sendDuelDeclinedLocked(sess *session.Session, id uint32, name string) {
	queueDuel(sess, BuildDuel(duelSubDeclined, id, func(wr *buffer.Writer) { duelString8(wr, name, 24) }))
}

func (w *World) sendDuelAcceptedLocked(sess *session.Session, id uint32) {
	queueDuel(sess, BuildDuel(duelSubAccepted, id, nil))
}

// duelIDsLocked is the opponent's and its pet's object ids as p's client
// knows them (0xFFFFFFFF none: not in its view, no pet, a pet out).
func (w *World) duelIDsLocked(sess *session.Session, d *Duel, p *Player) [2]uint32 {
	ids := [2]uint32{0xFFFFFFFF, 0xFFFFFFFF}
	foe := d.other(p.ServerID)
	if id := w.objectIDSeenByLocked(sess, foe); id != 0 {
		ids[0] = id
	}
	if ps := w.pets[foe]; ps != nil && !d.petOut[foe] {
		if id := w.objectIDSeenByLocked(sess, ps.npc.ServerID); id != 0 {
			ids[1] = id
		}
	}
	return ids
}

// sendDuelStateLocked is sub 7 for a fight on: whom p's window may attack.
func (w *World) sendDuelStateLocked(sess *session.Session, d *Duel, p *Player) {
	ids := w.duelIDsLocked(sess, d, p)
	d.sent[d.side(p.ServerID)] = ids
	queueDuel(sess, BuildDuel(duelSubState, d.ID, func(wr *buffer.Writer) {
		wr.WriteByte(1)
		wr.WriteByte(1)
		wr.WriteUint32(ids[0])
		wr.WriteUint32(ids[1])
	}))
}

// refreshDuelStateLocked sends sub 7 again when the ids p's client knows its
// opponent or the pet by changed (one out of view and back, a pet summoned).
func (w *World) refreshDuelStateLocked(sess *session.Session, d *Duel, p *Player) {
	if w.duelIDsLocked(sess, d, p) != d.sent[d.side(p.ServerID)] {
		w.sendDuelStateLocked(sess, d, p)
	}
}

// sendDuelEndLocked is sub 7 for a fight over: nobody to attack.
func (w *World) sendDuelEndLocked(sess *session.Session, d *Duel) {
	queueDuel(sess, BuildDuel(duelSubState, d.ID, func(wr *buffer.Writer) {
		wr.WriteByte(0)
		wr.WriteByte(0)
		wr.WriteUint32(0xFFFFFFFF)
		wr.WriteUint32(0xFFFFFFFF)
	}))
}

// sendDuelPlaceLocked is sub 8: the duel's centre and world, for the compass.
func (w *World) sendDuelPlaceLocked(sess *session.Session, d *Duel) {
	queueDuel(sess, BuildDuel(duelSubPlace, d.ID, func(wr *buffer.Writer) {
		wr.WriteUint32(math.Float32bits(d.X))
		wr.WriteUint32(math.Float32bits(d.Y))
		wr.WriteUint32(math.Float32bits(d.Z))
		wr.WriteByte(d.World)
	}))
}

// BuildDuelBox is a 0x0034 (Msg0034, 0x00BDBD58): the box id the answer
// echoes, the question (at most 127 characters) and its answers (at most 63
// each), as handlers' buildQuestPopupMulti writes it.
func BuildDuelBox(id uint32, question string, options []string) []byte {
	buf := make([]byte, 2+4+4+254+1+len(options)*(4+126))
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeQuestPopupMulti))
	w.WriteUint32(id)
	duelWide(w, question, 256)
	w.WriteByte(byte(len(options)))
	for _, o := range options {
		duelWide(w, o, 128)
	}
	return buf[:w.Position()]
}

func (w *World) sendDuelBoxLocked(sess *session.Session, id uint32, question string, options []string) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeQuestPopupMulti)
	msg.SetData(BuildDuelBox(id, question, options))
	sess.Queue().AddWithSequence(msg)
}
