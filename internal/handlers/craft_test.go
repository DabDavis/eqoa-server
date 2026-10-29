package handlers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Tradeskills against a fixture: a character carrying a hammer (an Armor
// Crafting tool, wear 23) worn in Held, some ore and gems, and recipes of
// its own making, the World's view of it stood in for, the dice scripted and
// the clock set by hand. Every item, name and number is made up.

type craftFixture struct {
	t     *testing.T
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	h     *CraftHandler
	sess  *session.Session
	sid   uint32
	view  entity.CraftView
	dice  []float64
	clock time.Time

	hammer, ore, gem, plate int // item patterns
}

const (
	testPlateIcon = 7001
	testRecipe    = 101 // Test Plate: 2 ore (lost on a failure) and a gem (kept)
)

func newCraftFixture(t *testing.T) *craftFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/craft.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	database.InvalidateCraftRecipes()
	f := &craftFixture{t: t, db: db, inv: database.NewInventoryStore(db)}
	rec := &database.CharacterRecord{CharName: "Smith", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f.sid = rec.ServerID
	f.sess = testSession()
	f.view = entity.CraftView{Name: "Smith", CharServerID: f.sid, Level: 20, Stats: [7]int{60, 60, 60, 60, 60, 60, 60}}
	f.clock = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	f.h = NewCraftHandler(db, f.inv)
	f.h.view = func(*session.Session) (entity.CraftView, bool) { return f.view, f.view.Name != "" }
	f.h.now = func() time.Time { return f.clock }
	f.h.roll = func() float64 {
		if len(f.dice) == 0 {
			t.Fatal("the dice ran out")
		}
		v := f.dice[0]
		f.dice = f.dice[1:]
		return v
	}
	f.hammer = f.item("Test Hammer", 23, 1, 1, 100, 0, 7000, "")
	f.ore = f.item("Test Ore", -1, 1, 20, 0, 1, 7002, "")
	f.gem = f.item("Test Gem", -1, 1, 20, 0, 1, 7003, "")
	f.plate = f.item("Test Plate", 5, 3, 1, 50, 0, testPlateIcon, "A plate for the tests.")
	if _, err := db.Exec(`UPDATE itempattern SET str = 2, AC = 5 WHERE patternid = ?`, f.plate); err != nil {
		t.Fatal(err)
	}
	f.recipe(testRecipe, 1, 0, 1, "Test Plate", f.plate, 1, 10, 40, true, 0,
		fmt.Sprintf(`[{"item": %d, "count": 2, "keep": false}, {"item": %d, "count": 1, "keep": true}]`, f.ore, f.gem))
	return f
}

// item adds an item pattern: worn at wear (-1 nowhere), of that level,
// stacking to maxStack, of that condition, craft 1 for a tradeskill's
// component.
func (f *craftFixture) item(name string, wear, levelReq, maxStack, maxHP, craft, icon int, desc string) int {
	f.t.Helper()
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (10, 0, ?, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, ?, ?, 0, 32767, 1023, 0, 0, 0, ?, ?, ?, 0, 0)`,
		icon, wear, levelReq, maxStack, maxHP, craft, name, desc)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// recipe writes a recipe row.
func (f *craftFixture) recipe(id, skill, comp, level int, name string, product, yield, difficulty, xp int, discover bool, scroll int, ingredients string) {
	f.t.Helper()
	d := 0
	if discover {
		d = 1
	}
	if _, err := f.db.Exec(`INSERT INTO craftrecipes (id, skill, comp, level, rarity, name, product, yield, ingredients,
		difficulty, xp, discover, scroll, description, source) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, 'A recipe for the tests.', 'test')`,
		id, skill, comp, level, name, product, yield, ingredients, difficulty, xp, d, scroll); err != nil {
		f.t.Fatal(err)
	}
	database.InvalidateCraftRecipes()
}

// give gives the character n of an item, worn at equip slot loc (-1:
// carried), and returns its row's itemid.
func (f *craftFixture) give(pattern, n, hp, loc int) int {
	f.t.Helper()
	id, err := f.inv.AddCharItem(f.sid, pattern, n, hp, 0, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	if loc >= 0 {
		if err := f.inv.SetEquipLoc(f.sid, id, loc); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

// standard is the carried set: the hammer in Held (list index 0), n ore (1)
// and two gems (2); the client's list as a MemoryDump leaves it.
func (f *craftFixture) standard(ore int) {
	f.t.Helper()
	f.give(f.hammer, 1, 100, equipHeld)
	f.give(f.ore, ore, 0, -1)
	f.give(f.gem, 2, 0, -1)
	f.loadList()
}

func (f *craftFixture) loadList() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvList(inventoryOnly(rows)))
}

func inventoryOnly(rows []database.CharInventoryEntry) []database.CharInventoryEntry {
	var out []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location != 1 {
			out = append(out, r)
		}
	}
	return out
}

func (f *craftFixture) learn(ids ...int) {
	f.t.Helper()
	if _, err := f.db.AddCharRecipes(f.sid, ids...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *craftFixture) sent() [][]byte {
	var out [][]byte
	for _, m := range f.sess.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// req sends 0x1101: the sub-op, the token (any: never compared) and what
// fill writes; it returns what the server queued.
func (f *craftFixture) req(sub byte, fill func(w *buffer.Writer)) [][]byte {
	f.t.Helper()
	payload := build(opcodeCraftRequest, 64, func(w *buffer.Writer) {
		w.WriteByte(sub)
		w.WriteUint32(0x01020304)
		if fill != nil {
			fill(w)
		}
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, opcodeCraftRequest)
	msg.SetData(payload)
	if err := f.h.HandleCraft(f.sess, msg); err != nil {
		f.t.Fatal(err)
	}
	return f.sent()
}

func (f *craftFixture) create(id uint32) [][]byte {
	return f.req(craftReqCreate, func(w *buffer.Writer) { w.WriteUint32(id); w.WriteUint32(0) })
}

func (f *craftFixture) combine(ids ...int) [][]byte {
	return f.req(craftReqCombine, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(len(ids)))
		for _, id := range ids {
			w.Write7BitEncodedInt64(int64(id))
		}
	})
}

func (f *craftFixture) u32req(sub byte, words ...uint32) [][]byte {
	return f.req(sub, func(w *buffer.Writer) {
		for _, v := range words {
			w.WriteUint32(v)
		}
	})
}

// rowOf is the character's row of an item (its first), and whether it has
// one.
func (f *craftFixture) rowOf(pattern int) (database.CharInventoryEntry, bool) {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, r := range rows {
		if r.Pattern.PatternID == pattern {
			return r, true
		}
	}
	return database.CharInventoryEntry{}, false
}

func (f *craftFixture) stack(pattern int) int {
	r, ok := f.rowOf(pattern)
	if !ok {
		return 0
	}
	return r.StackLeft
}

func (f *craftFixture) skill(skill int) database.CraftSkill {
	f.t.Helper()
	rows, err := f.db.CharCraftSkills(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows[skill]
}

// --- the wire, written here without the server's writers ---

type wire struct{ b []byte }

func (x *wire) u8(v byte) *wire { x.b = append(x.b, v); return x }
func (x *wire) u16(v uint16) *wire {
	x.b = binary.LittleEndian.AppendUint16(x.b, v)
	return x
}
func (x *wire) u32(v uint32) *wire {
	x.b = binary.LittleEndian.AppendUint32(x.b, v)
	return x
}
func (x *wire) zz(v int64) *wire {
	u := uint64(v<<1) ^ uint64(v>>63)
	for {
		c := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			c |= 0x80
		}
		x.b = append(x.b, c)
		if u == 0 {
			return x
		}
	}
}
func (x *wire) wstr(s string) *wire {
	u := utf16.Encode([]rune(s))
	x.u32(uint32(len(u)))
	for _, c := range u {
		x.u16(c)
	}
	return x
}

// reply is the start of a 0x1102: the opcode and the sub-op.
func reply(sub byte) *wire { return (&wire{}).u16(0x1102).u8(sub) }

// removeMsg is 0x003C as CLIENT reads it: list index, sound 0, the count.
func removeMsg(idx, n int) []byte {
	return (&wire{}).u16(uint16(protocol.OpcodeRemoveInvItem)).u8(byte(idx)).u8(0).zz(int64(n)).b
}

// hpMsg is 0x0060: the record's key, its condition.
func hpMsg(key uint32, hp int) []byte {
	return (&wire{}).u16(uint16(protocol.OpcodeAdjustItemHP)).u32(key).zz(int64(hp)).b
}

// redLine is 0x0A7B: a line in Sandstorm's red.
func redLine(text string) []byte {
	return (&wire{}).u16(uint16(protocol.OpcodeColoredChat)).wstr(text).u8(255).u8(63).u8(63).b
}

// netItemHead is a 0x003B's (or an item record's) first fields: quantity,
// condition, the key, the item.
func netItemHead(t *testing.T, rec []byte) (qty, hp int64, key uint32, pattern int64) {
	t.Helper()
	r := buffer.NewReader(rec)
	qty, _ = r.Read7BitEncodedInt64()
	hp, _ = r.Read7BitEncodedInt64()
	r.Read7BitEncodedInt64() // charges
	r.Read7BitEncodedInt64() // equip slot
	r.Read7BitEncodedInt64() // quick slot
	key, _ = r.ReadUint32()
	pattern, err := r.Read7BitEncodedInt64()
	if err != nil {
		t.Fatalf("a short item record: % X", rec)
	}
	return
}

func mustBytes(t *testing.T, what string, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Fatalf("%s:\n got % X\nwant % X", what, got, want)
	}
}

// --- Create Item ---

// A success: 9 first (absolute level and rank, the XP, the product's icon
// and name), then the tool's wear by its key, every ingredient taken by its
// list index AS IT IS when sent (the ore's record leaves the list, so the
// gems move up from 2 to 1), then the product (0x003B, under the list's next
// key); all of it kept.
func TestCraftCreateMakesTheProduct(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(2) // exactly the ore the recipe takes
	f.learn(testRecipe)
	f.dice = []float64{0.59} // under 0.60: level 1 rank 0 (10 points) at difficulty 10, stats 60 + 60
	got := f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem,
		protocol.OpcodeRemoveInvItem, protocol.OpcodeAddInvItem)
	want := reply(9).u32(0).u32(0).u32(1).u32(1).u32(0).u32(98).u32(40).u32(0).u32(testPlateIcon).wstr("Test Plate").b
	mustBytes(t, "9", got[0], want)
	mustBytes(t, "the hammer's wear", got[1], hpMsg(0, 90))
	mustBytes(t, "the ore", got[2], removeMsg(1, 2))
	mustBytes(t, "the gem (index 1 once the ore left)", got[3], removeMsg(1, 1))
	if qty, hp, key, p := netItemHead(t, got[4][2:]); qty != 1 || hp != 50 || key != 3 || p != int64(f.plate) {
		t.Fatalf("0x003B: %d of item %d at %d, key %d; want 1 of %d at 50, key 3", qty, p, hp, key, f.plate)
	}
	// whole: one new plate at its full condition, not worn, on no quick
	// slot, under key 3, the item record as the item store has it
	plate, err := database.NewItemPatternStore(f.db).GetItemPattern(f.plate)
	if err != nil {
		t.Fatal(err)
	}
	made := database.CharInventoryEntry{StackLeft: 1, RemainHP: 50, EquipLoc: -1, Location: -1, Pattern: *plate}
	mustBytes(t, "0x003B", got[4], addInvItemPayload(&made, 3))
	if f.stack(f.ore) != 0 || f.stack(f.gem) != 1 || f.stack(f.plate) != 1 {
		t.Fatalf("kept: ore %d, gems %d, plates %d; want 0, 1, 1", f.stack(f.ore), f.stack(f.gem), f.stack(f.plate))
	}
	if h, _ := f.rowOf(f.hammer); h.RemainHP != 90 || h.EquipLoc != equipHeld {
		t.Fatalf("the hammer: %d HP, worn at %d", h.RemainHP, h.EquipLoc)
	}
	if s := f.skill(1); !s.Row || s.Level != 1 || s.Rank != 0 || s.XP != 40 {
		t.Fatalf("Armor Crafting kept as %+v", s)
	}
	// the mirror is the client's list as it now is: hammer, gems, plate
	l := sessionInvList(f.sess)
	if len(l.recs) != 3 || l.recs[1].patternID != f.gem || l.recs[2].patternID != f.plate || l.recs[2].key != 3 {
		t.Fatalf("the mirror: %+v", l.recs)
	}
}

// A failure takes only what a failure loses (the ore; the gem is kept),
// says so (10, reagentBroke 1), and wears the tool all the same; a recipe of
// kept ingredients alone takes nothing and says no reagent broke.
func TestCraftCreateFailureTakesWhatAFailureLoses(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	f.learn(testRecipe)
	f.dice = []float64{0.60} // not under 0.60: a failure
	got := f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem)
	mustBytes(t, "10", got[0], reply(10).u32(0).u32(1).b)
	mustBytes(t, "the wear", got[1], hpMsg(0, 90))
	mustBytes(t, "the ore lost", got[2], removeMsg(1, 2))
	if f.stack(f.ore) != 3 || f.stack(f.gem) != 2 || f.stack(f.plate) != 0 {
		t.Fatalf("after a failure: ore %d, gems %d, plates %d; want 3, 2, 0", f.stack(f.ore), f.stack(f.gem), f.stack(f.plate))
	}
	if s := f.skill(1); s.Row {
		t.Fatalf("a failure gave XP: %+v", s)
	}

	// kept ingredients alone: nothing taken, no reagent broke
	f.recipe(102, 1, 0, 1, "Test Polish", f.plate, 1, 10, 40, false, 0, fmt.Sprintf(`[{"item": %d, "count": 1, "keep": true}]`, f.gem))
	f.learn(102)
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.99}
	got = f.create(102)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP)
	mustBytes(t, "10 with nothing lost", got[0], reply(10).u32(0).u32(0).b)
	if f.stack(f.gem) != 2 {
		t.Fatalf("a kept gem was taken: %d left", f.stack(f.gem))
	}
}

// No tool in Held: a silent 3 and our line, nothing taken or worn; a tool
// below the recipe's tier, the same with its own line; a better one makes it.
func TestCraftCreateNeedsTheTool(t *testing.T) {
	f := newCraftFixture(t)
	f.give(f.hammer, 1, 100, -1) // carried, not worn
	f.give(f.ore, 5, 0, -1)
	f.give(f.gem, 2, 0, -1)
	f.loadList()
	f.learn(testRecipe)
	got := f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "3", got[0], reply(3).b)
	mustBytes(t, "the line", got[1], redLine("You need your Armor Crafting tool equipped."))

	// a tool of another skill in Held does not do (wear 22, Weapon Crafting)
	g := newCraftFixture(t)
	tongs := g.item("Test Tongs", 22, 46, 1, 100, 0, 7009, "")
	g.give(tongs, 1, 100, equipHeld)
	g.give(g.ore, 5, 0, -1)
	g.give(g.gem, 2, 0, -1)
	g.loadList()
	g.learn(testRecipe)
	got = g.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "another skill's tool", got[1], redLine("You need your Armor Crafting tool equipped."))

	// tier: a level 3 recipe, a skill of level 3, the level 1 hammer (tier 1)
	h := newCraftFixture(t)
	h.standard(5)
	h.recipe(103, 1, 2, 3, "Test Bronze Plate", h.plate, 1, 30, 60, true, 0, fmt.Sprintf(`[{"item": %d, "count": 2, "keep": false}]`, h.ore))
	h.learn(103)
	if err := h.db.SetCharCraftSkill(h.sid, 1, database.CraftSkill{Level: 3}); err != nil {
		t.Fatal(err)
	}
	got = h.create(103)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "3", got[0], reply(3).b)
	mustBytes(t, "the tier's line", got[1], redLine("Your Test Hammer is not good enough for that."))
	if h.stack(h.ore) != 5 {
		t.Fatal("a refusal took the ore")
	}
	// the control: a level 11 hammer (tier 3) in Held makes it
	if _, err := h.db.Exec(`UPDATE itempattern SET levelreq = 11 WHERE patternid = ?`, h.hammer); err != nil {
		t.Fatal(err)
	}
	h.dice = []float64{0.0}
	got = h.create(103)
	if got[0][2] != craftRepMade {
		t.Fatalf("a tier 3 hammer: reply %d", got[0][2])
	}
}

// The tool wears 10 an attempt; at 0 it breaks: that attempt fails (11, the
// ore lost), the hammer stays at 0 for a blacksmith, and the next attempt is
// refused before anything is taken.
func TestCraftCreateBreaksTheTool(t *testing.T) {
	f := newCraftFixture(t)
	f.give(f.hammer, 1, 10, equipHeld)
	f.give(f.ore, 5, 0, -1)
	f.give(f.gem, 2, 0, -1)
	f.loadList()
	f.learn(testRecipe)
	got := f.create(testRecipe) // no dice: a broken tool rolls none
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem)
	mustBytes(t, "11", got[0], reply(11).u32(0).u32(1).b)
	mustBytes(t, "the hammer at 0", got[1], hpMsg(0, 0))
	mustBytes(t, "the ore lost", got[2], removeMsg(1, 2))
	if h, ok := f.rowOf(f.hammer); !ok || h.RemainHP != 0 || h.EquipLoc != equipHeld {
		t.Fatalf("the broken hammer: %+v (kept %v)", h, ok)
	}
	f.clock = f.clock.Add(craftCooldown)
	got = f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "3", got[0], reply(3).b)
	mustBytes(t, "the line", got[1], redLine("Your Test Hammer is broken."))
	if f.stack(f.ore) != 3 {
		t.Fatalf("a broken hammer's attempt took ore: %d left", f.stack(f.ore))
	}
}

// Within a second of an attempt: 5 ("You have not yet recovered."), nothing
// else; a second on, the next attempt is made.
func TestCraftCreateCooldown(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(6)
	f.learn(testRecipe)
	f.dice = []float64{0.9}
	f.create(testRecipe)
	f.clock = f.clock.Add(craftCooldown - time.Millisecond)
	got := f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply)
	mustBytes(t, "5", got[0], reply(5).u32(0).b)
	if f.stack(f.ore) != 4 {
		t.Fatalf("a refused attempt took ore: %d left", f.stack(f.ore))
	}
	f.clock = f.clock.Add(time.Millisecond)
	f.dice = []float64{0.9}
	if got = f.create(testRecipe); got[0][2] != craftRepFailed {
		t.Fatalf("a second on: reply %d", got[0][2])
	}
}

// Every check before anything is taken: a recipe not in the book (7), what
// it needs not held, no room for the product, a skill below its level (3
// and a line each).
func TestCraftCreateRefusals(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(1) // one ore of the two
	got := f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply)
	mustBytes(t, "not in the book", got[0], reply(7).b)

	f.learn(testRecipe)
	got = f.create(testRecipe)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "3", got[0], reply(3).b)
	mustBytes(t, "lacking", got[1], redLine("You do not have what this recipe needs."))

	// a level 2 recipe, a level 6 hammer (tier 2), the skill at level 1
	f.recipe(104, 1, 1, 2, "Test Iron Plate", f.plate, 1, 20, 50, true, 0, fmt.Sprintf(`[{"item": %d, "count": 1, "keep": false}]`, f.ore))
	f.learn(104)
	if _, err := f.db.Exec(`UPDATE itempattern SET levelreq = 6 WHERE patternid = ?`, f.hammer); err != nil {
		t.Fatal(err)
	}
	got = f.create(104)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "too low", got[1], redLine("Your tradeskill level is not high enough to create that item."))
	if f.stack(f.ore) != 1 || f.stack(f.gem) != 2 {
		t.Fatal("a refusal took something")
	}
}

// No room: a full list (40 records) and nothing of the recipe leaving it;
// the control: the same list whose ore the recipe takes whole makes room.
func TestCraftCreateNeedsRoom(t *testing.T) {
	for _, c := range []struct {
		ore  int
		room bool
	}{{5, false}, {2, true}} {
		f := newCraftFixture(t)
		f.give(f.hammer, 1, 100, equipHeld)
		f.give(f.ore, c.ore, 0, -1)
		f.give(f.gem, 2, 0, -1)
		junk := f.item("Test Pebble", -1, 1, 1, 0, 0, 7008, "")
		for i := 0; i < invListCap-3; i++ {
			f.give(junk, 1, 0, -1)
		}
		f.loadList()
		f.learn(testRecipe)
		f.dice = []float64{0.0}
		got := f.create(testRecipe)
		if !c.room {
			mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
			mustBytes(t, "no room", got[1], redLine("You have no room for it."))
			if f.stack(f.ore) != 5 {
				t.Fatal("no room, and the ore went")
			}
			continue
		}
		if got[0][2] != craftRepMade || f.stack(f.plate) != 1 {
			t.Fatalf("the ore's record leaving makes room: reply %d, %d plates", got[0][2], f.stack(f.plate))
		}
	}
}

// Every request that sets the client's pending flag is answered, a short
// one or one with no character too (7, or 12/2 for an Experiment); 5 sets
// none and is not.
func TestCraftAnswersEveryPendingRequest(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(2)
	for _, sub := range []byte{craftReqRemove, craftReqCreate, craftReqArrange, craftReqBook, craftReqInspect} {
		mustBytes(t, fmt.Sprintf("a short %d", sub), onlyOne(t, f.req(sub, nil)), reply(7).b)
	}
	mustBytes(t, "a short combine", onlyOne(t, f.req(craftReqCombine, nil)), reply(12).u8(2).u32(0).b)
	mustBytes(t, "five items", onlyOne(t, f.combine(f.ore, f.ore, f.gem, f.gem, f.gem)), reply(12).u8(2).u32(0).b)
	if got := f.req(craftReqScribe, nil); len(got) != 0 {
		t.Fatalf("a short scribe was answered: %d messages", len(got))
	}
	// no player in the world
	f.view.Name = ""
	for _, sub := range []byte{craftReqRemove, craftReqCreate, craftReqArrange, craftReqBook, craftReqInspect} {
		mustBytes(t, fmt.Sprintf("no player, %d", sub), onlyOne(t, f.u32req(sub, testRecipe, testRecipe)), reply(7).b)
	}
	mustBytes(t, "no player, combine", onlyOne(t, f.combine(f.ore)), reply(12).u8(2).u32(0).b)
	// the control: a request served is answered by its own reply, not 7
	f.view.Name = "Smith"
	if got := onlyOne(t, f.u32req(craftReqBook, 1)); got[2] != craftRepBook {
		t.Fatalf("a book request answered %d", got[2])
	}
}

func onlyOne(t *testing.T, msgs [][]byte) []byte {
	t.Helper()
	if len(msgs) != 1 {
		t.Fatalf("sent %s, want one message", ops(msgs))
	}
	return msgs[0]
}

// containsLine is whether a message is a red line saying text.
func containsLine(msgs [][]byte, text string) bool {
	for _, m := range msgs {
		if bytes.Equal(m, redLine(text)) {
			return true
		}
	}
	return false
}
