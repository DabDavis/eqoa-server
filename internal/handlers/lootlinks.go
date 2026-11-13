package handlers

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Loot lines with item links (the owner, 2026-11-11: "when loot is looted make the items
// as item links for players to click like we already implemented"). Every line the server
// says of loot taken or rolled on names its items as links (itemlinks.go) to a client that
// took links, and goes as it always went, byte for byte, to every other (a PS2, an older
// vanilla, Item Links off): the looter's "You receive", its group's "receives", the raid's
// need before greed, its rolls, ties, wins and claims, and the master looter's lines.
//
// A link of a loot line is a snapshot of the record as the corpse held it (wireRecord's,
// the count taken), kept as a player's link is (an hour from its last line, among the
// itemLinkMost), under no character (owner 0) and the item's pattern (its row -PatternID),
// so the same record looted again, by anyone, is the same link. It is made only when its
// line first goes to a client that takes links, so a line only PS2s hear makes none. Its
// clicks are the hearer's own asks, counted as every ask is (itemLinkAsksMost a window); a
// line makes no ask.

// linkedItem is an item a server line names, linked where links are taken (linkedf).
type linkedItem struct{ rec database.CharInventoryEntry }

// lootItem is rec, n of it, as a loot line names and links it.
func lootItem(rec database.CharInventoryEntry, n int) linkedItem {
	if n > 0 {
		rec.StackLeft = n
	}
	return linkedItem{rec}
}

// linkedItems are items a line names one after another, sep between (linkedf).
type linkedItems struct {
	items []linkedItem
	sep   string
}

// linkedCount is n of an item as lootStackName names it: "n name", the name linked.
type linkedCount struct {
	n    int
	item linkedItem
}

// serverLine is a line the server says, with the links of the items it names.
type serverLine struct {
	text string
	line chatText // its links not yet made until made is
	made bool
}

// plainLine is a server line naming no item.
func plainLine(text string) *serverLine { return &serverLine{text: text, line: plainText(text)} }

// placeholder marks a linked item's place in linkedf's formatted text: a NUL, its index
// as a private-use character, a NUL (no item name or player name holds a NUL).
const placeholderBase = 0xE000

// linkedf is fmt.Sprintf(format, args...) with each linkedItem (or linkedItems) arg in
// it as its item's name, a link to that record where the line goes to a client that takes
// links; who is the looter or the corpse a link is logged by.
func (h *NPCWindowHandler) linkedf(who, format string, args ...any) *serverLine {
	return linkedLine(h.links, who, format, args...)
}

// linkedLine is linkedf over a store of links (nil: the names plain).
func linkedLine(store *ItemLinks, who, format string, args ...any) *serverLine {
	var items []database.CharInventoryEntry
	mark := func(it linkedItem) string {
		items = append(items, it.rec)
		return "\x00" + string(rune(placeholderBase+len(items)-1)) + "\x00"
	}
	fa := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case linkedItem:
			fa[i] = mark(v)
		case linkedCount:
			fa[i] = fmt.Sprintf("%d %s", v.n, mark(v.item))
		case linkedItems:
			parts := make([]string, len(v.items))
			for j, it := range v.items {
				parts[j] = mark(it)
			}
			fa[i] = strings.Join(parts, v.sep)
		default:
			fa[i] = a
		}
	}
	runes := []rune(fmt.Sprintf(format, fa...))
	var b strings.Builder
	var links []textLink
	n := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == 0 && i+2 < len(runes) && runes[i+2] == 0 {
			if k := int(runes[i+1]) - placeholderBase; k >= 0 && k < len(items) {
				rec := items[k]
				name := rec.Pattern.ItemName
				size := utf8.RuneCountInString(name)
				if size > 0 && store != nil {
					links = append(links, textLink{at: n, n: size, src: &linkSource{links: store, owner: 0, who: who,
						itemID: -int(rec.Pattern.PatternID), rec: cloneEntry(wireRecord(rec)), name: name}})
				}
				b.WriteString(name)
				n += size
				i += 2
				continue
			}
		}
		b.WriteRune(runes[i])
		n++
	}
	text := b.String()
	return &serverLine{text: text, line: chatText{text: text, links: links}}
}

// relay sends the line to sess as op's payload of its text: with its links (made now, at
// the first client that takes them) to a client that takes links, else payload alone.
func (l *serverLine) relay(sess *session.Session, op protocol.GameOpcode, payload []byte) {
	if sess == nil {
		return
	}
	if !l.made && len(l.line.links) > 0 && sess.ItemLinks() {
		l.line = l.line.made()
		l.made = true
	}
	if !l.made {
		queueReliable(sess, op, payload)
		return
	}
	relayTo(sess, op, payload, l.line, l.text, wideLineMost)
}

// client sends the line as 0x0A7A (SendClientMessage's), white.
func (l *serverLine) client(sess *session.Session) {
	l.relay(sess, protocol.OpcodeClientMessage, clientMessagePayload(l.text))
}

// colored sends the line as 0x0A7B (SendColoredChat's) in r, g, b.
func (l *serverLine) colored(sess *session.Session, r, g, b uint8) {
	l.relay(sess, protocol.OpcodeColoredChat, coloredChatPayload(l.text, r, g, b))
}
