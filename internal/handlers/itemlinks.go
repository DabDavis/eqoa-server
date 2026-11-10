package handlers

import (
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Item links in chat, EQ1's (the GM Jibrish's petition 1463, 2026-11-10): a
// player puts a link to one of its own items in a chat line, and whoever clicks
// it sees the item, its figures as the linker held it then, in a small window.
//
// The server offers it at every world entry (protocol.OpcodeItemLinksOffer, a
// u8 1; a PS2 ignores it) and a client that takes links answers 1. Such a
// client sends a typed line holding a link as OpcodeItemLinkSay in place of the
// retail message: the line's text as typed (each link's item name in its
// place, so the text alone is the line a client without links would have
// sent), the way it goes (as 0x000B, as a shout, as the guild's) and each link's
// place in the text and the record it names, by its list (the pack, the bank)
// and the key the client files it under. A link is made only of a record its
// sender holds now, read as every record a client is sent is read (the heirloom
// view on it), and kept as it was then: what happens to the item after does not
// change the link (a snapshot: ItemLinks). Its span of the line becomes the
// server's word for the item's name, never the client's.
//
// The line then goes as any typed line goes, its links along with it: a command
// (#, /bug, a tell's /t name, a Summoning Portal's name) takes its text alone
// and the links are dropped; what is said keeps them through every relay of
// chat.go and the guild handler (say, shout, group, raid, guild, tell and its
// echo, whisper). The text between the links is cleaned as every relayed line
// is (consoletext.go), the line cut at chatLineMost with the names counted, a
// link the cut goes through plain text. Each hearer whose client took links is
// sent the retail message it would have been sent, whole, wrapped in
// OpcodeItemLinkLine with the links' places in it as it shows it (after the
// speaker's words and its own profanity mask); every other hearer, a PS2 among
// them, the retail message alone, byte for byte as a line typed without links.
// A click asks for the link by its id (OpcodeItemLinkAsk) and is answered with
// the record (OpcodeItemLinkRecord), anyone's, while it lives: an hour from when
// it was last linked, at most itemLinkMost of them, the oldest going first.

const (
	// itemLinksLayout is the layout of OpcodeItemLinkLine and OpcodeItemLinkRecord
	// the offer names.
	itemLinksLayout = 1
	// itemLinksPerLine is the most links in one line: those past it are its
	// plain text.
	itemLinksPerLine = 4
	// itemLinkTTL is how long a link lives after it was made or last linked
	// again; itemLinkMost the most kept, the oldest dropped first.
	itemLinkTTL  = time.Hour
	itemLinkMost = 10000
	// itemLinkAsksMost asks in itemLinkAskWindow a session, the rest dropped.
	itemLinkAsksMost  = 30
	itemLinkAskWindow = 10 * time.Second
	// wideLineMost is the most UTF-16 units of a line 0x0A7A, 0x0A7B and
	// 0x069E carry (writeWideString's 2048 bytes, less the end), and
	// guildLineMost 0x0468 sub 5's (512 bytes): a link must end within them.
	wideLineMost  = 1023
	guildLineMost = 255
)

// OpcodeItemLinkSay's routes: as 0x000B, as 0x0C01 sub 2, as 0x0578 sub 12.
const (
	itemLinkRouteSay   byte = 0
	itemLinkRouteShout byte = 1
	itemLinkRouteGuild byte = 2
)

// --- the links -------------------------------------------------------------------

// ItemLinks is the links made, each its item as its linker held it then, by id.
type ItemLinks struct {
	mu    sync.Mutex
	byID  map[uint32]*list.Element // of *itemLink
	order *list.List               // oldest first: made, or linked again
	rows  map[itemLinkRow][]*itemLink
	next  uint32 // the last id given
	now   func() time.Time

	asks      map[*session.Session]*itemLinkAsks
	asksSwept time.Time
}

// itemLinkRow is a character's item row: what a link is of.
type itemLinkRow struct {
	owner  uint32 // the character (CharServerID)
	itemID int    // charinventory.itemid
}

// itemLink is one link: its item, its name in the line, its linker.
type itemLink struct {
	id   uint32
	row  itemLinkRow
	who  string // the linker's name, for the log
	name string
	rec  database.CharInventoryEntry // wireRecord's, with pointers of its own
	at   time.Time                   // made, or last linked again
}

// itemLinkAsks is one session's asks in its window.
type itemLinkAsks struct {
	from    time.Time
	n       int
	dropped bool
}

// NewItemLinks makes an empty store, its ids counted on from a random number.
func NewItemLinks() *ItemLinks {
	return &ItemLinks{
		byID:  map[uint32]*list.Element{},
		order: list.New(),
		rows:  map[itemLinkRow][]*itemLink{},
		next:  rand.Uint32(),
		now:   time.Now,
		asks:  map[*session.Session]*itemLinkAsks{},
	}
}

// expired is whether a link last linked at lives no more at now.
func expired(at, now time.Time) bool { return now.Sub(at) > itemLinkTTL }

// make is the link of an item row as rec and name are now: the one there is,
// its time refreshed, when the same character linked the same row as the same
// record within its life (so a link said over and over costs nothing), else a
// new one; made is whether it is new. The expired go first, and the oldest when
// there are itemLinkMost.
func (s *ItemLinks) make(owner uint32, who string, itemID int, rec database.CharInventoryEntry, name string) (id uint32, made bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.sweepLocked(now)
	row := itemLinkRow{owner, itemID}
	for _, l := range s.rows[row] {
		if l.name == name && reflect.DeepEqual(l.rec, rec) {
			l.at = now
			s.order.MoveToBack(s.byID[l.id])
			return l.id, false
		}
	}
	for len(s.byID) >= itemLinkMost {
		s.dropLocked(s.order.Front())
	}
	l := &itemLink{id: s.nextIDLocked(), row: row, who: who, name: name, rec: rec, at: now}
	s.addLocked(l)
	return l.id, true
}

// nextIDLocked is the next id: never 0, never one in use.
func (s *ItemLinks) nextIDLocked() uint32 {
	for {
		s.next++
		if _, used := s.byID[s.next]; s.next != 0 && !used {
			return s.next
		}
	}
}

func (s *ItemLinks) addLocked(l *itemLink) {
	s.byID[l.id] = s.order.PushBack(l)
	s.rows[l.row] = append(s.rows[l.row], l)
}

// sweepLocked drops the links past their life, the oldest first.
func (s *ItemLinks) sweepLocked(now time.Time) {
	for e := s.order.Front(); e != nil && expired(e.Value.(*itemLink).at, now); e = s.order.Front() {
		s.dropLocked(e)
	}
}

func (s *ItemLinks) dropLocked(e *list.Element) {
	if e == nil {
		return
	}
	l := s.order.Remove(e).(*itemLink)
	delete(s.byID, l.id)
	of := s.rows[l.row]
	for i, x := range of {
		if x == l {
			of = append(of[:i], of[i+1:]...)
			break
		}
	}
	if len(of) == 0 {
		delete(s.rows, l.row)
	} else {
		s.rows[l.row] = of
	}
}

// get is the link of id while it lives.
func (s *ItemLinks) get(id uint32) (itemLink, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.byID[id]
	if e == nil {
		return itemLink{}, false
	}
	l := e.Value.(*itemLink)
	if expired(l.at, s.now()) {
		s.dropLocked(e)
		return itemLink{}, false
	}
	return *l, true
}

// asked counts one of sess's asks: false past itemLinkAsksMost in its window,
// first true for the first one dropped in a window (the one logged). The
// windows ended long ago are swept now and then.
func (s *ItemLinks) asked(sess *session.Session) (ok, first bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Sub(s.asksSwept) > itemLinkAskWindow {
		for k, a := range s.asks {
			if now.Sub(a.from) > itemLinkAskWindow {
				delete(s.asks, k)
			}
		}
		s.asksSwept = now
	}
	a := s.asks[sess]
	if a == nil || now.Sub(a.from) >= itemLinkAskWindow {
		a = &itemLinkAsks{from: now}
		s.asks[sess] = a
	}
	if a.n >= itemLinkAsksMost {
		first = !a.dropped
		a.dropped = true
		return false, first
	}
	a.n++
	return true, false
}

// Len is how many links live now.
func (s *ItemLinks) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(s.now())
	return len(s.byID)
}

// --- a keep restart ----------------------------------------------------------------

// keepLinks is the store RegisterDefaultHandlers made, the one a keep restart
// writes down and fills again.
var keepLinks atomic.Pointer[ItemLinks]

// KeptItemLink is a link as a restart carries it, so a link in a player's chat
// still opens after it.
type KeptItemLink struct {
	ID     uint32                      `json:"id"`
	Owner  uint32                      `json:"owner"`
	Item   int                         `json:"item"`
	Who    string                      `json:"who,omitempty"`
	Name   string                      `json:"name"`
	Record database.CharInventoryEntry `json:"record"`
	At     time.Time                   `json:"at"`
}

// keep is every link that lives, the oldest first.
func (s *ItemLinks) keep() []KeptItemLink {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked(s.now())
	out := make([]KeptItemLink, 0, len(s.byID))
	for e := s.order.Front(); e != nil; e = e.Next() {
		l := e.Value.(*itemLink)
		out = append(out, KeptItemLink{ID: l.id, Owner: l.row.owner, Item: l.row.itemID, Who: l.who, Name: l.name, Record: l.rec, At: l.at})
	}
	return out
}

// restore puts kept links back as they were, but those past their life, and
// counts the ids on after the largest kept; it reports how many it put back.
func (s *ItemLinks) restore(kept []KeptItemLink) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	ks := append([]KeptItemLink(nil), kept...)
	sort.SliceStable(ks, func(i, j int) bool { return ks[i].At.Before(ks[j].At) })
	var largest uint32
	n := 0
	for _, k := range ks {
		largest = max(largest, k.ID)
		if _, used := s.byID[k.ID]; k.ID == 0 || used || k.Name == "" || expired(k.At, now) {
			continue
		}
		s.addLocked(&itemLink{id: k.ID, row: itemLinkRow{k.Owner, k.Item}, who: k.Who, name: k.Name, rec: cloneEntry(k.Record), at: k.At})
		n++
	}
	for len(s.byID) > itemLinkMost {
		s.dropLocked(s.order.Front())
		n--
	}
	if largest != 0 {
		s.next = largest
	}
	return n
}

// KeepItemLinks is what a keep restart writes of the links (KeptItemLink).
func KeepItemLinks() []KeptItemLink {
	if s := keepLinks.Load(); s != nil {
		return s.keep()
	}
	return nil
}

// RestoreItemLinks puts the links a keep restart wrote back, and reports how
// many: none past their life. A file without them leaves the store as it is.
func RestoreItemLinks(kept []KeptItemLink) int {
	if s := keepLinks.Load(); s != nil && len(kept) > 0 {
		return s.restore(kept)
	}
	return 0
}

// cloneEntry is e with pointers of its own (the record's nullable figures), so
// nothing done to the row it was read from reaches it.
func cloneEntry(e database.CharInventoryEntry) database.CharInventoryEntry {
	v := reflect.ValueOf(&e.Pattern).Elem()
	for i := 0; i < v.NumField(); i++ {
		if f := v.Field(i); f.Kind() == reflect.Pointer && !f.IsNil() {
			c := reflect.New(f.Type().Elem())
			c.Elem().Set(f.Elem())
			f.Set(c)
		}
	}
	return e
}

// --- a line with links ------------------------------------------------------------

// chatText is a chat line on its way, with the links in it: none for a line
// typed without (every PS2's), and only a line said carries them (a command
// takes the text alone).
type chatText struct {
	text  string
	links []textLink // ascending, apart
}

// plainText is a line without links.
func plainText(s string) chatText { return chatText{text: s} }

// textLink is one link of a chatText: its item's name, in runes of the text.
type textLink struct {
	at, n int
	id    uint32      // its link, once made (made)
	src   *linkSource // what makes it
}

// linkSource is an item a line links, read as its linker holds it: what its
// link is made of (ItemLinks.make), when the line is said.
type linkSource struct {
	links  *ItemLinks
	owner  uint32
	who    string
	itemID int
	rec    database.CharInventoryEntry
	name   string
}

// make is the source's link (ItemLinks.make), logged once when it is new.
func (src *linkSource) make() uint32 {
	if src == nil || src.links == nil {
		return 0
	}
	id, made := src.links.make(src.owner, src.who, src.itemID, src.rec, src.name)
	if made {
		log.Printf("ItemLinks: %s linked %q (item %d, link %d)", src.who, src.name, src.itemID, id)
	}
	return id
}

// clean is the line as it is relayed (consoleLine's, as every relayed line
// is): the text between the links cleaned, the links' names (the server's,
// clean already) as they are, the whole cut at most characters. A link the cut
// goes through is no link: what is left of its name is plain text. With no
// links it is consoleLine(text, most), and with them it is too, the names in
// their places (a name is never empty and holds no line end).
func (c chatText) clean(most int) chatText {
	if len(c.links) == 0 {
		return plainText(consoleLine(c.text, most))
	}
	runes := []rune(c.text)
	var b strings.Builder
	n, pos := 0, 0
	out := chatText{}
	plain := func(s string) {
		s = consoleLine(s, len(s)) // cleaned; never cut, its characters no more than its bytes
		b.WriteString(s)
		n += utf8.RuneCountInString(s)
	}
	for _, l := range c.links {
		plain(string(runes[pos:l.at]))
		out.links = append(out.links, textLink{at: n, n: l.n, id: l.id, src: l.src})
		b.WriteString(string(runes[l.at : l.at+l.n]))
		n += l.n
		pos = l.at + l.n
	}
	plain(string(runes[pos:]))
	text := []rune(b.String())
	if len(text) > most {
		text = text[:most]
	}
	out.text = string(text)
	kept := out.links[:0]
	for _, l := range out.links {
		if l.at+l.n <= len(text) {
			kept = append(kept, l)
		}
	}
	out.links = kept
	return out
}

// wrap is the line inside a relay's words: prefix before it, suffix after,
// the links moved along.
func (c chatText) wrap(prefix, suffix string) chatText {
	k := utf8.RuneCountInString(prefix)
	out := chatText{text: prefix + c.text + suffix}
	for _, l := range c.links {
		l.at += k
		out.links = append(out.links, l)
	}
	return out
}

// tail is the line from where s, a suffix of its text (a tell's body:
// parseTell's), starts: the links in it moved to it, any before it dropped.
func (c chatText) tail(s string) chatText {
	if len(c.links) == 0 || !strings.HasSuffix(c.text, s) {
		return plainText(s)
	}
	k := utf8.RuneCountInString(c.text[:len(c.text)-len(s)])
	out := plainText(s)
	for _, l := range c.links {
		if l.at >= k {
			l.at -= k
			out.links = append(out.links, l)
		}
	}
	return out
}

// made is the line with each of its links made (linkSource.make): a link that
// could not be is dropped. A relay makes them once it says the line, so a line
// no one is told (a command, a silenced speaker's, one cut away) makes none.
func (c chatText) made() chatText {
	if len(c.links) == 0 {
		return c
	}
	out := chatText{text: c.text}
	for _, l := range c.links {
		if l.id == 0 {
			l.id = l.src.make()
		}
		if l.id != 0 {
			out.links = append(out.links, l)
		}
	}
	return out
}

// wireSpan is a link as OpcodeItemLinkLine names it: UTF-16 units into the
// inner message's text, and its id.
type wireSpan struct {
	at, n int
	id    uint32
}

// wireSpans are the line's links in shown, its text as one hearer is shown it
// (lineFor's: as many characters, each where the text's is), each ending
// within most units (what the inner message carries of its text).
func (c chatText) wireSpans(shown string, most int) []wireSpan {
	if len(c.links) == 0 {
		return nil
	}
	units := make([]int, 0, len(shown)+1) // the units before each character
	u := 0
	for _, r := range shown {
		units = append(units, u)
		if r >= 0x10000 {
			u += 2
		} else {
			u++
		}
	}
	units = append(units, u)
	if len(units)-1 != utf8.RuneCountInString(c.text) {
		return nil
	}
	var out []wireSpan
	for _, l := range c.links {
		if l.id == 0 || l.n < 1 || l.at < 0 || l.at+l.n >= len(units) {
			continue
		}
		at, end := units[l.at], units[l.at+l.n]
		if end > most || len(out) == itemLinksPerLine {
			continue
		}
		out = append(out, wireSpan{at, end - at, l.id})
	}
	return out
}

// relayTo queues one hearer's line: payload is the retail message (its opcode
// first) a client without links is sent, of shown, line's text as this hearer
// is shown it. A client that took links is sent the same wrapped in
// OpcodeItemLinkLine with them, while line holds any that land in it.
func relayTo(sess *session.Session, op protocol.GameOpcode, payload []byte, line chatText, shown string, most int) {
	if len(line.links) > 0 && sess.ItemLinks() {
		if spans := line.wireSpans(shown, most); len(spans) > 0 {
			queueReliable(sess, protocol.OpcodeItemLinkLine, buildItemLinkLine(spans, payload))
			return
		}
	}
	queueReliable(sess, op, payload)
}

// --- on the wire -----------------------------------------------------------------

// buildItemLinksOffer is OpcodeItemLinksOffer from the server: a u8, the layout.
func buildItemLinksOffer() []byte {
	return build(protocol.OpcodeItemLinksOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(itemLinksLayout)
	})
}

// SendItemLinksOffer offers the session's client links in chat, at every
// world entry: a client that takes them answers, a PS2 ignores it.
func SendItemLinksOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeItemLinksOffer, buildItemLinksOffer())
}

// buildItemLinkLine is OpcodeItemLinkLine: the links (at most
// itemLinksPerLine), then inner whole, its opcode first.
func buildItemLinkLine(spans []wireSpan, inner []byte) []byte {
	if len(spans) > itemLinksPerLine {
		spans = spans[:itemLinksPerLine]
	}
	return build(protocol.OpcodeItemLinkLine, 2+1+8*len(spans)+len(inner), func(w *buffer.Writer) {
		w.WriteByte(byte(len(spans)))
		for _, s := range spans {
			w.WriteUint16(uint16(s.at))
			w.WriteUint16(uint16(s.n))
			w.WriteUint32(s.id)
		}
		w.WriteBytes(inner)
	})
}

// buildItemLinkRecord is OpcodeItemLinkRecord for a link that lives: its id,
// 1, and the item as 0x003B's body, keyed by the id (rec is wireRecord's: worn
// nowhere, on no quick slot).
func buildItemLinkRecord(id uint32, rec *database.CharInventoryEntry, capes bool) []byte {
	p := &rec.Pattern
	size := 2 + 4 + 1 + 1024 + 4*(len(p.ItemName)+len(database.ImportedTag)+len(p.ItemDesc))
	return build(protocol.OpcodeItemLinkRecord, size, func(w *buffer.Writer) {
		w.WriteUint32(id)
		w.WriteByte(1)
		writeSingleItem(w, rec, int(id), capes)
	})
}

// buildItemLinkGone is OpcodeItemLinkRecord for no such link: its id, 0.
func buildItemLinkGone(id uint32) []byte {
	return build(protocol.OpcodeItemLinkRecord, 2+4+1, func(w *buffer.Writer) {
		w.WriteUint32(id)
		w.WriteByte(0)
	})
}

// itemLinkSay is an OpcodeItemLinkSay as read.
type itemLinkSay struct {
	route  byte
	target uint32
	units  []uint16
	links  []askedLink
}

// askedLink is one link as the client sent it.
type askedLink struct {
	start, length int // UTF-16 units into the text
	list          byte
	key           uint32
}

// readItemLinkSay reads an OpcodeItemLinkSay's body (Data(): after the
// opcode). Anything after the links is a later client's and is left aside.
func readItemLinkSay(data []byte) (itemLinkSay, error) {
	r := &clientReader{b: data}
	var s itemLinkSay
	s.route = r.u8()
	s.target = r.u32()
	n := int(r.u32())
	if r.err {
		return s, errors.New("cut short")
	}
	if s.route > itemLinkRouteGuild {
		return s, fmt.Errorf("route %d", s.route)
	}
	if n < 1 || n > 1023 {
		return s, fmt.Errorf("a text of %d units", n)
	}
	b := r.take(2 * n)
	count := int(r.u8())
	if r.err {
		return s, errors.New("cut short")
	}
	s.units = make([]uint16, n)
	for i := range s.units {
		s.units[i] = binary.LittleEndian.Uint16(b[2*i:])
	}
	for range count {
		v := r.take(9)
		if v == nil {
			return s, errors.New("its links cut short")
		}
		s.links = append(s.links, askedLink{
			start:  int(binary.LittleEndian.Uint16(v)),
			length: int(binary.LittleEndian.Uint16(v[2:])),
			list:   v[4],
			key:    binary.LittleEndian.Uint32(v[5:]),
		})
	}
	return s, nil
}

// --- the handlers ----------------------------------------------------------------

// itemLinkRows reads one of a character's item rows as its client is sent it
// (database.InventoryStore: the heirloom view on it).
type itemLinkRows interface {
	GetCharItem(serverID uint32, itemID int) (database.CharInventoryEntry, error)
}

// HandleItemLinksAnswer is OpcodeItemLinksOffer from a client: a u8, nonzero
// to be sent lines with links, 0 for the retail lines (a wider answer is read
// by its first byte). Nothing is answered.
func (h *ChatHandler) HandleItemLinksAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		log.Printf("ItemLinks: short answer (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := data[0] != 0
	sess.SetItemLinks(on)
	log.Printf("ItemLinks: %s %s", sess.RemoteAddr(), map[bool]string{true: "takes links", false: "takes plain lines"}[on])
	return nil
}

// HandleItemLinkSay is OpcodeItemLinkSay: a typed line with its links, routed
// as the retail message it stands for would be (route 0 as 0x000B, 1 as a
// shout, 2 as the guild's), each link that resolves to a record the sender
// holds now a link and the rest the text the client sent.
func (h *ChatHandler) HandleItemLinkSay(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	say, err := readItemLinkSay(msg.Data())
	if err != nil {
		log.Printf("ItemLinks: a line from %s dropped: %v", sess.RemoteAddr(), err)
		return nil
	}
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		log.Printf("ItemLinks: a line from %s, which has no player, dropped", sess.RemoteAddr())
		return nil
	}
	units := say.units
	if say.route == itemLinkRouteGuild {
		// as the guild's 0x0578 sub 12 reads its text (clientReader.str16)
		for i, c := range units {
			if c == 0 {
				units = units[:i]
				break
			}
		}
	}
	line := h.linkedLine(sess, player, units, say.links)
	switch say.route {
	case itemLinkRouteSay:
		h.typedLine(sess, player, line, say.target)
	case itemLinkRouteShout:
		h.shoutLine(sess, player, line)
	case itemLinkRouteGuild:
		if h.guildChat != nil {
			h.guildChat(sess, player, line)
		}
	}
	return nil
}

// linkedLine is a line's text with its links: each asked link, in the order
// sent, is one when its span lies in the text (not splitting a character),
// after the last one taken, and names a record the sender holds now
// (linkSourceOf), at most itemLinksPerLine; its span then becomes the record's
// name, the server's word. The text of every other is as the client sent it.
func (h *ChatHandler) linkedLine(sess *session.Session, player *entity.Player, units []uint16, asked []askedLink) chatText {
	type taken struct {
		start, end int
		src        *linkSource
	}
	var links []taken
	last := 0
	for _, a := range asked {
		if len(links) == itemLinksPerLine {
			break
		}
		start, end := a.start, a.start+a.length
		if a.length < 1 || start < last || end > len(units) || splitsPair(units, start) || splitsPair(units, end) {
			continue
		}
		if src := h.linkSourceOf(sess, player, a.list, a.key); src != nil {
			links = append(links, taken{start, end, src})
			last = end
		}
	}
	if len(links) < len(asked) {
		log.Printf("ItemLinks: %s's line: %d of %d links are plain text", player.Name, len(asked)-len(links), len(asked))
	}
	if len(links) == 0 {
		return plainText(string(utf16.Decode(units)))
	}
	var b strings.Builder
	var out chatText
	n, pos := 0, 0
	for _, l := range links {
		plain := string(utf16.Decode(units[pos:l.start]))
		b.WriteString(plain)
		n += utf8.RuneCountInString(plain)
		k := utf8.RuneCountInString(l.src.name)
		out.links = append(out.links, textLink{at: n, n: k, src: l.src})
		b.WriteString(l.src.name)
		n += k
		pos = l.end
	}
	b.WriteString(string(utf16.Decode(units[pos:])))
	out.text = b.String()
	return out
}

// splitsPair is whether a span's edge at i falls between a character's two
// UTF-16 units.
func splitsPair(units []uint16, i int) bool {
	return i > 0 && i < len(units) && units[i-1] >= 0xD800 && units[i-1] < 0xDC00 && units[i] >= 0xDC00 && units[i] < 0xE000
}

// linkSourceOf is the record the sender's client files under key in its list
// (0 the pack, 1 the bank), resolved through the session's copy of that list
// as every request is (never as a position), and read as the client is sent it;
// nil when the list does not hold it, its row is not the sender's in that list
// now, or its name shows as nothing.
func (h *ChatHandler) linkSourceOf(sess *session.Session, player *entity.Player, which byte, key uint32) *linkSource {
	var l *invList
	switch which {
	case 0:
		l = sessionInvList(sess)
	case 1:
		l = sessionBankList(sess)
	}
	if l == nil || h.linkRows == nil || h.links == nil {
		return nil
	}
	rec, ok := l.at(l.index(key))
	if !ok {
		return nil
	}
	row, err := h.linkRows.GetCharItem(player.CharServerID, rec.itemID)
	if err != nil || (row.Location == database.LocBank) != (which == 1) {
		return nil
	}
	name := consoleLine(itemNameShown(&row.Pattern), itemNameMost)
	if name == "" {
		return nil
	}
	return &linkSource{links: h.links, owner: player.CharServerID, who: player.Name, itemID: row.ItemID,
		rec: cloneEntry(wireRecord(row)), name: name}
}

// HandleItemLinkAsk is OpcodeItemLinkAsk: a u32 link id, answered to a session
// in the world with OpcodeItemLinkRecord, the record for its own client's
// capes (readsCapes), or 0 for no such link. At most itemLinkAsksMost in
// itemLinkAskWindow a session; the rest are dropped, one line logged a window.
func (h *ChatHandler) HandleItemLinkAsk(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("ItemLinks: short ask (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if h.links == nil || !sess.CharacterInWorld() {
		return nil
	}
	id := binary.LittleEndian.Uint32(data)
	who := sess.RemoteAddr().String()
	if h.gameWorld != nil {
		if p := h.gameWorld.SessionPlayer(sess); p != nil {
			who = p.Name
		}
	}
	if ok, first := h.links.asked(sess); !ok {
		if first {
			log.Printf("ItemLinks: %s asked for more than %d links in %v: the rest dropped till then", who, itemLinkAsksMost, itemLinkAskWindow)
		}
		return nil
	}
	l, ok := h.links.get(id)
	if !ok {
		queueReliable(sess, protocol.OpcodeItemLinkRecord, buildItemLinkGone(id))
		log.Printf("ItemLinks: %s asked for link %d: no such link", who, id)
		return nil
	}
	queueReliable(sess, protocol.OpcodeItemLinkRecord, buildItemLinkRecord(id, &l.rec, readsCapes(sess)))
	log.Printf("ItemLinks: %s viewed link %d (%q, %s's)", who, id, l.name, l.who)
	return nil
}
