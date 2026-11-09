package handlers

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// The heirloom capes in the shops and at the dyers (the user, 2026-11-09: "add heirloom capes to the genie and
// heirloom vendors, also allow dying"; capes.go, genie.go, dyer.go). Item names are made up.

// merchantCount is a 0x00B7's record count (after the merchant id and the two
// rates), the list's size.
func merchantCount(t *testing.T, d []byte) int {
	t.Helper()
	r := buffer.NewReader(d[2:])
	r.ReadUint32()
	r.Read7BitEncodedInt64()
	r.Read7BitEncodedInt64()
	n, err := r.Read7BitEncodedInt64()
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

// capeRecord adds a cape record: style, colour and name.
func capeRecord(t *testing.T, db *database.SQLiteDB, name string, style int, color int64) int {
	t.Helper()
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1394465037, 0, 28, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, ?, '', ?, ?)`,
		name, style, color)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// stock puts records on an npcitem list after what it holds.
func stock(t *testing.T, db *database.SQLiteDB, npcid int, items ...int) {
	t.Helper()
	for _, pid := range items {
		if _, err := db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber)
			VALUES (?, 1, 0, ?, (SELECT COALESCE(MAX(listnumber), -1) + 1 FROM npcitem WHERE npcid = ?))`, npcid, pid, npcid); err != nil {
			t.Fatal(err)
		}
	}
}

// A shop lists a cape only to a client that draws capes, after what it
// listed before; a client that does not (a PS2) is listed what it always was,
// and its buy names the record its list showed. The control: on a server
// without capes no client is listed one.
func TestShopsListCapesOnlyToClientsThatDrawThem(t *testing.T) {
	capesOn(t)
	f := newGenieFixture(t)
	cape := capeRecord(t, f.db, "Heirloom Nightborne Cloak", 2, 0)
	stock(t, f.db, 5002, cape)
	outfitter := &entity.NPC{Name: "Heirloom Outfitter", NPCID: 5002, NPCType: 0x81}
	names := func() []string { // as the shop's window and its buys read it
		var out []string
		for _, w := range f.windows.merchantList(f.p, outfitter, readsCapes(f.sess)).wares {
			out = append(out, w.Pattern.ItemName)
		}
		return out
	}
	capeSession(f.sess)
	if got := names(); strings.Join(got, ",") != "Heirloom Blade,Heirloom Nightborne Cloak" {
		t.Fatalf("to a client that draws capes: %v", got)
	}
	f.sess.SetCapes(false) // a PS2
	if got := names(); strings.Join(got, ",") != "Heirloom Blade" {
		t.Fatalf("to a PS2: %v (want its list as before)", got)
	}
	f.sess.SetCapes(true)
	entity.SetCapes(false)
	if got := names(); len(got) != 1 {
		t.Fatalf("the control, a server without capes: %v", got)
	}
}

// The genie sells every cape to a client that draws them (genie.go): "Your
// wares" asks which, "Heirloom gear" opens its outfitter's list (the cape it
// stocks among it) and "Heirloom capes" every cape (capeStock), bought from as
// any shop. The controls: a PS2 and a server without capes are shown the gear
// at once, as before, and no cape in it.
func TestTheGenieSellsCapes(t *testing.T) {
	capesOn(t)
	f := newGenieFixture(t)
	azure := capeRecord(t, f.db, "Heirloom Azure Knight's Cape", 1, 0)
	crimson := capeRecord(t, f.db, "Heirloom Crimson Cloak", 2, 0xB01818FF)
	stock(t, f.db, 5002, azure)
	stock(t, f.db, capeStock, azure, crimson)

	// a PS2: the gear at once, as before, the cape left out
	msgs := f.answer(f.summon(), genieShop)
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeMerchantBox || merchantCount(t, msgs[0]) != 1 {
		t.Fatalf("a PS2's wares: %s", ops(msgs))
	}

	capeSession(f.sess)
	box := f.summon()
	msgs = f.answer(box, genieShop)
	if len(msgs) == 0 || f.genieNPC() == nil {
		t.Fatalf("the wares' question: %s, genie %v", ops(msgs), f.genieNPC())
	}
	for _, m := range msgs {
		if opOf(m) == protocol.OpcodeMerchantBox {
			t.Fatal("a shop opened before the question was answered")
		}
	}
	wares := boxOf(t, msgs)
	if !IsGenieBox(wares) || !bytes.Contains(joinBytes(msgs), utf16le("Which wares, master?")) ||
		!bytes.Contains(joinBytes(msgs), utf16le("Heirloom capes")) {
		t.Fatalf("no wares question in %q", msgs)
	}
	gid := entity.GenieWindowID(f.genieNPC().ServerID)
	msgs = f.answer(wares, 1) // Heirloom capes
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeMerchantBox || f.genieNPC() != nil {
		t.Fatalf("the capes: sent %s, genie %v", ops(msgs), f.genieNPC())
	}
	if r := buffer.NewReader(msgs[0][2:]); func() uint32 { id, _ := r.ReadUint32(); return id }() != gid || merchantCount(t, msgs[0]) != 2 ||
		!bytes.Contains(msgs[0], utf16le("Heirloom Crimson Cloak")) || bytes.Contains(msgs[0], utf16le("Heirloom Blade")) {
		t.Fatalf("the capes' list: %d records", merchantCount(t, msgs[0]))
	}
	sent := f.send(protocol.OpcodeMerchantBuy, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(1) // the crimson cloak
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(gid)
	})
	if len(sent) == 0 || opOf(sent[0]) != protocol.OpcodeAddInvItem {
		t.Fatalf("a cape bought from the genie: sent %s", ops(sent))
	}
	if loc, _ := f.where(crimson); loc != 0 {
		t.Fatalf("the crimson cloak not bought: location %d", loc)
	}
	f.loadLists()

	// the gear, the cape its outfitter stocks among it
	msgs = f.answer(f.summon(), genieShop)
	msgs = f.answer(boxOf(t, msgs), 0)
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeMerchantBox || merchantCount(t, msgs[0]) != 2 ||
		!bytes.Contains(msgs[0], utf16le("Heirloom Blade")) || !bytes.Contains(msgs[0], utf16le("Heirloom Azure Knight's Cape")) {
		t.Fatalf("the gear: sent %s", ops(msgs))
	}

	// the control: a server without capes shows the gear at once
	entity.SetCapes(false)
	msgs = f.answer(f.summon(), genieShop)
	if len(msgs) != 1 || opOf(msgs[0]) != protocol.OpcodeMerchantBox || merchantCount(t, msgs[0]) != 1 {
		t.Fatalf("no capes on the server: sent %s", ops(msgs))
	}
}

// boxOf is the id of the question box among msgs (a popup's first u32).
func boxOf(t *testing.T, msgs [][]byte) uint32 {
	t.Helper()
	for _, m := range msgs {
		if len(m) >= 6 && opOf(m) == protocol.OpcodeQuestPopupMulti {
			return binary.LittleEndian.Uint32(m[2:6])
		}
	}
	t.Fatalf("no box in %s", ops(msgs))
	return 0
}

// An Armor Dyer on a server with capes dyes the cape (dyer.go): "Robe, cape,
// shield or weapons" names it second, "My cape" is the first question's third
// answer, "All my armor" takes it in, "Take every dye off" takes its dye off;
// its dye is chardyes' cape slot and the colour the character's cape is drawn
// in (CapeTint), over the record's own. The control: without capes the dyer's
// "All my armor" leaves the cape's slot alone.
func TestTheArmorDyerDyesTheCape(t *testing.T) {
	capesOn(t)
	tw := newTalkWorld(t, 100, 100)
	db, err := database.NewSQLite(t.TempDir() + "/dyer.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	rec := &database.CharacterRecord{CharName: "Talker", AccountID: 1, Level: 10}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	tw.p.CharServerID = rec.ServerID
	tw.p.CapeStyle, tw.p.CapeColor = 2, 0x7020A0FF
	d := NewDyer(tw.w, db)
	dyer := &entity.NPC{ServerID: 3101, Name: "Armor Dyer", NPCType: 0x80}
	questMsgs(tw.s)
	talk := func() []string {
		t.Helper()
		if !d.Talk(tw.s, tw.p, dyer) {
			t.Fatal("the dyer did not answer")
		}
		return questMsgs(tw.s)
	}
	pick := func(msgs []string, n byte) []string {
		t.Helper()
		if !d.Answer(tw.s, tw.p, boxID(msgs), n) {
			t.Fatalf("not the dyer's box: %q", msgs)
		}
		return questMsgs(tw.s)
	}
	capeDye := func() uint32 {
		t.Helper()
		c, err := db.CharDyes(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		return c[database.DyeCape]
	}

	got := talk()
	if !hasMsg(got, "box:Armor Dyer: I dye armor and capes*") ||
		!strings.HasSuffix(got[len(got)-1], ":What shall I dye?|All my armor|One piece of armor|My cape|Robe, cape, shield or weapons|Take every dye off") {
		t.Fatalf("the dyer's question with capes: %q", got)
	}
	got = pick(got, 3)
	if !strings.Contains(got[len(got)-1], ":Which one?|Robe|Cape|Shield|Primary weapon|Secondary weapon") {
		t.Fatalf("the robe's question: %q", got)
	}
	pick(pick(pick(got, 1), 1), 1) // Cape; Reds and pinks; Crimson
	if c := capeDye(); c != 0xA0001EFF || tw.p.CapeTint() != 0xA0001EFF {
		t.Fatalf("the cape crimson: row %08X, drawn %08X", c, tw.p.CapeTint())
	}
	pick(pick(pick(talk(), 0), 4), 5) // All my armor, Blues and teals, Navy
	if navy := database.RGBA(database.DiscTints[4]); capeDye() != navy {
		t.Fatalf("all my armor: the cape %08X, want navy", capeDye())
	}
	pick(talk(), 4) // Take every dye off
	if capeDye() != 0 || tw.p.CapeTint() != 0x7020A0FF {
		t.Fatalf("every dye off: the cape %08X, drawn %08X (want its record's)", capeDye(), tw.p.CapeTint())
	}

	entity.SetCapes(false)
	got = talk()
	if !strings.Contains(got[len(got)-1], "|Robe, shield or weapons|") || hasMsg(got, "box:Armor Dyer: I dye armor and capes*") {
		t.Fatalf("the control, no capes: %q", got)
	}
	pick(pick(pick(got, 0), 4), 5) // All my armor, Blues and teals, Navy
	if capeDye() != 0 {
		t.Fatalf("the control: all my armor dyed the cape %08X on a server without capes", capeDye())
	}
}

// #dye names the cape on a server with capes, and on one without it is no slot
// (the control).
func TestDyeCommandCape(t *testing.T) {
	c, gs, gp, _, _ := gmWorld(t)
	store, _ := gmRows(t, c)
	c.h.SetDyeStore(store.DB())
	gmReplies(t, gs)
	c.say(gs, "#dye cape ff0000")
	if got := gmReplies(t, gs); gp.Dyes[database.DyeCape] != 0 || !hasLine(got, `no slot "cape"`) {
		t.Fatalf("no capes: dyed %08X, said %q", gp.Dyes[database.DyeCape], got)
	}
	capesOn(t)
	c.say(gs, "#dye cape ff0000")
	if gp.Dyes[database.DyeCape] != 0xFF0000FF {
		t.Fatalf("with capes: %08X", gp.Dyes[database.DyeCape])
	}
}

// A character's cape goes out in its dye in its list's capes (0x7E27), and in
// its record's colour without one.
func TestTheListsCapesCarryTheDye(t *testing.T) {
	capesOn(t)
	f := newCapeListFixture(t)
	van := f.session(t, 41)
	f.h.SendCharacterSelectEQOA(van)
	f.answer(t, van, 1)
	color := func() uint32 {
		d := sentOp(van, protocol.OpcodeCapesList)
		return binary.LittleEndian.Uint32(d[2+1+4+1:])
	}
	if c := color(); c != 0xC01010FF {
		t.Fatalf("undyed: %08X, want the record's", c)
	}
	who := database.GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	if _, _, err := f.h.store.DB().SetCharDye(f.sid, database.DyeCape, 0x2050E0FF, who, "test"); err != nil {
		t.Fatal(err)
	}
	f.h.SendCharacterSelectEQOA(van)
	if c := color(); c != 0x2050E0FF {
		t.Fatalf("dyed: %08X, want the dye", c)
	}
}
