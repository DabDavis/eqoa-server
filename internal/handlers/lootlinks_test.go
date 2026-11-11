package handlers

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Loot lines with item links (lootlinks.go; the owner, 2026-11-11). Each against its
// control: a client that took links (a vanilla-like session) is sent the line wrapped in
// 0x7E51, its inner message the very bytes every other client (a PS2-like one: never
// answered the offer) is sent, which are the line as it was before links.

// linkedOnly is the one 0x7E51 among msgs whose inner message's text holds part: its
// links and inner message; fails without exactly one.
func linkedOnly(t *testing.T, msgs [][]byte, part string) ([]wireSpan, []byte) {
	t.Helper()
	var spans []wireSpan
	var inner []byte
	n := 0
	for _, m := range msgs {
		if sp, in, ok := unwrap(t, m); ok && strings.Contains(string(utf16Text(t, in)), part) {
			spans, inner = sp, in
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d linked lines holding %q in %s", n, part, ops(msgs))
	}
	return spans, inner
}

func utf16Text(t *testing.T, inner []byte) string {
	t.Helper()
	u := innerUnits(t, inner)
	r := make([]rune, len(u))
	for i, c := range u {
		r[i] = rune(c)
	}
	return string(r)
}

// plainHas is whether msgs hold want byte for byte, and no 0x7E51.
func plainHas(t *testing.T, msgs [][]byte, want []byte) bool {
	t.Helper()
	found := false
	for _, m := range msgs {
		if protocol.GameOpcode(m[0])|protocol.GameOpcode(m[1])<<8 == protocol.OpcodeItemLinkLine {
			t.Fatalf("a client without links was sent a 0x7E51: % x", m)
		}
		found = found || bytes.Equal(m, want)
	}
	return found
}

func pendingOf(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

// A take from a corpse: the looter (links) and its group (one with links, one a PS2) are
// told the item as a link, the PS2 the old line byte for byte; the link is the record as
// the corpse held it, the count taken, and one link for every line of the same take. A
// take with no one taking links makes no link (the control).
func TestLootLinesLinkTheItem(t *testing.T) {
	f := newCorpseFixture(t, 1)
	f.loadLists()
	f.h.links = NewItemLinks()
	f.player.Name = "Ann"
	f.sess.SetItemLinks(true)
	vanilla := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 77), Port: 7077}, 77, 1, 1, 2, true)
	vanilla.SetItemLinks(true)
	ps2 := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 78), Port: 7078}, 78, 1, 1, 2, true)
	f.h.groupOf = func(*entity.Player) []*session.Session { return []*session.Session{vanilla, ps2} }
	f.h.OpenCorpse(f.sess, f.npc)
	f.sent()
	name := f.corpseItemName(t)
	f.send(protocol.OpcodeClientLoot, func(w *buffer.Writer) { w.WriteUint32(0); w.WriteUint32(1) })

	group := groupLootLine("Ann", 1, name)
	if !plainHas(t, pendingOf(ps2), clientMessagePayload(group)) {
		t.Fatalf("the PS2 was not sent %q as ever", group)
	}
	spans, inner := linkedOnly(t, pendingOf(vanilla), group)
	if !bytes.Equal(inner, clientMessagePayload(group)) || len(spans) != 1 || on(t, spans[0], inner) != name {
		t.Fatalf("the group's linked line: %+v % x", spans, inner)
	}
	mine, innerMine := linkedOnly(t, f.sent(), lootStackLine(1, name))
	if !bytes.Equal(innerMine, clientMessagePayload(lootStackLine(1, name))) || len(mine) != 1 || on(t, mine[0], innerMine) != name {
		t.Fatalf("the looter's linked line: %+v", mine)
	}
	if mine[0].id != spans[0].id {
		t.Fatalf("one take, two links: %d and %d", mine[0].id, spans[0].id)
	}
	l, ok := f.h.links.get(spans[0].id)
	if !ok || l.rec.Pattern.ItemName != name || l.rec.StackLeft != 1 || l.rec.Location != -1 || l.rec.EquipLoc != -1 || l.row.owner != 0 {
		t.Fatalf("the link: %+v %v", l, ok)
	}

	// the control: no one takes links, no link is made, every line as it was
	g := newCorpseFixture(t, 1)
	g.loadLists()
	g.h.links = NewItemLinks()
	other := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 79), Port: 7079}, 79, 1, 1, 2, true)
	g.h.groupOf = func(*entity.Player) []*session.Session { return []*session.Session{other} }
	g.h.OpenCorpse(g.sess, g.npc)
	g.sent()
	g.send(protocol.OpcodeLootTakeAll, nil)
	if !plainHas(t, g.sent(), clientMessagePayload(lootStackLine(1, name))) ||
		!plainHas(t, pendingOf(other), clientMessagePayload(groupLootLine(g.player.Name, 1, name))) {
		t.Fatal("the plain lines")
	}
	if n := g.h.links.Len(); n != 0 {
		t.Fatalf("%d links made for clients that take none", n)
	}
}

// corpseItemName is the name of the fixture's corpse's first record.
func (f *corpseFixture) corpseItemName(t *testing.T) string {
	t.Helper()
	ls := f.h.loots()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	c := ls.corpses[f.npc.ServerID]
	if c == nil || len(c.items) == 0 {
		t.Fatal("no corpse item")
	}
	return c.items[0].Pattern.ItemName
}

// A raid's need before greed: the announcement, each roll and the win go as links to the
// member that takes them and as the old lines, byte for byte, to a PS2 member; a roll
// offered by line (no windows) too; the item's link is one link throughout.
func TestRaidRollLinesLinkTheItem(t *testing.T) {
	r := newRaidLootRig(t, 3)
	r.f.h.links = NewItemLinks()
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Ring of Vox", 1, 7, 1, 0, 0, 1)
	r.sess[1].SetItemLinks(true)
	r.kill(entity.LootNeedGreed)
	sword := r.rollOf("Fine Steel Sword")
	rgb := func(text string) []byte { return coloredChatPayload(text, raidR, raidG, raidB) }

	ann := "Need before greed on Lady Vox's loot: Fine Steel Sword, Ring of Vox. Need, Greed or Pass within 120 seconds."
	vMsgs, pMsgs := r.msgs(1), r.msgs(2)
	if !plainHas(t, pMsgs, rgb(ann)) {
		t.Fatalf("the PS2's announcement: %q", raidLinesOf(pMsgs))
	}
	spans, inner := linkedOnly(t, vMsgs, "Need before greed")
	if !bytes.Equal(inner, rgb(ann)) || len(spans) != 2 || on(t, spans[0], inner) != "Fine Steel Sword" || on(t, spans[1], inner) != "Ring of Vox" {
		t.Fatalf("the announcement's links: %+v", spans)
	}
	swordLink := spans[0].id
	// the roll offered by line, to a client without windows
	offer := fmt.Sprintf("Roll %d: Fine Steel Sword. /t need %d, /t greed %d or /t pass %d within 120 seconds.", sword.id, sword.id, sword.id, sword.id)
	if offer != rollOfferLine(sword, sword.seats[1]) {
		t.Fatalf("the offer's words: %q", rollOfferLine(sword, sword.seats[1]))
	}
	if !plainHas(t, pMsgs, rgb(fmt.Sprintf("Roll %d: Fine Steel Sword. /t need %d, /t greed %d or /t pass %d within 120 seconds.", sword.id, sword.id, sword.id, sword.id))) {
		t.Fatal("the PS2's offer")
	}
	if sp, in := linkedOnly(t, vMsgs, fmt.Sprintf("Roll %d: Fine Steel Sword.", sword.id)); !bytes.Equal(in, rgb(offer)) || sp[0].id != swordLink {
		t.Fatalf("the offer's link: %+v", sp)
	}

	r.dice = []int{40, 7}
	r.f.h.RaidRollChoice(r.sess[0], sword.id, rollNeed)
	r.f.h.RaidRollChoice(r.sess[1], sword.id, rollGreed)
	r.f.h.RaidRollChoice(r.sess[2], sword.id, rollPass)
	vMsgs, pMsgs = r.msgs(1), r.msgs(2)
	for _, line := range []string{"Ann rolls Need on Fine Steel Sword: 40.", "Rb rolls Greed on Fine Steel Sword: 7.", "Ann wins Fine Steel Sword with Need 40."} {
		if !plainHas(t, pMsgs, rgb(line)) {
			t.Fatalf("the PS2 was not sent %q: %q", line, raidLinesOf(pMsgs))
		}
		sp, in := linkedOnly(t, vMsgs, line)
		if !bytes.Equal(in, rgb(line)) || len(sp) != 1 || on(t, sp[0], in) != "Fine Steel Sword" || sp[0].id != swordLink {
			t.Fatalf("%q's link: %+v", line, sp)
		}
	}
	if !plainHas(t, pMsgs, rgb("You pass on Fine Steel Sword.")) {
		t.Fatal("the PS2's pass")
	}
	// the winner (no links) is told its take as ever
	if !plainHas(t, r.msgs(0), clientMessagePayload("You receive Fine Steel Sword from the corpse.")) {
		t.Fatal("the winner's take")
	}
}

// A master looter's lines: the list by number and an assign, the count outside the link.
func TestMasterLooterLinesLinkTheItem(t *testing.T) {
	r := newRaidLootRig(t, 2)
	r.f.h.links = NewItemLinks()
	r.item("Fine Steel Sword", 1, 12, 0, 5, 0, 1)
	r.item("Bone Chips", 20, 0, 0, 0, 0, 1)
	r.kill(entity.LootMaster)
	r.drain()
	r.sess[0].SetItemLinks(true)
	r.f.h.OpenCorpse(r.sess[0], r.f.npc)
	msgs := r.msgs(0)
	sp, in := linkedOnly(t, msgs, "1 Fine Steel Sword")
	text := utf16Text(t, in)
	if !strings.Contains(text, "1 Fine Steel Sword, 2 10 Bone Chips.") || len(sp) != 2 || on(t, sp[0], in) != "Fine Steel Sword" || on(t, sp[1], in) != "Bone Chips" {
		t.Fatalf("the list %q: %+v", text, sp)
	}
	r.f.h.MasterAssign(r.sess[0], 1, 0, "rb")
	line := "Ann assigns 10 Bone Chips to Rb."
	if !plainHas(t, r.msgs(1), coloredChatPayload(line, raidR, raidG, raidB)) {
		t.Fatal("the PS2's assign line")
	}
	sp, in = linkedOnly(t, r.msgs(0), line)
	if len(sp) != 1 || on(t, sp[0], in) != "Bone Chips" {
		t.Fatalf("the assign's link: %+v", sp)
	}
	if l, ok := r.f.h.links.get(sp[0].id); !ok || l.rec.StackLeft != 10 {
		t.Fatalf("the assigned stack's link: %+v", l.rec)
	}
}

// linkedf: the text is Sprintf's with the names in place, the spans in characters on
// them; with no store, no links and the same text.
func TestLinkedfPlacesTheNames(t *testing.T) {
	h := &NPCWindowHandler{links: NewItemLinks()}
	rec := func(name string) database.CharInventoryEntry {
		return database.CharInventoryEntry{StackLeft: 1, Pattern: database.ItemPatternRecord{PatternID: 7, ItemName: name}}
	}
	l := h.linkedf("Ann", "%s gets %s and %s (%d).", "Ann", lootItem(rec("Épée"), 0), linkedItems{[]linkedItem{lootItem(rec("A"), 0), lootItem(rec("B c"), 0)}, ", "}, 3)
	if l.text != "Ann gets Épée and A, B c (3)." {
		t.Fatalf("%q", l.text)
	}
	var got []string
	for _, k := range l.line.links {
		got = append(got, string([]rune(l.text)[k.at:k.at+k.n]))
	}
	if strings.Join(got, "|") != "Épée|A|B c" {
		t.Fatalf("spans %q", got)
	}
	if p := (&NPCWindowHandler{}).linkedf("x", "%s.", lootItem(rec("Z"), 0)); p.text != "Z." || len(p.line.links) != 0 {
		t.Fatalf("with no store: %+v", p)
	}
}
