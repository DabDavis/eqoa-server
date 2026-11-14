package gmweb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A GM's npctint row is the game's at once (entity/npctint.go): the client in
// the world that draws tints is sent the set with the name in it, in any case
// the GM typed it, and again at a change of colour; the row deleted, the
// empty set. A row that names no one, or a colour past 32 bits, is refused
// and the clients are told nothing. The control: the PS2's session beside it
// is sent nothing through all of it.
func TestTintEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	online := func(n int) *session.Session {
		sess := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9200 + n}, uint32(n), uint32(n), 1, 2, true)
		sess.SetCharacterInWorld(true)
		return sess
	}
	van, ps2 := online(1), online(2)
	van.SetNPCTints(true)
	tints := func(s *session.Session) [][]byte {
		var out [][]byte
		for _, m := range s.Queue().PeekPending() {
			if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNPCTints {
				out = append(out, d[2:])
			}
		}
		return out
	}

	code, out := c.op("insert_row", map[string]any{"table": "npctint",
		"values": map[string]any{"name": "Yhafiran", "color": 0x558C50FF, "note": "the test's"}})
	if code != 200 {
		t.Fatalf("insert: %d %v", code, out)
	}
	if got := tints(van); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 0, 8, 'y', 'h', 'a', 'f', 'i', 'r', 'a', 'n', 0xFF, 0x50, 0x8C, 0x55}) {
		t.Fatalf("after the insert: % x", got)
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") || !strings.Contains(live, "1 client") {
		t.Errorf("live: %v", out["live"])
	}
	if w := f.world.NPCTints(); len(w) != 1 || w[0].Name != "yhafiran" {
		t.Errorf("the world holds %v", w)
	}

	if code, out := c.op("update_row", map[string]any{"table": "npctint", "key": map[string]any{"name": "Yhafiran"},
		"values": map[string]any{"color": 0x203020FF}}); code != 200 {
		t.Fatalf("update: %d %v", code, out)
	}
	if got := tints(van); len(got) != 2 || !bytes.HasSuffix(got[1], []byte{0xFF, 0x20, 0x30, 0x20}) {
		t.Fatalf("after the update: % x", got)
	}

	for _, bad := range []map[string]any{{"name": "  ", "color": 0x558C50FF}, {"name": "Poxathixius", "color": float64(1 << 33)}} {
		if code, out := c.op("insert_row", map[string]any{"table": "npctint", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	if len(tints(van)) != 2 {
		t.Errorf("a refused row told the clients something: %d messages", len(tints(van)))
	}

	if code, out := c.op("delete_row", map[string]any{"table": "npctint", "key": map[string]any{"name": "Yhafiran"}}); code != 200 {
		t.Fatalf("delete: %d %v", code, out)
	}
	if got := tints(van); len(got) != 3 || !bytes.Equal(got[2], []byte{0, 0}) {
		t.Fatalf("after the delete: % x", got)
	}

	// a row a tool wrote (SQL) reaches the clients with reload_npcs, and not
	// before it
	mustExec(t, f.db, `INSERT INTO npctint (name, color, source) VALUES ('Yhafiran', 1435259135, 'seed')`)
	if len(tints(van)) != 3 {
		t.Fatal("a row written outside the panel went out by itself")
	}
	if code, out := c.op("reload_npcs", map[string]any{}); code != 200 || !strings.Contains(fmt.Sprint(out["message"]), "1 name(s) tinted, sent to 1 client(s)") {
		t.Fatalf("reload_npcs: %d %v", code, out)
	}
	if got := tints(van); len(got) != 4 || !bytes.Contains(got[3], []byte("yhafiran")) {
		t.Fatalf("after reload_npcs: % x", got)
	}
	if len(tints(ps2)) != 0 {
		t.Fatalf("the PS2 was sent %d tint messages", len(tints(ps2)))
	}
}

// The auras in the panel (claude-notes/auras-20261113.md): npctint's aura
// columns reach a layout 3 client at once, and a kind, node count or source
// past what the client reads is refused; an itemaura row is the world's at
// once (and gone with its delete), audited, and one of a kind past 6 refused;
// the trim kinds (5, 6) and a dark aura (a black colour) kept.
func TestAuraEditsAreLive(t *testing.T) {
	f := newFixture(t)
	c := f.browser()
	c.login("gm")
	van := f.s.d.Sessions.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9300}, 30, 30, 1, 2, true)
	van.SetCharacterInWorld(true)
	van.SetNPCTintsLayout(3)
	sent := func() int {
		n := 0
		for _, m := range van.Queue().PeekPending() {
			if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeNPCTints {
				n++
			}
		}
		return n
	}
	code, out := c.op("insert_row", map[string]any{"table": "npctint",
		"values": map[string]any{"name": "a skeleton", "aura_kind": 3, "aura_source": float64(0x88EC092D), "aura_nodes": 12}})
	if code != 200 || sent() != 1 {
		t.Fatalf("insert: %d %v; %d sent", code, out, sent())
	}
	if w := f.world.NPCTints(); len(w) != 1 || w[0].AuraKind != 3 || w[0].AuraSource != 0x88EC092D || w[0].AuraNodes != 12 {
		t.Fatalf("the world holds %+v", w)
	}
	for _, bad := range []map[string]any{{"name": "x", "aura_kind": 7}, {"name": "x", "aura_nodes": 33}, {"name": "x", "aura_source": float64(1 << 33)}} {
		if code, out := c.op("insert_row", map[string]any{"table": "npctint", "values": bad}); code != 400 {
			t.Errorf("insert %v: %d %v, want refused", bad, code, out)
		}
	}
	// the trim kinds, and a dark aura (a black glow) that lights nothing but
	// is named: kept and sent to the layout 3 client
	code, out = c.op("insert_row", map[string]any{"table": "npctint",
		"values": map[string]any{"name": "dust", "glow": float64(0x000000FF), "aura_kind": 5}})
	if code != 200 || sent() != 2 {
		t.Fatalf("a dark trim: %d %v; %d sent", code, out, sent())
	}
	if w := f.world.NPCTints(); len(w) != 2 {
		t.Fatalf("the dark trim row not in the world: %+v", w)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "npctint", "values": map[string]any{"name": "y", "aura_kind": 6}}); code != 200 {
		t.Errorf("kind 6: %d %v", code, out)
	}

	mustExec(t, f.db, `INSERT INTO itempattern (patternid, itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
		VALUES (424242, 0, 0, 1, 0, 7, 0, 1, 1, 0, 0, 0, 0, 1, 1, 0, 0, 32767, 32767, 0, 0, 0, 0, 'Glowing Bracer', '', 4, 0)`)
	code, out = c.op("insert_row", map[string]any{"table": "itemaura",
		"values": map[string]any{"patternid": 424242, "color": float64(0x000000FF), "kind": 5, "note": "the test's: a dark trim"}})
	if code != 200 || f.world.ItemAuraCount() != 1 {
		t.Fatalf("itemaura insert: %d %v; the world holds %d", code, out, f.world.ItemAuraCount())
	}
	if live, _ := out["live"].(string); !strings.Contains(live, "at once") {
		t.Errorf("live: %v", out["live"])
	}
	if code, out := c.op("insert_row", map[string]any{"table": "itemaura", "values": map[string]any{"patternid": 424243, "kind": 7}}); code != 400 {
		t.Errorf("a kind of 7: %d %v, want refused", code, out)
	}
	if code, out := c.op("delete_row", map[string]any{"table": "itemaura", "key": map[string]any{"patternid": 424242}}); code != 200 || f.world.ItemAuraCount() != 0 {
		t.Fatalf("itemaura delete: %d %v", code, out)
	}
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'itemaura'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("%d itemaura audit lines, %v", n, err)
	}
	// an item that is a circlet (kind 8, its height in nodes up to 100)
	if code, out := c.op("insert_row", map[string]any{"table": "itemaura", "values": map[string]any{"patternid": 424243, "color": float64(0xE01818FF), "kind": 8, "source": float64(0x1D875ECD), "nodes": 72}}); code != 200 {
		t.Fatalf("itemaura circlet insert: %d %v", code, out)
	}
	if code, out := c.op("insert_row", map[string]any{"table": "itemaura", "values": map[string]any{"patternid": 424244, "kind": 8, "nodes": 101}}); code != 400 {
		t.Fatalf("itemaura circlet past 100: %d %v", code, out)
	}
	if code, out := c.op("delete_row", map[string]any{"table": "itemaura", "key": map[string]any{"patternid": 424243}}); code != 200 {
		t.Fatalf("itemaura circlet delete: %d %v", code, out)
	}
	// a character's own (charaura): a body aura kept, a slot past 7 and a
	// kind past 6 refused
	if code, out := c.op("insert_row", map[string]any{"table": "charaura", "values": map[string]any{"charid": 9001, "merc": 0, "slot": 7, "color": float64(0xA040FFFF), "kind": 2}}); code != 200 {
		t.Fatalf("charaura insert: %d %v", code, out)
	}
	// a held item's (slots 9 and 10, 2026-11-14): the epic's look kept
	if code, out := c.op("insert_row", map[string]any{"table": "charaura", "values": map[string]any{"charid": 9001, "merc": 0, "slot": 9, "color": 0, "kind": 7, "source": float64(0x5FD4AE7C)}}); code != 200 {
		t.Fatalf("charaura epic insert: %d %v", code, out)
	}
	// where along it (nodes, 0-100 on a hand)
	if code, out := c.op("insert_row", map[string]any{"table": "charaura", "values": map[string]any{"charid": 9001, "merc": 0, "slot": 10, "color": 0, "kind": 7, "source": float64(0xCBF4135A), "nodes": 45}}); code != 200 {
		t.Fatalf("charaura along insert: %d %v", code, out)
	}
	// a circlet on the helm's slot (kind 8)
	if code, out := c.op("insert_row", map[string]any{"table": "charaura", "values": map[string]any{"charid": 9001, "merc": 0, "slot": 5, "color": float64(0xE01818FF), "kind": 8, "source": float64(0x1D875ECD), "nodes": 75}}); code != 200 {
		t.Fatalf("charaura circlet insert: %d %v", code, out)
	}
	for _, bad := range []map[string]any{{"charid": 9001, "slot": 11}, {"charid": 9001, "slot": 0, "kind": 8}, {"charid": 9001, "slot": 9, "kind": 8}, {"charid": 9001, "slot": 5, "kind": 8, "nodes": 101}, {"charid": 9001, "slot": 9, "kind": 7, "nodes": 101}, {"charid": 9001, "slot": 7, "kind": 2, "nodes": 45}, {"charid": 9001, "slot": 1, "kind": 7}, {"charid": 0, "slot": 1},
		{"charid": 9001, "slot": 10, "kind": 2}, {"charid": 9001, "slot": 10, "kind": 8}, {"charid": 9001, "slot": 8, "kind": 7}} {
		if code, out := c.op("insert_row", map[string]any{"table": "charaura", "values": bad}); code != 400 {
			t.Errorf("charaura %v: %d %v, want refused", bad, code, out)
		}
	}
}
