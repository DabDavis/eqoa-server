package handlers

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// readNpcRoles reads OpcodeNpcRoles' parts back: name -> class, and each
// part's flags.
func readNpcRoles(t *testing.T, parts [][]byte) (map[string]int, []byte) {
	t.Helper()
	got := map[string]int{}
	var flags []byte
	for _, p := range parts {
		if len(p) > npcRolesPart+6 {
			t.Errorf("a part of %d bytes, past %d", len(p), npcRolesPart+6)
		}
		if protocol.GameOpcode(binary.LittleEndian.Uint16(p)) != protocol.OpcodeNpcRoles || p[2] != 1 {
			t.Fatalf("part % x: not OpcodeNpcRoles version 1", p[:4])
		}
		flags = append(flags, p[3])
		n := int(binary.LittleEndian.Uint16(p[4:]))
		b := p[6:]
		for i := 0; i < n; i++ {
			if b[0] != npcRoleSpellMerchant {
				t.Fatalf("role %d", b[0])
			}
			c, l := int(b[1]), int(b[2])
			got[string(b[3:3+l])] = c
			b = b[3+l:]
		}
		if len(b) != 0 {
			t.Fatalf("%d bytes after the part's %d NPCs", len(b), n)
		}
	}
	return got, flags
}

// The spell merchants by what they sell: a merchant whose npcitem list holds
// scrolls (the scroll art, worn nowhere), by the class whose scrolls it holds
// most; a merchant of other wares, a creature whose corpse drops a scroll (no
// merchant flag) and a worn item with the scroll's art are none (Heirloom
// petition 3092). Sent to a vanilla session (one that answered the bug report
// offer), never to a PS2's.
func TestNpcRolesNameTheSpellMerchants(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/roles.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	pattern := func(name string, icon, slot, classes int) int {
		t.Helper()
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
			 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
			 procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (10, 0, ?, 0, ?, 0, 0, 1, 0, 1, 0, 0, 1, 1, 0, 0, ?, 32767, 0, 0, 0, 0, ?, '', 0, 0)`,
			icon, slot, classes, name)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return int(id)
	}
	npc := func(name string, id, flags int, items ...int) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO npcs (npc_name, npc_id, npc_type) VALUES (?, ?, ?)`, name, id, flags); err != nil {
			t.Fatal(err)
		}
		for i, it := range items {
			if _, err := db.Exec(`INSERT INTO npcitem (npcid, stackleft, remainHP, patternid, listnumber) VALUES (?, 1, 100, ?, ?)`, id, it, i); err != nil {
				t.Fatal(err)
			}
		}
	}
	const wiz, enc, brd = 1 << 13, 1 << 12, 1 << 5
	fire := pattern("Fire Bolt", scrollIcon, -1, wiz)
	frost := pattern("Frost Bolt", scrollIcon, -1, wiz)
	icon := int32(scrollIcon)
	calm := pattern("Calm", int(uint32(icon)), -1, brd|enc) // stored unsigned
	mez := pattern("Mesmerize", scrollIcon, -1, enc)
	charm := pattern("Charm", scrollIcon, -1, enc)
	bread := pattern("Bread", 1234, -1, 32767)
	cloak := pattern("Scroll Cloak", scrollIcon, 5, 32767)
	npc("Merchant Timmerin", 10, 129, fire, frost, calm, bread)
	npc("Merchant Coridon", 11, 129, calm, mez, charm)
	npc("Merchant Perry", 12, 129, bread, cloak)
	npc("goblin lorekeeper", 13, 0, fire)
	npc("Merchant Timmerin", 14, 129, fire) // a second spawn's row

	s := database.NewSpellbookStore(db)
	got, err := s.SpellMerchants(scrollIcon)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"Merchant Timmerin": 13, "Merchant Coridon": 12}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("spell merchants %v, want %v", got, want)
	}

	SetNpcRoleStore(s)
	t.Cleanup(func() { SetNpcRoleStore(nil) })
	vanilla, ps2 := testSession(), testSession()
	vanilla.SetBugReports(true)
	SendNpcRoles(vanilla)
	SendNpcRoles(ps2)
	var parts [][]byte
	for _, m := range vanilla.Queue().PeekPending() {
		parts = append(parts, m.Data())
	}
	read, flags := readNpcRoles(t, parts)
	if fmt.Sprint(read) != fmt.Sprint(want) || string(flags) != "\x03" {
		t.Errorf("vanilla read %v in parts flagged % x, want %v in one (first and last)", read, flags, want)
	}
	if n := len(ps2.Queue().PeekPending()); n != 0 {
		t.Errorf("a PS2's session (no answer) sent %d messages", n)
	}
}

// A long list comes in parts, each within npcRolesPart, the first flagged
// first and the last last, every name in one of them; a class past 14 goes as
// 255. None: no part.
func TestNpcRolesParts(t *testing.T) {
	spell := map[string]int{}
	for i := 0; i < 400; i++ {
		spell[fmt.Sprintf("Merchant %s%03d", strings.Repeat("x", 20), i)] = i % 16
	}
	parts := buildNpcRoles(spell)
	if len(parts) < 3 {
		t.Fatalf("%d parts for %d names", len(parts), len(spell))
	}
	read, flags := readNpcRoles(t, parts)
	if len(read) != len(spell) {
		t.Errorf("%d names read back, want %d", len(read), len(spell))
	}
	for n, c := range spell {
		if want := map[bool]int{true: c, false: 255}[c <= 14]; read[n] != want {
			t.Errorf("%s: class %d, want %d", n, read[n], want)
		}
	}
	if flags[0] != 1 || flags[len(flags)-1] != 2 || strings.Count(string(flags), "\x00") != len(flags)-2 {
		t.Errorf("flags % x: want 01, 00..., 02", flags)
	}
	if p := buildNpcRoles(nil); len(p) != 0 {
		t.Errorf("no spell merchants: %d parts", len(p))
	}
}
