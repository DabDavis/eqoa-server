package handlers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
)

// testReader reads the wire as CLIENT does, written here without the
// server's reader; err is the first read past the end.
type testReader struct {
	b   []byte
	err error
}

func newTestReader(b []byte) *testReader { return &testReader{b: b} }

func (r *testReader) bytes(n int) []byte {
	if r.err != nil || len(r.b) < n {
		r.err = errors.New("short")
		return make([]byte, n)
	}
	out := r.b[:n]
	r.b = r.b[n:]
	return out
}

func (r *testReader) u32() uint32 { return binary.LittleEndian.Uint32(r.bytes(4)) }

func (r *testReader) zz() int64 {
	var u uint64
	for shift := 0; shift < 64; shift += 7 {
		c := r.bytes(1)[0]
		u |= uint64(c&0x7F) << shift
		if c&0x80 == 0 {
			break
		}
	}
	return int64(u>>1) ^ -int64(u&1)
}

func (r *testReader) wstr() string {
	n := r.u32()
	if n > 4096 {
		r.err = errors.New("a string too long")
		return ""
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(r.bytes(2))
	}
	return string(utf16.Decode(u))
}

// --- the Experiment (sub-op 4) ---

// Items that make nothing, none of them lost by any recipe's failure: 12/2,
// nothing taken.
func TestCraftExperimentNothing(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	got := f.combine(f.gem) // a gem is kept by every recipe
	mustBytes(t, "12/2", onlyOne(t, got), reply(12).u8(2).u32(0).b)
	// items not held in those numbers: 12/2 too, before any match
	got = f.combine(f.gem, f.gem, f.gem)
	mustBytes(t, "three of two gems", onlyOne(t, got), reply(12).u8(2).u32(0).b)
	if f.stack(f.gem) != 2 || f.stack(f.ore) != 5 {
		t.Fatal("nothing made, and something was taken")
	}
}

// Items that make nothing lose those some recipe loses on a failure (the
// ore), whether or not the combination is a recipe: 12/4 with one (id, 1)
// pair a unit, then 0x003C; the gem beside them is kept.
func TestCraftExperimentNothingLosesTheConsumable(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	got := f.combine(f.ore, f.gem) // the recipe wants two ore: no match
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeRemoveInvItem)
	want := reply(12).u8(4).u32(0).u32(1).u32(1).u32(uint32(f.ore)).u32(1).b
	mustBytes(t, "12/4", got[0], want)
	mustBytes(t, "the ore", got[1], removeMsg(1, 1))
	if f.stack(f.ore) != 4 || f.stack(f.gem) != 2 {
		t.Fatalf("ore %d, gems %d; want 4 and 2", f.stack(f.ore), f.stack(f.gem))
	}
}

// A matched recipe above the skill's level: 12/3, nothing taken or worn.
func TestCraftExperimentLevelTooLow(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	f.recipe(104, 1, 1, 2, "Test Iron Plate", f.plate, 1, 20, 50, true, 0, fmt.Sprintf(`[{"item": %d, "count": 2, "keep": true}]`, f.gem))
	if _, err := f.db.Exec(`UPDATE itempattern SET levelreq = 6 WHERE patternid = ?`, f.hammer); err != nil {
		t.Fatal(err)
	}
	got := f.combine(f.gem, f.gem)
	mustBytes(t, "12/3", onlyOne(t, got), reply(12).u8(3).u32(0).b)
	if h, _ := f.rowOf(f.hammer); h.RemainHP != 100 || f.stack(f.gem) != 2 {
		t.Fatal("a refused Experiment wore the hammer or took the gems")
	}
	// the control: at level 2 it is made
	if err := f.db.SetCharCraftSkill(f.sid, 1, database.CraftSkill{Level: 2}); err != nil {
		t.Fatal(err)
	}
	f.dice = []float64{0.0}
	if got = f.combine(f.gem, f.gem); got[0][3] != craftExpDiscovered {
		t.Fatalf("at level 2: 12/%d", got[0][3])
	}
}

// A recipe in the book made: 12/6 (the report, x2C, one pair a unit taken,
// the item made), and only then the wear, the takes and the product.
func TestCraftExperimentMakesARecipeInTheBook(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	f.learn(testRecipe)
	f.dice = []float64{0.1}
	got := f.combine(f.ore, f.gem, f.ore)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem,
		protocol.OpcodeRemoveInvItem, protocol.OpcodeAddInvItem)
	head := reply(12).u8(6).u32(0).
		u32(0).u32(1).u32(1).u32(0).u32(98).u32(40).u32(0).u32(testRecipe).
		u32(3).u32(uint32(f.ore)).u32(1).u32(uint32(f.ore)).u32(1).u32(uint32(f.gem)).u32(1).b
	if len(got[0]) <= len(head) {
		t.Fatalf("12/6 is %d bytes", len(got[0]))
	}
	mustBytes(t, "12/6 up to its item", got[0][:len(head)], head)
	if qty, hp, _, p := netItemHead(t, got[0][len(head):]); qty != 1 || hp != 50 || p != int64(f.plate) {
		t.Fatalf("12/6's item: %d of %d at %d", qty, p, hp)
	}
	mustBytes(t, "the wear after the reply", got[1], hpMsg(0, 90))
	mustBytes(t, "the ore", got[2], removeMsg(1, 2))
	mustBytes(t, "the gem", got[3], removeMsg(2, 1))
	if f.stack(f.plate) != 1 || f.stack(f.ore) != 3 || f.stack(f.gem) != 1 {
		t.Fatal("not what was made and taken")
	}
}

// A recipe not in the book, discoverable: 12/7 with x2C its id and ten
// times the XP (400 at level 1: four ranks); its Yes (5 with x2C) puts it in
// the book (1); a 5 of anything else, or with nothing offered, is ignored.
// Made again before it is scribed it is a 12/7 again, at a make's XP.
func TestCraftExperimentDiscoveryThenScribe(t *testing.T) {
	f := newCraftFixture(t)
	f.give(f.hammer, 1, 100, equipHeld)
	f.give(f.ore, 9, 0, -1)
	f.give(f.gem, 5, 0, -1)
	f.loadList()
	f.learn(102) // a recipe the server lacks: no harm
	if got := f.u32req(craftReqScribe, testRecipe); len(got) != 0 {
		t.Fatalf("a scribe with nothing offered was answered: %s", ops(got))
	}
	f.dice = []float64{0.1}
	got := f.combine(f.ore, f.ore, f.gem)
	if got[0][2] != craftRepExperiment || got[0][3] != craftExpDiscovered {
		t.Fatalf("a discovery: % X", got[0][:4])
	}
	head := reply(12).u8(7).u32(0).u32(0).u32(1).u32(1).u32(4).u32(98).u32(400).u32(0).u32(testRecipe).b
	mustBytes(t, "12/7's report and x2C", got[0][:len(head)], head)
	if s := f.skill(1); s.Level != 1 || s.Rank != 4 || s.XP != 0 {
		t.Fatalf("after 400 XP: %+v", s)
	}

	// the same made again, not scribed: 12/7 again, 40 XP
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	got = f.combine(f.ore, f.ore, f.gem)
	again := reply(12).u8(7).u32(0).u32(0).u32(1).u32(1).u32(4).u32(98).u32(40).u32(0).u32(testRecipe).b
	mustBytes(t, "a second discovery's report", got[0][:len(again)], again)

	if got := f.u32req(craftReqScribe, testRecipe+1); len(got) != 0 {
		t.Fatalf("a scribe of another recipe was answered: %s", ops(got))
	}
	got = f.u32req(craftReqScribe, testRecipe)
	mustBytes(t, "1", onlyOne(t, got), reply(1).u32(0).b)
	book, err := f.db.CharRecipeBook(f.sid)
	if err != nil || len(book) != 2 || book[1].Recipe != testRecipe || book[1].Ord != 1 {
		t.Fatalf("the book after: %+v (%v)", book, err)
	}
	// offered once: a second Yes changes nothing
	if got := f.u32req(craftReqScribe, testRecipe); len(got) != 0 {
		t.Fatalf("a second scribe was answered: %s", ops(got))
	}
	// in the book now, the same Experiment is a 12/6
	f.clock = f.clock.Add(craftCooldown)
	f.dice = []float64{0.1}
	if got = f.combine(f.ore, f.ore, f.gem); got[0][3] != craftExpMade {
		t.Fatalf("a recipe scribed, made again: 12/%d", got[0][3])
	}
}

// A recipe an Experiment may not find (discover 0) is not matched until it
// is in the book; then it is made as one.
func TestCraftExperimentDoesNotFindTheUndiscoverable(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(5)
	f.recipe(105, 1, 0, 1, "Test Secret", f.plate, 1, 10, 40, false, 0, fmt.Sprintf(`[{"item": %d, "count": 2, "keep": true}]`, f.gem))
	got := f.combine(f.gem, f.gem)
	mustBytes(t, "not found", onlyOne(t, got), reply(12).u8(2).u32(0).b)
	f.learn(105)
	f.dice = []float64{0.1}
	if got = f.combine(f.gem, f.gem); got[0][3] != craftExpMade {
		t.Fatalf("in the book: 12/%d", got[0][3])
	}
}

// An Experiment's failure: 12/4 with what it lost; the tool breaking, 12/5;
// a broken tool, no tool or one too weak: 12/2 and the line, nothing taken.
func TestCraftExperimentFailures(t *testing.T) {
	f := newCraftFixture(t)
	f.give(f.hammer, 1, 20, equipHeld)
	f.give(f.ore, 9, 0, -1)
	f.give(f.gem, 2, 0, -1)
	f.loadList()
	f.dice = []float64{0.95}
	got := f.combine(f.ore, f.ore, f.gem)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem)
	mustBytes(t, "12/4", got[0], reply(12).u8(4).u32(0).u32(1).u32(2).u32(uint32(f.ore)).u32(1).u32(uint32(f.ore)).u32(1).b)
	mustBytes(t, "the wear", got[1], hpMsg(0, 10))

	f.clock = f.clock.Add(craftCooldown)
	got = f.combine(f.ore, f.ore, f.gem) // 10 HP left: this attempt breaks it
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeAdjustItemHP, protocol.OpcodeRemoveInvItem)
	mustBytes(t, "12/5", got[0], reply(12).u8(5).u32(0).u32(1).u32(2).u32(uint32(f.ore)).u32(1).u32(uint32(f.ore)).u32(1).b)

	f.clock = f.clock.Add(craftCooldown)
	got = f.combine(f.ore, f.ore, f.gem)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "a broken hammer", got[0], reply(12).u8(2).u32(0).b)
	mustBytes(t, "its line", got[1], redLine("Your Test Hammer is broken."))
	if f.stack(f.ore) != 5 {
		t.Fatalf("ore %d, want 5 (two failures took two each, the refusal none)", f.stack(f.ore))
	}

	// within the cooldown, a matched recipe: 12/2 and the line
	g := newCraftFixture(t)
	g.standard(9)
	g.dice = []float64{0.95}
	g.combine(g.ore, g.ore, g.gem)
	got = g.combine(g.ore, g.ore, g.gem)
	mustOps(t, got, opcodeCraftReply, protocol.OpcodeColoredChat)
	mustBytes(t, "recovering", got[1], redLine("You have not yet recovered."))
}

// --- the book: 6, 7, 3, 1 ---

// 6 lists the skill's recipes in the book's order, each as VIRec144 reads
// it; another skill's are not listed.
func TestCraftBookListsTheSkillsRecipesInOrder(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(2)
	ing := fmt.Sprintf(`[{"item": %d, "count": 1, "keep": false}]`, f.ore)
	f.recipe(201, 1, 3, 2, "Test Greaves", f.plate, 1, 20, 50, true, 0, ing)
	f.recipe(202, 0, 0, 1, "Test Blade", f.plate, 1, 10, 40, true, 0, ing)
	f.recipe(203, 1, 10, 9, "Test Special", f.plate, 1, 90, 300, true, 0, ing)
	f.learn(203, testRecipe, 202, 201) // orders 0, 1, 2, 3
	got := onlyOne(t, f.u32req(craftReqBook, 1))
	want := reply(15).u32(0).u32(3).
		zz(0).zz(0).zz(203).u8(9).u8(1).u8(10).u8(0).wstr("Test Special").
		zz(0).zz(1).zz(testRecipe).u8(1).u8(1).u8(0).u8(0).wstr("Test Plate").
		zz(0).zz(3).zz(201).u8(2).u8(1).u8(3).u8(0).wstr("Test Greaves").b
	mustBytes(t, "15", got, want)
	got = onlyOne(t, f.u32req(craftReqBook, 3))
	mustBytes(t, "an empty Jewel book (Sandstorm's)", got, reply(15).u32(0).u32(0).b)
}

// A book past what the stand-in's fixed 4,096 bytes held comes whole: every
// record read back, the count right (the control: the old fixed buffer cuts
// it, records short of the count).
func TestCraftBookOfHundredsComesWhole(t *testing.T) {
	var list []craftBookEntry
	for i := 0; i < 300; i++ {
		r := &database.CraftRecipe{ID: 1000 + i, Skill: 1, Comp: i % 11, Level: 1 + i%10, Rarity: i % 4,
			Name: fmt.Sprintf("Test Recipe Number %03d With A Long Name", i)}
		list = append(list, craftBookEntry{recipe: r, ord: i})
	}
	read := func(payload []byte) (count uint32, records int) {
		r := newTestReader(payload[3:])
		r.u32()
		count = r.u32()
		for records = 0; records < int(count); records++ {
			r.zz()
			r.zz()
			id := r.zz()
			r.bytes(4)
			name := r.wstr()
			if r.err != nil || id != int64(1000+records) || name != fmt.Sprintf("Test Recipe Number %03d With A Long Name", records) {
				return count, records
			}
		}
		return count, records
	}
	if n, got := read(craftBookPayload(list)); n != 300 || got != 300 {
		t.Fatalf("a book of 300: count %d, %d records read", n, got)
	}
	// the stand-in's writer: a fixed 4,096 bytes, written past without a word
	old := build(opcodeCraftReply, 4096, func(w *buffer.Writer) {
		w.WriteByte(craftRepBook)
		w.WriteUint32(0)
		w.WriteUint32(uint32(len(list)))
		for _, e := range list {
			w.Write7BitEncodedInt64(0)
			w.Write7BitEncodedInt64(int64(e.ord))
			w.Write7BitEncodedInt64(int64(e.recipe.ID))
			w.WriteByte(byte(e.recipe.Level))
			w.WriteByte(byte(e.recipe.Skill))
			w.WriteByte(byte(e.recipe.Comp))
			w.WriteByte(byte(e.recipe.Rarity))
			w.WriteStringUnicode(e.recipe.Name)
		}
	})
	if n, got := read(old); n != 300 || got >= 300 {
		t.Fatalf("the control: 4,096 bytes held %d of %d", got, n)
	}
}

// 7: the recipe's detail (16), the product's name, level, description and
// stats, the ingredients by name, the tool with "(equipped)" for one good
// enough in the hand; one not in the book, or no recipe at all, a silent 7.
func TestCraftInspect(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(2)
	got := onlyOne(t, f.u32req(craftReqInspect, testRecipe))
	mustBytes(t, "not in the book", got, reply(7).b)
	f.learn(testRecipe)
	got = onlyOne(t, f.u32req(craftReqInspect, testRecipe))
	want := reply(16).u32(0).zz(testRecipe).zz(-1).zz(-1).u8(1).u8(1).u8(0).u8(0).u16(1).u8(3).
		wstr("Test Plate").wstr("A plate for the tests.").
		zz(2).zz(0).zz(2).zz(14).zz(5).
		zz(2).wstr("Test Ore").u16(2).wstr("Test Gem").u16(1).
		zz(1).wstr("Armor Crafting tool, tier 1").u8(1).b
	mustBytes(t, "16", got, want)
	mustBytes(t, "no such recipe", onlyOne(t, f.u32req(craftReqInspect, 999)), reply(7).b)

	// the hammer taken off: no "(equipped)"; the product without a
	// description: the recipe's
	rows, _ := f.inv.GetCharacterItems(f.sid)
	for _, r := range rows {
		if r.Pattern.PatternID == f.hammer {
			f.inv.SetEquipLoc(f.sid, r.ItemID, -1)
		}
	}
	f.db.Exec(`UPDATE itempattern SET itemdesc = '' WHERE patternid = ?`, f.plate)
	got = onlyOne(t, f.u32req(craftReqInspect, testRecipe))
	want = reply(16).u32(0).zz(testRecipe).zz(-1).zz(-1).u8(1).u8(1).u8(0).u8(0).u16(1).u8(3).
		wstr("Test Plate").wstr("A recipe for the tests.").
		zz(2).zz(0).zz(2).zz(14).zz(5).
		zz(2).wstr("Test Ore").u16(2).wstr("Test Gem").u16(1).
		zz(1).wstr("Armor Crafting tool, tier 1").u8(0).b
	mustBytes(t, "16 with the hammer off", got, want)
}

// recv08 is CLIENT's Recv08 over its one skill's book (decomp 0x00BFE2B0),
// for the comparison.
func recv08(orders map[int]int, from, to int) {
	fo, okf := orders[from]
	at, okt := orders[to]
	if !okf || !okt {
		return
	}
	for id, o := range orders {
		if o > fo {
			orders[id] = o - 1
		}
	}
	for id, o := range orders {
		if o >= at {
			orders[id] = o + 1
		}
	}
	orders[from] = at
}

// 3: the book reordered by Recv08's rule, kept (moved up it lands before the
// target, moved down after), the client's one-skill book left with exactly
// the server's orders; 8 echoes (from, to). A recipe the book lacks: 7,
// nothing moved. 1: out of the book, 2 with the id first.
func TestCraftArrangeAndRemove(t *testing.T) {
	f := newCraftFixture(t)
	f.standard(2)
	ing := fmt.Sprintf(`[{"item": %d, "count": 1, "keep": false}]`, f.ore)
	f.recipe(201, 1, 3, 2, "Test Greaves", f.plate, 1, 20, 50, true, 0, ing)
	f.recipe(202, 0, 0, 1, "Test Blade", f.plate, 1, 10, 40, true, 0, ing)
	f.recipe(203, 1, 4, 2, "Test Helm", f.plate, 1, 20, 50, true, 0, ing)
	f.learn(203, testRecipe, 202, 201)                   // 203:0 101:1 202:2 201:3
	client := map[int]int{203: 0, testRecipe: 1, 201: 3} // the Armor book the client holds

	orders := func() map[int]int {
		book, err := f.db.CharRecipeBook(f.sid)
		if err != nil {
			t.Fatal(err)
		}
		m := map[int]int{}
		for _, r := range book {
			m[r.Recipe] = r.Ord
		}
		return m
	}
	// the Armor book's order after each move: 201 moved up to 203 lands
	// before it; moved down to 101, after it; 203 moved down to 201, after
	// it (the Blade, another skill's, keeps its place among them)
	for _, mv := range []struct {
		from, to int
		want     []int
	}{
		{201, 203, []int{201, 203, testRecipe}},
		{201, testRecipe, []int{203, testRecipe, 201}},
		{203, 201, []int{testRecipe, 201, 203}},
	} {
		got := onlyOne(t, f.u32req(craftReqArrange, uint32(mv.from), uint32(mv.to)))
		mustBytes(t, fmt.Sprintf("8 for %d to %d", mv.from, mv.to), got, reply(8).u32(0).u32(uint32(mv.from)).u32(uint32(mv.to)).b)
		recv08(client, mv.from, mv.to)
		o := orders()
		for id, want := range client {
			if o[id] != want {
				t.Fatalf("after %d to %d: recipe %d at %d on the server, %d on the client (%v)", mv.from, mv.to, id, o[id], want, o)
			}
		}
		for i := 1; i < len(mv.want); i++ {
			if o[mv.want[i-1]] >= o[mv.want[i]] {
				t.Fatalf("after %d to %d: the order %v, want %v", mv.from, mv.to, o, mv.want)
			}
		}
	}

	before := orders()
	mustBytes(t, "a recipe not in the book", onlyOne(t, f.u32req(craftReqArrange, 999, 201)), reply(7).b)
	if fmt.Sprint(orders()) != fmt.Sprint(before) {
		t.Fatal("a move of a recipe not in the book moved something")
	}

	got := onlyOne(t, f.u32req(craftReqRemove, testRecipe))
	mustBytes(t, "2", got, reply(2).u32(testRecipe).u32(0).b)
	if _, ok := orders()[testRecipe]; ok {
		t.Fatal("removed, and still in the book")
	}
	if len(orders()) != 3 {
		t.Fatalf("the rest of the book: %v", orders())
	}
}

// The MemoryDump's six slots are the character's: {99, level, rank, 98} a
// skill with a row, Sandstorm's defaults without.
func TestCraftDumpSlotsAreTheCharacters(t *testing.T) {
	f := newCraftFixture(t)
	if got := f.h.DumpSlots(f.sid); got != craftSlotsDefault() {
		t.Fatalf("no rows: %v", got)
	}
	f.db.SetCharCraftSkill(f.sid, 1, database.CraftSkill{Level: 4, Rank: 7, XP: 33})
	f.db.SetCharCraftSkill(f.sid, 5, database.CraftSkill{Level: 2, Rank: 0})
	got := f.h.DumpSlots(f.sid)
	want := [6][4]uint32{{99, 1, 0, 98}, {99, 4, 7, 98}, {99, 1, 0, 98}, {99, 1, 0, 98}, {99, 99, 99, 98}, {99, 2, 0, 98}}
	if got != want {
		t.Fatalf("slots %v, want %v", got, want)
	}
	// and the dump writes them after the speed and the token
	w := buffer.NewWriter(make([]byte, 4096))
	writeMemoryDumpTail(w, &entity.Player{Speed: 8.8}, nil, got)
	r := newTestReader(w.Bytes())
	r.zz()
	r.zz()
	r.bytes(2)
	r.zz()
	r.u32() // speed
	if tok := r.u32(); tok != 0 {
		t.Fatalf("the token %d", tok)
	}
	for i := range want {
		for j := range want[i] {
			if v := r.u32(); v != want[i][j] {
				t.Fatalf("dumped slot %d word %d: %d, want %d", i, j, v, want[i][j])
			}
		}
	}
}

// #craftskill sets the GM's own skill and sends the six slots (14, 96 bytes,
// no token); #craftlearn puts recipes in the GM's book.
func TestCraftGMCommands(t *testing.T) {
	f := newCraftFixture(t)
	said, err := f.h.SetSkill(f.sess, 3, 7, 4)
	if err != nil || said != "Jewel Crafting: level 7, rank 4" {
		t.Fatalf("SetSkill: %q %v", said, err)
	}
	got := onlyOne(t, f.sent())
	want := reply(14)
	for i, s := range [6][4]uint32{{99, 1, 0, 98}, {99, 1, 0, 98}, {99, 1, 0, 98}, {99, 7, 4, 98}, {99, 99, 99, 98}, {99, 99, 99, 98}} {
		_ = i
		for _, v := range s {
			want.u32(v)
		}
	}
	mustBytes(t, "14", got, want.b)
	if s := f.skill(3); s.Level != 7 || s.Rank != 4 || s.XP != 0 {
		t.Fatalf("kept as %+v", s)
	}
	for _, bad := range [][3]int{{6, 1, 0}, {1, 0, 0}, {1, 11, 0}, {1, 5, 10}} {
		if _, err := f.h.SetSkill(f.sess, bad[0], bad[1], bad[2]); err == nil {
			t.Errorf("SetSkill %v accepted", bad)
		}
	}
	f.sent()

	ing := fmt.Sprintf(`[{"item": %d, "count": 1, "keep": false}]`, f.ore)
	f.recipe(202, 0, 0, 1, "Test Blade", f.plate, 1, 10, 40, true, 0, ing)
	f.recipe(203, 1, 4, 2, "Test Helm", f.plate, 1, 20, 50, true, 0, ing)
	if said, err := f.h.Learn(f.sess, []string{"all", "1"}); err != nil || said[:1] != "2" {
		t.Fatalf("#craftlearn all 1: %q %v", said, err)
	}
	if said, err := f.h.Learn(f.sess, []string{"all"}); err != nil || said[:1] != "1" {
		t.Fatalf("#craftlearn all: %q %v", said, err)
	}
	if _, err := f.h.Learn(f.sess, []string{"999"}); err == nil {
		t.Fatal("#craftlearn of no recipe")
	}
	book, _ := f.db.CharRecipeBook(f.sid)
	if len(book) != 3 || book[0].Recipe != testRecipe || book[1].Recipe != 203 || book[2].Recipe != 202 {
		t.Fatalf("the GM's book: %+v", book)
	}
}
