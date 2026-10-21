package handlers

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Scribing against a temporary database, held to what Sandstorm sent a druid
// of level 23 (eqoa harvest, 2026-10-21): the scroll 39th in a full list, an
// ability at display index 18 with id 458, level 4 and range 30. The spells'
// names and other fields are made up.

type scribeFixture struct {
	t      *testing.T
	db     *database.SQLiteDB
	inv    *database.InventoryStore
	spells *database.SpellbookStore
	h      *EquipHandler
	sess   *session.Session
	player *entity.Player
	sid    uint32
}

const (
	classDruid  = 7
	classCleric = 9
	raceHalfing = 7
	druidMask   = 1 << classDruid
	healerMask  = 0x380 // druid, shaman, cleric
)

func newScribeFixture(t *testing.T) *scribeFixture {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/scribe.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &scribeFixture{t: t, db: db, inv: database.NewInventoryStore(db), spells: database.NewSpellbookStore(db)}
	rec := &database.CharacterRecord{CharName: "Scribe", AccountID: 1}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f.sid = rec.ServerID
	f.player = &entity.Player{ServerID: 1001, CharServerID: f.sid, Name: "Scribe", Class: classDruid, Race: raceHalfing, Level: 23}
	f.sess = testSession()
	f.h = NewEquipHandler(nil, f.inv, f.spells, nil)
	f.h.playerOf = func(*session.Session) *entity.Player { return f.player }
	return f
}

// spell adds a spell pattern under its own id: level 4, range 30.0 (high
// half 0x41F0), made-up rest.
func (f *scribeFixture) spell(id int, name string, level int) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
		iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?, ?, 0, 0, 16880, 2, 15, 1234, -5678, 1, 1, -1, ?, 'A made-up spell.')`, id, level, name); err != nil {
		f.t.Fatal(err)
	}
}

// know puts a spell in the book at a display index, off the bar.
func (f *scribeFixture) know(id, order int) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
		VALUES (?, ?, ?, 0, -1, 0, -1)`, f.sid, id, order); err != nil {
		f.t.Fatal(err)
	}
}

// item adds an item pattern: a scroll (the scroll's art, worn nowhere) or not.
func (f *scribeFixture) item(name string, scroll bool, classes, level, maxStack int) int {
	f.t.Helper()
	icon := int64(scrollIcon)
	if !scroll {
		icon = 209763529
	}
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (24, 0, ?, 0, -1, 0, 0, 1, 0, 0, 0, 0, ?, ?, 0, 0, ?, 1023, 2, 0, 0, 0, ?, '', 0, 0)`,
		icon, level, maxStack, classes, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// own gives the character n of an item in the inventory and returns its key
// once loadList has run (keys are positions after a MemoryDump).
func (f *scribeFixture) own(pattern, n int) {
	f.t.Helper()
	if _, err := f.inv.AddCharItem(f.sid, pattern, n, 0, 0, 0); err != nil {
		f.t.Fatal(err)
	}
}

// loadList is what SendMemoryDump leaves: the client's list, key = position.
func (f *scribeFixture) loadList() {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	f.sess.SetInventoryList(newInvList(rows))
}

// use sends UseItem(key, 0) as the item menu does, the target none.
func (f *scribeFixture) use(key uint32) [][]byte {
	f.t.Helper()
	payload := build(protocol.OpcodeInteractItem, 16, func(w *buffer.Writer) {
		w.WriteUint32(key)
		w.WriteUint32(0xFFFFFFFF)
		w.WriteByte(0)
	})[2:]
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeInteractItem)
	msg.SetData(payload)
	r := NewRegistry()
	f.h.RegisterHandlers(r)
	h, ok := r.Get(protocol.OpcodeInteractItem)
	if !ok {
		f.t.Fatal("no handler for 0x003E")
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

func (f *scribeFixture) book() []database.SpellRow {
	f.t.Helper()
	rows, err := f.spells.GetSpellRows(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A book of 18 (display indices 0..17) and a full list of 39 things before
// the scroll, as Sandstorm's druid had: the scroll leaves by 0x003C at index
// 39, and 0x00B3 starts with Sandstorm's bytes (stamp 0, flag 1, id 458,
// display index 18, off the bar, lockout 0, +0x14 -1, level 4, range 30).
func TestScribeIsSandstorms(t *testing.T) {
	f := newScribeFixture(t)
	for i := 0; i < 18; i++ {
		f.spell(1000+i, "Known "+string(rune('A'+i)), 1)
		f.know(1000+i, i)
	}
	f.spell(458, "Minor Remedy", 4)
	filler := f.item("Pebble", false, 0x7FFF, 1, 1)
	for i := 0; i < 39; i++ {
		f.own(filler, 1)
	}
	scroll := f.item("Minor Remedy", true, healerMask, 4, 1)
	f.own(scroll, 1)
	f.loadList()

	msgs := f.use(39)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAbilityAdd)
	if want := mustHex(t, "3c00270002"); !bytes.Equal(msgs[0], want) {
		t.Errorf("0x003C % X, want Sandstorm's % X", msgs[0], want)
	}
	if want := mustHex(t, "b300000000000100000094072400010001080000f041"); !bytes.HasPrefix(msgs[1], want) {
		t.Errorf("0x00B3 starts % X\n           want % X", msgs[1][:len(want)], want)
	}

	// The book holds it as the dump will send it, and the scroll is gone
	// from the database and from the client's list.
	var got database.SpellRow
	for _, r := range f.book() {
		if r.SpellID == 458 {
			got = r
		}
	}
	if got != (database.SpellRow{SpellID: 458, AddedOrder: 18, OnHotBar: 0, WhereOnHotBar: -1}) {
		t.Errorf("spells row %+v", got)
	}
	entries, _ := f.spells.GetCharacterSpells(f.sid)
	last := entries[len(entries)-1]
	if last.Pattern.SpellID != 458 || last.Unk1 != 0 || last.ShowHide != -1 {
		t.Errorf("book's last %+v", last)
	}
	if !bytes.Equal(msgs[1], buildAbilityAdd(0, true, &last)) {
		t.Error("0x00B3 is not the record the next MemoryDump sends")
	}
	for _, r := range f.rowsOf() {
		if r.Pattern.PatternID == scroll {
			t.Error("the scroll is still in the database")
		}
	}
	if l := sessionInvList(f.sess); len(l.recs) != 39 || l.index(39) != -1 {
		t.Errorf("the client's list holds %d, key 39 at %d", len(l.recs), l.index(39))
	}
}

func (f *scribeFixture) rowsOf() []database.CharInventoryEntry {
	f.t.Helper()
	rows, err := f.inv.GetCharacterItems(f.sid)
	if err != nil {
		f.t.Fatal(err)
	}
	return rows
}

// Sandstorm's three refusals, byte for byte, each with the scroll kept and
// the book as it was: a cleric's scroll, one of level 24, one known.
func TestScribeRefusalsAreSandstorms(t *testing.T) {
	f := newScribeFixture(t)
	f.spell(1, "Smite", 1)
	f.spell(2, "Cure", 24)
	f.spell(3, "Mending", 1)
	f.know(3, 0)
	f.own(f.item("Smite", true, 1<<classCleric, 1, 1), 1)
	f.own(f.item("Cure", true, healerMask, 24, 1), 1)
	f.own(f.item("Mending", true, healerMask, 1, 1), 1)
	f.loadList()
	for key, want := range []string{
		// "This item can't be used by your class."
		"7b0a26000000540068006900730020006900740065006d002000630061006e002700740020006200650020007500730065006400200062007900200079006f0075007200200063006c006100730073002e00ff3f3f",
		// "You are not high enough level to equip that item."
		"7b0a3100000059006f007500200061007200650020006e006f00740020006800690067006800200065006e006f0075006700680020006c006500760065006c00200074006f002000650071007500690070002000740068006100740020006900740065006d002e00ff3f3f",
		// "You already know this spell."
		"7b0a1c00000059006f007500200061006c007200650061006400790020006b006e006f0077002000740068006900730020007300700065006c006c002e00ff3f3f",
	} {
		msgs := f.use(uint32(key))
		mustOps(t, msgs, protocol.OpcodeColoredChat)
		if w := mustHex(t, want); !bytes.Equal(msgs[0], w) {
			t.Errorf("key %d: % X\n want % X", key, msgs[0], w)
		}
	}
	if n := len(f.rowsOf()); n != 3 {
		t.Errorf("%d items left, want the 3 scrolls", n)
	}
	if b := f.book(); len(b) != 1 {
		t.Errorf("book %+v", b)
	}
}

// The control: a potion with a spell's name is not a scroll (no scroll
// art), so the request is the equip toggle's, which wears nothing worn
// nowhere: nothing is sent and nothing changes.
func TestScribeNeedsTheScrollsArt(t *testing.T) {
	f := newScribeFixture(t)
	f.spell(1, "Invisibility", 1)
	f.own(f.item("Invisibility", false, 0x7FFF, 1, 1), 1)
	f.loadList()
	if msgs := f.use(0); len(msgs) != 0 {
		t.Errorf("sent %s", ops(msgs))
	}
	if len(f.book()) != 0 || len(f.rowsOf()) != 1 {
		t.Error("a potion was scribed")
	}
}

// A scroll whose spell has no record here is refused with this server's
// own line, and kept.
func TestScribeWithoutARecord(t *testing.T) {
	f := newScribeFixture(t)
	f.own(f.item("Unwritten Verse", true, druidMask, 1, 1), 1)
	f.loadList()
	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeColoredChat)
	if !bytes.Equal(msgs[0], coloredChatPayload(scribeNoSpell, 255, 63, 63)) {
		t.Errorf("% X", msgs[0])
	}
	if len(f.rowsOf()) != 1 {
		t.Error("the scroll went")
	}
}

// One of a stack is taken: the row and the client's record keep the rest.
func TestScribeOneOfAStack(t *testing.T) {
	f := newScribeFixture(t)
	f.spell(7, "Barkskin", 1)
	f.own(f.item("Barkskin (DRD)", true, druidMask, 1, 9), 2)
	f.loadList()
	msgs := f.use(0)
	mustOps(t, msgs, protocol.OpcodeRemoveInvItem, protocol.OpcodeAbilityAdd)
	rows := f.rowsOf()
	if len(rows) != 1 || rows[0].StackLeft != 1 {
		t.Errorf("rows %+v", rows)
	}
	if l := sessionInvList(f.sess); len(l.recs) != 1 || l.recs[0].qty != 1 {
		t.Errorf("the client's list %+v", l.recs)
	}
}

func TestScrollSpellName(t *testing.T) {
	for in, want := range map[string]string{
		"Root (SHA/DRD)":          "root",
		"Invisibility (SHA, WIZ)": "invisibility",
		"Taunt (SK/PAL)":          "taunt",
		"Camouflage (DRD)":        "camouflage",
		"Selo`s Accelerando+":     "selo's accelerando",
		"  Fire Bolt$ ":           "fire bolt",
		"Illusion: Rat":           "illusion: rat",
		"Rage (of the Bear)":      "rage (of the bear)", // not a class tag
	} {
		if got := spellKey(in); got != want {
			t.Errorf("spellKey(%q) = %q, want %q", in, got, want)
		}
	}
	spells := []database.SpellName{
		{SpellID: 183, Name: "Taunt", Level: 5},
		{SpellID: 674, Name: "Provoke", Level: 24},
		{SpellID: 1606, Name: "Taunt", Level: 8},
		{SpellID: 1607, Name: "Provoke", Level: 24},
	}
	for _, c := range []struct {
		scroll string
		level  int
		want   int
	}{
		{"Taunt (SK/PAL)", 8, 1606}, // the one at the scroll's level
		{"Provoke", 24, 674},        // the same spell twice: the first
		{"Taunt", 30, 183},          // none at its level: the first of its name
	} {
		if s, ok := scrollSpell(spells, c.scroll, c.level); !ok || s.SpellID != c.want {
			t.Errorf("scrollSpell(%q, %d) = %d %v, want %d", c.scroll, c.level, s.SpellID, ok, c.want)
		}
	}
	if _, ok := scrollSpell(spells, "Bait", 34); ok {
		t.Error("a scroll of no spell found one")
	}
}

// Display indices from 252 are not shown; the next one follows the last shown.
func TestNextDisplayIndex(t *testing.T) {
	book := []database.CharSpellEntry{{AddedOrder: 0}, {AddedOrder: 252}, {AddedOrder: 3}, {AddedOrder: 1}}
	if n := nextDisplayIndex(book); n != 4 {
		t.Errorf("next %d, want 4", n)
	}
	if n := nextDisplayIndex(nil); n != 0 {
		t.Errorf("an empty book's next %d", n)
	}
}
