package handlers

import (
	"encoding/binary"
	"log"
	"slices"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// selectinfo.go — more of each character at select, for a client that shows it
// (protocol.OpcodeSelectInfo, OpcodeSelectInfoData; the owner, 2026-11-12: "in
// char sel screen where it says Level 1 / Alchemist, let it be Level 1 / 0 CMs /
// Ratman / Alchemist", and "also add color customize to wisp char sel").
//
// The character list (0x002C) carries a character's wire race, the game's race a
// race of racedefs passes as (entity.WireRace), and nothing of its masteries. So
// after a session's first list the server offers more (a u32, 1); a client that
// takes it answers 1 and is sent, at once and after every list, each listed
// character's own race (its id and racedefs' name), its mastery points (spent and
// unspent: what it has earned, charmastery's points + spent) and the race option
// rows its Customize re-chooses (entity.SelectRows). Version 2 also includes the
// stored sex and custom appearance values for previewing variants.
// A PS2 ignores the offer and is sent none.

const selectInfoAppearanceVersion uint8 = 2

// selectInfo is one character's entry.
type selectInfo struct {
	serverID uint32
	race     int
	name     string // racedefs' name; empty for a game race
	cms      int
	rows     []string
	sex      uint8
	looks    []database.CharLook
}

// buildSelectInfoOffer is OpcodeSelectInfo from the server: a u32, 1.
func buildSelectInfoOffer() []byte {
	return build(protocol.OpcodeSelectInfo, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendSelectInfoOffer offers the session's client the select info, once a
// session (after its first character list).
func SendSelectInfoOffer(sess *session.Session) {
	if sess.OfferSelectInfoOnce() {
		queueReliable(sess, protocol.OpcodeSelectInfo, buildSelectInfoOffer())
	}
}

// selectRaceName is the name a character of race is shown by at select: a
// race of racedefs' own (or the froglok's), empty for one of the game's ten,
// which the client names in its own words.
func selectRaceName(race int) string {
	if race >= 0 && race <= 9 {
		return ""
	}
	return entity.RaceName(race)
}

// selectInfoOf is records' entries: their races, their mastery points
// (charmastery's, 0 for a character with no row) and the rows select
// re-chooses.
func (h *CharacterHandler) selectInfoOf(records []*database.CharacterRecord, extended bool) []selectInfo {
	out := make([]selectInfo, 0, len(records))
	for _, c := range records {
		e := selectInfo{serverID: uint32(c.ServerID), race: c.Race, name: selectRaceName(c.Race),
			rows: entity.SelectRows(c.Race, c.Sex, extended)}
		if c.Sex == 1 {
			e.sex = 1
		}
		if h.store != nil {
			if m, err := h.store.LoadMastery(uint32(c.ServerID)); err == nil {
				e.cms = m.Points + m.Spent
			} else {
				log.Printf("SelectInfo: %s's masteries: %v", c.CharName, err)
			}
			if extended && c.Race >= 10 && len(e.rows) > 0 {
				if looks, err := h.store.DB().CharLooks(e.serverID); err == nil {
					e.looks = slices.DeleteFunc(looks, func(l database.CharLook) bool { return !slices.Contains(e.rows, l.Part) })
				} else {
					log.Printf("SelectInfo: %s's appearance: %v", c.CharName, err)
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// encodeSelectInfo is OpcodeSelectInfoData's body (the opcode's doc): the
// first 65,535 entries, a name and a key cut at 255 bytes, the points at
// 65,535, the rows at 255.
func encodeSelectInfo(list []selectInfo, version uint8) []byte {
	if len(list) > 65535 {
		list = list[:65535]
	}
	b := binary.LittleEndian.AppendUint16([]byte{version}, uint16(len(list)))
	for _, e := range list {
		b = binary.LittleEndian.AppendUint32(b, e.serverID)
		b = append(b, byte(max(0, min(255, e.race))))
		b = appendStr8(b, e.name)
		b = binary.LittleEndian.AppendUint16(b, uint16(max(0, min(65535, e.cms))))
		rows := e.rows
		if len(rows) > 255 {
			rows = rows[:255]
		}
		b = append(b, byte(len(rows)))
		for _, k := range rows {
			b = appendStr8(b, k)
		}
		if version >= selectInfoAppearanceVersion {
			looks := e.looks[:min(len(e.looks), 255)]
			b = append(b, e.sex, byte(len(looks)))
			for _, l := range looks {
				b = appendStr8(b, l.Part)
				b = binary.LittleEndian.AppendUint32(b, l.Value)
			}
		}
	}
	return b
}

// sendSelectInfo sends a client that took the offer the select info of a
// list's characters; nothing to any other.
func (h *CharacterHandler) sendSelectInfo(sess *session.Session, records []*database.CharacterRecord) {
	version := sess.SelectInfoVersion()
	if version == 0 {
		return
	}
	body := encodeSelectInfo(h.selectInfoOf(records, version >= selectInfoAppearanceVersion), version)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeSelectInfoData)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeSelectInfoData)), body...))
	sess.Queue().AddWithSequence(msg)
}

// SelectInfoHandler takes a client's answer to the select info's offer.
type SelectInfoHandler struct {
	chars *CharacterHandler
}

// NewSelectInfoHandler creates the select info's handler; chars reads the
// account's characters.
func NewSelectInfoHandler(chars *CharacterHandler) *SelectInfoHandler {
	return &SelectInfoHandler{chars: chars}
}

// RegisterHandlers registers it.
func (h *SelectInfoHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeSelectInfo, h.HandleSelectInfo)
}

// HandleSelectInfo takes the client's answer: a u32 version, or zero to stop.
func (h *SelectInfoHandler) HandleSelectInfo(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("SelectInfo: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	version := uint8(min(binary.LittleEndian.Uint32(data[:4]), uint32(selectInfoAppearanceVersion)))
	if version != 0 && !sess.SelectInfoOffered() {
		log.Printf("SelectInfo: %s asked for it, which was not offered", sess.RemoteAddr())
		return nil
	}
	sess.SetSelectInfo(version)
	if version == 0 || h.chars == nil || h.chars.store == nil || sess.AccountID() <= 0 {
		return nil
	}
	if sess.State() != session.StateCharacterSelect || sess.CharacterInWorld() {
		return nil
	}
	records, err := h.chars.store.GetByAccountID(sess.AccountID())
	if err != nil {
		log.Printf("SelectInfo: account %d's characters: %v", sess.AccountID(), err)
		return nil
	}
	h.chars.sendSelectInfo(sess, h.chars.sessionChars(sess, records))
	return nil
}
