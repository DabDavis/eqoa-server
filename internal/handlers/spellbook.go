package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// SpellbookHandler takes the spell book's requests (CLIENT VIWndSpellBook)
// and answers each the way CLIENT applies it, keeping the spells table in
// step with what the client then shows:
//
//	0x0033 stamp, u8 display index, u8 slot -> 0x00B4 the same (Msg00B4:
//	       the ability toggles on that slot; any other there comes off)
//	0x003A stamp, u8 from, u8 to            -> 0x00B6 the same (Msg00B6:
//	       one display index moved to another's place, the rest shifted)
//	0x00FE stamp                            -> 0x00FF stamp (Msg00FF: every
//	       ability, display index below 252, off the bar)
//	0x0032 stamp, u8 bar slot, u32 target   -> the cast (cast.go)
type SpellbookHandler struct {
	gameWorld *entity.World
	store     *database.SpellbookStore
}

func NewSpellbookHandler(gw *entity.World, store *database.SpellbookStore) *SpellbookHandler {
	return &SpellbookHandler{gameWorld: gw, store: store}
}

func (h *SpellbookHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeAddSkillToBar, h.handleMemorize)
	r.Register(protocol.OpcodeArrangeAbility, h.handleArrange)
	r.Register(protocol.OpcodeUnmemorizeAll, h.handleUnmemorizeAll)
	r.Register(protocol.OpcodeUseSkill, h.handleUseSkill)
}

// rows reads the request's stamp and two bytes, and the character's rows.
func (h *SpellbookHandler) rows(sess *session.Session, msg *protocol.Message, two bool) (uint32, byte, byte, uint32, []database.SpellRow, bool) {
	if h.gameWorld == nil || h.store == nil {
		log.Printf("Spellbook: 0x%04X with no world or spell store", uint16(msg.Opcode))
		return 0, 0, 0, 0, nil, false
	}
	p := h.gameWorld.SessionPlayer(sess)
	if p == nil {
		log.Printf("Spellbook: 0x%04X from a session with no player", uint16(msg.Opcode))
		return 0, 0, 0, 0, nil, false
	}
	data := msg.Data()
	if len(data) < 4 || (two && len(data) < 6) {
		return 0, 0, 0, 0, nil, false
	}
	r := buffer.NewReader(data)
	stamp, _ := r.ReadUint32()
	var a, b byte
	if two {
		a, _ = r.ReadByte()
		b, _ = r.ReadByte()
	}
	rows, err := h.store.GetSpellRows(p.CharServerID)
	if err != nil {
		log.Printf("Spellbook: rows for %d: %v", p.CharServerID, err)
		return 0, 0, 0, 0, nil, false
	}
	return stamp, a, b, p.CharServerID, rows, true
}

func (h *SpellbookHandler) save(id uint32, rows []database.SpellRow, before map[int]database.SpellRow) {
	for _, r := range rows {
		if before[r.SpellID] != r {
			if err := h.store.SetSpellRow(id, r); err != nil {
				log.Printf("Spellbook: save %d/%d: %v", id, r.SpellID, err)
			}
		}
	}
}

func snapshot(rows []database.SpellRow) map[int]database.SpellRow {
	m := make(map[int]database.SpellRow, len(rows))
	for _, r := range rows {
		m[r.SpellID] = r
	}
	return m
}

// memorize applies Msg00B4 to the rows.
func memorize(rows []database.SpellRow, index, slot int) bool {
	k := -1
	for i := range rows {
		if rows[i].AddedOrder == index {
			k = i
			break
		}
	}
	if k < 0 {
		return false
	}
	for i := range rows {
		if rows[i].WhereOnHotBar == slot && rows[i].AddedOrder != index {
			rows[i].OnHotBar = 0
			rows[i].WhereOnHotBar = -1
		}
	}
	if rows[k].OnHotBar != 0 {
		rows[k].OnHotBar = 0
	} else {
		rows[k].OnHotBar = 1
	}
	rows[k].WhereOnHotBar = slot
	return true
}

// arrange applies Msg00B6 to the rows.
func arrange(rows []database.SpellRow, from, to int) {
	k := -1
	for i := range rows {
		if rows[i].AddedOrder == from {
			k = i
		}
		if from < rows[i].AddedOrder && rows[i].AddedOrder < 252 {
			rows[i].AddedOrder--
		}
	}
	for i := range rows {
		if rows[i].AddedOrder >= to && rows[i].AddedOrder < 252 {
			rows[i].AddedOrder++
		}
	}
	if k >= 0 {
		rows[k].AddedOrder = to
	}
}

func (h *SpellbookHandler) handleMemorize(sess *session.Session, msg *protocol.Message) error {
	stamp, index, slot, id, rows, ok := h.rows(sess, msg, true)
	if !ok {
		return nil
	}
	before := snapshot(rows)
	if !memorize(rows, int(index), int(slot)) {
		log.Printf("Spellbook: %d: no ability at display index %d", id, index)
		return nil
	}
	log.Printf("Spellbook: %d: display index %d onto slot %d", id, index, slot)
	h.save(id, rows, before)
	queueReliable(sess, protocol.OpcodeAbilityMemorize, build(protocol.OpcodeAbilityMemorize, 16, func(w *buffer.Writer) {
		w.WriteUint32(stamp)
		w.WriteByte(index)
		w.WriteByte(slot)
	}))
	return nil
}

func (h *SpellbookHandler) handleArrange(sess *session.Session, msg *protocol.Message) error {
	stamp, from, to, id, rows, ok := h.rows(sess, msg, true)
	if !ok {
		return nil
	}
	before := snapshot(rows)
	arrange(rows, int(from), int(to))
	h.save(id, rows, before)
	queueReliable(sess, protocol.OpcodeAbilityDisplayMove, build(protocol.OpcodeAbilityDisplayMove, 16, func(w *buffer.Writer) {
		w.WriteUint32(stamp)
		w.WriteByte(from)
		w.WriteByte(to)
	}))
	return nil
}

func (h *SpellbookHandler) handleUnmemorizeAll(sess *session.Session, msg *protocol.Message) error {
	stamp, _, _, id, rows, ok := h.rows(sess, msg, false)
	if !ok {
		return nil
	}
	before := snapshot(rows)
	for i := range rows {
		if rows[i].AddedOrder < 252 {
			rows[i].OnHotBar = 0
			rows[i].WhereOnHotBar = -1
		}
	}
	h.save(id, rows, before)
	queueReliable(sess, protocol.OpcodeAbilityClearBar, build(protocol.OpcodeAbilityClearBar, 8, func(w *buffer.Writer) {
		w.WriteUint32(stamp)
	}))
	return nil
}
