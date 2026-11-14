package handlers

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// armorListFixture is a character wearing an ornate breastplate (an item
// record of model 9 on the chest: set 9), its chest dyed and its chest's trim
// dyed, at character select.
type armorListFixture struct {
	h    *CharacterHandler
	look *LiveLookHandler
	sm   *session.Manager
	sid  uint32
}

func newArmorListFixture(t *testing.T) *armorListFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/armor.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := database.NewSQLiteCharacterStore(db)
	rec := &database.CharacterRecord{CharName: "Gilded", AccountID: 5, Level: 60}
	if err := store.Create(rec); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (0, 0, 1, 0, 5, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Ornate Breastplate', '', 9, 0)`)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := res.LastInsertId()
	inv := database.NewInventoryStore(db)
	id, err := inv.AddCharItem(rec.ServerID, int(pid), 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.SetEquipLoc(rec.ServerID, id, 1); err != nil {
		t.Fatal(err)
	}
	who := database.GMEdit{AccountID: 1, Username: "gm", Via: "game"}
	for slot, c := range map[int]uint32{database.DyeChest: 0x808080FF, database.DyeChestTrim: 0xD10000FF} {
		if _, _, err := db.SetCharDye(rec.ServerID, slot, c, who, "test"); err != nil {
			t.Fatal(err)
		}
	}
	w := entity.NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	h := &CharacterHandler{store: store, inventoryStore: inv, sessionManager: sm, gameWorld: w}
	return &armorListFixture{h: h, look: NewLiveLookHandler(h), sm: sm, sid: rec.ServerID}
}

func (f *armorListFixture) session(n byte) *session.Session {
	s := f.sm.CreateSession(testAddr(n), uint32(n), uint32(n), uint16(n), uint16(n), true)
	s.SetAccountID(5)
	s.SetState(session.StateCharacterSelect)
	return s
}

func (f *armorListFixture) answer(t *testing.T, s *session.Session, v uint32) {
	t.Helper()
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeLiveLook)
	msg.SetData(u32b(v))
	if err := f.look.HandleLiveLook(s, msg); err != nil {
		t.Fatal(err)
	}
}

// A client that answers the live look's offer with bit 2 (vanilla: it draws
// set 9 and the trims) is told so in the look in force's third u32 and listed
// its characters with set 9 and the six trims after the live game's 71 bytes
// (95 a record, read whole). Without bit 2 the look in force's third u32 is 0
// and the record the live game's 71 with set 4; a PS2's (the disc's 32 bytes)
// set 4. Bit 2 without bit 0 grants nothing.
func TestSetNineAndTrimsAtCharacterSelect(t *testing.T) {
	f := newArmorListFixture(t)

	ps2 := f.session(41)
	f.h.SendCharacterSelectEQOA(ps2)
	gear, whole := listGearOf(t, sentOp(ps2, protocol.OpcodeCharacterSelect), entity.DiscGearSize)
	if len(gear) != 1 || !whole || gear[0][gearArmor] != entity.PlainPlateSet {
		t.Fatalf("the PS2's list: %d records, whole %v, chest set %d", len(gear), whole, gear[0][gearArmor])
	}

	old := f.session(42)
	f.h.SendCharacterSelectEQOA(old)
	f.answer(t, old, 1|2)
	if d := sentOp(old, protocol.OpcodeLiveLookInForce); len(d) < 14 || binary.LittleEndian.Uint32(d[10:]) != 0 || old.ArmorLook() {
		t.Fatalf("an older vanilla's look in force: %x", d)
	}
	gear, whole = listGearOf(t, sentOp(old, protocol.OpcodeCharacterSelect), entity.LiveGearSize)
	if len(gear) != 1 || !whole || gear[0][19] != entity.PlainPlateSet {
		t.Fatalf("an older vanilla's list: whole %v, chest set %d", whole, gear[0][19])
	}

	van := f.session(43)
	f.h.SendCharacterSelectEQOA(van)
	f.answer(t, van, 1|2|liveLookArmor)
	if d := sentOp(van, protocol.OpcodeLiveLookInForce); binary.LittleEndian.Uint32(d[10:]) != liveLookArmor || !van.ArmorLook() {
		t.Fatalf("this server's armour in force: %x", d)
	}
	gear, whole = listGearOf(t, sentOp(van, protocol.OpcodeCharacterSelect), entity.ArmorGearSize)
	if len(gear) != 1 || !whole {
		t.Fatalf("the list: %d records, read whole %v", len(gear), whole)
	}
	g := gear[0]
	if g[19] != entity.OrnateSet || binary.BigEndian.Uint32(g[43:]) != 0x808080FF {
		t.Fatalf("chest set %d, colour %08x", g[19], binary.BigEndian.Uint32(g[43:]))
	}
	if tr := binary.BigEndian.Uint32(g[entity.LiveGearSize:]); tr != 0xD10000FF {
		t.Fatalf("the chest's trim: %08x", tr)
	}
	if tr := binary.BigEndian.Uint32(g[entity.LiveGearSize+4:]); tr != 0 {
		t.Fatalf("an undyed trim: %08x", tr)
	}

	alone := f.session(44)
	f.h.SendCharacterSelectEQOA(alone)
	f.answer(t, alone, liveLookArmor)
	if alone.ArmorLook() || alone.LiveLook() {
		t.Fatal("bit 2 alone granted something")
	}
}

// The Armor Dyers ask "Armor trim" last in their first question, whatever the
// server (after "Take every dye off": More leaves the cursor on the trim), and
// dye a piece's trim, or all six, into chardyes' trim slots; "Take every dye
// off" takes the trims off too. The control: the body's slots keep their dye.
func TestTheDyerDyesTheTrim(t *testing.T) {
	for _, capes := range []bool{false, true} {
		entity.SetCapes(capes)
		menu := dyerMenu()
		if len(menu) > maxDialogOptions || menu[len(menu)-1].label != "Armor trim" {
			t.Fatalf("capes %v: %d answers, last %q", capes, len(menu), menu[len(menu)-1].label)
		}
		if menu[len(menu)-2].sub == nil && menu[len(menu)-2].slots == nil && len(menu) == 6 {
			t.Fatalf("capes %v: More's second line takes every dye off", capes)
		}
		for _, c := range menu[len(menu)-1].sub {
			for _, s := range c.slots {
				if !database.IsTrimSlot(s) {
					t.Fatalf("%q dyes slot %d, not a trim", c.label, s)
				}
			}
		}
	}
	entity.SetCapes(false)
	trim := dyerMenu()[len(dyerMenu())-1]
	if trim.sub[0].label != "All my armor's trim" || len(trim.sub[0].slots) != database.DyeTrims {
		t.Fatalf("the trim's first answer: %+v", trim.sub[0])
	}
	labels := []string{}
	for _, c := range trim.sub[1:] {
		labels = append(labels, c.label)
	}
	if strings.Join(labels, ",") != "Chest or robe top trim,Bracers and gloves trim,Legs or robe skirt trim,Boots trim,Helm trim" {
		t.Fatalf("the pieces: %v", labels)
	}
	if len(dyeSlotsHere()) != database.DyeSlots-1 || !database.IsTrimSlot(dyeSlotsHere()[len(dyeSlotsHere())-1]) {
		t.Fatal("every dye off does not take the trims off")
	}
	if s, ok := database.DyeSlotByName("chesttrim"); !ok || s != database.DyeChestTrim {
		t.Fatal("#dye chesttrim")
	}
}

// The dyer's trim, talked through: "Armor trim", a piece, a family, a colour
// dyes that piece's trim alone; "All my armor's trim" all six; the body's dye
// is left as it was, and "Take every dye off" takes the trims off too.
func TestTheDyerDyesTheTrimTalkedThrough(t *testing.T) {
	entity.SetCapes(false)
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
			t.Fatalf("no dyer's box in %q", msgs)
		}
		return questMsgs(tw.s)
	}
	dyes := func() [database.DyeSlots]uint32 {
		c, err := db.CharDyes(rec.ServerID)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	got := talk()
	if !strings.HasSuffix(got[len(got)-1], "|Take every dye off|Armor trim") {
		t.Fatalf("the first question: %q", got[len(got)-1])
	}
	pick(pick(pick(talk(), 0), 0), 2) // All my armor, whites, grey
	got = pick(talk(), 4)             // Armor trim
	if !strings.Contains(got[len(got)-1], ":Which one?|All my armor's trim|Chest or robe top trim|Bracers and gloves trim|Legs or robe skirt trim|Boots trim|Helm trim") {
		t.Fatalf("the trims: %q", got[len(got)-1])
	}
	got = pick(got, 3) // Legs or robe skirt trim
	if !strings.Contains(got[len(got)-1], ":Which colors for legs or robe skirt trim?|") {
		t.Fatalf("the families: %q", got[len(got)-1])
	}
	pick(pick(got, 4), 3) // Blues and teals, Royal blue
	c := dyes()
	if c[database.DyeLegsTrim] != 0x2050E0FF || c[database.DyeChestTrim] != 0 || c[database.DyeLegs] != 0x808080FF {
		t.Fatalf("the legs' trim royal blue: %08X, chest trim %08X, legs %08X", c[database.DyeLegsTrim], c[database.DyeChestTrim], c[database.DyeLegs])
	}
	if _, _, on, ok := tw.w.DyeTarget(tw.s); !ok || on[database.DyeLegsTrim] != 0x2050E0FF {
		t.Fatal("not on the character where it stands")
	}
	pick(pick(pick(pick(talk(), 4), 2), 1), 0) // Armor trim, Bracers and gloves trim, reds, red
	c = dyes()
	if c[database.DyeBracerTrim] != 0xD10000FF || c[database.DyeGlovesTrim] != 0xD10000FF || c[database.DyeChestTrim] != 0 || c[database.DyeHelmTrim] != 0 {
		t.Fatalf("bracers and gloves: bracer %08X gloves %08X chest %08X helm %08X", c[database.DyeBracerTrim], c[database.DyeGlovesTrim], c[database.DyeChestTrim], c[database.DyeHelmTrim])
	}
	pick(pick(pick(pick(talk(), 4), 5), 1), 0) // Armor trim, Helm trim, reds, red
	if c := dyes(); c[database.DyeHelmTrim] != 0xD10000FF {
		t.Fatalf("the helm's trim: %08X", c[database.DyeHelmTrim])
	}
	pick(pick(pick(pick(talk(), 4), 0), 1), 0) // Armor trim, all, reds, red
	c = dyes()
	for i := 0; i < database.DyeTrims; i++ {
		if c[database.DyeTrimOf(i)] != 0xD10000FF {
			t.Fatalf("all trims red: %s %08X", database.DyeSlotNames[database.DyeTrimOf(i)], c[database.DyeTrimOf(i)])
		}
	}
	if c[database.DyeChest] != 0x808080FF {
		t.Fatalf("the body's dye: %08X", c[database.DyeChest])
	}
	pick(talk(), 3) // Take every dye off
	if c := dyes(); c != ([database.DyeSlots]uint32{}) {
		t.Fatalf("every dye off: %08X", c)
	}
}
