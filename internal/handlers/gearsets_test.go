package handlers

import (
	"bytes"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The HUD's item quick slots and weapon sets against a temporary database, held
// to the wire formats SOE's live client reads and writes
// (claude-notes/live-toolbars-gear-sets-20261024.md). Item names are made up.

type gearFixture struct {
	t      *testing.T
	inv    *database.InventoryStore
	db     *database.SQLiteDB
	h      *EquipHandler
	sess   *session.Session
	player *entity.Player
	sid    uint32
}

func newGearFixture(t *testing.T) *gearFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/gear.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &gearFixture{t: t, db: db, inv: database.NewInventoryStore(db)}
	rec := &database.CharacterRecord{CharName: "Gear", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f.sid = rec.ServerID
	f.player = &entity.Player{ServerID: 1002, CharServerID: f.sid, Name: "Gear", Level: 20}
	f.sess = testSession()
	ch := &CharacterHandler{inventoryStore: f.inv}
	f.h = NewEquipHandler(nil, f.inv, nil, ch)
	f.h.playerOf = func(*session.Session) *entity.Player { return f.player }
	return f
}

// pattern adds an item: its wear slot, attack type and type (procanim).
func (f *gearFixture) pattern(name string, wear, attack, typ int) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, 209763529, 0, ?, 0, 0, 1, 0, ?, 0, 0, 1, 1, 0, 0, 32767, 1023, ?, 0, 0, 0, ?, '', 0, 0)`,
		wear, attack, typ, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// own puts one of an item in the inventory, worn in slot (-1 none), and
// returns its charinventory.itemid.
func (f *gearFixture) own(pattern, slot int) int {
	f.t.Helper()
	id, err := f.inv.AddCharItem(f.sid, pattern, 1, 0, 0, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	if slot >= 0 {
		if err := f.inv.SetEquipLoc(f.sid, id, slot); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

// loadList is what SendMemoryDump leaves: the client's list, key = position.
func (f *gearFixture) loadList() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvList(rows))
}

// send hands the handler one client message (opcode, payload) and returns
// every message the session then holds, each with its opcode first.
func (f *gearFixture) send(op protocol.GameOpcode, fill func(w *buffer.Writer)) [][]byte {
	f.t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, op)
	msg.SetData(build(op, 256, fill)[2:])
	r := NewRegistry()
	f.h.RegisterHandlers(r)
	h, ok := r.Get(op)
	if !ok {
		f.t.Fatalf("no handler for %#04x", op)
	}
	if err := h(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func (f *gearFixture) prepare(key uint32, pos byte) [][]byte {
	return f.send(protocol.OpcodePrepareItem, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteByte(pos)
	})
}

func (f *gearFixture) worn() map[int]int {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	out := map[int]int{}
	for _, e := range rows {
		if e.EquipLoc >= 0 {
			out[e.EquipLoc] = e.ItemID
		}
	}
	return out
}

func (f *gearFixture) quick() map[int]int {
	f.t.Helper()
	q, err := f.inv.QuickSlotsOf(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return q
}

// A potion goes on slots 0-1, a gem on 2-3 (the live client's) and a weapon on
// 3 (the disc's), each answered as Msg000A reads it: the item's list index and
// the slot. Anything else is refused, unanswered and not kept (the control).
func TestPrepareItemTakesWhatEitherClientDoes(t *testing.T) {
	f := newGearFixture(t)
	potion := f.own(f.pattern("Draught", -1, 0, 2), -1)
	gem := f.own(f.pattern("Stone", -1, 0, 7), -1)
	sword := f.own(f.pattern("Blade", 14, 1, 0), -1)
	f.loadList()
	for _, c := range []struct {
		key  uint32
		pos  byte
		item int
		ok   bool
	}{
		{0, 1, potion, true}, {1, 2, gem, true}, {1, 3, gem, true}, {2, 3, sword, true},
		{0, 2, potion, false}, {1, 0, gem, false}, {2, 1, sword, false}, {2, 2, sword, false}, {0, 4, potion, false},
	} {
		before := f.quick()
		out := f.prepare(c.key, c.pos)
		if !c.ok {
			if len(out) != 0 || len(f.quick()) != len(before) {
				t.Errorf("key %d on slot %d: answered %x, kept %v: want refused", c.key, c.pos, out, f.quick())
			}
			continue
		}
		want := []byte{0x0A, 0x00, byte(c.key), c.pos}
		if len(out) != 1 || !bytes.Equal(out[0], want) {
			t.Errorf("key %d on slot %d: answered %x, want %x", c.key, c.pos, out, want)
		}
		if got := f.quick()[c.item]; got != int(c.pos) {
			t.Errorf("key %d: kept on slot %d, want %d", c.key, got, c.pos)
		}
	}
}

// One item a slot and one slot an item, as Msg000A keeps them: a second item
// on a slot takes the first off, and an item moved comes off its old slot.
func TestQuickSlotHoldsOneItem(t *testing.T) {
	f := newGearFixture(t)
	p := f.pattern("Draught", -1, 0, 2)
	a, b := f.own(p, -1), f.own(p, -1)
	f.loadList()
	f.prepare(0, 0)
	f.prepare(1, 0)
	if q := f.quick(); len(q) != 1 || q[b] != 0 {
		t.Fatalf("after b onto 0: %v, want only b on 0", q)
	}
	f.prepare(1, 1)
	if q := f.quick(); len(q) != 1 || q[b] != 1 {
		t.Fatalf("after b onto 1: %v, want b on 1 only", q)
	}
	_ = a
}

// An item that leaves the inventory leaves its quick slot: banked or deleted.
func TestBankAndDeleteDropTheQuickSlot(t *testing.T) {
	f := newGearFixture(t)
	p := f.pattern("Draught", -1, 0, 2)
	a, b := f.own(p, -1), f.own(p, -1)
	if err := f.inv.SetQuickSlot(f.sid, 0, a); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.SetQuickSlot(f.sid, 1, b); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.MoveCharItem(f.sid, a, 1); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.DeleteCharItem(f.sid, b); err != nil {
		t.Fatal(err)
	}
	if q := f.quick(); len(q) != 0 {
		t.Fatalf("quick slots %v, want none", q)
	}
}

// The MemoryDump writes a prepared item's slot in its record's fifth field and
// every other inventory record's as -1; writeSingleItem (an item that arrives
// later, the bank, a corpse, a merchant) never carries one.
func TestDumpCarriesTheQuickSlot(t *testing.T) {
	e := &database.CharInventoryEntry{ItemID: 7, Location: 0}
	field := func(write func(w *buffer.Writer)) int64 {
		buf := make([]byte, 4096)
		w := buffer.NewWriter(buf)
		write(w)
		r := buffer.NewReader(buf[:w.Position()])
		var v int64
		for i := 0; i < 5; i++ {
			v, _ = r.Read7BitEncodedInt64()
		}
		return v
	}
	if got := field(func(w *buffer.Writer) { writeItem(w, e, 0, 2) }); got != 2 {
		t.Errorf("the dump's record: quick slot %d, want 2", got)
	}
	if got := field(func(w *buffer.Writer) { writeSingleItem(w, e, 0) }); got != -1 {
		t.Errorf("any other record: quick slot %d, want -1", got)
	}
}

// A set is the hands as worn: the main hand's and the off hand's items, by
// pattern on the wire after the name, as the live dispatcher reads 0x0021
// (u8 set, u32 count + UTF-16, u32, u32); the name is cut to the 15 characters
// the client keeps.
func TestPrepareWeaponIsTheHandsAsWorn(t *testing.T) {
	f := newGearFixture(t)
	swordP, shieldP := f.pattern("Blade", 12, 1, 0), f.pattern("Buckler", 13, 0, 0)
	sword, shield := f.own(swordP, 14), f.own(shieldP, 17)
	f.own(f.pattern("Tunic", 5, 0, 0), 5)
	f.loadList()
	out := f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(2)
		writeWideString(w, "Tank", 64)
	})
	want := []byte{0x21, 0x00, 2, 4, 0, 0, 0, 'T', 0, 'a', 0, 'n', 0, 'k', 0,
		byte(swordP), byte(swordP >> 8), 0, 0, byte(shieldP), byte(shieldP >> 8), 0, 0}
	if len(out) != 1 || !bytes.Equal(out[0], want) {
		t.Fatalf("answer %x, want %x", out, want)
	}
	sets, err := f.inv.GearSetsOf(f.sid)
	if err != nil {
		t.Fatal(err)
	}
	if g := sets[2]; g.Name != "Tank" || g.MainItem != sword || g.OffItem != shield || g.MainPattern != swordP || g.OffPattern != shieldP {
		t.Fatalf("set 2 = %+v", g)
	}
	if sets[0] != database.EmptyGearSet {
		t.Fatalf("set 0 = %+v, want empty", sets[0])
	}
	// a long name keeps 15 characters; set 4 is no set (the control)
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(1)
		writeWideString(w, "ABCDEFGHIJKLMNOPQRST", 64)
	})
	if out := f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(4)
		writeWideString(w, "Nope", 64)
	}); len(out) != 0 {
		t.Fatalf("set 4 answered %x", out)
	}
	sets, _ = f.inv.GearSetsOf(f.sid)
	if sets[1].Name != "ABCDEFGHIJKLMNO" {
		t.Fatalf("set 1's name %q, want 15 characters", sets[1].Name)
	}
}

// Taking a set up puts its main hand in its slot, then its off hand, each with
// EquipItem naming the client's list index; set 4 (bare hands) takes off every
// hand slot with UnequipItem. A set with no off hand empties it.
func TestUseWeaponSetEquipsTheSet(t *testing.T) {
	f := newGearFixture(t)
	swordP, daggerP, axeP := f.pattern("Blade", 14, 1, 0), f.pattern("Dirk", 14, 3, 0), f.pattern("Greataxe", 15, 2, 0)
	sword, dagger := f.own(swordP, 14), f.own(daggerP, 15)
	axe := f.own(axeP, -1)
	f.loadList()
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(0)
		writeWideString(w, "Dual", 64)
	})
	// the two-hander on instead (it takes both hands off)
	for _, c := range []equipChange{{sword, -1}, {dagger, -1}, {axe, 16}} {
		if err := f.inv.SetEquipLoc(f.sid, c.itemID, c.equipLoc); err != nil {
			t.Fatal(err)
		}
	}
	use := func(set byte) [][]byte {
		return f.send(protocol.OpcodeUseWeaponSet, func(w *buffer.Writer) { w.WriteByte(set) })
	}
	out := use(0)
	// list order: sword 0, dagger 1, axe 2
	want := [][]byte{{0x3F, 0x00, 0, 14}, {0x3F, 0x00, 1, 15}}
	if len(out) != 2 || !bytes.Equal(out[0], want[0]) || !bytes.Equal(out[1], want[1]) {
		t.Fatalf("set 0: %x, want %x", out, want)
	}
	if w := f.worn(); w[14] != sword || w[15] != dagger || w[16] != 0 {
		t.Fatalf("worn after set 0: %v", w)
	}
	// already as the set has it: nothing is sent
	if out := use(0); len(out) != 0 {
		t.Fatalf("set 0 again: %x, want nothing", out)
	}
	out = use(4)
	if len(out) != 2 || !bytes.Equal(out[0], []byte{0x40, 0x00, 14}) || !bytes.Equal(out[1], []byte{0x40, 0x00, 15}) {
		t.Fatalf("bare hands: %x", out)
	}
	if w := f.worn(); len(w) != 0 {
		t.Fatalf("worn after bare hands: %v", w)
	}
	// a set of the two-hander alone, taken up over a shield: the shield comes off
	shield := f.own(f.pattern("Buckler", 13, 0, 0), -1)
	f.loadList()
	if err := f.inv.SetEquipLoc(f.sid, axe, 16); err != nil {
		t.Fatal(err)
	}
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(1)
		writeWideString(w, "Big", 64)
	})
	if err := f.inv.SetEquipLoc(f.sid, axe, -1); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.SetEquipLoc(f.sid, sword, 14); err != nil {
		t.Fatal(err)
	}
	if err := f.inv.SetEquipLoc(f.sid, shield, 17); err != nil {
		t.Fatal(err)
	}
	use(1)
	if w := f.worn(); w[16] != axe || len(w) != 1 {
		t.Fatalf("worn after set 1: %v, want the two-hander alone", w)
	}
}

// The world entry's sets: all four, an empty one as no name and -1, -1.
func TestSendGearSetsSendsFour(t *testing.T) {
	f := newGearFixture(t)
	p := f.pattern("Blade", 12, 1, 0)
	f.own(p, 14)
	f.loadList()
	f.send(protocol.OpcodePrepareWeapon, func(w *buffer.Writer) {
		w.WriteByte(3)
		writeWideString(w, "X", 64)
	})
	f.h.SendGearSets(f.sess)
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, m.Data())
	}
	if len(out) != 4 {
		t.Fatalf("%d messages, want 4", len(out))
	}
	empty := []byte{0x21, 0x00, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	if !bytes.Equal(out[0], empty) {
		t.Errorf("set 0: %x, want %x", out[0], empty)
	}
	if out[3][2] != 3 || out[3][3] != 1 || out[3][7] != 'X' {
		t.Errorf("set 3: %x", out[3])
	}
}

// The HUD's item quick slot asks with interaction 1 (UseItem(i, 1)): the same
// use or equip as the item menu's 0; 2 is not a request (the control).
func TestInteractionOneIsTheQuickSlotsUse(t *testing.T) {
	f := newGearFixture(t)
	f.own(f.pattern("Blade", 12, 1, 0), -1)
	f.loadList()
	interact := func(b byte) [][]byte {
		return f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
			w.WriteUint32(0)
			w.WriteUint32(0xFFFFFFFF)
			w.WriteByte(b)
		})
	}
	if out := interact(2); len(out) != 0 {
		t.Fatalf("interaction 2: %x, want nothing", out)
	}
	if out := interact(1); len(out) != 1 || !bytes.Equal(out[0], []byte{0x3F, 0x00, 0, 14}) {
		t.Fatalf("interaction 1: %x, want the equip", out)
	}
}

// An item is put on only by a player CLIENT's CanUse lets wear it (the class
// mask, the race mask, the level), refused in Sandstorm's red lines (the
// user, 2026-10-29: a level 2 shadow knight wore level 42 bracers): a level 2
// shadow knight is refused a level 42 bracer, an item another class's mask
// names and one another race's does; the control: a level 1 one of its own
// goes on, and a mask of 0 (2,189 of the live server's items) is not checked.
func TestAnItemIsWornOnlyByWhoMayWearIt(t *testing.T) {
	f := newGearFixture(t)
	f.player.Class, f.player.Race, f.player.Level = 3, 2, 2 // a dark elf shadow knight
	mk := func(name string, lvl, classes, races int) {
		id := f.pattern(name, 7, 0, 0)
		if _, err := f.db.Exec(`UPDATE itempattern SET levelreq = ?, classuse = ?, raceuse = ? WHERE patternid = ?`,
			lvl, classes, races, id); err != nil {
			t.Fatal(err)
		}
		f.own(id, -1)
	}
	mk("Bracers of Forty Two", 42, 527, 1023) // 0: too high
	mk("Paladin's Bracers", 1, 1<<2, 1023)    // 1: another class
	mk("Elven Bracers", 1, 1<<3, 1<<1)        // 2: its class, another race
	mk("Bone Bracers", 1, 1<<3, 1<<2)         // 3: its own, level 1
	mk("Bracers of No Mask", 1, 0, 0)         // 4: no masks: not checked
	f.loadList()
	equip := func(key uint32) [][]byte {
		return f.send(protocol.OpcodeInteractItem, func(w *buffer.Writer) {
			w.WriteUint32(key)
			w.WriteUint32(0xFFFFFFFF)
			w.WriteByte(0)
		})
	}
	for key, want := range map[uint32]string{0: scribeTooLow, 1: scribeWrongClass, 2: scribeWrongRace} {
		out := equip(key)
		if len(out) != 1 || !bytes.Contains(out[0], utf16le(want)) {
			t.Errorf("item %d: %x, want %q", key, out, want)
		}
	}
	if w := f.worn(); len(w) != 0 {
		t.Fatalf("refused items were put on: %v", w)
	}
	if out := equip(3); len(out) != 1 || out[0][0] != 0x3F {
		t.Errorf("its own level 1 bracers: %x, want the equip", out)
	}
	if out := equip(4); len(out) == 0 || out[len(out)-1][0] != 0x3F {
		t.Errorf("bracers with no masks: %x, want the equip", out)
	}
}
