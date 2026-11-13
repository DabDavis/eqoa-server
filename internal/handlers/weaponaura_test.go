package handlers

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// twistedVirtue is the Heirloom Sword of Twisted Virtue's colour (itempattern
// 714382): the Sword of Virtue's model in crimson (the owner, 2026-11-13).
const twistedVirtue = 0xB0102AFF

// A held item's own colour (itempattern.color) is its hand's in the live
// game's look where the hand has no dye: main hand, off hand and shield, each
// by the hand wornLook puts it in. White (plain), a disc tint's index and none
// are nothing of its own. A PS2's channel is the same bytes whatever the
// colour: the disc's 162 have no place for it. The control: the player's own
// dyes untouched, and a weapon dye over the colour kept.
func TestAHeldItemsOwnColourIsItsHands(t *testing.T) {
	sword := func(loc int, color int64) database.CharInventoryEntry {
		return database.CharInventoryEntry{EquipLoc: loc, Pattern: database.ItemPatternRecord{EquipSlot: 12, Model: int64(int32(-1997797075)), Color: color}}
	}
	shield := database.CharInventoryEntry{EquipLoc: 17, Pattern: database.ItemPatternRecord{EquipSlot: 13, Model: 876411255, Color: 0x0000C0FF}}
	p := &entity.Player{}
	wornLook(p, []database.CharInventoryEntry{sword(14, twistedVirtue), sword(15, 0xFFFFFFFF), shield})
	if p.HeldColors != [3]uint32{twistedVirtue, 0, 0x0000C0FF} {
		t.Fatalf("held colours %08X", p.HeldColors)
	}
	for _, plain := range []int64{0xFFFFFFFF, 5, 0} {
		q := &entity.Player{}
		wornLook(q, []database.CharInventoryEntry{sword(16, plain)})
		if q.HeldColors != [3]uint32{} || q.Primary == 0 {
			t.Errorf("a two-hander of colour %#x: %08X (primary %#x), want none", plain, q.HeldColors, q.Primary)
		}
	}
	d := p.LookDyes()
	if d[database.DyePrimary] != twistedVirtue || d[database.DyeSecondary] != 0 || d[database.DyeShield] != 0x0000C0FF {
		t.Fatalf("look dyes %08X", *d)
	}
	if p.Dyes != (entity.Dyes{}) {
		t.Fatal("the player's own dyes changed")
	}
	p.Dyes[database.DyePrimary] = 0x00FF00FF // dyed green: the item's own colour wins (the owner, 2026-11-13)
	if got := p.LookDyes()[database.DyePrimary]; got != twistedVirtue {
		t.Errorf("a dyed sword: %08X, want its own colour", got)
	}
	p.Dyes[database.DyeSecondary] = 0x00FF00FF // a dyed hand holding a plain item keeps its dye
	if got := p.LookDyes()[database.DyeSecondary]; got != 0x00FF00FF {
		t.Errorf("a dyed plain item: %08X, want the dye", got)
	}
	p.Dyes = entity.Dyes{}

	// the live game's channel: the hands' colours at 99-110
	disc := entity.SerializePlayerUpdate(1, p)
	live := entity.LiveChannel(disc, p.LookDyes())
	for i, want := range []uint32{twistedVirtue, 0xFFFFFFFF, 0x0000C0FF} {
		if got := binary.BigEndian.Uint32(live[99+4*i:]); got != want {
			t.Errorf("the live channel's item %d: %08X, want %08X", i, got, want)
		}
	}
	// a PS2's: the same bytes as a plain sword's
	plain := &entity.Player{}
	wornLook(plain, []database.CharInventoryEntry{sword(14, 0xFFFFFFFF), sword(15, 0xFFFFFFFF), shield})
	if !bytes.Equal(disc, entity.SerializePlayerUpdate(1, plain)) || len(disc) != entity.EntityStateSize {
		t.Error("a PS2's channel differs by a held item's colour")
	}
}

// The character list carries a held item's colour to a client of the live
// look (the gear's held-item colours, 31-42), so character select draws it
// as the world does; a PS2's list is the disc's 32 bytes as ever.
func TestTheListCarriesAHeldItemsColour(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/aura.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Dust", AccountID: 5, Level: 60}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 0, 0, 12, 0, 0, 0, 0, 1, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Heirloom Sword of Twisted Virtue', '', ?, ?)`,
		int64(int32(-1997797075)), int64(twistedVirtue))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	inv := database.NewInventoryStore(db)
	id, err := inv.AddCharItem(rec.ServerID, int(pid), 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.SetEquipLoc(rec.ServerID, id, 14); err != nil {
		t.Fatal(err)
	}
	sm := session.NewManager()
	w := entity.NewWorld()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, inventoryStore: inv, sessionManager: sm, gameWorld: w}

	ps2 := sm.CreateSession(testAddr(41), 41, 41, 41, 41, true)
	ps2.SetAccountID(5)
	ps2.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(ps2)
	gear, whole := listGearOf(t, sentOp(ps2, protocol.OpcodeCharacterSelect), entity.DiscGearSize)
	if len(gear) != 1 || !whole || binary.LittleEndian.Uint32(gear[0][5:]) != 0x88EC092D {
		t.Fatalf("the PS2's list: %d records, whole %v, %x", len(gear), whole, gear)
	}

	van := sm.CreateSession(testAddr(42), 42, 42, 42, 42, true)
	van.SetAccountID(5)
	van.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(van)
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLiveLook)
	msg.SetData(u32b(1))
	if err := NewLiveLookHandler(h).HandleLiveLook(van, msg); err != nil {
		t.Fatal(err)
	}
	gear, whole = listGearOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.LiveGearSize)
	if len(gear) != 1 || !whole {
		t.Fatalf("the live list: %d records, whole %v", len(gear), whole)
	}
	if got := binary.BigEndian.Uint32(gear[0][31:]); got != twistedVirtue {
		t.Errorf("the live record's main hand: %08X, want %08X", got, uint32(twistedVirtue))
	}
	if got := binary.BigEndian.Uint32(gear[0][35:]); got != 0xFFFFFFFF {
		t.Errorf("an empty off hand: %08X, want white", got)
	}
}
