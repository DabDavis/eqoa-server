package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// linkWorld is a chatWorld whose players hold items: a database with their
// rows, their clients' lists as their dumps leave them, the guild handler wired
// to the chat as the server wires it, and the links' clock in the test's hand.
type linkWorld struct {
	*chatWorld
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	gh    *GuildHandler
	clock time.Time
}

func newLinkWorld(t *testing.T) *linkWorld {
	t.Helper()
	c := newChatWorld(t)
	db, err := database.NewSQLite(t.TempDir() + "/links.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := &linkWorld{chatWorld: c, db: db, inv: database.NewInventoryStore(db), clock: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	c.h.linkRows = w.inv
	c.h.links.now = func() time.Time { return w.clock }
	w.gh = NewGuildHandler(c.sm)
	w.gh.SetGameWorld(c.gw)
	w.gh.chatGate = c.h.gmChatGate
	c.h.guildChat = w.gh.chatLine // as RegisterDefaultHandlers wires it
	return w
}

// item adds an item record: its name, the most of a stack, its durability's
// most and its STR.
func (w *linkWorld) item(name string, maxStack, maxHP, str int) int {
	w.t.Helper()
	res, err := w.db.Exec(`INSERT INTO itempattern
		(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype,
		 weapondamage, unk5, levelreq, maxstack, maxhp, duration, classuse, raceuse,
		 procanim, lore, unk6, craft, itemname, itemdesc, model, color, str)
		VALUES (10, 0, 209763529, 0, 12, 0, 0, 1, 0, 1, 30, 0, 1, ?, ?, 0, 32767, 1023, 0, 0, 0, 0, ?, 'A record for a test.', 0, 0, ?)`,
		maxStack, maxHP, name, str)
	if err != nil {
		w.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id)
}

// own gives character sid a stack of n of an item at durability hp, in the
// pack (location 0) or the bank (1), and returns its row.
func (w *linkWorld) own(sid uint32, pattern, n, hp, location int) int {
	w.t.Helper()
	id, err := w.inv.AddCharItem(sid, pattern, n, hp, 0, location)
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

// lists is what a MemoryDump leaves sess: its character's pack and bank,
// each record keyed by its position.
func (w *linkWorld) lists(sess *session.Session, sid uint32) {
	w.t.Helper()
	rows, err := w.inv.GetCharacterItems(sid)
	if err != nil {
		w.t.Fatal(err)
	}
	var pack, bank []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == database.LocBank {
			bank = append(bank, r)
		} else {
			pack = append(pack, r)
		}
	}
	sess.SetInventoryList(newInvList(pack))
	sess.SetBankList(newInvList(bank))
}

// sentLink is a link as OpcodeItemLinkSay carries it.
type sentLink struct {
	start, length int
	list          byte
	key           uint32
}

// linkOn is a link on the first sub in text (UTF-16 units) to the record of
// key in list.
func linkOn(text, sub string, list byte, key uint32) sentLink {
	i := strings.Index(text, sub)
	if i < 0 {
		panic(fmt.Sprintf("%q is not in %q", sub, text))
	}
	return sentLink{len(utf16.Encode([]rune(text[:i]))), len(utf16.Encode([]rune(sub))), list, key}
}

// linkSayBody is an OpcodeItemLinkSay's body, as vanilla sends one.
func linkSayBody(route byte, target uint32, text string, links ...sentLink) []byte {
	u := utf16.Encode([]rune(text))
	b := []byte{route}
	b = binary.LittleEndian.AppendUint32(b, target)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(u)))
	for _, c := range u {
		b = binary.LittleEndian.AppendUint16(b, c)
	}
	b = append(b, byte(len(links)))
	for _, l := range links {
		b = binary.LittleEndian.AppendUint16(b, uint16(l.start))
		b = binary.LittleEndian.AppendUint16(b, uint16(l.length))
		b = append(b, l.list)
		b = binary.LittleEndian.AppendUint32(b, l.key)
	}
	return b
}

const noTarget = 0xFFFFFFFF

// linkSay is s's client sending a line with links.
func (w *linkWorld) linkSay(s *session.Session, route byte, target uint32, text string, links ...sentLink) {
	w.t.Helper()
	if err := w.h.HandleItemLinkSay(s, incoming(protocol.OpcodeItemLinkSay, linkSayBody(route, target, text, links...))); err != nil {
		w.t.Fatal(err)
	}
}

// ask is s's client asking for a link; the answer's status and record (nil
// for none: the ask dropped).
func (w *linkWorld) ask(s *session.Session, id uint32) (status int, record []byte) {
	w.t.Helper()
	if err := w.h.HandleItemLinkAsk(s, incoming(protocol.OpcodeItemLinkAsk, u32b(id))); err != nil {
		w.t.Fatal(err)
	}
	status = -1
	for _, m := range drain(s) {
		if protocol.GameOpcode(binary.LittleEndian.Uint16(m)) != protocol.OpcodeItemLinkRecord {
			w.t.Fatalf("an ask answered with % x", m)
		}
		if got := binary.LittleEndian.Uint32(m[2:]); got != id {
			w.t.Fatalf("the answer to link %d names %d", id, got)
		}
		status, record = int(m[6]), m[7:]
	}
	return status, record
}

// drain is everything queued for s, opcode first, taken.
func drain(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// unwrap is a 0x7E51's links and its inner message; ok false for any other
// message.
func unwrap(t *testing.T, m []byte) (spans []wireSpan, inner []byte, ok bool) {
	t.Helper()
	if len(m) < 3 || protocol.GameOpcode(binary.LittleEndian.Uint16(m)) != protocol.OpcodeItemLinkLine {
		return nil, nil, false
	}
	n := int(m[2])
	b := m[3:]
	if n < 1 || n > itemLinksPerLine || len(b) < 8*n+2 {
		t.Fatalf("a 0x7E51 of %d links: % x", n, m)
	}
	for i := 0; i < n; i++ {
		spans = append(spans, wireSpan{int(binary.LittleEndian.Uint16(b[8*i:])), int(binary.LittleEndian.Uint16(b[8*i+2:])), binary.LittleEndian.Uint32(b[8*i+4:])})
	}
	return spans, b[8*n:], true
}

// innerUnits is a chat message's text, its UTF-16 units, read as CLIENT
// reads each (0x0A7A and 0x0A7B the string first; 0x069E after a u32; 0x0468
// sub 5 after the sub and a u32).
func innerUnits(t *testing.T, inner []byte) []uint16 {
	t.Helper()
	off := 2
	switch op := protocol.GameOpcode(binary.LittleEndian.Uint16(inner)); op {
	case protocol.OpcodeClientMessage, protocol.OpcodeColoredChat:
	case protocol.OpcodeGroupChat:
		off = 6
	case protocol.OpcodeGuildInfo:
		if inner[2] != 5 {
			t.Fatalf("0x0468 sub %d wrapped", inner[2])
		}
		off = 7
	default:
		t.Fatalf("a %#04x wrapped", uint16(op))
	}
	n := int(binary.LittleEndian.Uint32(inner[off:]))
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(inner[off+4+2*i:])
	}
	return u
}

// on is the text a span covers in a wrapped message's inner text.
func on(t *testing.T, s wireSpan, inner []byte) string {
	t.Helper()
	u := innerUnits(t, inner)
	if s.at < 0 || s.at+s.n > len(u) {
		t.Fatalf("a span %d+%d past a text of %d units", s.at, s.n, len(u))
	}
	return string(utf16.Decode(u[s.at : s.at+s.n]))
}

// linksTo is every link s was sent in 0x7E51s (taken), and their inner
// messages' texts.
func linksTo(t *testing.T, s *session.Session) (spans []wireSpan, texts []string) {
	t.Helper()
	for _, m := range drain(s) {
		if sp, inner, ok := unwrap(t, m); ok {
			spans = append(spans, sp...)
			texts = append(texts, string(utf16.Decode(innerUnits(t, inner))))
		}
	}
	return spans, texts
}

// recordOf is what writeSingleItem writes of row as a link keyed id sends it
// (wireRecord's: worn nowhere, on no quick slot).
func recordOf(row database.CharInventoryEntry, id uint32) []byte {
	w := buffer.NewWriter(make([]byte, 8192))
	e := wireRecord(row)
	writeSingleItem(w, &e, int(id), false)
	return w.Bytes()
}

// Every chat path, a line with a link against the same line typed without
// one: a hearer whose client never answered the offer (a PS2, an older
// vanilla) or answered 0 (its row turned off) is sent the line byte for byte
// as the line without links is sent today; one that answered 1 the same
// message whole, wrapped in 0x7E51, its link's span on the item's name in that
// message's text. Alice speaks (answered 1); Bob answered 1, Carl never, Dana
// 0.
func TestItemLinkLinesOnEveryPath(t *testing.T) {
	const sword = "Rusty Sword"
	type rig struct {
		*linkWorld
		ss []*session.Session
		ps []*entity.Player
	}
	everyone := []int{0, 1, 2, 3}
	group := func(r rig) {
		for i := 1; i < len(r.ps); i++ {
			r.gw.GroupInvite(r.ss[0], r.gw.IDSeenBy(r.ss[0], r.ps[i]))
			r.gw.GroupAccept(r.ss[i], r.gw.IDSeenBy(r.ss[i], r.ps[0]))
		}
		if n := len(r.gw.GroupMembers(r.ps[0])); n != 4 {
			t.Fatalf("a group of %d", n)
		}
	}
	guild := func(r rig) {
		g, err := r.gw.CreateGuild("Testers", r.ps[0])
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range r.ps[1:] {
			if err := r.gw.JoinGuild(g, p); err != nil {
				t.Fatal(err)
			}
		}
	}
	say := func(r rig, text string) { r.say(r.ss[0], text) }
	whisperTo := func(i int) func(r rig) uint32 {
		return func(r rig) uint32 { return r.gw.IDSeenBy(r.ss[0], r.ps[i]) }
	}
	cases := []struct {
		name    string
		text    string
		prepare func(r rig)
		plain   func(r rig, text string) // the line as the retail message
		route   byte
		target  func(r rig) uint32
		op      protocol.GameOpcode // the message wrapped
		hear    []int
	}{
		{name: "say", text: "wts Rusty Sword, cheap", plain: say, op: protocol.OpcodeClientMessage, hear: everyone},
		{name: "shout (0x0C01)", text: "wts Rusty Sword, cheap", route: itemLinkRouteShout, op: protocol.OpcodeColoredChat, hear: everyone,
			plain: func(r rig, text string) {
				r.send(r.ss[0], protocol.OpcodeClientShout, append([]byte{2}, wide(text, noTarget)...))
			}},
		{name: "shout (mode 3)", text: "wts Rusty Sword, cheap", op: protocol.OpcodeColoredChat, hear: everyone, plain: say,
			prepare: func(r rig) { r.mode(r.ss[0], ChatModeShout) }},
		{name: "group", text: "need a Rusty Sword?", op: protocol.OpcodeGroupChat, hear: everyone, plain: say,
			prepare: func(r rig) { group(r); r.mode(r.ss[0], ChatModeGroup) }},
		{name: "raid", text: "need a Rusty Sword?", op: protocol.OpcodeColoredChat, hear: everyone, plain: say,
			prepare: func(r rig) {
				group(r)
				r.say(r.ss[0], "/raidaoe")
				if !r.gw.GroupIsRaid(r.ps[0]) {
					t.Fatal("no raid")
				}
				r.mode(r.ss[0], ChatModeGroup)
			}},
		{name: "guild (mode 4)", text: "a Rusty Sword for the bank", op: protocol.OpcodeGuildInfo, hear: everyone, plain: say,
			prepare: func(r rig) { guild(r); r.mode(r.ss[0], ChatModeGuild) }},
		{name: "guild (0x0578)", text: "a Rusty Sword for the bank", route: itemLinkRouteGuild, op: protocol.OpcodeGuildInfo, hear: everyone,
			prepare: guild,
			plain: func(r rig, text string) {
				if err := r.gh.HandleGuildCommand(r.ss[0], guildCmd(guildCmdChat, wideb(text))); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "tell to a client with links", text: "/t Bob is this Rusty Sword yours?", op: protocol.OpcodeColoredChat, hear: []int{0, 1}, plain: say},
		{name: "tell to a PS2", text: "/t carl is this Rusty Sword yours?", op: protocol.OpcodeColoredChat, hear: []int{0, 2}, plain: say},
		{name: "tell to one turned off", text: "/tell Dana  is this Rusty Sword yours?", op: protocol.OpcodeColoredChat, hear: []int{0, 3}, plain: say},
		{name: "whisper", text: "psst, Rusty Sword", op: protocol.OpcodeColoredChat, hear: []int{0, 1}, target: whisperTo(1),
			prepare: func(r rig) { r.mode(r.ss[0], ChatModeWhisper) },
			plain: func(r rig, text string) {
				r.send(r.ss[0], protocol.OpcodeClientSayChat, wide(text, r.gw.IDSeenBy(r.ss[0], r.ps[1])))
			}},
		{name: "whisper to a PS2", text: "psst, Rusty Sword", op: protocol.OpcodeColoredChat, hear: []int{0, 2}, target: whisperTo(2),
			prepare: func(r rig) { r.mode(r.ss[0], ChatModeWhisper) },
			plain: func(r rig, text string) {
				r.send(r.ss[0], protocol.OpcodeClientSayChat, wide(text, r.gw.IDSeenBy(r.ss[0], r.ps[2])))
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := rig{linkWorld: newLinkWorld(t)}
			for i, n := range []string{"Alice", "Bob", "Carl", "Dana"} {
				s, p := r.join(i+1, n, 25000+float32(i), 16000, 0)
				r.ss, r.ps = append(r.ss, s), append(r.ps, p)
			}
			r.own(r.ps[0].CharServerID, r.item(sword, 1, 50, 5), 1, 40, 0)
			r.lists(r.ss[0], r.ps[0].CharServerID)
			r.ss[0].SetItemLinks(true)
			r.ss[1].SetItemLinks(true)
			r.ss[3].SetItemLinks(false) // Carl never answers
			if c.prepare != nil {
				c.prepare(r)
			}
			for _, s := range r.ss {
				drain(s)
			}
			c.plain(r, c.text)
			plain := make([][][]byte, len(r.ss))
			for i, s := range r.ss {
				plain[i] = drain(s)
			}
			target := uint32(noTarget)
			if c.target != nil {
				target = c.target(r)
			}
			r.linkSay(r.ss[0], c.route, target, c.text, linkOn(c.text, sword, 0, 0))
			var id uint32
			for i, s := range r.ss {
				got := drain(s)
				if heard := slices.Contains(c.hear, i); heard != (len(plain[i]) > 0) {
					t.Fatalf("%s heard the line without links: %v, want %v (% x)", r.ps[i].Name, len(plain[i]) > 0, heard, plain[i])
				}
				if len(got) != len(plain[i]) {
					t.Fatalf("%s: %d messages with the link, %d without", r.ps[i].Name, len(got), len(plain[i]))
				}
				for k := range got {
					spans, inner, wrapped := unwrap(t, got[k])
					links := r.ss[i].ItemLinks()
					isLine := protocol.GameOpcode(binary.LittleEndian.Uint16(plain[i][k])) == c.op
					switch {
					case !links || !isLine:
						if wrapped || !bytes.Equal(got[k], plain[i][k]) {
							t.Errorf("%s's message %d: % x, the line without links % x", r.ps[i].Name, k, got[k], plain[i][k])
						}
					case !wrapped:
						t.Errorf("%s takes links, sent % x", r.ps[i].Name, got[k])
					default:
						if !bytes.Equal(inner, plain[i][k]) {
							t.Errorf("%s's wrapped message % x, the line without links % x", r.ps[i].Name, inner, plain[i][k])
						}
						if len(spans) != 1 || on(t, spans[0], inner) != sword || spans[0].id == 0 {
							t.Errorf("%s's links %+v in %q", r.ps[i].Name, spans, string(utf16.Decode(innerUnits(t, inner))))
						}
						if id != 0 && spans[0].id != id {
							t.Errorf("%s was sent link %d, another hearer %d", r.ps[i].Name, spans[0].id, id)
						}
						id = spans[0].id
					}
				}
			}
			if id == 0 {
				t.Fatal("no hearer was sent the link")
			}
			if n := r.h.links.Len(); n != 1 {
				t.Fatalf("%d links made, want the one", n)
			}
		})
	}
}

// A link is its record as the linker held it when it linked it: the record
// writeSingleItem writes of the row then, keyed by the link. The item worn
// down and its stack spent after leave the link as it was; the control, the
// same row linked again, is a new link of the item as it is now; and the same
// row linked as the same record again is the same link, its life begun again.
func TestItemLinkIsTheRecordAsItWasLinked(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, _ := w.join(2, "Bob", 25010, 16000, 0)
	bob.SetItemLinks(true)
	sword := w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	arrows := w.own(pa.CharServerID, w.item("Bundle of Arrows", 100, 0, 0), 20, 0, 0)
	w.lists(alice, pa.CharServerID)
	text := "my Rusty Sword and a Bundle of Arrows"
	links := []sentLink{linkOn(text, "Rusty Sword", 0, 0), linkOn(text, "Bundle of Arrows", 0, 1)}
	rowOf := func(id int) database.CharInventoryEntry {
		e, err := w.inv.GetCharItem(pa.CharServerID, id)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	idsOf := func() []uint32 {
		spans, texts := linksTo(t, bob)
		if len(spans) != 2 || len(texts) != 1 || texts[0] != "Alice: "+text {
			t.Fatalf("Bob was sent %+v in %q", spans, texts)
		}
		return []uint32{spans[0].id, spans[1].id}
	}

	w.linkSay(alice, 0, noTarget, text, links...)
	ids := idsOf()
	want := map[uint32][]byte{ids[0]: recordOf(rowOf(sword), ids[0]), ids[1]: recordOf(rowOf(arrows), ids[1])}
	if ids[0] == ids[1] {
		t.Fatalf("two records, one link %d", ids[0])
	}
	if _, err := w.db.Exec(`UPDATE charinventory SET remainHP = 7 WHERE itemid = ?`, sword); err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.Exec(`UPDATE charinventory SET stackleft = 19 WHERE itemid = ?`, arrows); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if status, rec := w.ask(bob, id); status != 1 || !bytes.Equal(rec, want[id]) {
			t.Fatalf("link %d: status %d, record % x, want % x", id, status, rec, want[id])
		}
	}

	// the control: linked again, the items as they are now, under new links
	w.linkSay(alice, 0, noTarget, text, links...)
	again := idsOf()
	for i, row := range []int{sword, arrows} {
		if again[i] == ids[i] {
			t.Fatalf("a changed record linked under its old link %d", ids[i])
		}
		if status, rec := w.ask(bob, again[i]); status != 1 || !bytes.Equal(rec, recordOf(rowOf(row), again[i])) {
			t.Fatalf("the new link %d: status %d, record % x", again[i], status, rec)
		}
		if bytes.Equal(recordOf(rowOf(row), again[i]), recordOf(rowOf(row), 0)) {
			t.Fatal("the key is not in the record")
		}
	}
	// the old ones stay as they were
	if status, rec := w.ask(bob, ids[0]); status != 1 || !bytes.Equal(rec, want[ids[0]]) {
		t.Fatalf("the old link after the new one: %d % x", status, rec)
	}
	// the same row as the same record: the same link, its life from now
	w.clock = w.clock.Add(50 * time.Minute)
	w.linkSay(alice, 0, noTarget, text, links...)
	if third := idsOf(); !reflect.DeepEqual(third, again) {
		t.Fatalf("linked again as it was: %v, want %v", third, again)
	}
	w.clock = w.clock.Add(50 * time.Minute) // 100 minutes after it was made, 50 after the last
	if status, _ := w.ask(bob, again[0]); status != 1 {
		t.Fatal("a link linked again within its life died at its first hour")
	}
	if status, _ := w.ask(bob, ids[0]); status != 0 {
		t.Fatal("a link not linked again outlived its hour")
	}
	if n := w.h.links.Len(); n != 2 {
		t.Fatalf("%d links, want the two that live", n)
	}
}

// A link that does not resolve to a record its sender holds now is no link:
// its span stays the text the client sent, and a line left with none goes as
// the retail message to every client. The controls: the pack's and the bank's
// records, each by its own list's key, are links, their spans the server's
// names for them, not the client's.
func TestItemLinkRefusals(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, pb := w.join(2, "Bob", 25010, 16000, 0)
	bob.SetItemLinks(true)
	sword := w.item("Rusty Sword", 1, 50, 5)
	w.own(pa.CharServerID, sword, 1, 40, 0)                             // Alice's pack, key 0
	w.own(pa.CharServerID, w.item("Banked Shield", 1, 60, 0), 1, 60, 1) // her bank, key 0
	axe := w.item("Bob's Axe", 1, 30, 0)
	for range 3 {
		w.own(pb.CharServerID, axe, 1, 30, 0) // Bob's pack, keys 0..2
	}
	w.lists(alice, pa.CharServerID)
	w.lists(bob, pb.CharServerID)
	const text = "see Shiny Blade now"
	blade := linkOn(text, "Shiny Blade", 0, 0)
	at := func(l sentLink, list byte, key uint32) sentLink { l.list, l.key = list, key; return l }
	cases := []struct {
		name  string
		links []sentLink
		want  string // what Bob is shown
		names []string
	}{
		{"the pack's record", []sentLink{blade}, "Alice: see Rusty Sword now", []string{"Rusty Sword"}},
		{"the bank's record by its bank key", []sentLink{at(blade, 1, 0)}, "Alice: see Banked Shield now", []string{"Banked Shield"}},
		{"a key the list does not hold", []sentLink{at(blade, 0, 2)}, "Alice: " + text, nil},
		{"a list neither the pack nor the bank", []sentLink{at(blade, 2, 0)}, "Alice: " + text, nil},
		{"a span past the text", []sentLink{{start: 15, length: 8}}, "Alice: " + text, nil},
		{"an empty span", []sentLink{{start: 4, length: 0}}, "Alice: " + text, nil},
		{"one over the one before", []sentLink{linkOn(text, "see Shiny", 0, 0), linkOn(text, "Shiny Blade", 1, 0)},
			"Alice: Rusty Sword Blade now", []string{"Rusty Sword"}},
		{"one before the one before", []sentLink{linkOn(text, "now", 0, 0), linkOn(text, "see", 1, 0)},
			"Alice: see Shiny Blade Rusty Sword", []string{"Rusty Sword"}},
	}
	for _, c := range cases {
		w.linkSay(alice, 0, noTarget, text, c.links...)
		drain(alice)
		msgs := drain(bob)
		if len(msgs) != 1 {
			t.Fatalf("%s: Bob was sent %d messages", c.name, len(msgs))
		}
		spans, inner, wrapped := unwrap(t, msgs[0])
		if !wrapped {
			inner = msgs[0]
		}
		if got := string(utf16.Decode(innerUnits(t, inner))); got != c.want {
			t.Errorf("%s: Bob is shown %q, want %q", c.name, got, c.want)
		}
		var names []string
		for _, s := range spans {
			names = append(names, on(t, s, inner))
		}
		if !reflect.DeepEqual(names, c.names) || wrapped != (len(c.names) > 0) {
			t.Errorf("%s: links on %q (wrapped %v), want %q", c.name, names, wrapped, c.names)
		}
	}

	// a client links only its own items: a list naming another's row (one out
	// of step, or forged) resolves to nothing, the character's rows being read
	// by its own id; the control, Bob linking his own
	bobsRows, _ := w.inv.GetCharacterItems(pb.CharServerID)
	alice.SetInventoryList(newInvList(bobsRows))
	w.linkSay(alice, 0, noTarget, text, blade)
	if spans, _ := linksTo(t, bob); len(spans) != 0 {
		t.Fatalf("Alice linked Bob's row: %+v", spans)
	}
	w.linkSay(bob, 0, noTarget, text, blade)
	if spans, texts := linksTo(t, bob); len(spans) != 1 || texts[0] != "Bob: see Bob's Axe now" {
		t.Fatalf("Bob's own: %+v %q", spans, texts)
	}

	// past four links in a line the rest are its plain text
	w.lists(alice, pa.CharServerID)
	gems := []string{"Gem One", "Gem Two", "Gem Three", "Gem Four", "Gem Five"}
	for _, g := range gems {
		w.own(pa.CharServerID, w.item(g, 1, 10, 0), 1, 10, 0)
	}
	w.lists(alice, pa.CharServerID) // keys 1..5 the gems (0 the sword)
	five := "x1 x2 x3 x4 x5"
	var ls []sentLink
	for i := range 5 {
		ls = append(ls, linkOn(five, fmt.Sprintf("x%d", i+1), 0, uint32(i+1)))
	}
	w.linkSay(alice, 0, noTarget, five, ls...)
	spans, texts := linksTo(t, bob)
	if len(spans) != itemLinksPerLine || len(texts) != 1 || texts[0] != "Alice: Gem One Gem Two Gem Three Gem Four x5" {
		t.Fatalf("five links: %+v in %q", spans, texts)
	}

	// an id never made, and 0, are no link
	for _, id := range []uint32{12345, 0, spans[0].id + 1000} {
		if status, rec := w.ask(bob, id); status != 0 || len(rec) != 0 {
			t.Fatalf("link %d: status %d, % x", id, status, rec)
		}
	}
	if status, _ := w.ask(bob, spans[0].id); status != 1 {
		t.Fatalf("the control: link %d: status %d", spans[0].id, status)
	}

	// a message that is not one is dropped whole: an unknown route, a text of
	// none or past 1023 units, its links cut short
	for name, body := range map[string][]byte{
		"route 3":           linkSayBody(3, noTarget, text, blade),
		"no text":           linkSayBody(0, noTarget, "", blade),
		"a text of 1024":    linkSayBody(0, noTarget, strings.Repeat("x", 1024), blade),
		"its links cut off": linkSayBody(0, noTarget, text, blade)[:len(linkSayBody(0, noTarget, text, blade))-3],
		"no link count":     linkSayBody(0, noTarget, text)[:len(linkSayBody(0, noTarget, text))-1],
	} {
		if err := w.h.HandleItemLinkSay(alice, incoming(protocol.OpcodeItemLinkSay, body)); err != nil {
			t.Fatal(err)
		}
		if got := drain(bob); len(got) != 0 {
			t.Errorf("%s: Bob was sent % x", name, got)
		}
	}
}

// The filters hold with the names counted: what a PS2 cannot type is left out
// of the text between the links (a PS2's line is the line without links, byte
// for byte), each hearer's profanity mask covers the names too, and the line
// is cut at chatLineMost with the names in it, a link the cut goes through its
// plain text (the control: one that ends at the cut stays). A command takes the
// text alone, and a line no one is told makes no link.
func TestItemLinkLinesAreFilteredWithTheNamesCounted(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, _ := w.join(2, "Bob", 25010, 16000, 0)
	carl, _ := w.join(3, "Carl", 25020, 16000, 0)
	dana, pd := w.join(4, "Dana", 25030, 16000, 0)
	bob.SetItemLinks(true)
	dana.SetItemLinks(true)
	pd.ProfanityFilterOff = true
	w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	w.own(pa.CharServerID, w.item("Ass Kicker", 1, 50, 5), 1, 40, 0)
	w.lists(alice, pa.CharServerID)
	shown := func(s *session.Session) (string, []string) {
		t.Helper()
		msgs := drain(s)
		if len(msgs) != 1 {
			t.Fatalf("%d messages", len(msgs))
		}
		spans, inner, wrapped := unwrap(t, msgs[0])
		if !wrapped {
			inner = msgs[0]
		}
		var names []string
		for _, sp := range spans {
			names = append(names, on(t, sp, inner))
		}
		return string(utf16.Decode(innerUnits(t, inner))), names
	}
	plainFor := func(text string) []byte {
		w.say(alice, text)
		drain(alice)
		drain(bob)
		drain(dana)
		m := drain(carl)
		if len(m) != 1 {
			t.Fatalf("Carl heard %d", len(m))
		}
		return m[0]
	}

	// a bar, private-use characters and a line end left out; the span still on the name
	text := "a|b\uE000c\r\n Rusty Sword |d\U0001F600"
	w.linkSay(alice, 0, noTarget, text, linkOn(text, "Rusty Sword", 0, 0))
	if got, names := shown(bob); got != "Alice: abc  Rusty Sword d" || !reflect.DeepEqual(names, []string{"Rusty Sword"}) {
		t.Errorf("Bob is shown %q, links on %q", got, names)
	}
	carlHeard := drain(carl)
	drain(alice)
	drain(dana)
	if want := plainFor(text); len(carlHeard) != 1 || !bytes.Equal(carlHeard[0], want) {
		t.Errorf("the PS2 was sent % x, the line without links % x", carlHeard, want)
	}

	// the profanity mask: Bob's on (every login's), Dana's off; the name masked in its span
	text = "shit, an Ass Kicker"
	w.linkSay(alice, 0, noTarget, text, linkOn(text, "Ass Kicker", 0, 1))
	if got, names := shown(bob); got != "Alice: ****, an *** Kicker" || !reflect.DeepEqual(names, []string{"*** Kicker"}) {
		t.Errorf("Bob is shown %q, links on %q", got, names)
	}
	if got, names := shown(dana); got != "Alice: shit, an Ass Kicker" || !reflect.DeepEqual(names, []string{"Ass Kicker"}) {
		t.Errorf("Dana is shown %q, links on %q", got, names)
	}
	drain(alice)
	drain(carl)

	// the cut at 127 with the names counted: through the name, no link (what is
	// left of it plain text); ending at the cut, a link (the control)
	for _, c := range []struct {
		pad  int
		link bool
	}{{120, false}, {115, true}, {116, false}} {
		text := strings.Repeat("x", c.pad) + " Rusty Sword"
		w.linkSay(alice, 0, noTarget, text, linkOn(text, "Rusty Sword", 0, 0))
		got, names := shown(bob)
		want := "Alice: " + consoleLine(text, chatLineMost)
		if got != want || (len(names) == 1) != c.link {
			t.Errorf("%d before the name: shown %q (want %q), links on %q", c.pad, got, want, names)
		}
		drain(alice)
		drain(carl)
		drain(dana)
	}

	// a command takes the text alone: no link made, nothing said
	before := w.h.links.Len()
	for _, text := range []string{"#goto Rusty Sword", "/bug my Rusty Sword vanished", "/fly Rusty Sword"} {
		w.linkSay(alice, 0, noTarget, text, linkOn(text, "Rusty Sword", 0, 0))
		got := drain(alice)
		if len(got) != 1 || protocol.GameOpcode(binary.LittleEndian.Uint16(got[0])) != protocol.OpcodeErrorMessage {
			t.Errorf("%q: Alice was answered % x", text, got)
		}
		for _, s := range []*session.Session{bob, carl, dana} {
			if m := drain(s); len(m) != 0 {
				t.Errorf("%q: said to another: % x", text, m)
			}
		}
	}
	// a silenced speaker is told so; nothing goes, nothing is made
	w.h.gm.mu.Lock()
	w.h.gm.silenced = map[uint32]bool{pa.CharServerID: true}
	w.h.gm.mu.Unlock()
	w.linkSay(alice, 0, noTarget, "my Rusty Sword", linkOn("my Rusty Sword", "Rusty Sword", 0, 0))
	if got := drain(bob); len(got) != 0 {
		t.Errorf("a silenced speaker's line went: % x", got)
	}
	if n := w.h.links.Len(); n != before {
		t.Fatalf("%d links made by lines no one was told", n-before)
	}
}

// A link lives an hour from when it was made or last linked: just before it
// it opens, just after it is gone. At most itemLinkMost live, the oldest going
// first; ids are never 0 and never one in use.
func TestItemLinksLiveAnHourAndTheOldestGoFirst(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, _ := w.join(2, "Bob", 25010, 16000, 0)
	bob.SetItemLinks(true)
	w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	w.lists(alice, pa.CharServerID)
	start := w.clock
	w.linkSay(alice, 0, noTarget, "Rusty Sword", linkOn("Rusty Sword", "Rusty Sword", 0, 0))
	spans, _ := linksTo(t, bob)
	if len(spans) != 1 {
		t.Fatalf("links %+v", spans)
	}
	w.clock = start.Add(itemLinkTTL - time.Second)
	if status, _ := w.ask(bob, spans[0].id); status != 1 {
		t.Fatal("gone before its hour")
	}
	w.clock = start.Add(itemLinkTTL + time.Second)
	if status, _ := w.ask(bob, spans[0].id); status != 0 {
		t.Fatal("still there past its hour")
	}

	s := NewItemLinks()
	now := start
	s.now = func() time.Time { return now }
	rec := database.CharInventoryEntry{StackLeft: 1}
	first, _ := s.make(1, "A", 1, rec, "Gem")
	second, _ := s.make(1, "A", 2, rec, "Gem")
	for i := 3; i <= itemLinkMost+1; i++ {
		s.make(1, "A", i, rec, "Gem")
	}
	if _, ok := s.get(first); ok {
		t.Fatal("past the most, the oldest still lives")
	}
	if _, ok := s.get(second); !ok || s.Len() != itemLinkMost {
		t.Fatalf("the next oldest: %v, %d live", ok, s.Len())
	}
	now = now.Add(itemLinkTTL + time.Second)
	if s.make(9, "B", 1, rec, "Gem"); s.Len() != 1 {
		t.Fatalf("%d live after their hour, the one made since among them", s.Len())
	}
	s.next = 0xFFFFFFFE
	a, _ := s.make(9, "B", 2, rec, "Gem")
	b, _ := s.make(9, "B", 3, rec, "Gem")
	if a != 0xFFFFFFFF || b != 1 {
		t.Fatalf("ids %#x, %#x across the wrap", a, b)
	}
	s.next = a - 1
	if c, _ := s.make(9, "B", 4, rec, "Gem"); c != 2 {
		t.Fatalf("an id in use given again: %#x", c)
	}
}

// The links go through a keep restart: written down, read back in a new
// process's store (through the file's JSON), every one opens as it did, and
// the next is numbered after the largest kept; one past its hour stays behind,
// and a file without them leaves the store empty.
func TestItemLinksGoThroughAKeepRestart(t *testing.T) {
	was := keepLinks.Load()
	t.Cleanup(func() { keepLinks.Store(was) })
	clock := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	old := NewItemLinks()
	old.now = func() time.Time { return clock }
	old.next = 1000
	str, ac := 5, 12
	sword := database.CharInventoryEntry{ItemID: 77, StackLeft: 1, RemainHP: 40, EquipLoc: -1, Location: -1,
		Pattern: database.ItemPatternRecord{PatternID: 9, ItemName: "Rusty Sword", ItemDesc: "Old.", MaxHP: 50, Str: &str, AC: &ac}}
	ids := []uint32{}
	for i, name := range []string{"Rusty Sword", "Gem One", "Gem Two"} {
		rec := cloneEntry(sword)
		rec.ItemID, rec.Pattern.ItemName = 77+i, name
		id, made := old.make(198, "Alice", rec.ItemID, rec, name)
		if !made {
			t.Fatal("not made")
		}
		ids = append(ids, id)
		clock = clock.Add(time.Minute)
	}
	keepLinks.Store(old)
	b, err := json.Marshal(KeepItemLinks())
	if err != nil {
		t.Fatal(err)
	}
	var kept []KeptItemLink
	if err := json.Unmarshal(b, &kept); err != nil || len(kept) != 3 {
		t.Fatalf("%v, %d kept", err, len(kept))
	}

	fresh := NewItemLinks()
	fresh.now = func() time.Time { return clock }
	keepLinks.Store(fresh)
	if n := RestoreItemLinks(kept); n != 3 {
		t.Fatalf("%d put back", n)
	}
	for _, id := range ids {
		was, _ := old.get(id)
		got, ok := fresh.get(id)
		if !ok || got.name != was.name || got.who != "Alice" || !bytes.Equal(buildItemLinkRecord(id, &got.rec, false), buildItemLinkRecord(id, &was.rec, false)) {
			t.Fatalf("link %d after the restart: %+v, before %+v", id, got, was)
		}
	}
	if *fresh.byID[ids[0]].Value.(*itemLink).rec.Pattern.Str != 5 {
		t.Fatal("the record's figures lost")
	}
	if id, made := fresh.make(198, "Alice", 500, sword, "Rusty Sword"); !made || id != ids[2]+1 {
		t.Fatalf("the next link %d (made %v), want %d", id, made, ids[2]+1)
	}
	// the same row as the same record after the restart: the same link
	again := cloneEntry(sword)
	again.ItemID, again.Pattern.ItemName = 78, "Gem One"
	if id, made := fresh.make(198, "Alice", 78, again, "Gem One"); made || id != ids[1] {
		t.Fatalf("linked again after the restart: %d (made %v), want %d", id, made, ids[1])
	}

	// past its hour by the new process's clock: not put back, though the ids go on past it
	later := NewItemLinks()
	later.now = func() time.Time { return clock.Add(itemLinkTTL - 90*time.Second) }
	keepLinks.Store(later)
	if n := RestoreItemLinks(kept); n != 1 || later.next != ids[2] {
		t.Fatalf("%d put back an hour on (want the last made), next %d", n, later.next)
	}
	// a file without them
	empty := NewItemLinks()
	keepLinks.Store(empty)
	if n := RestoreItemLinks(nil); n != 0 || empty.Len() != 0 {
		t.Fatalf("%d from nothing", n)
	}
}

// At most itemLinkAsksMost asks a session in itemLinkAskWindow are answered,
// the rest dropped till the window ends; each session has its own (the
// control: under the bound every ask is answered).
func TestItemLinkAsksAreBounded(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, _ := w.join(2, "Bob", 25010, 16000, 0)
	carl, _ := w.join(3, "Carl", 25020, 16000, 0)
	bob.SetItemLinks(true)
	w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	w.lists(alice, pa.CharServerID)
	w.linkSay(alice, 0, noTarget, "Rusty Sword", linkOn("Rusty Sword", "Rusty Sword", 0, 0))
	spans, _ := linksTo(t, bob)
	id := spans[0].id
	drain(carl)
	drain(alice)
	for i := range itemLinkAsksMost {
		if status, _ := w.ask(bob, id); status != 1 {
			t.Fatalf("ask %d under the bound: %d", i+1, status)
		}
		w.clock = w.clock.Add(100 * time.Millisecond)
	}
	if status, _ := w.ask(bob, id); status != -1 {
		t.Fatalf("an ask past the bound answered %d", status)
	}
	if status, _ := w.ask(bob, 4242); status != -1 {
		t.Fatal("an ask past the bound answered")
	}
	if status, _ := w.ask(carl, id); status != 1 {
		t.Fatal("another session's ask dropped")
	}
	w.clock = w.clock.Add(itemLinkAskWindow)
	if status, _ := w.ask(bob, id); status != 1 {
		t.Fatal("not answered in the next window")
	}
	// a session out of the world is answered nothing
	carl.SetCharacterInWorld(false)
	if status, _ := w.ask(carl, id); status != -1 {
		t.Fatal("a session out of the world answered")
	}
}

// The offer goes at every world entry, once, whatever the server's settings (a
// PS2 ignores it); a client's answer is its session's (a u8 1 links, 0 plain
// lines, a wider one by its first byte, an empty one nothing), through the
// registry too; a session that never answered takes none.
func TestItemLinksOfferAndAnswer(t *testing.T) {
	h := &CharacterHandler{}
	in := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCharacterInWorld)
	sess := entitySlotsSession()
	for range 2 {
		if err := h.HandleCharacterInWorld(sess, in); err != nil {
			t.Fatal(err)
		}
	}
	var offers [][]byte
	for _, m := range sess.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 2 && protocol.GameOpcode(binary.LittleEndian.Uint16(d)) == protocol.OpcodeItemLinksOffer {
			offers = append(offers, d)
		}
	}
	if want := []byte{0x4F, 0x7E, 1}; len(offers) != 1 || !bytes.Equal(offers[0], want) {
		t.Fatalf("offered %x, want one %x", offers, want)
	}

	sess = entitySlotsSession()
	if sess.ItemLinks() {
		t.Fatal("a session that never answered takes links")
	}
	ch := NewChatHandler(nil)
	for _, c := range []struct {
		data []byte
		want bool
	}{{[]byte{1}, true}, {nil, true}, {[]byte{0}, false}, {[]byte{1, 0, 0, 0}, true}, {[]byte{0, 0, 0, 0}, false}} {
		if err := ch.HandleItemLinksAnswer(sess, incoming(protocol.OpcodeItemLinksOffer, c.data)); err != nil {
			t.Fatal(err)
		}
		if sess.ItemLinks() != c.want {
			t.Fatalf("after %x: %v, want %v", c.data, sess.ItemLinks(), c.want)
		}
	}
	r := NewRegistry()
	ch.RegisterHandlers(r)
	if err := r.Handle(sess, incoming(protocol.OpcodeItemLinksOffer, []byte{1})); err != nil || !sess.ItemLinks() {
		t.Fatalf("through the registry: %v, %v", err, sess.ItemLinks())
	}
	for _, op := range []protocol.GameOpcode{protocol.OpcodeItemLinkSay, protocol.OpcodeItemLinkAsk} {
		if _, ok := r.Get(op); !ok {
			t.Errorf("%s not registered", op)
		}
	}
}

// The messages on the wire, as the client's test reads them: the offer a u8 1;
// a wrapped line its links (count, then u16 start, u16 length, u32 id each)
// and the inner message whole; a record's answer the id, 1 and the record, its
// key the id; no such link the id and 0. And a line with links read back.
func TestItemLinksOnTheWire(t *testing.T) {
	if got, want := buildItemLinksOffer(), []byte{0x4F, 0x7E, 1}; !bytes.Equal(got, want) {
		t.Errorf("the offer %x, want %x", got, want)
	}
	inner := clientMessagePayload("Alice: Rusty Sword")
	want := append([]byte{0x51, 0x7E, 1, 7, 0, 11, 0, 0x04, 0x03, 0x02, 0x01}, inner...)
	if got := buildItemLinkLine([]wireSpan{{7, 11, 0x01020304}}, inner); !bytes.Equal(got, want) {
		t.Errorf("the line %x, want %x", got, want)
	}
	if got, want := buildItemLinkGone(0x01020304), []byte{0x53, 0x7E, 0x04, 0x03, 0x02, 0x01, 0}; !bytes.Equal(got, want) {
		t.Errorf("no such link %x, want %x", got, want)
	}
	str := 5
	rec := database.CharInventoryEntry{ItemID: 3, StackLeft: 1, RemainHP: 40, EquipLoc: 4, Location: 0,
		Pattern: database.ItemPatternRecord{PatternID: 9, ItemName: "Rusty Sword", ItemDesc: strings.Repeat("Long. ", 300), MaxHP: 50, Str: &str}}
	got := buildItemLinkRecord(0x01020304, &rec, false)
	if !bytes.Equal(got[:7], []byte{0x53, 0x7E, 0x04, 0x03, 0x02, 0x01, 1}) || !bytes.Equal(got[7:], recordOf(rec, 0x01020304)) {
		t.Errorf("the record % x", got)
	}
	// wireRecord's in the record: worn nowhere, on no quick slot (the zigzags of -1)
	if got[7+3] != 1 || got[7+4] != 1 {
		t.Errorf("worn %d, quick slot %d", got[7+3], got[7+4])
	}

	text := "a \U0001F600 Rusty Sword"
	body := linkSayBody(2, 0xABCD, text, linkOn(text, "Rusty Sword", 1, 0x0102))
	say, err := readItemLinkSay(body)
	if err != nil || say.route != 2 || say.target != 0xABCD || string(utf16.Decode(say.units)) != text ||
		len(say.links) != 1 || say.links[0] != (askedLink{start: 5, length: 11, list: 1, key: 0x0102}) {
		t.Fatalf("read back %+v, %v", say, err)
	}
	if _, err := readItemLinkSay(append(body, 9, 9)); err != nil {
		t.Fatalf("a later client's bytes after the links: %v", err)
	}
}

// A line with no link that resolves is the line without links, and a span
// that would split a character's two UTF-16 units is none.
func TestItemLinkSpansKeepToCharacters(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, _ := w.join(2, "Bob", 25010, 16000, 0)
	bob.SetItemLinks(true)
	w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	w.lists(alice, pa.CharServerID)
	text := "\U0001F600Rusty Sword"
	for _, l := range []sentLink{{start: 1, length: 12}, {start: 0, length: 1}} {
		w.linkSay(alice, 0, noTarget, text, l)
		if spans, _ := linksTo(t, bob); len(spans) != 0 {
			t.Errorf("a span %+v through a character: %+v", l, spans)
		}
	}
	w.linkSay(alice, 0, noTarget, text, sentLink{start: 2, length: 11})
	if spans, texts := linksTo(t, bob); len(spans) != 1 || texts[0] != "Alice: Rusty Sword" {
		t.Errorf("the control: %+v %q", spans, texts)
	}
}

// Through the registry as the server wires it (RegisterDefaultHandlers): the
// answer, a line with links and a click reach their handlers, the records are
// read from the inventory store, the guild's route reaches the guild handler
// (here a speaker in no guild, answered 0x0579 as 0x0578 sub 12 is), and the
// links made are the ones a keep restart writes down.
func TestItemLinksAsTheServerWiresThem(t *testing.T) {
	was := keepLinks.Load()
	t.Cleanup(func() { keepLinks.Store(was) })
	db, err := database.NewSQLite(t.TempDir() + "/wired.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	inv := database.NewInventoryStore(db)
	r := NewRegistry()
	RegisterDefaultHandlers(r, database.NewSQLiteCharacterStore(db), database.NewSQLiteAccountStore(db),
		mgr, "127.0.0.1", w, inv, database.NewSpellbookStore(db))
	t.Cleanup(func() { // the engines the registry starts are the process's (invis_test.go)
		if q := Quests(); q != nil {
			q.Stop()
		}
		if e := Triggers(); e != nil {
			e.Stop()
		}
		if a := Auctions(); a != nil {
			a.Stop()
		}
	})
	lw := &linkWorld{chatWorld: &chatWorld{t: t, sm: mgr, gw: w}, db: db, inv: inv}
	join := func(id int, name string) (*session.Session, *entity.Player) {
		s := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(id)), Port: 7000 + id}, uint32(id), uint32(id), uint16(id), 2, true)
		p := w.CreatePlayer(s, &database.CharacterRecord{ServerID: uint32(900 + id), CharName: name, X: 100, Y: 10, Z: 100})
		s.SetCharacterInWorld(true)
		return s, p
	}
	alice, pa := join(1, "Alice")
	bob, _ := join(2, "Bob")
	lw.own(pa.CharServerID, lw.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	lw.lists(alice, pa.CharServerID)
	handle := func(s *session.Session, op protocol.GameOpcode, body []byte) {
		t.Helper()
		if err := r.Handle(s, incoming(op, body)); err != nil {
			t.Fatal(err)
		}
	}
	handle(bob, protocol.OpcodeItemLinksOffer, []byte{1})
	text := "wts Rusty Sword"
	handle(alice, protocol.OpcodeItemLinkSay, linkSayBody(0, noTarget, text, linkOn(text, "Rusty Sword", 0, 0)))
	spans, texts := linksTo(t, bob)
	if len(spans) != 1 || len(texts) != 1 || texts[0] != "Alice: wts Rusty Sword" {
		t.Fatalf("Bob was sent %+v %q", spans, texts)
	}
	if kept := KeepItemLinks(); len(kept) != 1 || kept[0].ID != spans[0].id || kept[0].Owner != pa.CharServerID || kept[0].Name != "Rusty Sword" {
		t.Fatalf("a keep restart would write %+v", kept)
	}
	handle(bob, protocol.OpcodeItemLinkAsk, u32b(spans[0].id))
	if got := lastOf(drain(bob), protocol.OpcodeItemLinkRecord); len(got) < 7 || got[6] != 1 {
		t.Fatalf("the click answered % x", got)
	}
	drain(alice)
	handle(alice, protocol.OpcodeItemLinkSay, linkSayBody(itemLinkRouteGuild, noTarget, text, linkOn(text, "Rusty Sword", 0, 0)))
	if got := lastOf(drain(alice), protocol.OpcodeGuildCommandFailed); !bytes.Equal(got, buildGuildFailed(guildCmdChat, guildFailNotInGuild)) {
		t.Fatalf("the guild's route from one in no guild: % x", got)
	}
}

// The guild's route reads its text as 0x0578 sub 12 reads it, to its first NUL;
// the link before it stays one (the control: the same text as 0x0578).
func TestItemLinkGuildLinesEndAtANUL(t *testing.T) {
	w := newLinkWorld(t)
	alice, pa := w.join(1, "Alice", 25000, 16000, 0)
	bob, pb := w.join(2, "Bob", 25010, 16000, 0)
	bob.SetItemLinks(true)
	w.own(pa.CharServerID, w.item("Rusty Sword", 1, 50, 5), 1, 40, 0)
	w.lists(alice, pa.CharServerID)
	g, err := w.gw.CreateGuild("Testers", pa)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.gw.JoinGuild(g, pb); err != nil {
		t.Fatal(err)
	}
	text := "a Rusty Sword\x00 and what is past it"
	if err := w.gh.HandleGuildCommand(alice, guildCmd(guildCmdChat, wideb(text))); err != nil {
		t.Fatal(err)
	}
	plain := lastOf(drain(bob), protocol.OpcodeGuildInfo)
	w.linkSay(alice, itemLinkRouteGuild, noTarget, text, linkOn(text, "Rusty Sword", 0, 0), linkOn(text, "past it", 0, 0))
	msgs := drain(bob)
	if len(msgs) != 1 {
		t.Fatalf("Bob was sent %d", len(msgs))
	}
	spans, inner, ok := unwrap(t, msgs[0])
	if !ok || !bytes.Equal(inner, plain) || len(spans) != 1 || on(t, spans[0], inner) != "Rusty Sword" {
		t.Fatalf("Bob: %+v % x, the line as 0x0578 % x", spans, inner, plain)
	}
	if got := string(utf16.Decode(innerUnits(t, inner))); got != "Alice: a Rusty Sword" {
		t.Fatalf("the line %q", got)
	}
}
