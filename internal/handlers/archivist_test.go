package handlers

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// The Spell Archivist (petition 1279) against a database, a real world and the
// handlers the client's requests reach: a character in the world beside him,
// known to its session by an object id on a slot as the client names him, the
// window handler his 0x004C reaches and the interact handler his boxes'
// answers (0x0035) reach. The spells, scrolls and shops are made up.

const (
	archivistObject = 100001 // the id the client names him by
	archivistShop   = 162588 // his npcs.npc_id, his catalog
	archivistWizard = 13
)

type archivistFixture struct {
	t       *testing.T
	db      *database.SQLiteDB
	spells  *database.SpellbookStore
	inv     *database.InventoryStore
	w       *entity.World
	sess    *session.Session
	p       *entity.Player
	windows *NPCWindowHandler
	ih      *InteractHandler
	a       *Archivist
	npc     *entity.NPC
	slot    int
}

// newArchivistFixture is the rig over db, a temporary database when nil: a
// human wizard of level 30 (classes and race as the tests set them) standing
// beside a Spell Archivist.
func newArchivistFixture(t *testing.T, db *database.SQLiteDB) *archivistFixture {
	t.Helper()
	if db == nil {
		var err error
		if db, err = database.NewSQLite(t.TempDir() + "/archivist.db"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
	}
	rec := &database.CharacterRecord{CharName: "Scholar", AccountID: 1, Level: 30}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	f := &archivistFixture{t: t, db: db, spells: database.NewSpellbookStore(db), inv: database.NewInventoryStore(db)}
	f.w = entity.NewWorld()
	mgr := session.NewManager()
	f.w.SessionManager = mgr
	f.sess = mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 12), Port: 7012}, 12, 1, 1, 2, true)
	f.p = &entity.Player{ServerID: 1012, CharServerID: rec.ServerID, Name: "Scholar", X: 100, Y: 54, Z: 100, Level: 30,
		Class: archivistWizard, HP: 100, MaxHP: 100, Tunar: 1000}
	f.w.Players[f.p.ServerID] = f.p
	f.sess.SetECSEntity(f.p)
	f.sess.SetCharacterInWorld(true)
	f.sess.SetInventoryList(newInvList(nil))
	f.windows = NewNPCWindowHandler(mgr, f.inv)
	f.windows.SetGameWorld(f.w)
	f.ih = NewInteractHandler(mgr)
	f.ih.SetGameWorld(f.w)
	f.a = NewArchivist(f.spells, f.windows)
	f.ih.archivist = f.a
	f.npc = f.stand(SpellArchivistName, archivistShop, 0x81, archivistObject)
	return f
}

// stand puts an NPC beside the player on the session's next slot under the
// object id.
func (f *archivistFixture) stand(name string, npcid, npcType int, object uint32) *entity.NPC {
	f.t.Helper()
	n := &entity.NPC{ServerID: entity.EntityID(9000 + f.slot), Name: name, NPCID: npcid, NPCType: npcType,
		X: 101, Y: 54, Z: 100, Level: 50, HP: 1000, MaxHP: 1000}
	f.w.NPCs[n.ServerID] = n
	vs := entity.GetOrCreateVisibility(f.sess)
	vs.ChannelEntity[f.slot], vs.ChannelObjectID[f.slot] = n.ServerID, object
	f.slot++
	return n
}

// spell adds a spell pattern under its own id.
func (f *archivistFixture) spell(id int, name string, level int, desc string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO spellpattern (spellid, abilitylvl, unk2, unk3, range, casttime, power,
		iconColor, icon, scope, recast, eqprequire, spellname, spelldesc)
		VALUES (?, ?, 0, 0, 16880, 2, 15, 1234, -5678, 1, 1, -1, ?, ?)`, id, level, name, desc); err != nil {
		f.t.Fatal(err)
	}
}

// classSpell is a classspells row.
func (f *archivistFixture) classSpell(class, id int, source string) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO classspells (class, spellid, source) VALUES (?, ?, ?)`, class, id, source); err != nil {
		f.t.Fatal(err)
	}
}

// item adds an item record: the scroll's art or a potion's, worn in slot (-1
// nowhere), for the classes' and races' masks at the level.
func (f *archivistFixture) item(name string, scroll bool, slot, classes, races, level int) int {
	f.t.Helper()
	icon := int64(scrollIcon)
	if !scroll {
		icon = 209763529
	}
	res, err := f.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (24, 0, ?, 0, ?, 0, 0, 1, 0, 0, 0, 0, ?, 1, 0, 0, ?, ?, 2, 0, 0, 0, ?, '', 0, 0)`,
		icon, slot, level, classes, races, name)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// scroll is a scroll for the classes, any race, at the level.
func (f *archivistFixture) scroll(name string, classes, level int) int {
	return f.item(name, true, -1, classes, 1023, level)
}

// shop is an npcs row (a merchant's flag in npc_type, or none) and its npcitem
// list.
func (f *archivistFixture) shop(name string, npcType, npcid int, items ...int) {
	f.t.Helper()
	if _, err := f.db.Exec(`INSERT INTO npcs (npc_name, npc_type, npc_id) VALUES (?, ?, ?)`, name, npcType, npcid); err != nil {
		f.t.Fatal(err)
	}
	for i, pid := range items {
		if _, err := f.db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 0, ?, ?)`,
			npcid, pid, i); err != nil {
			f.t.Fatal(err)
		}
	}
}

// know puts a spell in the book at a display index, on the bar's slot (-1 off).
func (f *archivistFixture) know(id, order, slot int) {
	f.t.Helper()
	on := 0
	if slot >= 0 {
		on = 1
	}
	if _, err := f.db.Exec(`INSERT INTO spells (serverid, spellid, addedorder, onHotBar, whereonBar, unk1, showhide)
		VALUES (?, ?, ?, ?, ?, 0, -1)`, f.p.CharServerID, id, order, on, slot); err != nil {
		f.t.Fatal(err)
	}
}

// book is the book's rows as "id@index" ("id@index/slot" on the bar), in
// display order.
func (f *archivistFixture) book() []string {
	f.t.Helper()
	rows, err := f.db.Query(`SELECT spellid, addedorder, onHotBar, whereonBar FROM spells WHERE serverid = ? ORDER BY addedorder, id`,
		f.p.CharServerID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, order, on, slot int
		if err := rows.Scan(&id, &order, &on, &slot); err != nil {
			f.t.Fatal(err)
		}
		s := fmt.Sprintf("%d@%d", id, order)
		if on != 0 || slot != -1 {
			s += fmt.Sprintf("/%d", slot)
		}
		out = append(out, s)
	}
	return out
}

// open is the client's 0x004C for the object, as InteractWithTarget sends it.
func (f *archivistFixture) open(object uint32) []string {
	f.t.Helper()
	f.sess.Queue().GetPending()
	if err := f.windows.HandleMerchantOpen(f.sess, incoming(protocol.OpcodeMerchantDiag, u32b(object))); err != nil {
		f.t.Fatal(err)
	}
	return archivistMsgs(f.sess)
}

// pick is the client's 0x0035: the box's id and the answer.
func (f *archivistFixture) pick(box uint32, option byte) []string {
	f.t.Helper()
	f.sess.Queue().GetPending()
	if err := f.ih.HandleDialogueOption(f.sess, incoming(protocol.OpcodeDialogueBoxOption, append(u32b(box), option))); err != nil {
		f.t.Fatal(err)
	}
	return archivistMsgs(f.sess)
}

// menu opens his box and returns its id, checked.
func (f *archivistFixture) menu() uint32 {
	f.t.Helper()
	got := f.open(archivistObject)
	want := "ask#*:" + archivistQuestion + "|Browse your scrolls|Grant basic spells|Clear my spellbook|Nothing, thank you"
	if len(got) != 2 || !archivistLike(got[0], want) || got[1] != "op:00D7" {
		f.f("his question: %q, want %q and 0x00D7", got, want)
	}
	box := boxID(got)
	if !IsArchivistBox(box) || box&0xFF000000 != 0x5D000000 {
		f.f("his box %08X is not under 0x5D", box)
	}
	return box
}

func (f *archivistFixture) f(format string, args ...any) {
	f.t.Helper()
	f.t.Fatalf(format, args...)
}

// archivistLike is whether a message reads as want, a "*" in it any run of
// characters but a "|" (an id or a number).
func archivistLike(got, want string) bool {
	parts := strings.Split(want, "*")
	if !strings.HasPrefix(got, parts[0]) {
		return false
	}
	rest := got[len(parts[0]):]
	for _, p := range parts[1:] {
		i := strings.Index(rest, p)
		if i < 0 || strings.Contains(rest[:i], "|") {
			return false
		}
		rest = rest[i+len(p):]
	}
	return rest == ""
}

// archivistMsgs is what the session was sent and took: "box:", "ask#id:text|a|b",
// "add:id@index flag N" (0x00B3), "remove:index" (0x00B8), "shop:id N" (0x00B7:
// the merchant and how many records), "red:"/"yellow:"/"colour:" lines, "item"
// (0x003B) and "op:XXXX" for the rest.
func archivistMsgs(s *session.Session) []string {
	var out []string
	for _, m := range s.Queue().GetPending() {
		d := m.Data()
		if len(d) < 2 {
			continue
		}
		op := protocol.GameOpcode(binary.LittleEndian.Uint16(d))
		r := buffer.NewReader(d[2:])
		wide := func() string {
			n, _ := r.ReadUint32()
			u := make([]uint16, n)
			for i := range u {
				u[i], _ = r.ReadUint16()
			}
			return string(utf16.Decode(u))
		}
		switch op {
		case protocol.OpcodeDialogueBox:
			r.ReadUint32()
			out = append(out, "box:"+wide())
		case protocol.OpcodeQuestPopupMulti:
			id, _ := r.ReadUint32()
			text := wide()
			n, _ := r.ReadByte()
			opts := []string{}
			for i := 0; i < int(n); i++ {
				opts = append(opts, wide())
			}
			out = append(out, fmt.Sprintf("ask#%d:%s|%s", id, text, strings.Join(opts, "|")))
		case protocol.OpcodeAbilityAdd:
			r.ReadUint32()
			flag, _ := r.ReadUint32()
			id, _ := r.Read7BitEncodedInt64()
			index, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("add:%d@%d flag %d", id, index, flag))
		case protocol.OpcodeAbilityRemove:
			r.ReadUint32()
			index, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("remove:%d", index))
		case protocol.OpcodeMerchantBox:
			id, _ := r.ReadUint32()
			r.Read7BitEncodedInt64()
			r.Read7BitEncodedInt64()
			n, _ := r.Read7BitEncodedInt64()
			out = append(out, fmt.Sprintf("shop:%d %d", id, n))
		case protocol.OpcodeColoredChat:
			text := wide()
			red, _ := r.ReadByte()
			green, _ := r.ReadByte()
			switch {
			case red == 255 && green == 63:
				out = append(out, "red:"+text)
			case red == 255 && green == 255:
				out = append(out, "yellow:"+text)
			default:
				out = append(out, "colour:"+text)
			}
		case protocol.OpcodeAddInvItem:
			out = append(out, "item")
		default:
			out = append(out, fmt.Sprintf("op:%04X", uint16(op)))
		}
	}
	return out
}

// wantMsgs checks got message by message (archivistLike).
func (f *archivistFixture) wantMsgs(what string, got []string, want ...string) {
	f.t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(want); i++ {
		ok = archivistLike(got[i], want[i])
	}
	if !ok {
		f.t.Errorf("%s: sent\n  %q\nwant\n  %q", what, got, want)
	}
}

// A request for his window asks first, under 0x5D with the request cleared
// (0x00D7) and no shop yet; "Browse your scrolls" opens his shop under the id
// the client named him by, his class's scrolls alone, and a buy off it works.
// The controls: another merchant's request opens its shop at once, as his does
// with no Archivist served; a close keeps his box and an old one does nothing;
// walked out of reach, an answer is refused in red and nothing opens; "Nothing,
// thank you" sends nothing.
func TestTheArchivistAsksBeforeHisShop(t *testing.T) {
	f := newArchivistFixture(t, nil)
	wizScroll := f.scroll("Fire Bolt", 1<<archivistWizard, 5)
	clrScroll := f.scroll("Minor Healing", 1<<9, 1)
	f.shop(SpellArchivistName, 0x81, archivistShop, wizScroll, clrScroll)
	f.shop("Merchant", 0x81, 7, wizScroll)
	f.stand("Merchant", 7, 0x81, 100002)

	box := f.menu()
	f.wantMsgs("Browse your scrolls", f.pick(box, archivistBrowse), fmt.Sprintf("shop:%d 1", archivistObject))
	f.wantMsgs("his scroll bought", f.buy(archivistObject, 0), "item", "op:0052")

	// another merchant's shop opens at once
	f.wantMsgs("a merchant's request", f.open(100002), "shop:100002 1")

	// a close keeps the box; "Nothing, thank you" sends nothing; then it is old
	box = f.menu()
	f.wantMsgs("a close", f.pick(box, 255))
	f.wantMsgs("nothing, thank you", f.pick(box, archivistNothing))
	f.wantMsgs("an old box", f.pick(box, archivistBrowse))

	// out of reach
	box = f.menu()
	f.p.X += 50
	f.wantMsgs("browsing out of reach", f.pick(box, archivistBrowse), "red:You are too far from the Spell Archivist.")
	f.p.X -= 50

	// no Archivist served: his shop opens at once, as before petition 1279
	f.windows.archivist = nil
	f.wantMsgs("no Archivist served", f.open(archivistObject), fmt.Sprintf("shop:%d 1", archivistObject))
}

// buy is the client's 0x004A: the list's index, one, the merchant's id.
func (f *archivistFixture) buy(merchant uint32, index int) []string {
	f.t.Helper()
	f.sess.Queue().GetPending()
	payload := build(protocol.OpcodeMerchantBuy, 32, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(int64(index))
		w.Write7BitEncodedInt64(1)
		w.WriteUint32(merchant)
	})[2:]
	if err := f.windows.HandleMerchantBuy(f.sess, incoming(protocol.OpcodeMerchantBuy, payload)); err != nil {
		f.t.Fatal(err)
	}
	return archivistMsgs(f.sess)
}

// grant answers his box "Grant basic spells".
func (f *archivistFixture) grant() []string {
	f.t.Helper()
	return f.pick(f.menu(), archivistGrant)
}

// "Grant basic spells" for a human wizard of 30: every spell a merchant sells
// as a scroll the wizard could scribe (one sold under a class tag too, one a
// spell another class has as well), the record the scribe would write (of two
// records of one name, the wizard's), written after the book's last index off
// the bar in level and name order, each sent as 0x00B3 with the flag 0, then
// one box. Not granted: a scroll only a creature holds (its corpse's loot), a
// potion of a spell's name, a scroll worn somewhere, another class's, one the
// race may not use, one above the level, one whose spell is above it, a were's,
// a spell the class has as a master class's, one only another class has as a
// mastery's, a class spell no merchant sells (a quest's), one the book holds by
// record or by exact name; the "+" record held, the base spell is granted. A
// second grant gives nothing; with every class open to every race the race's
// scroll is granted.
func TestTheArchivistGrantsTheBasicSpells(t *testing.T) {
	f := newArchivistFixture(t, nil)
	const wiz, enc, clr = 1 << archivistWizard, 1 << 12, 1 << 9
	type sp struct {
		id         int
		name       string
		level      int
		desc       string
		wizSource  string // the wizard's classspells row ("" none)
		otherClass int    // another class's row, of otherSource
		other      string
	}
	for _, s := range []sp{
		{id: 1623, name: "Return Home", level: 1, wizSource: "dump"},
		{id: 101, name: "Fire Bolt", level: 5, wizSource: "dump"},
		{id: 102, name: "Frost Bolt", level: 10, wizSource: "vendor"},
		{id: 103, name: "Loot Bolt", level: 5, wizSource: "dump"},
		{id: 104, name: "Potion Bolt", level: 5, wizSource: "dump"},
		{id: 105, name: "Worn Bolt", level: 5, wizSource: "dump"},
		{id: 106, name: "Cleric Ward", level: 5, otherClass: 9, other: "dump"},
		{id: 107, name: "Elf Ward", level: 6, wizSource: "dump"},
		{id: 108, name: "High Bolt", level: 31, wizSource: "dump"},
		{id: 109, name: "Deep Bolt", level: 31, wizSource: "dump"},
		{id: 110, name: "Ratwere Bite", level: 5, wizSource: "dump"},
		{id: 111, name: "Djinni Bolt", level: 20, wizSource: "eqoa-data mca"},
		{id: 112, name: "Mastered Bolt", level: 20, otherClass: 12, other: "eqoa-data cm"},
		{id: 113, name: "Shared Shield", level: 20, otherClass: 12, other: "vendor"},
		{id: 114, name: "Arcane Veil", level: 10, otherClass: 12, other: "dump"},
		{id: 115, name: "Arcane Veil", level: 12, wizSource: "eqoa-data"},
		{id: 116, name: "Quest Bolt", level: 5, wizSource: "dump"},
		{id: 117, name: "Known Bolt", level: 5, wizSource: "dump"},
		{id: 118, name: "Echo Bolt", level: 8, wizSource: "dump"},
		{id: 119, name: "Echo Bolt", level: 8},
		{id: 120, name: "Ward of Hope", level: 15, wizSource: "dump"},
		{id: 121, name: "Ward of Hope +", level: 15, wizSource: "eqoa-data"},
		{id: 122, name: "Tidal Form", level: 9, desc: "This spell grants the power of the beast without changing your form.", wizSource: "dump"},
	} {
		f.spell(s.id, s.name, s.level, s.desc)
		if s.wizSource != "" {
			f.classSpell(archivistWizard, s.id, s.wizSource)
		}
		if s.other != "" {
			f.classSpell(s.otherClass, s.id, s.other)
		}
	}
	sold := []int{
		f.scroll("Fire Bolt", wiz, 5),
		f.scroll("Frost Bolt (WIZ)", wiz, 10),
		f.item("Potion Bolt", false, -1, wiz, 1023, 5),
		f.item("Worn Bolt", true, 3, wiz, 1023, 5),
		f.scroll("Cleric Ward", clr, 5),
		f.item("Elf Ward", true, -1, wiz, 1<<1, 6),
		f.scroll("High Bolt", wiz, 31),
		f.scroll("Deep Bolt", wiz, 20),
		f.scroll("Ratwere Bite", wiz, 5),
		f.scroll("Djinni Bolt", wiz, 20),
		f.scroll("Mastered Bolt", wiz|enc, 20),
		f.scroll("Shared Shield", wiz|enc, 20),
		f.scroll("Arcane Veil", wiz|enc, 10),
		f.scroll("Known Bolt", wiz, 5),
		f.scroll("Echo Bolt", wiz, 8),
		f.scroll("Ward of Hope", wiz, 15),
		f.scroll("Tidal Form", wiz, 9),
	}
	f.shop("Scroll Merchant", 0x81, 41, sold...)
	f.shop("a scroll-carrying goblin", 0, 42, f.scroll("Loot Bolt", wiz, 5)) // a corpse's loot
	f.shop(SpellArchivistName, 0x81, archivistShop)
	f.know(1623, 0, 0)
	f.know(117, 1, -1)
	f.know(119, 2, -1) // Echo Bolt, by another record
	f.know(121, 3, -1) // Ward of Hope +

	f.wantMsgs("the grant", f.grant(),
		"add:101@4 flag 0", "add:102@5 flag 0", "add:115@6 flag 0", "add:120@7 flag 0", "add:113@8 flag 0",
		"box:Spell Archivist: There: 5 spells of your calling written into your book, every one the merchants sell up to level 30 that you lacked.")
	want := []string{"1623@0/0", "117@1", "119@2", "121@3", "101@4", "102@5", "115@6", "120@7", "113@8"}
	if got := f.book(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the book: %v, want %v", got, want)
	}

	f.wantMsgs("a second grant", f.grant(),
		"box:Spell Archivist: Your book holds every spell of your calling the merchants sell up to level 30 already.")

	was := entity.Creation()
	defer entity.SetCreationRules(was)
	entity.SetCreationRules(entity.CreationRules{Races: true, AllClasses: true})
	f.wantMsgs("every class open to every race", f.grant(), "add:107@9 flag 0",
		"box:Spell Archivist: There: 1 spell of your calling written into your book, every one the merchants sell up to level 30 that you lacked.")
}

// The grant stops at level 60 whatever the level (the petition's "cap 60"),
// and a book with no room past index 251 takes what fits and says how many
// more there were; a full one takes none.
func TestTheArchivistGrantsUpToSixtyAndWhatFits(t *testing.T) {
	f := newArchivistFixture(t, nil)
	const wiz = 1 << archivistWizard
	var sold []int
	for i, lv := range []int{58, 59, 60, 61} {
		id := 201 + i
		name := fmt.Sprintf("Bolt of the %dth", lv)
		f.spell(id, name, lv, "")
		f.classSpell(archivistWizard, id, "dump")
		sold = append(sold, f.scroll(name, wiz, lv))
	}
	f.shop("Scroll Merchant", 0x81, 41, sold...)
	f.p.Level = 65
	f.wantMsgs("a level 65's grant", f.grant(), "add:201@0 flag 0", "add:202@1 flag 0", "add:203@2 flag 0",
		"box:Spell Archivist: There: 3 spells of your calling written into your book, every one the merchants sell up to level 60 that you lacked.")

	// a book to index 250: one fits, at 251
	if _, err := f.db.Exec(`DELETE FROM spells WHERE serverid = ?`, f.p.CharServerID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 250; i++ {
		f.spell(1000+i, fmt.Sprintf("Filler %d", i), 1, "")
		f.know(1000+i, i, -1)
	}
	f.spell(1999, "Far Filler", 1, "")
	f.know(1999, 300, -1) // an index past the book's, which leaves the next index alone
	f.wantMsgs("a book with one index left", f.grant(), "add:201@251 flag 0",
		"box:Spell Archivist: There: 1 spell written into your book, which has no room for 2 more. Make room in it and ask again.")
	f.wantMsgs("a full book", f.grant(), "box:Spell Archivist: Your book has no room for another spell.")
	var past int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM spells WHERE serverid = ? AND addedorder >= 252 AND spellid != 1999`,
		f.p.CharServerID).Scan(&past); err != nil || past != 0 {
		t.Errorf("%d spells written past index 251 (%v)", past, err)
	}
}

// "Clear my spellbook" says what it takes and asks Yes or No: No keeps the
// book; Yes takes every row but Return Home's (both its records, its bar slot
// kept) and sends 0x00B8 by each row's index (a spell with two rows of one
// index twice, a memorized one too: the bar is drawn from the records), but not
// for a spell sharing Return Home's index or one past 251, which the player is
// told leave at the next login; then a box. A book holding only Return Home
// has nothing to clear.
func TestTheArchivistClearsTheBook(t *testing.T) {
	f := newArchivistFixture(t, nil)
	for _, s := range []struct {
		id    int
		name  string
		level int
	}{{1623, "Return Home", 1}, {2300, "Return Home", 1}, {101, "Fire Bolt", 5}, {102, "Frost Bolt", 10},
		{113, "Shared Shield", 20}, {115, "Arcane Veil", 12}, {120, "Ward of Hope", 15}} {
		f.spell(s.id, s.name, s.level, "")
	}
	f.shop(SpellArchivistName, 0x81, archivistShop)
	f.know(1623, 0, 0)
	f.know(101, 1, 1)
	f.know(102, 2, -1)
	f.know(120, 3, -1)
	f.know(120, 3, -1) // the same spell twice at one index, as one live book holds one
	f.know(2300, 5, -1)
	f.know(113, 5, -1) // sharing Return Home's index
	f.know(115, 300, -1)
	before := f.book()

	ask := func() uint32 {
		t.Helper()
		got := f.pick(f.menu(), archivistClear)
		f.wantMsgs("the clear's question", got, "box:Spell Archivist: "+archivistClearIs,
			"ask#*:Spell Archivist: 'Shall I clear your book?'|Yes, clear it|No")
		return boxID(got)
	}
	f.wantMsgs("No", f.pick(ask(), 1), "box:Spell Archivist: As you wish: your book stays as it is.")
	if got := f.book(); strings.Join(got, " ") != strings.Join(before, " ") {
		t.Fatalf("No changed the book: %v, was %v", got, before)
	}

	box := ask()
	f.wantMsgs("a close of the question", f.pick(box, 255))
	f.wantMsgs("Yes", f.pick(box, 0), "remove:1", "remove:2", "remove:3", "remove:3",
		"box:Spell Archivist: Your book is clear but for Return Home: 5 spells gone.",
		"yellow:Shared Shield, Arcane Veil will be gone from your book at your next login.")
	if got, want := f.book(), []string{"1623@0/0", "2300@5"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the book after: %v, want %v", got, want)
	}

	f.wantMsgs("a book of Return Home alone", f.pick(ask(), 0),
		"box:Spell Archivist: Your book holds nothing but Return Home: there is nothing to clear.")
}

// His words fit CLIENT's boxes (a question 127 characters, an answer 63, a box
// 217, six answers a box) at their longest, and his mark is no other box's.
func TestTheArchivistsWordsFitTheirBoxes(t *testing.T) {
	for _, q := range []string{archivistQuestion, archivistClearAsk} {
		if n := len([]rune(q)); n > questAskChars {
			t.Errorf("%q is %d characters, a question shows %d", q, n, questAskChars)
		}
	}
	answers := append([]string{archivistClearYes, archivistClearNo}, archivistAnswers...)
	for _, a := range answers {
		if n := len([]rune(a)); n > questAnswerChars {
			t.Errorf("%q is %d characters, an answer shows %d", a, n, questAnswerChars)
		}
	}
	if len(archivistAnswers) > maxDialogOptions {
		t.Errorf("%d answers, a box holds %d", len(archivistAnswers), maxDialogOptions)
	}
	for _, s := range []string{archivistClearIs, fmt.Sprintf(archivistGranted, spellCount(251), 60),
		fmt.Sprintf(archivistGrantedSome, spellCount(251), 251), fmt.Sprintf(archivistHasAll, 60), archivistBookFull,
		fmt.Sprintf(archivistCleared, spellCount(251)), archivistNothingToClear, archivistKept, archivistTrouble} {
		if n := len([]rune(SpellArchivistName + ": " + s)); n > questBoxChars {
			t.Errorf("%q is %d characters with his name, a box shows %d", s, n, questBoxChars)
		}
	}
	box := uint32(archivistBoxMark | 7)
	if IsDyerBox(box) || IsGenieBox(box) || IsRehabBox(box) || IsPortalBox(box) || IsTriggerBox(box) || entity.IsDuelBox(box) ||
		box&0xFF000000 == questBoxMark || box&0xFF000000 == advisorBoxMark || box&0xFF000000 == coachBoxMark ||
		box&0xFF000000 == boardBoxMark || box&0xFF000000 == moogieBoxMark || box&0xFF000000 == dabrishBoxMark {
		t.Errorf("box %08X is another's too", box)
	}
}
