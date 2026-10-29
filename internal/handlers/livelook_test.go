package handlers

import (
	"encoding/binary"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// listGearOf reads a 0x002C payload (opcode first) as a client of the given
// gear size does: the zigzag count, then each record's name (a u32 length
// and the bytes), nine zigzags and its raw gear; the gear of each, and
// whether the payload was read to its end exactly.
func listGearOf(t *testing.T, d []byte, gear int) ([][]byte, bool) {
	t.Helper()
	pos := 2
	zz := func() int64 {
		var u uint64
		for shift := 0; ; shift += 7 {
			if pos >= len(d) {
				return 0
			}
			b := d[pos]
			pos++
			u |= uint64(b&0x7F) << shift
			if b&0x80 == 0 {
				break
			}
		}
		return int64(u>>1) ^ -int64(u&1)
	}
	n := zz()
	var out [][]byte
	for i := int64(0); i < n; i++ {
		if pos+4 > len(d) {
			return out, false
		}
		l := int(binary.LittleEndian.Uint32(d[pos:]))
		pos += 4 + l
		for k := 0; k < 9; k++ {
			zz()
		}
		if pos+gear > len(d) {
			return out, false
		}
		out = append(out, d[pos:pos+gear])
		pos += gear
	}
	return out, pos == len(d)
}

// lookOpsQueued is the opcode of each guaranteed message queued for s, in order.
func lookOpsQueued(s *session.Session) []protocol.GameOpcode {
	var out []protocol.GameOpcode
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 {
			out = append(out, opcode(d))
		}
	}
	return out
}

// The live look's offer comes once, AFTER a session's first character list,
// so a PS2's list keeps the sequence number it always had and is the disc's
// record (32 raw bytes). A client that answers 1 is told the live look is in
// force and sent its list again as the live CLIENT's records (71 raw bytes,
// its dyes in them); answering 0 goes back. The control: a session that never
// answers (a PS2) gets the disc's list every time, and no second offer.
func TestLiveLookOfferAnswerAndList(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/look.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Tinctoria", AccountID: 5, Level: 1}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	who := database.GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	if _, _, err := db.SetCharDye(rec.ServerID, database.DyeChest, 0xD10000FF, who, "test"); err != nil {
		t.Fatal(err)
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, sessionManager: sm, gameWorld: w}
	look := NewLiveLookHandler(h)

	ps2 := sm.CreateSession(testAddr(21), 21, 21, 21, 21, true)
	ps2.SetAccountID(5)
	ps2.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(ps2)
	if got := lookOpsQueued(ps2); len(got) != 2 || got[0] != protocol.OpcodeCharacterSelect || got[1] != protocol.OpcodeLiveLook {
		t.Fatalf("a first list: %v, want the list then the offer", got)
	}
	if d := sentOp(ps2, protocol.OpcodeLiveLook); binary.LittleEndian.Uint32(d[2:]) != 1 {
		t.Fatalf("the offer: %x", d)
	}
	if gear, whole := listGearOf(t, sentOp(ps2, protocol.OpcodeCharacterSelect), entity.DiscGearSize); len(gear) != 1 || !whole {
		t.Fatalf("the PS2's list: %d records, read whole %v", len(gear), whole)
	}
	h.SendCharacterSelectEQOA(ps2)
	if got := lookOpsQueued(ps2); len(got) != 3 || got[2] != protocol.OpcodeCharacterSelect {
		t.Fatalf("a second list: %v, want no second offer", got)
	}

	van := sm.CreateSession(testAddr(22), 22, 22, 22, 22, true)
	van.SetAccountID(5)
	van.SetState(session.StateCharacterSelect)
	h.SendCharacterSelectEQOA(van)
	answer := func(v uint32) {
		msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLiveLook)
		msg.SetData(u32b(v))
		if err := look.HandleLiveLook(van, msg); err != nil {
			t.Fatal(err)
		}
	}
	answer(1)
	got := lookOpsQueued(van)
	if len(got) != 4 || got[2] != protocol.OpcodeLiveLookInForce || got[3] != protocol.OpcodeCharacterSelect {
		t.Fatalf("after the answer: %v, want the look in force then the list again", got)
	}
	if d := sentOp(van, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[2:]) != 1 || !van.LiveLook() {
		t.Fatalf("in force: %x, session %v", d, van.LiveLook())
	}
	gear, whole := listGearOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.LiveGearSize)
	if len(gear) != 1 || !whole {
		t.Fatalf("the live list: %d records, read whole %v", len(gear), whole)
	}
	if chest := binary.BigEndian.Uint32(gear[0][43:]); chest != 0xD10000FF {
		t.Fatalf("the live record's chest: %08x", chest)
	}
	if legs := binary.BigEndian.Uint32(gear[0][43+12:]); legs != 0xFFFFFFFF {
		t.Fatalf("an undyed slot: %08x", legs)
	}

	answer(0)
	got = lookOpsQueued(van)
	if len(got) != 6 || got[4] != protocol.OpcodeLiveLookInForce || got[5] != protocol.OpcodeCharacterSelect || van.LiveLook() {
		t.Fatalf("back to the disc's: %v (session %v)", got, van.LiveLook())
	}
	if gear, whole := listGearOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.DiscGearSize); len(gear) != 1 || !whole {
		t.Fatalf("the disc's list again: %d records, read whole %v", len(gear), whole)
	}

	// in the world: no list is sent again, the look is in force for the
	// channels
	van.SetCharacterInWorld(true)
	van.SetState(session.StateInGame)
	answer(1)
	if got := lookOpsQueued(van); len(got) != 7 || got[6] != protocol.OpcodeLiveLookInForce || !van.LiveLook() {
		t.Fatalf("in the world: %v", got)
	}
}

// A character's dyes are put on its player at the login (loadDyes), and a
// GM's #dye, written with its audit line, on the character where it stands.
func TestDyesAtLoginAndByCommand(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	store, _ := gmRows(t, c)
	db := store.DB()
	c.h.SetDyeStore(db)

	c.say(gs, "#dye chest ff0000")
	if gp.Dyes[database.DyeChest] != 0xFF0000FF {
		t.Fatalf("#dye chest ff0000: %08x on the player", gp.Dyes[database.DyeChest])
	}
	got, err := db.CharDyes(1)
	if err != nil || got[database.DyeChest] != 0xFF0000FF {
		t.Fatalf("the row: %08x %v", got[database.DyeChest], err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'chardyes'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit lines %d %v", n, err)
	}
	c.say(gs, "#dye armor t13")
	if gp.Dyes[database.DyeRobe] != database.RGBA(database.DiscTints[13]) || gp.Dyes[database.DyeHelm] != database.RGBA(database.DiscTints[13]) || gp.Dyes[database.DyePrimary] != 0 {
		t.Fatalf("#dye armor t13: %08x", gp.Dyes)
	}
	c.say(gs, "#dye all none")
	if gp.Dyes != (entity.Dyes{}) {
		t.Fatalf("#dye all none: %08x", gp.Dyes)
	}
	if got, _ := db.CharDyes(1); got != ([database.DyeSlots]uint32{}) {
		t.Fatalf("rows left: %08x", got)
	}
	c.say(gs, "#dye chest 00000000")
	if gp.Dyes[database.DyeChest] != 0 {
		t.Fatal("alpha 0 was taken")
	}

	h := &CharacterHandler{store: store}
	c.say(gs, "#dye legs 400080")
	p := &entity.Player{Name: "Again"}
	h.loadDyes(p, 1)
	if p.Dyes[database.DyeLegs] != 0x400080FF {
		t.Fatalf("at a login: %08x", p.Dyes)
	}
}
