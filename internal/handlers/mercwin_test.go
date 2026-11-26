package handlers

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// mercwin.go: vanilla's merc windows. The offer and its answer; each ask by
// /merc's rules (the gates, a swap into a filled slot, a two-hander, a full
// merc, a full pack, a LORE item); the state's bytes; the spell window's ten
// slots kept and cast from; and a PS2, which never answers, kept to the boxes.

// mercWinState is OpcodeMercWindowState read back.
type mercWinState struct {
	flags, slot        byte
	name               string
	class, race, level byte
	figures            [5]uint32
	stats              [7]uint32
	most               byte
	recs               []mercWinRec
}

type mercWinRec struct {
	id   uint32
	kind byte
	item dumpedItem
}

func readMercWinState(t *testing.T, d []byte) mercWinState {
	t.Helper()
	if len(d) < 3 || binary.LittleEndian.Uint16(d) != uint16(protocol.OpcodeMercWindowState) || (d[2] != mercWindowLayout && d[2] != mercWindowLayoutUse) {
		t.Fatalf("not a state of layout 1 or 2: % x", d[:min(len(d), 8)])
	}
	r := buffer.NewReader(d[3:])
	b := func() byte { v, _ := r.ReadByte(); return v }
	var s mercWinState
	s.flags, s.slot = b(), b()
	nb, _ := r.ReadBytes(int(b()))
	s.name = string(nb)
	s.class, s.race, s.level = b(), b(), b()
	for i := range s.figures {
		s.figures[i], _ = r.ReadUint32()
	}
	for i := range s.stats {
		s.stats[i], _ = r.ReadUint32()
	}
	s.most = b()
	n := int(b())
	for range n {
		id, _ := r.ReadUint32()
		kind := b()
		it, ok := readDumpedItem(r)
		if !ok {
			t.Fatalf("record %d of %d not read", len(s.recs), n)
		}
		s.recs = append(s.recs, mercWinRec{id: id, kind: kind, item: it})
	}
	if r.Remaining() != 0 {
		t.Fatalf("%d bytes after the records", r.Remaining())
	}
	return s
}

// queuedWhole is queued's messages of op put back together from their
// fragments (a state of many records goes in pieces: queue.go).
func queuedWhole(s *session.Session, op protocol.GameOpcode) [][]byte {
	var out [][]byte
	var cur []byte
	for _, m := range s.Queue().PeekPending() {
		if m.Opcode != op {
			continue
		}
		cur = append(cur, m.Data()...)
		if m.Type != protocol.MessageTypeSegmentReliable {
			out, cur = append(out, cur), nil
		}
	}
	return out
}

// lastMercWinState is the last state queued to s, and whether there was one.
func lastMercWinState(t *testing.T, s *session.Session) (mercWinState, bool) {
	t.Helper()
	got := queuedWhole(s, protocol.OpcodeMercWindowState)
	if len(got) == 0 {
		return mercWinState{}, false
	}
	return readMercWinState(t, got[len(got)-1]), true
}

// lastMercResult is the last OpcodeMercWindowResult queued: (of, op, done).
func lastMercResult(t *testing.T, s *session.Session) [3]byte {
	t.Helper()
	got := queued(s, protocol.OpcodeMercWindowResult)
	if len(got) == 0 {
		t.Fatal("no result")
	}
	d := got[len(got)-1]
	return [3]byte{d[2], d[3], d[4]}
}

// winAsk is the client's OpcodeMercWindowAsk.
func winAsk(t *testing.T, h *MercWindowHandler, s *session.Session, slot, op byte, fields ...[]byte) {
	t.Helper()
	data := []byte{slot, op}
	for _, f := range fields {
		data = append(data, f...)
	}
	if err := h.HandleAsk(s, incoming(protocol.OpcodeMercWindowAsk, data)); err != nil {
		t.Fatal(err)
	}
}

// packKey is the client's key of the pack's record of the item.
func packKey(t *testing.T, s *session.Session, pattern int) []byte {
	t.Helper()
	l := sessionInvList(s)
	for i := 0; ; i++ {
		r, ok := l.at(i)
		if !ok {
			t.Fatalf("no %d in the client's list", pattern)
		}
		if r.patternID == pattern {
			return u32b(r.key)
		}
	}
}

// mercRecID is the merc's record of the item.
func mercRecID(t *testing.T, db *database.SQLiteDB, char uint32, pattern int) []byte {
	t.Helper()
	var id uint32
	if err := db.QueryRow(`SELECT id FROM charmercitem WHERE serverid = ? AND slot = 1 AND patternid = ?`, char, pattern).Scan(&id); err != nil {
		t.Fatalf("the merc holds no %d: %v", pattern, err)
	}
	return u32b(id)
}

// The offer is its opcode and a u8, 1, only where world.mercs is on; the
// answer is kept (more than 1 read as 1, 0 off, a short one changes nothing).
func TestMercWindowOfferAndAnswer(t *testing.T) {
	c := newChatWorld(t)
	s, _ := c.join(1, "Alda", 100, 100, 0)
	SetMercs(false)
	SendMercWindowOffer(s)
	if got := queued(s, protocol.OpcodeMercWindowOffer); len(got) != 0 {
		t.Errorf("offered with world.mercs off: % x", got)
	}
	SetMercs(true)
	t.Cleanup(func() { SetMercs(false) })
	SendMercWindowOffer(s)
	if got := queued(s, protocol.OpcodeMercWindowOffer); len(got) != 1 || !bytes.Equal(got[0], []byte{0x72, 0x7E, 1}) {
		t.Errorf("the offer % x", got)
	}
	h := NewMercWindowHandler(c.gw, nil)
	for _, a := range []struct {
		in   []byte
		want uint8
	}{{[]byte{1}, 1}, {nil, 1}, {[]byte{2}, 2}, {[]byte{9}, 2}, {[]byte{0}, 0}} {
		h.HandleAnswer(s, incoming(protocol.OpcodeMercWindowOffer, a.in))
		if s.MercWindow() != a.want {
			t.Errorf("after % x: layout %d, want %d", a.in, s.MercWindow(), a.want)
		}
	}
	// kept over a keep restart
	s.SetMercWindow(1)
	if s.Keep().MercWindow != 1 {
		t.Error("a keep restart loses the answer")
	}
}

// The window's asks by the player's rules: give by the pack's key; equip into
// a slot of its wear, a filled slot swapped (the old piece carried); a level
// gate refused; a two-hander takes the hands, a shield then the two-hander off;
// unequip; take back; a full pack and a LORE item already held keep it with the
// merc; a full merc refuses a give. Every ask answered, each done one followed
// by the state.
func TestMercWindowAsksByTheRules(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 0, 0, 0, -1) // a warrior of 30
	r.anna.SetMercWindow(1)
	h := NewMercWindowHandler(r.gw, m)
	r.pack(db, [2]int{itBreastplate, 1}, [2]int{900001, 1}, [2]int{itTooHigh, 1}, [2]int{itGreatsword, 1}, [2]int{itBuckler, 1},
		[2]int{itStone, 1})
	r.drain()
	char := r.pa.CharServerID
	done := func(op byte, want byte) {
		t.Helper()
		if got := lastMercResult(t, r.anna); got != [3]byte{0x73, op, want} {
			t.Fatalf("ask %d: result % x, want done %d", op, got, want)
		}
	}
	for _, it := range []int{itBreastplate, 900001, itTooHigh, itGreatsword, itBuckler, itStone} {
		winAsk(t, h, r.anna, 0, mercAskGive, packKey(t, r.anna, it))
		done(mercAskGive, 1)
		if n, _ := mercHas(t, db, char, 1, it); n != 1 || packHas(t, db, char, it) != 0 {
			t.Fatalf("item %d not given", it)
		}
	}
	if st, ok := lastMercWinState(t, r.anna); !ok || st.flags&mercStateOpen != 0 {
		t.Fatalf("after a give: state %v, flags %d (a change opens nothing)", ok, st.flags)
	}
	r.drain()
	// into the chest, then the tunic into the filled chest: the breastplate carried
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, itBreastplate), []byte{5})
	done(mercAskEquip, 1)
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, 900001), []byte{5})
	done(mercAskEquip, 1)
	if _, e := mercHas(t, db, char, 1, 900001); e != 5 {
		t.Fatalf("the tunic in %d", e)
	}
	if _, e := mercHas(t, db, char, 1, itBreastplate); e != -1 {
		t.Fatalf("the breastplate swapped out to %d, want carried", e)
	}
	// a slot not of its wear, and a level gate
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, itBreastplate), []byte{0})
	done(mercAskEquip, 0)
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, itTooHigh), []byte{0xFF})
	done(mercAskEquip, 0)
	if _, e := mercHas(t, db, char, 1, itTooHigh); e != -1 {
		t.Error("a level 30 wears a level 55 crown")
	}
	// a two-hander, then a shield
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, itGreatsword), []byte{0xFF})
	done(mercAskEquip, 1)
	if w := r.mercInfo(1).Worn; !w[16] || w[14] || w[17] {
		t.Fatalf("the two-hander: hands %v", w[14:19])
	}
	winAsk(t, h, r.anna, 1, mercAskEquip, mercRecID(t, db, char, itBuckler), []byte{17})
	done(mercAskEquip, 1)
	if _, e := mercHas(t, db, char, 1, itGreatsword); e != -1 {
		t.Errorf("the two-hander stays worn (%d) under a shield", e)
	}
	// the state shows it: the tunic and the shield worn, the rest carried, an
	// heirloom where nothing of its own is
	st, ok := lastMercWinState(t, r.anna)
	if !ok {
		t.Fatal("no state after the equips")
	}
	kinds := map[byte]int{}
	for _, rec := range st.recs {
		kinds[rec.kind]++
		if rec.kind == mercRecWorn && rec.item.pattern == 900001 && rec.item.equip != 5 {
			t.Errorf("the tunic's record in %d", rec.item.equip)
		}
		if rec.kind == mercRecHeir && (rec.id != 0 || rec.item.pattern == 714001) {
			t.Errorf("an heirloom %+v (its chest is under the tunic)", rec)
		}
	}
	if kinds[mercRecWorn] != 2 || kinds[mercRecCarried] != 4 || kinds[mercRecHeir] == 0 {
		t.Errorf("records by kind %v", kinds)
	}
	// unequip, then take back
	winAsk(t, h, r.anna, 1, mercAskUnequip, mercRecID(t, db, char, 900001))
	done(mercAskUnequip, 1)
	if _, e := mercHas(t, db, char, 1, 900001); e != -1 {
		t.Error("not taken off")
	}
	winAsk(t, h, r.anna, 1, mercAskTake, mercRecID(t, db, char, 900001))
	done(mercAskTake, 1)
	if packHas(t, db, char, 900001) != 1 {
		t.Error("not taken back")
	}
	// a full pack keeps it with the merc
	l := sessionInvList(r.anna)
	l.cap = 1
	winAsk(t, h, r.anna, 1, mercAskTake, mercRecID(t, db, char, itBreastplate))
	done(mercAskTake, 0)
	if n, _ := mercHas(t, db, char, 1, itBreastplate); n != 1 {
		t.Error("a full pack lost the breastplate")
	}
	l.cap = 40
	// a LORE item its owner holds already stays with the merc
	r.pack(db, [2]int{itStone, 1})
	winAsk(t, h, r.anna, 1, mercAskTake, mercRecID(t, db, char, itStone))
	done(mercAskTake, 0)
	if n, _ := mercHas(t, db, char, 1, itStone); n != 1 || packHas(t, db, char, itStone) != 1 {
		t.Error("a second LORE item came into the pack")
	}
	// a full merc refuses a give, the item kept in the pack
	for range database.MercItemsMax {
		if _, err := db.Exec(`INSERT INTO charmercitem (serverid, slot, patternid, stack, equiploc) VALUES (?, 1, ?, 1, -1)`, char, itBuckler); err != nil {
			t.Fatal(err)
		}
	}
	winAsk(t, h, r.anna, 1, mercAskGive, packKey(t, r.anna, itStone))
	done(mercAskGive, 0)
	if packHas(t, db, char, itStone) != 1 {
		t.Error("a full merc took the stone")
	}
	// a record it no longer holds
	winAsk(t, h, r.anna, 1, mercAskTake, u32b(999999))
	done(mercAskTake, 0)
}

// The state's bytes: the header, the figures and each record as 0x003B's.
func TestMercWindowStateBytes(t *testing.T) {
	sh := entity.MercSheet{Name: "Kel", Class: 13, Race: 2, Level: 30, HP: 800, MaxHP: 817, Power: 1400, MaxPower: 1480, AC: 240,
		Stats: [7]int{85, 131, 65, 102, 55, 149, 70}}
	pat := database.ItemPatternRecord{PatternID: itBreastplate, ItemName: "Mithril Breastplate", EquipSlot: 5}
	recs := []mercWindowRec{{id: 77, kind: mercRecWorn, e: database.CharInventoryEntry{StackLeft: 1, RemainHP: 50, EquipLoc: 5, Pattern: pat}},
		{kind: mercRecHeir, e: database.CharInventoryEntry{StackLeft: 1, EquipLoc: 0, Pattern: database.ItemPatternRecord{PatternID: 714188, ItemName: "Heirloom head"}}}}
	d := buildMercWindowState(1, mercStateOpen, 2, sh, recs, false)
	head := []byte{0x74, 0x7E, 1, 1, 2, 3, 'K', 'e', 'l', 13, 2, 30}
	if !bytes.Equal(d[:len(head)], head) {
		t.Fatalf("head % x, want % x", d[:len(head)], head)
	}
	st := readMercWinState(t, d)
	if st.figures != [5]uint32{800, 817, 1400, 1480, 240} || st.stats != [7]uint32{85, 131, 65, 102, 55, 149, 70} || st.most != 40 {
		t.Errorf("figures %v stats %v most %d", st.figures, st.stats, st.most)
	}
	if len(st.recs) != 2 || st.recs[0].id != 77 || st.recs[0].kind != 1 || st.recs[0].item.key != 77 || st.recs[0].item.equip != 5 ||
		st.recs[0].item.pattern != itBreastplate || st.recs[0].item.quick != -1 || st.recs[1].id != 0 || st.recs[1].kind != 2 {
		t.Errorf("records %+v", st.recs)
	}
}

// The spell window: open sends the book with the default ten (flag 4 off);
// a set keeps the ten in the row and the kit (the merc casts from them), a
// spell set again moves, clear empties, default unsets; an unknown spell is
// refused. The PS2's Spells box turns one on onto an empty slot.
func TestMercSpellWindowSlots(t *testing.T) {
	r, m, db := newMercFixture(t)
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1) // a wizard: Ice Bolt
	r.anna.SetMercWindow(1)
	h := NewMercWindowHandler(r.gw, m)
	r.drain()
	ask := func(op byte, fields ...byte) {
		t.Helper()
		if err := h.HandleSpellsAsk(r.anna, incoming(protocol.OpcodeMercSpellsAsk, append([]byte{1, op}, fields...))); err != nil {
			t.Fatal(err)
		}
	}
	last := func() []byte {
		t.Helper()
		got := queuedWhole(r.anna, protocol.OpcodeMercSpellsState)
		if len(got) == 0 {
			t.Fatal("no spells state")
		}
		return got[len(got)-1]
	}
	slotsOf := func(d []byte) []int {
		var out []int
		for i := range database.MercSpellSlotCount {
			out = append(out, int(binary.LittleEndian.Uint32(d[len(d)-40+4*i:])))
		}
		return out
	}
	ask(mercSpellsOpen)
	d := last()
	if d[2] != 1 || d[3] != mercStateOpen || d[4] != 1 {
		t.Fatalf("open: % x", d[:8])
	}
	if s := slotsOf(d); s[0] != 950 {
		t.Fatalf("the default ten %v, want Ice Bolt first", s)
	}
	// set Ice Bolt on slot 3: kept
	ask(mercSpellsSet, 3, 0xB6, 0x03, 0, 0) // 950
	if got := lastMercResult(t, r.anna); got != [3]byte{0x76, mercSpellsSet, 1} {
		t.Fatalf("set: % x", got)
	}
	if d := last(); d[3]&mercStateSet == 0 || !slices.Equal(slotsOf(d), []int{0, 0, 0, 950, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("after the set: flags %d slots %v (it moved from slot 0)", d[3], slotsOf(d))
	}
	row, _, _ := db.Merc(r.pa.CharServerID, 1)
	if !slices.Equal(row.SpellSlots, []int{0, 0, 0, 950, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("the row keeps %v", row.SpellSlots)
	}
	if k, _ := m.kit(row, 30); !slices.Equal(k.Loadout, row.SpellSlots) {
		t.Error("a kit from its row (a login, a keep restart) lost the slots")
	}
	// an unknown spell refused
	ask(mercSpellsSet, 0, 0x39, 0x30, 0, 0)
	if got := lastMercResult(t, r.anna); got[2] != 0 {
		t.Error("a spell it does not know was slotted")
	}
	// clear: none cast; the PS2's Spells box shows it off, and turning it on
	// puts it on the first empty slot
	ask(mercSpellsClear)
	if spells, _ := r.gw.MercSpells(r.anna, 1); len(spells) != 1 || !spells[0].Off {
		t.Fatalf("cleared: the Spells box shows %+v", spells)
	}
	r.drain()
	m.menuSpells(r.anna, r.pa, 1, 0)
	r.answer(r.msgs(r.anna), "Ice Bolt: off")
	row, _, _ = db.Merc(r.pa.CharServerID, 1)
	if row.SpellSlots[0] != 950 || slices.Contains(row.SpellsOff, 950) {
		t.Fatalf("the box's on: slots %v off %v", row.SpellSlots, row.SpellsOff)
	}
	// default: none set again
	ask(mercSpellsDefault)
	if row, _, _ = db.Merc(r.pa.CharServerID, 1); row.SpellSlots != nil {
		t.Errorf("default left %v", row.SpellSlots)
	}
	if d := last(); d[3]&mercStateSet != 0 {
		t.Error("default still says set")
	}
}

// A PS2 never answers the offer: /merc inv and the menu's Spells ask their
// boxes, no window's state goes, and a window's ask from it does nothing.
func TestMercWindowPS2KeepsTheBoxes(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	m.hireWith(r.anna, r.pa, 13, 0, 0, -1)
	r.pack(db, [2]int{itBreastplate, 1})
	m.Command(r.anna, r.pa, "give mithril")
	r.drain()
	m.Command(r.anna, r.pa, "inv")
	m.Command(r.anna, r.pa, "spells")
	if n := len(queued(r.anna, protocol.OpcodeQuestPopupMulti)); n != 2 {
		t.Errorf("%d boxes, want the Inventory's and the Spells'", n)
	}
	h := NewMercWindowHandler(r.gw, m)
	winAsk(t, h, r.anna, 1, mercAskTake, mercRecID(t, db, r.pa.CharServerID, itBreastplate))
	for _, op := range []protocol.GameOpcode{protocol.OpcodeMercWindowState, protocol.OpcodeMercSpellsState, protocol.OpcodeMercWindowResult} {
		if got := queued(r.anna, op); len(got) != 0 {
			t.Errorf("a PS2 was sent %v: % x", op, got[0][:min(len(got[0]), 8)])
		}
	}
	if n, _ := mercHas(t, db, r.pa.CharServerID, 1, itBreastplate); n != 1 {
		t.Error("a PS2's stray ask moved an item")
	}
	// the control: the same with the window answered opens it
	r.anna.SetMercWindow(1)
	r.drain()
	m.Command(r.anna, r.pa, "inv")
	if st, ok := lastMercWinState(t, r.anna); !ok || st.flags&mercStateOpen == 0 || len(queued(r.anna, protocol.OpcodeQuestPopupMulti)) != 0 {
		t.Errorf("vanilla's /merc inv: state %v flags %d", ok, st.flags)
	}
}

// An alchemist's window shows its owner's potion stock (kind 3, each potion's
// count its stack; a draught too), takes a stack or a count of one into the
// pack (a LORE one only while none is held), and while it is open a brew
// sends the state again at most every 5 s; closed, none. Another class's
// window shows no stock; the PS2's Potion stock box lists the draught.
func TestMercWindowStock(t *testing.T) {
	r, m, db := newMercFixture(t)
	invItems(t, db)
	const itDraught = 910010
	putItem(t, db, database.ItemPatternRecord{PatternID: itDraught, ItemName: "Infused Draught", EquipSlot: -1, LevelReq: 1, MaxStack: 20})
	now := m.now()
	m.now = func() time.Time { return now }
	m.hireWith(r.anna, r.pa, 14, 0, 0, 0) // an alchemist
	r.relist(db)                          // her client's list: an empty pack
	r.anna.SetMercWindow(1)
	h := NewMercWindowHandler(r.gw, m)
	brew := func(pattern, n int) {
		m.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r.pa, Slot: 1, PatternID: pattern, Count: n})
	}
	brew(itPotion, 120)
	brew(itDraught, 30)
	brew(itStone, 1)
	r.drain()
	stock := func() map[int64]int64 {
		t.Helper()
		st, ok := lastMercWinState(t, r.anna)
		if !ok {
			t.Fatal("no state")
		}
		out := map[int64]int64{}
		for _, rec := range st.recs {
			if rec.kind == mercRecStock {
				if int64(rec.id) != rec.item.pattern {
					t.Errorf("a stock record's id %d, its item %d", rec.id, rec.item.pattern)
				}
				out[rec.item.pattern] = rec.item.stack
			}
		}
		return out
	}
	winAsk(t, h, r.anna, 1, mercAskOpen)
	if got := stock(); got[itPotion] != 120 || got[itDraught] != 30 || got[itStone] != 1 {
		t.Fatalf("the stock shown %v", got)
	}
	// a stack, then a count
	winAsk(t, h, r.anna, 1, mercAskStock, u32b(itPotion))
	if got := lastMercResult(t, r.anna); got[2] != 1 || packHas(t, db, r.pa.CharServerID, itPotion) != 100 {
		t.Fatalf("a stack: result % x, pack %d", got, packHas(t, db, r.pa.CharServerID, itPotion))
	}
	winAsk(t, h, r.anna, 1, mercAskStock, u32b(itDraught), u32b(5))
	if packHas(t, db, r.pa.CharServerID, itDraught) != 5 {
		t.Fatalf("five draughts: pack %d", packHas(t, db, r.pa.CharServerID, itDraught))
	}
	if got := stock(); got[itPotion] != 20 || got[itDraught] != 25 {
		t.Fatalf("after the takes %v", got)
	}
	// a LORE one while one is held: refused, kept in the stock
	r.pack(db, [2]int{itStone, 1})
	winAsk(t, h, r.anna, 1, mercAskStock, u32b(itStone))
	if got := lastMercResult(t, r.anna); got[2] != 0 || packHas(t, db, r.pa.CharServerID, itStone) != 1 {
		t.Fatalf("a second LORE stone: result % x", got)
	}
	if got := stock(); got[itStone] != 1 {
		t.Error("the refused stone left the stock")
	}
	// brews while open: at most every 5 s
	r.drain()
	now = now.Add(6 * time.Second)
	brew(itPotion, 1)
	brew(itPotion, 1)
	if n := len(queuedWhole(r.anna, protocol.OpcodeMercWindowState)); n != 1 {
		t.Fatalf("two brews in a moment sent %d states, want 1", n)
	}
	if st, _ := lastMercWinState(t, r.anna); st.flags&mercStateOpen != 0 {
		t.Error("a brew's state opens the window")
	}
	r.drain()
	now = now.Add(6 * time.Second)
	brew(itPotion, 1)
	if n := len(queuedWhole(r.anna, protocol.OpcodeMercWindowState)); n != 1 {
		t.Fatalf("a brew 6 s later sent %d", n)
	}
	// closed: none
	winAsk(t, h, r.anna, 1, mercAskClose)
	r.drain()
	now = now.Add(6 * time.Second)
	brew(itPotion, 1)
	if n := len(queuedWhole(r.anna, protocol.OpcodeMercWindowState)); n != 0 {
		t.Errorf("a closed window was sent %d states", n)
	}
	// the PS2's box lists the draught
	m.menuStock(r.anna, r.pa, 0)
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "|Infused Draught x25") {
		t.Errorf("the Potion stock box: %q", got)
	}
	// another class's window: no stock
	r2, m2, db2 := newMercFixture(t)
	invItems(t, db2)
	m2.hireWith(r2.anna, r2.pa, 0, 0, 0, -1)
	m2.note(entity.MercNote{Kind: entity.MercNoteBrewed, Owner: r2.pa, Slot: 1, PatternID: itPotion, Count: 5})
	r2.anna.SetMercWindow(1)
	r2.drain()
	m2.Command(r2.anna, r2.pa, "inv")
	if st, ok := lastMercWinState(t, r2.anna); !ok || slices.ContainsFunc(st.recs, func(x mercWinRec) bool { return x.kind == mercRecStock }) {
		t.Errorf("a warrior's window: state %v with stock", ok)
	}
}
