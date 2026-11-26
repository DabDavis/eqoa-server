package handlers

import (
	"encoding/binary"
	"log"
	"math"
	"slices"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mercwin.go — a merc's inventory and spell windows in vanilla, like the
// player's own (the owner, 2026-11-15: "the merc inventory should pop up like
// player inventory menu also show the merc equipped gear. players can replace
// it and/or give them new equipment like a player", and "also merc spell menu
// should be like players also with spell book and 10 spells";
// protocol.OpcodeMercWindowOffer..OpcodeMercSpellsState hold the formats).
//
// Once the player is in the world (world.mercs on) the server offers the
// windows; a client that answers 1 (vanilla) has the merc's menu's Items >
// Inventory, /merc [name] inv, its menu's Spells and /merc [name] spells open
// its windows (OpcodeMercWindowState, OpcodeMercSpellsState with flag 1) in
// place of the boxes, and is sent the merc's state again after every change
// by any road. Its asks are /merc's own moves (mercinv.go: giveItem, equipAt,
// takeItem; every rule theirs), and the ten spell slots are the merc's
// loadout (entity/mercloadout.go, charmerc.spell_slots). A PS2 never answers:
// it is sent none of it and keeps the boxes.

// mercWindowLayout is the layout the offer names (the base one, which every
// client that takes the windows reads) and the spell window's state is sent in.
const mercWindowLayout = 1

// mercWindowLayoutUse is the inventory state's layout with a carried meal or
// drink its own kind (mercRecUse), which a client answering 2 to the offer
// reads (petition 3285); a client answering 1 is sent layout 1, the record
// carried.
const mercWindowLayoutUse = 2

// The window's asks (OpcodeMercWindowAsk) and the spell window's
// (OpcodeMercSpellsAsk).
const (
	mercAskOpen    = 1
	mercAskGive    = 2
	mercAskEquip   = 3
	mercAskUnequip = 4
	mercAskTake    = 5
	mercAskStock   = 6 // take from the owner's potion stock
	mercAskClose   = 7 // the window closed
	mercAskUse     = 8 // the merc eats or drinks a carried meal or drink (petition 3285)

	mercSpellsOpen    = 1
	mercSpellsSet     = 2
	mercSpellsClear   = 3
	mercSpellsDefault = 4
)

// The state's flags.
const (
	mercStateOpen = 1 << iota
	mercStateGone
	mercStateSet // the spell window's: its ten set by its hirer
)

// The records' kinds in the state.
const (
	mercRecCarried = 0
	mercRecWorn    = 1
	mercRecHeir    = 2
	mercRecStock   = 3 // the owner's potion stock (an alchemist's: mercbrew.go), its id the item's
	mercRecUse     = 4 // carried, a meal or a grocer's drink its owner may Use (layout 2's)
)

// mercStockEvery is the least time between two states a brew or a throw's
// change to the stock sends an open window (a take is sent at once).
const mercStockEvery = 5 * time.Second

// mercWinOpen is a window open: its merc's slot, when the stock last went.
type mercWinOpen struct {
	slot   int
	pushed time.Time
}

// SendMercWindowOffer offers the session's client the merc windows, where
// world.mercs is on; elsewhere nothing goes.
func SendMercWindowOffer(sess *session.Session) {
	if !MercsOn() {
		return
	}
	queueReliable(sess, protocol.OpcodeMercWindowOffer, build(protocol.OpcodeMercWindowOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(mercWindowLayout)
	}))
}

// mercWindows is whether the session's client takes the windows.
func mercWindows(sess *session.Session) bool {
	return sess.MercWindow() >= mercWindowLayout
}

// mercWindowUse is whether the session's client reads layout 2 (mercRecUse).
func mercWindowUse(sess *session.Session) bool {
	return sess.MercWindow() >= mercWindowLayoutUse
}

// MercWindowHandler takes the offer's answer and the windows' asks.
type MercWindowHandler struct {
	world *entity.World
	mercs *Mercs
}

// NewMercWindowHandler is the handler over the World and the mercs' handler.
func NewMercWindowHandler(w *entity.World, m *Mercs) *MercWindowHandler {
	return &MercWindowHandler{world: w, mercs: m}
}

// RegisterHandlers registers the answer and the asks.
func (h *MercWindowHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeMercWindowOffer, h.HandleAnswer)
	r.Register(protocol.OpcodeMercWindowAsk, h.HandleAsk)
	r.Register(protocol.OpcodeMercSpellsAsk, h.HandleSpellsAsk)
}

// HandleAnswer is the client's answer: a u8, the layout it reads (at most
// ours: 2 a client that reads mercRecUse), 0 to stop.
func (h *MercWindowHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("MercWindow: a short answer from %s", sess.RemoteAddr())
		return nil
	}
	sess.SetMercWindow(min(data[0], mercWindowLayoutUse))
	return nil
}

// askPlayer is the asking session's player where the mercs are on.
func (h *MercWindowHandler) askPlayer(sess *session.Session) *entity.Player {
	if h.world == nil || h.mercs == nil || !mercWindows(sess) {
		return nil
	}
	p := h.world.SessionPlayer(sess)
	if p == nil {
		return nil
	}
	if !MercsOn() {
		SendErrorMessage(sess, mercOffLine)
		return nil
	}
	return p
}

// HandleAsk is OpcodeMercWindowAsk: one move, answered by
// OpcodeMercWindowResult and, done, the merc's state.
func (h *MercWindowHandler) HandleAsk(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		log.Printf("MercWindow: a short ask from %s", sess.RemoteAddr())
		return nil
	}
	p := h.askPlayer(sess)
	if p == nil {
		return nil
	}
	slot, op, rest := int(data[0]), data[1], data[2:]
	ok := h.mercs.windowAsk(sess, p, slot, op, rest)
	sendMercResult(sess, byte(protocol.OpcodeMercWindowAsk&0xFF), op, ok)
	return nil
}

// HandleSpellsAsk is OpcodeMercSpellsAsk: answered by OpcodeMercWindowResult
// and, done, the spell window's state.
func (h *MercWindowHandler) HandleSpellsAsk(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		log.Printf("MercWindow: a short spells ask from %s", sess.RemoteAddr())
		return nil
	}
	p := h.askPlayer(sess)
	if p == nil {
		return nil
	}
	slot, op, rest := int(data[0]), data[1], data[2:]
	ok := h.mercs.spellsAsk(sess, p, slot, op, rest)
	sendMercResult(sess, byte(protocol.OpcodeMercSpellsAsk&0xFF), op, ok)
	return nil
}

// sendMercResult is OpcodeMercWindowResult.
func sendMercResult(sess *session.Session, of, op byte, ok bool) {
	var done byte
	if ok {
		done = 1
	}
	queueReliable(sess, protocol.OpcodeMercWindowResult, build(protocol.OpcodeMercWindowResult, 5, func(w *buffer.Writer) {
		w.WriteByte(of)
		w.WriteByte(op)
		w.WriteByte(done)
	}))
}

// windowRow is the player's hire an ask names (0: its only one, as /merc
// without a name; several, the line asks which).
func (m *Mercs) windowRow(sess *session.Session, p *entity.Player, slot int, verb string) (database.MercRow, bool) {
	if m.db == nil {
		SendErrorMessage(sess, mercNoneLine)
		return database.MercRow{}, false
	}
	rows, _ := m.db.Mercs(p.CharServerID)
	if slot = m.oneSlot(sess, rows, slot, verb); slot == 0 {
		return database.MercRow{}, false
	}
	return m.mercRowOf(sess, p, slot)
}

// windowAsk does one of the window's asks (protocol.OpcodeMercWindowAsk), by
// /merc's rules; true when it was done.
func (m *Mercs) windowAsk(sess *session.Session, p *entity.Player, slot int, op byte, rest []byte) bool {
	row, ok := m.windowRow(sess, p, slot, "inv")
	if !ok {
		return false
	}
	u32 := func() (int, bool) {
		if len(rest) < 4 {
			return 0, false
		}
		return int(binary.LittleEndian.Uint32(rest)), true
	}
	switch op {
	case mercAskOpen:
		return m.openWindow(sess, p, row)
	case mercAskClose:
		m.winMu.Lock()
		delete(m.winOpen, p.CharServerID)
		m.winMu.Unlock()
		return true
	case mercAskStock:
		if len(rest) < 4 {
			return false
		}
		id, n := int(binary.LittleEndian.Uint32(rest)), 0
		if len(rest) >= 8 {
			n = int(binary.LittleEndian.Uint32(rest[4:]))
		}
		for _, s := range m.stockRows(p) {
			if s.pat.PatternID == id {
				ok := m.takeStockN(sess, p, s, n)
				m.pushWindow(sess, p, row.Slot)
				return ok
			}
		}
		SendErrorMessage(sess, "Your alchemists' stock holds none of that.")
		m.pushWindow(sess, p, row.Slot)
		return false
	case mercAskGive:
		key, ok := u32()
		l := sessionInvList(sess)
		if !ok || l == nil {
			return false
		}
		r, ok := l.at(l.index(uint32(key)))
		if !ok {
			SendErrorMessage(sess, "That is no longer in your pack.")
			return false
		}
		return m.giveItem(sess, p, row, r.itemID)
	case mercAskEquip, mercAskUnequip, mercAskTake, mercAskUse:
		id, ok := u32()
		if !ok {
			return false
		}
		items, err := m.db.MercItems(row.ServerID, row.Slot)
		if err != nil {
			SendErrorMessage(sess, mercKitFailed)
			return false
		}
		i := slices.IndexFunc(items, func(it database.MercItem) bool { return it.ID == id })
		if i < 0 {
			SendErrorMessage(sess, "Your mercenary no longer holds that.")
			m.pushWindow(sess, p, row.Slot)
			return false
		}
		it := items[i]
		switch op {
		case mercAskTake:
			return m.takeItem(sess, p, row, it)
		case mercAskUse:
			return m.useItem(sess, p, row, it)
		case mercAskUnequip:
			if it.EquipLoc < 0 {
				return true // carried already
			}
			return m.equip(sess, p, row, it, items)
		}
		want := -1
		if len(rest) >= 5 {
			want = int(int8(rest[4]))
		}
		if it.EquipLoc >= 0 && want < 0 {
			return true // worn already
		}
		return m.equipAt(sess, p, row, it, items, want)
	}
	log.Printf("MercWindow: ask %d from %s not known", op, sess.RemoteAddr())
	return false
}

// openWindow opens vanilla's window on the hire (the state with flag 1);
// false for a client that takes none (the boxes go instead).
func (m *Mercs) openWindow(sess *session.Session, p *entity.Player, row database.MercRow) bool {
	if !mercWindows(sess) {
		return false
	}
	m.winMu.Lock()
	m.winOpen[p.CharServerID] = &mercWinOpen{slot: row.Slot, pushed: m.now()}
	m.winMu.Unlock()
	m.sendWindow(sess, p, row, mercStateOpen)
	return true
}

// stockChanged sends the player's open window its stock after a brew or a
// throw changed it, at most every mercStockEvery; none open, nothing.
func (m *Mercs) stockChanged(p *entity.Player) {
	if p == nil || m.gw == nil {
		return
	}
	now := m.now()
	m.winMu.Lock()
	w := m.winOpen[p.CharServerID]
	due := w != nil && now.Sub(w.pushed) >= mercStockEvery
	if due {
		w.pushed = now
	}
	m.winMu.Unlock()
	if !due {
		return
	}
	if sess := m.gw.FindSessionByEntityID(p.ServerID); sess != nil {
		m.pushWindow(sess, p, w.slot)
	}
}

// pushWindow sends a window-taking client the hire's state after a change
// (not opening it).
func (m *Mercs) pushWindow(sess *session.Session, p *entity.Player, slot int) {
	if !mercWindows(sess) || m.db == nil {
		return
	}
	row, ok, err := m.db.Merc(p.CharServerID, slot)
	if err != nil || !ok {
		return
	}
	m.sendWindow(sess, p, row, 0)
}

// mercWindowRec is a record of the state: its id (0 an heirloom), its kind
// and the record.
type mercWindowRec struct {
	id   int
	kind byte
	e    database.CharInventoryEntry
}

// windowRecords is what the hire holds and wears, as the state lists them:
// its own worn, then its heirlooms where it wears nothing of its own (and
// nothing its own's wear takes off: itemsOver), then what it carries.
func (m *Mercs) windowRecords(p *entity.Player, row database.MercRow, level int) []mercWindowRec {
	items, err := m.db.MercItems(row.ServerID, row.Slot)
	if err != nil {
		return nil
	}
	var worn, carried []mercWindowRec
	var own []database.CharInventoryEntry
	for _, it := range items {
		pat := it.Pattern
		e := database.CharInventoryEntry{ItemID: -it.ID, StackLeft: max(it.Stack, 1), RemainHP: it.RemainHP, Charges: it.Charges, EquipLoc: it.EquipLoc, Pattern: pat,
			AuraTag: database.InstanceTag(-it.ID)} // the copy's own tag (instanceaura)
		if it.EquipLoc >= 0 {
			if m.heir != nil {
				m.heir.ViewForOwner(row.ServerID, row.Race, level, &e.Pattern) // its figures as it wears it (mercOwnItems)
			}
			own = append(own, e)
			worn = append(worn, mercWindowRec{id: it.ID, kind: mercRecWorn, e: e})
			continue
		}
		kind := byte(mercRecCarried)
		if mercUsable(&it.Pattern) {
			kind = mercRecUse // shown as carried to a layout-1 client (buildMercWindowState)
		}
		carried = append(carried, mercWindowRec{id: it.ID, kind: kind, e: e})
	}
	if heir, err := m.heirlooms(row.Class, row.Race, level); err == nil {
		for _, e := range itemsOver(heir, own) {
			if e.ItemID == 0 {
				worn = append(worn, mercWindowRec{kind: mercRecHeir, e: e})
			}
		}
	}
	out := append(worn, carried...)
	if row.Class == mercAlchemist {
		// its owner's potion stock, which it brews into and throws and drinks
		// from (mercbrew.go): each potion's record, its count the stack
		for _, s := range m.stockRows(p) {
			out = append(out, mercWindowRec{id: s.pat.PatternID, kind: mercRecStock,
				e: database.CharInventoryEntry{StackLeft: s.n, RemainHP: s.pat.MaxHP, EquipLoc: -1, Pattern: s.pat}})
		}
	}
	return out
}

// buildMercWindowState is OpcodeMercWindowState (the opcode's doc).
// layout is the client's (1, or 2 with mercRecUse; a record of that kind is
// carried in layout 1).
func buildMercWindowState(layout byte, flags byte, slot int, sh entity.MercSheet, recs []mercWindowRec, capes bool) []byte {
	layout = min(max(layout, mercWindowLayout), mercWindowLayoutUse)
	return build(protocol.OpcodeMercWindowState, 1<<16, func(w *buffer.Writer) {
		w.WriteByte(layout)
		w.WriteByte(flags)
		w.WriteByte(byte(slot))
		writeName8(w, sh.Name)
		w.WriteByte(byte(sh.Class))
		w.WriteByte(byte(sh.Race))
		w.WriteByte(byte(min(max(sh.Level, 0), 255)))
		for _, v := range []int{sh.HP, sh.MaxHP, sh.Power, sh.MaxPower, sh.AC} {
			w.WriteUint32(uint32(max(v, 0)))
		}
		for _, v := range sh.Stats {
			w.WriteUint32(uint32(max(v, 0)))
		}
		w.WriteByte(byte(database.MercItemsMax))
		n := min(len(recs), 255)
		w.WriteByte(byte(n))
		for _, r := range recs[:n] {
			w.WriteUint32(uint32(r.id))
			kind := r.kind
			if kind == mercRecUse && layout < mercWindowLayoutUse {
				kind = mercRecCarried
			}
			w.WriteByte(kind)
			e := r.e
			e.Location = -1
			writeItem(w, &e, r.id, -1, capes)
		}
	})
}

// writeName8 writes a string as the 0x7Exx opcodes do: a u8 byte count and
// the bytes (cut at 255).
func writeName8(w *buffer.Writer, s string) {
	b := []byte(s)
	if len(b) > 255 {
		b = b[:255]
	}
	w.WriteByte(byte(len(b)))
	w.WriteBytes(b)
}

// sendWindow sends the hire's state with the flags.
func (m *Mercs) sendWindow(sess *session.Session, p *entity.Player, row database.MercRow, flags byte) {
	kit, err := m.kit(row, p.Level)
	if err != nil {
		log.Printf("Merc: %s's %q's window: %v", p.Name, row.Name, err)
	}
	sh := m.gw.MercSheetOf(sess, row.Slot, kit)
	sh.Name = row.Name
	queueReliable(sess, protocol.OpcodeMercWindowState, buildMercWindowState(sess.MercWindow(), flags, row.Slot, sh, m.windowRecords(p, row, kit.Level), readsCapes(sess)))
}

// spellsAsk does one of the spell window's asks (protocol.OpcodeMercSpellsAsk);
// true when it was done.
func (m *Mercs) spellsAsk(sess *session.Session, p *entity.Player, slot int, op byte, rest []byte) bool {
	row, ok := m.windowRow(sess, p, slot, "spells")
	if !ok {
		return false
	}
	if op == mercSpellsOpen {
		return m.openSpellsWindow(sess, p, row.Slot)
	}
	book, slots, _, out := m.gw.MercLoadout(sess, row.Slot)
	if !out {
		SendErrorMessage(sess, mercNoneOut(row.Slot, []database.MercRow{row}))
		m.sendSpells(sess, p, row.Slot, 0)
		return false
	}
	switch op {
	case mercSpellsSet:
		if len(rest) < 5 {
			return false
		}
		at, id := int(rest[0]), int(binary.LittleEndian.Uint32(rest[1:5]))
		if at >= database.MercSpellSlotCount {
			return false
		}
		if id != 0 && !slices.ContainsFunc(book, func(b entity.MercBookSpell) bool { return b.Spell.SpellID == id }) {
			SendErrorMessage(sess, row.Name+" does not know that spell.")
			m.sendSpells(sess, p, row.Slot, 0)
			return false
		}
		if was := slices.Index(slots, id); id != 0 && was >= 0 {
			slots[was] = 0 // it moves
		}
		slots[at] = id
	case mercSpellsClear:
		slots = make([]int, database.MercSpellSlotCount)
	case mercSpellsDefault:
		slots = nil
	default:
		log.Printf("MercWindow: spells ask %d from %s not known", op, sess.RemoteAddr())
		return false
	}
	if err := m.db.SetMercSpellSlots(p.CharServerID, row.Slot, slots); err != nil {
		log.Printf("Merc: %s's %q's spell slots not kept: %v", p.Name, row.Name, err)
		SendErrorMessage(sess, mercKitFailed)
		return false
	}
	m.gw.SetMercLoadout(sess, row.Slot, slots)
	log.Printf("Merc: %s's %q's spell slots: %v", p.Name, row.Name, slots)
	m.sendSpells(sess, p, row.Slot, 0)
	return true
}

// openSpellsWindow opens vanilla's spell window on the merc of the slot out;
// false for a client that takes none (the Spells box goes instead).
func (m *Mercs) openSpellsWindow(sess *session.Session, p *entity.Player, slot int) bool {
	if !mercWindows(sess) {
		return false
	}
	if _, _, _, out := m.gw.MercLoadout(sess, slot); !out {
		SendErrorMessage(sess, mercNoneOut(slot, nil))
		return true
	}
	m.sendSpells(sess, p, slot, mercStateOpen)
	return true
}

// pushSpells sends a window-taking client the spell window's state after a
// change (the Spells box's), not opening it.
func (m *Mercs) pushSpells(sess *session.Session, p *entity.Player, slot int) {
	if mercWindows(sess) {
		m.sendSpells(sess, p, slot, 0)
	}
}

// sendSpells sends the spell window's state of the merc of the slot.
func (m *Mercs) sendSpells(sess *session.Session, p *entity.Player, slot int, flags byte) {
	book, slots, set, out := m.gw.MercLoadout(sess, slot)
	info, _ := m.menuMerc(sess, slot)
	if !out {
		flags = mercStateGone
	}
	if set {
		flags |= mercStateSet
	}
	var store *database.SpellPatternStore
	if m.db != nil {
		store = database.NewSpellPatternStore(m.db)
	}
	queueReliable(sess, protocol.OpcodeMercSpellsState, buildMercSpellsState(flags, slot, info, book, slots, func(id int) *database.SpellPatternRecord {
		if store == nil {
			return nil
		}
		rec, err := store.GetSpellPattern(id)
		if err != nil {
			return nil
		}
		return rec
	}))
}

// mercAbility is a book spell as an ability record (writeAbilityRecord's):
// its pattern's (pattern: the spellpattern row, nil none: made from the
// spell), its place in the book, on its slot or not.
func mercAbility(s *entity.CastSpell, pattern *database.SpellPatternRecord, index, slot int) database.CharSpellEntry {
	var sp database.SpellPatternRecord
	if pattern != nil {
		sp = *pattern
	} else {
		sp = database.SpellPatternRecord{SpellID: s.SpellID, AbilityLvl: s.Level, Range: int(math.Float32bits(s.Range) >> 16),
			Power: s.PowerCost, Icon: int64(s.Icon), Scope: s.Scope, SpellName: s.Name}
	}
	e := database.CharSpellEntry{AddedOrder: index, WhereOnHotBar: -1, ShowHide: 1, Pattern: sp}
	if slot >= 0 {
		e.OnHotBar, e.WhereOnHotBar = 1, slot
	}
	return e
}

// buildMercSpellsState is OpcodeMercSpellsState (the opcode's doc).
func buildMercSpellsState(flags byte, slot int, info entity.MercInfo, book []entity.MercBookSpell, slots []int, pattern func(int) *database.SpellPatternRecord) []byte {
	return build(protocol.OpcodeMercSpellsState, 1<<17, func(w *buffer.Writer) {
		w.WriteByte(mercWindowLayout)
		w.WriteByte(flags)
		w.WriteByte(byte(slot))
		writeName8(w, info.Name)
		w.WriteByte(byte(info.Class))
		w.WriteByte(byte(min(max(info.Level, 0), 255)))
		n := min(len(book), 250)
		w.WriteByte(byte(n))
		for i, b := range book[:n] {
			e := mercAbility(&b.Spell, pattern(b.Spell.SpellID), i, slices.Index(slots, b.Spell.SpellID))
			writeAbilityRecord(w, &e)
			writeName8(w, b.Use)
			var off byte
			if b.Off {
				off = 1
			}
			w.WriteByte(off)
		}
		for i := range database.MercSpellSlotCount {
			id := 0
			if i < len(slots) {
				id = slots[i]
			}
			w.WriteUint32(uint32(id))
		}
	})
}
