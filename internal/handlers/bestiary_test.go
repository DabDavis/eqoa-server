package handlers

import (
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
)

// readBestiaryData reads a 0x7EF5 from the server: its page, pages, kinds and rows.
func readBestiaryData(t *testing.T, b []byte) (page, pages, kinds int, rows []recordRow) {
	t.Helper()
	if protocol.GameOpcode(binary.LittleEndian.Uint16(b)) != protocol.OpcodeBestiaryData || b[2] != 1 {
		t.Fatalf("opcode or version % X", b[:3])
	}
	page, pages, kinds = int(binary.LittleEndian.Uint16(b[3:])), int(binary.LittleEndian.Uint16(b[5:])), int(binary.LittleEndian.Uint16(b[7:]))
	p := 10
	str := func() string {
		n := int(b[p])
		s := string(b[p+1 : p+1+n])
		p += 1 + n
		return s
	}
	for i := 0; i < int(b[9]); i++ {
		style := b[p]
		p++
		rows = append(rows, recordRow{style, str(), str()})
	}
	if p != len(b) {
		t.Fatalf("%d bytes left over", len(b)-p)
	}
	return
}

// The pages: a zone's heading over its kinds, again at the top of a page its zone goes
// on into, never a heading alone at a page's foot, no page past bestiaryPageLines; a
// kind's row its kills and first day (the year when not this one), or "met" alone.
// No kinds: one empty page.
func TestBestiaryPages(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	first := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	var es []database.BestiaryEntry
	for i := 0; i < 14; i++ {
		es = append(es, database.BestiaryEntry{Kind: fmt.Sprintf("gnoll %02d", i), Zone: "Blackburrow", FirstSeen: first, Kills: i})
	}
	es = append(es, database.BestiaryEntry{Kind: "orc pawn", Zone: "Crushbone", FirstSeen: first.AddDate(-1, 0, 0), Kills: 3})
	for i := 0; i < 3; i++ {
		es = append(es, database.BestiaryEntry{Kind: fmt.Sprintf("bat %d", i), Zone: "Freeport", FirstSeen: first})
	}
	pages := bestiaryPages(es, now)
	if len(pages) != 2 {
		t.Fatalf("%d pages: %+v", len(pages), pages)
	}
	p0, p1 := pages[0], pages[1]
	if len(p0) != 14 || p0[0] != (recordRow{recordHeading, "Blackburrow", ""}) || p0[13].style != recordLine {
		t.Errorf("page 1: %+v", p0)
	}
	if p0[1] != (recordRow{recordLine, "gnoll 00", "met Nov 3"}) || p0[2] != (recordRow{recordLine, "gnoll 01", "1 killed, first Nov 3"}) {
		t.Errorf("page 1's rows: %+v %+v", p0[1], p0[2])
	}
	if p1[0] != (recordRow{recordHeading, "Blackburrow", ""}) || p1[1].label != "gnoll 13" ||
		p1[2] != (recordRow{recordHeading, "Crushbone", ""}) || p1[3] != (recordRow{recordLine, "orc pawn", "3 killed, first Nov 3, 2025"}) ||
		p1[4].label != "Freeport" || len(p1) != 8 {
		t.Errorf("page 2: %+v", p1)
	}
	if empty := bestiaryPages(nil, now); len(empty) != 1 || len(empty[0]) != 0 {
		t.Errorf("no kinds: %+v", empty)
	}
}

// The handler: the world's notes written; the offer made by a server that keeps a
// bestiary, to a session with the QoL additions on, never to an [OG] character's or on
// world.vanilla_ui; a page asked is sent (a page past the last is the last), a second
// ask at once is not read again; an [OG] character's ask is answered with nothing.
func TestBestiaryOfferAndPages(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/bestiary.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); bestiaries.Store(nil) })
	c := newChatWorld(t)
	h := NewBestiaryHandler(c.gw, db)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	r := NewRegistry()
	h.RegisterHandlers(r)
	s, p := c.join(1, "Alice", 100, 100, 0)
	var notes []database.BestiaryNote
	for i := 0; i < 20; i++ {
		notes = append(notes, database.BestiaryNote{ServerID: p.CharServerID, Kind: fmt.Sprintf("rat %02d", i), Zone: "Freeport", At: now, Kill: i%2 == 0})
	}
	h.write(notes)

	SendBestiaryOffer(s)
	if offers := queued(s, protocol.OpcodeBestiary); len(offers) != 1 || binary.LittleEndian.Uint32(offers[0][2:]) != 1 {
		t.Fatalf("the offer % X", offers)
	}
	s.Queue().GetPending()
	ask := func(page uint16) {
		t.Helper()
		if err := r.Handle(s, incoming(protocol.OpcodeBestiaryData, binary.LittleEndian.AppendUint16(nil, page))); err != nil {
			t.Fatal(err)
		}
	}
	ask(9)
	ask(0) // at once: not answered
	data := queued(s, protocol.OpcodeBestiaryData)
	if len(data) != 1 {
		t.Fatalf("%d pages sent", len(data))
	}
	page, pages, kinds, rows := readBestiaryData(t, data[0])
	if page != 1 || pages != 2 || kinds != 20 || rows[0].label != "Freeport" || rows[1] != (recordRow{recordLine, "rat 13", "met Nov 26"}) {
		t.Fatalf("page %d/%d kinds %d rows %+v", page, pages, kinds, rows)
	}
	s.Queue().GetPending()
	now = now.Add(time.Second)
	ask(0)
	if data := queued(s, protocol.OpcodeBestiaryData); len(data) != 1 {
		t.Fatal("the first page not sent")
	} else if _, _, _, rows := readBestiaryData(t, data[0]); rows[1] != (recordRow{recordLine, "rat 00", "1 killed, first Nov 26"}) {
		t.Fatalf("page 1: %+v", rows)
	}
	s.Queue().GetPending()

	// an [OG] character: no offer, no page
	s.SetOG(true)
	now = now.Add(time.Second)
	SendBestiaryOffer(s)
	ask(0)
	if got := queued(s, protocol.OpcodeBestiary); len(got) != 0 {
		t.Error("an [OG] character was offered the bestiary")
	}
	if got := queued(s, protocol.OpcodeBestiaryData); len(got) != 0 {
		t.Error("an [OG] character was sent a page")
	}
	s.SetOG(false)
	SetVanillaUI(true)
	SendBestiaryOffer(s)
	SetVanillaUI(false)
	if got := queued(s, protocol.OpcodeBestiary); len(got) != 0 {
		t.Error("offered on world.vanilla_ui")
	}
	// no database: no offer
	bestiaries.Store(nil)
	SendBestiaryOffer(s)
	if got := queued(s, protocol.OpcodeBestiary); len(got) != 0 {
		t.Error("offered with no database")
	}
}

// The zones: a camp's cell names its zone (the map's names), a place in a cell with no
// camp the nearest camp's within reach, and a place far from any the memorial's
// words; a server world's place its own name.
func TestBestiaryZoneNames(t *testing.T) {
	var camp mapCamp
	for _, c := range mapCamps {
		if c.World == 0 && c.Zone == "North Kithicor" {
			camp = c
			break
		}
	}
	if camp.Zone == "" {
		t.Skip("no North Kithicor camp in the map's camps")
	}
	if got := bestiaryZone(0, camp.X, camp.Z); got != "North Kithicor" {
		t.Errorf("a camp's spot: %q", got)
	}
	if got := bestiaryZone(0, -50000, -50000); got == "North Kithicor" || got == "" {
		t.Errorf("far from every camp: %q", got)
	}
	if got := bestiaryZone(3, -90000, -90000); got != fallPlace(3, -90000, 0, -90000) {
		t.Errorf("Lavastorm far from its camps: %q", got)
	}
}
