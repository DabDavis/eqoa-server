package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// hideFixture is a gearFixture whose player wears a helm, a chest piece, a
// sword and a cape, with a character store and a World to set its looks in.
type hideFixture struct {
	*gearFixture
	hl *HideLookHandler
}

func newHideFixture(t *testing.T) *hideFixture {
	t.Helper()
	f := &hideFixture{gearFixture: newGearFixture(t)}
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	w.Players[f.player.ServerID] = f.player
	f.sess.SetECSEntity(f.player)
	f.sess.SetCharacterInWorld(true)
	ch := f.h.charHandler
	ch.gameWorld = w
	ch.store = database.NewSQLiteCharacterStore(f.db)
	f.hl = NewHideLookHandler(ch)
	item := func(name string, wear, attack, model, loc int) {
		id := f.pattern(name, wear, attack, 0)
		if _, err := f.db.Exec(`UPDATE itempattern SET model = ? WHERE patternid = ?`, model, id); err != nil {
			t.Fatal(err)
		}
		f.own(id, loc)
	}
	item("Crown", 1, 0, 3, 0)      // head, equip slot 0
	item("Plate", 5, 0, 4, 5)      // chest, 5
	item("Sword", 12, 1, 0x51, 14) // primary, 14
	item("Cloak", wearCape, 0, 2, equipCape)
	ch.applyEquipmentFromDB(f.player, f.sid, false)
	if p := f.player; p.Helm != 3 || p.Chest != 4 || p.Primary != 0x51 || p.CapeStyle != 2 || p.HiddenParts != 0 {
		t.Fatalf("the fixture's look: helm %d chest %d primary %#x cape %d hidden %#x", p.Helm, p.Chest, p.Primary, p.CapeStyle, p.HiddenParts)
	}
	return f
}

// set sends OpcodeHideLookSet and returns the hidden slots answered.
func (f *hideFixture) set(slot, hide byte) uint32 {
	f.t.Helper()
	f.sess.Queue().GetPending()
	if err := f.hl.HandleSet(f.sess, incoming(protocol.OpcodeHideLookSet, []byte{slot, hide})); err != nil {
		f.t.Fatal(err)
	}
	got := f.sess.Queue().GetPending()
	if len(got) != 1 || binary.LittleEndian.Uint16(got[0].Data()) != uint16(protocol.OpcodeHideLookState) || len(got[0].Data()) != 6 {
		f.t.Fatalf("the answer: %d messages", len(got))
	}
	return binary.LittleEndian.Uint32(got[0].Data()[2:])
}

// A slot hidden keeps its item worn and its stats, and the look goes bare in
// the channel, the character list and the cape; it is kept for the next
// login; shown again it is all back, the row gone.
func TestHidingAWornLook(t *testing.T) {
	f := newHideFixture(t)
	if got := buildHideLookOffer(0x21); string(got) != "\x6c\x7e\x01\x21\x00\x00\x00" {
		t.Fatalf("the offer: % x", got)
	}
	if got := f.set(0, 1); got != 1 {
		t.Fatalf("the helm hidden: %#x", got)
	}
	if got := f.set(5, 1); got != 1|1<<5 {
		t.Fatalf("the chest hidden: %#x", got)
	}
	p := f.player
	if p.HiddenParts != entity.HideHelm|entity.HideChest || p.Helm != 3 || p.Chest != 4 {
		t.Fatalf("hidden parts %#x, helm %d chest %d (worn still)", p.HiddenParts, p.Helm, p.Chest)
	}
	b := entity.SerializePlayerUpdate(7, p)
	if b[93] != 0 || b[98] != 0 || binary.LittleEndian.Uint32(b[67:71]) != 0x51 {
		t.Errorf("the channel: chest %d helm %d primary % x", b[93], b[98], b[67:71])
	}
	if locs, err := f.db.CharHiddenLooks(f.sid); err != nil || locs != 1|1<<5 {
		t.Fatalf("kept: %#x %v", locs, err)
	}
	if got := f.worn(); len(got) != 4 {
		t.Errorf("still worn: %v", got)
	}

	// the next login reads it back
	again := &entity.Player{CharServerID: f.sid}
	f.h.charHandler.applyEquipmentFromDB(again, f.sid, false)
	if again.HiddenLocs != 1|1<<5 || again.HiddenParts != entity.HideHelm|entity.HideChest {
		t.Errorf("at the next login: %#x %#x", again.HiddenLocs, again.HiddenParts)
	}
	items, _ := f.inv.GetCharacterItems(f.sid)
	g := characterGearHiding(items, f.h.charHandler.hiddenLooks(f.sid))
	if g[20] != 0xFF || g[25] != 0xFF || binary.LittleEndian.Uint32(g[5:9]) != 0x51 {
		t.Errorf("the character list: % x", g)
	}

	// the sword and the cape: the hand bare, its swing kept; no cape told
	f.set(14, 1)
	f.set(equipCape, 1)
	if p.CapeStyle != 0 || p.PrimaryType != 1 || p.HiddenParts&entity.HidePrimary == 0 {
		t.Errorf("sword and cape hidden: cape %d, type %d, parts %#x", p.CapeStyle, p.PrimaryType, p.HiddenParts)
	}
	if _, ok := characterCape(f.sid, items, f.h.charHandler.hiddenLooks(f.sid)); ok {
		t.Error("the list's cape is told hidden")
	}
	b = entity.SerializePlayerUpdate(7, p)
	if binary.LittleEndian.Uint32(b[67:71]) != 0 || b[91] != 1 {
		t.Errorf("the hidden sword: % x type %d", b[67:71], b[91])
	}

	// a slot past the cape's is refused, the state answered as it was
	if got := f.set(40, 1); got != 1|1<<5|1<<14|1<<22 {
		t.Errorf("slot 40: %#x", got)
	}

	for _, s := range []byte{0, 5, 14, equipCape} {
		f.set(s, 0)
	}
	if p.HiddenLocs != 0 || p.HiddenParts != 0 || p.CapeStyle != 2 {
		t.Errorf("all shown: %#x %#x cape %d", p.HiddenLocs, p.HiddenParts, p.CapeStyle)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM charhiddenlooks`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left: %d %v", n, err)
	}
}

// A hidden slot hides whatever is worn in it: a new helm in a hidden head
// slot is hidden too, and an item worn elsewhere is not.
func TestAHiddenSlotHidesWhatComesIntoIt(t *testing.T) {
	head := database.CharInventoryEntry{EquipLoc: 0, Pattern: database.ItemPatternRecord{EquipSlot: 1, Model: 9}}
	two := database.CharInventoryEntry{EquipLoc: 16, Pattern: database.ItemPatternRecord{EquipSlot: 15, Model: 0x77, AttackType: 2}}
	off := database.CharInventoryEntry{EquipLoc: 15, Pattern: database.ItemPatternRecord{EquipSlot: 14, Model: 0x66, AttackType: 3}}
	p := &entity.Player{HiddenLocs: 1<<0 | 1<<16}
	wornLook(p, []database.CharInventoryEntry{head, two, off})
	if p.HiddenParts != entity.HideHelm|entity.HidePrimary {
		t.Errorf("parts %#x, want the helm and the main hand", p.HiddenParts)
	}
	p.HiddenLocs = 1 << 14 // the primary's slot, empty: the two-hander shows
	wornLook(p, []database.CharInventoryEntry{head, two, off})
	if p.HiddenParts != 0 {
		t.Errorf("parts %#x, want none", p.HiddenParts)
	}
}
