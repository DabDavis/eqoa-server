package handlers

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

var recordNow = time.Unix(1_800_000_000, 0)

// fullRecord is a record with something in every part.
func fullRecord() database.CombatRecord {
	return database.CombatRecord{PvPKills: 12, PvPDeaths: 3, Assists: 4, Streak: 2, BestStreak: 5,
		BestKill: "Bob", BestKillLevel: 45, LastKill: "Bob", LastKillAt: recordNow.Add(-2 * time.Hour),
		LastDeath: "Jim", LastDeathLevel: 50, LastDeathAt: recordNow.Add(-5 * time.Minute),
		DuelWins: 5, DuelLosses: 2, DuelSurrenders: 1, DuelDraws: 1, WagerWon: 300, WagerLost: 100}
}

// In chat: a line each for the PvP counts, the streak and best kill, the last kill and
// death, the duels and the wagers, each within the 48 characters a PS2's chat row holds
// (417 pixels); a long name moves a part to a line of its own rather than past the row.
// A part with nothing in it is no line; the PvP half only where world.pvp is on or the
// record has some; no record at all is one line.
func TestRecordChatLines(t *testing.T) {
	got := recordLines("Dabran", fullRecord(), true, recordNow)
	want := []string{
		"Dabran: 12 PvP kills, 3 deaths, 4 assists.",
		"Kill streak 2 (best 5). Best kill: Bob (45).",
		"Last kill: Bob, 2h ago. Killed by: Jim, 5m ago.",
		"Duels: 5 won, 2 lost (1 surrendered), 1 drawn.",
		"Wagers: 300 tunar won, 100 lost.",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("a full record:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	r := fullRecord()
	r.BestKill, r.LastKill, r.LastDeath = "Lastofthelon", "Lastofthelon", "Wwwwwwwwwwww"
	long := recordLines("Mmmmmmmmmmmm", r, true, recordNow)
	if len(long) <= len(want) {
		t.Fatalf("long names packed into %d lines: %q", len(long), long)
	}
	for _, l := range append(got, long...) {
		if len(l) > recordChatMost {
			t.Errorf("%d characters, past a PS2's row: %q", len(l), l)
		}
	}
	duels := database.CombatRecord{DuelWins: 1, DuelLosses: 0}
	if got := recordLines("Alpha", duels, false, recordNow); len(got) != 1 || got[0] != "Alpha's duels: 1 won, 0 lost." {
		t.Fatalf("duels alone, no PvP here: %q", got)
	}
	if got := recordLines("Alpha", duels, true, recordNow); len(got) != 2 || got[0] != "Alpha: 0 PvP kills, 0 deaths, 0 assists." {
		t.Fatalf("duels alone, PvP here: %q", got)
	}
	dead := database.CombatRecord{PvPDeaths: 1, LastDeath: "Jim", LastDeathAt: recordNow.Add(-30 * time.Second)}
	if got := recordLines("Alpha", dead, false, recordNow); len(got) != 2 || got[1] != "Killed by: Jim, just now." {
		t.Fatalf("a death on a server since turned off: %q", got)
	}
	if got := recordLines("Beta", database.CombatRecord{}, true, recordNow); len(got) != 1 || got[0] != "Beta has no combat record yet." {
		t.Fatalf("no record: %q", got)
	}
}

// The window's rows: the PvP half (a heading and seven rows) where world.pvp is on or the
// record has some, then the duels (a heading and five), in the server's words; and the
// message's bytes, read back as the client reads them.
func TestRecordWindowRows(t *testing.T) {
	rows := recordRows(fullRecord(), true, recordNow)
	if len(rows) != 14 || rows[0] != (recordRow{recordHeading, "PvP", ""}) || rows[8] != (recordRow{recordHeading, "Duels", ""}) {
		t.Fatalf("rows %+v", rows)
	}
	for _, c := range []recordRow{{recordLine, "Kill streak", "2 (best 5)"}, {recordLine, "Best kill", "Bob, level 45"},
		{recordLine, "Last kill", "Bob, 2 hours ago"}, {recordLine, "Last killed by", "Jim, 5 minutes ago"},
		{recordLine, "Lost", "2 (1 surrendered)"}, {recordLine, "Tunar won", "300"}} {
		found := false
		for _, r := range rows {
			found = found || r == c
		}
		if !found {
			t.Errorf("no row %+v", c)
		}
	}
	if rows := recordRows(database.CombatRecord{DuelWins: 1}, false, recordNow); len(rows) != 6 || rows[0].label != "Duels" {
		t.Fatalf("no PvP here: %+v", rows)
	}
	if rows := recordRows(database.CombatRecord{}, true, recordNow); len(rows) != 14 || rows[5] != (recordRow{recordLine, "Best kill", "none"}) {
		t.Fatalf("an empty record, PvP here: %+v", rows)
	}

	name, back := readRecordData(t, buildCombatRecordData("Dabran", rows))
	if name != "Dabran" || len(back) != len(rows) {
		t.Fatalf("read back %q, %d rows", name, len(back))
	}
	for i := range rows {
		if back[i] != rows[i] {
			t.Fatalf("row %d read back %+v, sent %+v", i, back[i], rows[i])
		}
	}
	if s := recordString8(nil, strings.Repeat("é", 200)); s[0] != 254 || len(s) != 255 {
		t.Fatalf("a long string cut to %d bytes", s[0])
	}
}

// readRecordData is 0x7E21 read as a client reads it.
func readRecordData(t *testing.T, b []byte) (string, []recordRow) {
	t.Helper()
	if protocol.GameOpcode(binary.LittleEndian.Uint16(b)) != protocol.OpcodeCombatRecordData || b[2] != 1 {
		t.Fatalf("opcode or version % X", b[:3])
	}
	p := 3
	str := func() string {
		n := int(b[p])
		s := string(b[p+1 : p+1+n])
		p += 1 + n
		return s
	}
	name := str()
	n := int(b[p])
	p++
	var rows []recordRow
	for i := 0; i < n; i++ {
		style := b[p]
		p++
		rows = append(rows, recordRow{style, str(), str()})
	}
	if p != len(b) {
		t.Fatalf("%d bytes left over", len(b)-p)
	}
	return name, rows
}

// recordWorld is a chat world with the records kept in a database, three characters
// in it (ids 1-3: Alice, Bob, Cara), the handler registered as a server registers it.
type recordWorld struct {
	*chatWorld
	db *database.SQLiteDB
	h  *CombatRecordHandler
	r  *Registry
}

func newRecordWorld(t *testing.T) *recordWorld {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/record.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); records.Store(nil) })
	chars := database.NewSQLiteCharacterStore(db)
	for i, name := range []string{"Alice", "Bob", "Cara"} {
		rec := &database.CharacterRecord{CharName: name, AccountID: 1}
		if err := chars.Create(rec); err != nil || rec.ServerID != uint32(i+1) {
			t.Fatalf("%s: id %d, %v", name, rec.ServerID, err)
		}
	}
	c := newChatWorld(t)
	h := NewCombatRecordHandler(c.gw, db)
	h.now = func() time.Time { return recordNow }
	r := NewRegistry()
	h.RegisterHandlers(r)
	return &recordWorld{chatWorld: c, db: db, h: h, r: r}
}

// /record and #record and "/t record" (all a PS2 can send) answer the asker alone with
// its own record, "/record Bob" and "/t record Bob" with Bob's, offline or not, in any
// case; a name nobody has says so; "/t record hello there" is a tell, not a record.
func TestRecordCommandWords(t *testing.T) {
	w := newRecordWorld(t)
	s, _ := w.join(1, "Alice", 100, 100, 0)
	other, _ := w.join(2, "Bob", 101, 100, 0)
	if err := w.db.RecordDuelEnd(database.DuelEnd{A: database.RecordFighter{ServerID: 1, Name: "Alice", Level: 10},
		B: database.RecordFighter{ServerID: 3, Name: "Cara", Level: 12}, Loser: 3, At: recordNow}); err != nil {
		t.Fatal(err)
	}
	for _, word := range []string{"/record", "/RECORD", "#record", "/t record", "/tell record", "/record alice"} {
		w.say(s, word)
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != "Alice's duels: 1 won, 0 lost." {
			t.Fatalf("%s: told %q", word, got)
		}
		if got := heard(t, other); len(got) != 0 {
			t.Fatalf("%s reached another player: %q", word, texts(got))
		}
		s.Queue().GetPending()
	}
	for _, word := range []string{"/record cara", "/t record CARA", "#record Cara"} {
		w.say(s, word)
		if got := texts(heard(t, s)); len(got) != 1 || got[0] != "Cara's duels: 0 won, 1 lost." {
			t.Fatalf("%s: told %q", word, got)
		}
		s.Queue().GetPending()
	}
	w.say(s, "/record Bob")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "Bob has no combat record yet." {
		t.Fatalf("no record: told %q", got)
	}
	s.Queue().GetPending()
	w.say(s, "/record Nobody")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "No character is named Nobody." {
		t.Fatalf("no such character: told %q", got)
	}
	s.Queue().GetPending()
	w.gw.SetPvP(true)
	w.say(s, "/record")
	if got := texts(heard(t, s)); len(got) != 2 || got[0] != "Alice: 0 PvP kills, 0 deaths, 0 assists." {
		t.Fatalf("on a PvP server: told %q", got)
	}
	s.Queue().GetPending()
	w.say(s, "/t record hello there")
	if got := texts(heard(t, s)); len(got) != 1 || got[0] != "record is not connected." {
		t.Fatalf("a tell read as /record: %q", got)
	}
}

// A count written: each character it names counted in the database, its summary on its
// player in the world, its record sent to a client with the window (0x7E21) and to none
// without, and its target window told again to everyone targeting it (0x0010's guild
// line with the record's lines). The offer is made only by a server that keeps records.
func TestRecordWrittenAndShown(t *testing.T) {
	w := newRecordWorld(t)
	w.gw.SetPvP(true)
	sa, pa := w.join(1, "Alice", 100, 100, 0)
	sb, pb := w.join(2, "Bob", 101, 100, 0)
	sc, _ := w.join(3, "Cara", 102, 100, 0)
	pb.GuildName = "Blades"
	SendCombatRecordOffer(sa)
	if offers := queued(sa, protocol.OpcodeCombatRecord); len(offers) != 1 || binary.LittleEndian.Uint32(offers[0][2:]) != 1 {
		t.Fatalf("the offer % X", offers)
	}
	sa.Queue().GetPending()
	if err := w.r.Handle(sa, incoming(protocol.OpcodeCombatRecord, []byte{1, 0, 0, 0})); err != nil || !pa.RecordWindow {
		t.Fatalf("the answer: window %v, %v", pa.RecordWindow, err)
	}
	// Cara has Bob targeted
	th := NewTargetHandler(w.sm)
	th.SetGameWorld(w.gw)
	vs := entity.GetOrCreateVisibility(sc)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = pb.ServerID, 0x18802
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[4:], 0x18802)
	if err := th.HandleTarget(sc, incoming(protocol.OpcodeClassicTargetInfo, b)); err != nil {
		t.Fatal(err)
	}
	if _, _, guild, _ := pvpFaces(sc); guild != "Blades" {
		t.Fatalf("before: %q", guild)
	}
	sc.Queue().GetPending()

	kill := database.PvPKill{Killer: database.RecordFighter{ServerID: 1, Name: "Alice", Level: 30},
		Victim: database.RecordFighter{ServerID: 2, Name: "Bob", Level: 32}, At: recordNow}
	w.h.write(entity.RecordOp{Kill: &kill})
	if r, _ := w.db.CombatRecordOf(1); r.PvPKills != 1 || r.Streak != 1 || r.LastKill != "Bob" {
		t.Fatalf("the killer's row %+v", r)
	}
	if s := w.gw.RecordSummaryOf(pb); s.PvPDeaths != 1 {
		t.Fatalf("the victim's summary %+v", s)
	}
	data := queued(sa, protocol.OpcodeCombatRecordData)
	if len(data) != 1 {
		t.Fatalf("the killer's window was sent %d records", len(data))
	}
	if name, rows := readRecordData(t, data[0]); name != "Alice" || rows[1] != (recordRow{recordLine, "Kills", "1"}) {
		t.Fatalf("sent %q %+v", name, rows)
	}
	if got := queued(sb, protocol.OpcodeCombatRecordData); len(got) != 0 {
		t.Fatal("a client without the window was sent the record")
	}
	if _, _, guild, _ := pvpFaces(sc); guild != "Blades\nPvP 0-1" {
		t.Fatalf("the targeter's window: %q", guild)
	}

	// a client's ask: its record now
	sb.Queue().GetPending()
	if err := w.r.Handle(sb, incoming(protocol.OpcodeCombatRecordData, []byte{1, 0, 0, 0})); err != nil {
		t.Fatal(err)
	}
	if data := queued(sb, protocol.OpcodeCombatRecordData); len(data) != 1 {
		t.Fatalf("asked: %d sent", len(data))
	} else if name, rows := readRecordData(t, data[0]); name != "Bob" || rows[2] != (recordRow{recordLine, "Deaths", "1"}) || rows[7] != (recordRow{recordLine, "Last killed by", "Alice, just now"}) {
		t.Fatalf("asked: %q %+v", name, rows)
	}

	// a login's summary is read at its entry
	pb.GuildName = ""
	w.gw.SetRecordSummary(2, entity.RecordSummary{})
	w.gw.PlayerEntered(sb)
	if _, guild := w.gw.TargetInfoLines(pa, pb); guild != "PvP 0-1" {
		t.Fatalf("after the login's read: %q", guild)
	}

	// no database, no offer
	records.Store(nil)
	sd := session.NewManager().CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 9, 9, 9, 1, true)
	SendCombatRecordOffer(sd)
	if got := queued(sd, protocol.OpcodeCombatRecord); len(got) != 0 {
		t.Fatal("offered with no records kept")
	}
}
