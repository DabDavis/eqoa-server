package handlers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// bugWorld is csWorld with Ann a player (account 2) and Bob a GM (account
// 1), the support handler telling GMs, and the chat handler's /bug on it.
func bugWorld(t *testing.T) (*chatWorld, *SupportHandler, *database.SQLiteDB, *session.Session, *session.Session) {
	t.Helper()
	c, h, db, as, bs := csWorld(t)
	h.limit.off = false
	as.SetAccountID(2)
	bs.SetAccountID(1)
	h.SetSessions(c.sm)
	h.SetAccessLevels(levels{1: 1, 2: 0})
	c.h.SetAccessLevels(levels{1: 1, 2: 0})
	c.h.support = h
	return c, h, db, as, bs
}

// chat sends a typed line from s.
func chat(t *testing.T, c *chatWorld, s *session.Session, text string) {
	t.Helper()
	s.Queue().GetPending()
	if err := c.h.HandleSayChat(s, incoming(protocol.OpcodeClientSayChat, wide(text, 0xFFFFFFFF))); err != nil {
		t.Fatal(err)
	}
}

// sentText is every line s was sent since its queue was last emptied, one a line.
func sentText(t *testing.T, s *session.Session) string {
	t.Helper()
	var b strings.Builder
	for _, l := range heard(t, s) {
		if l.text != "" {
			b.WriteString(l.text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// views is each petition's server view, by ticket.
func views(t *testing.T, db *database.SQLiteDB) map[int64]serverView {
	t.Helper()
	rows, err := db.Query(`SELECT id, server_view FROM petitions ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]serverView{}
	for rows.Next() {
		var id int64
		var raw string
		rows.Scan(&id, &raw)
		var v serverView
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				t.Fatalf("ticket %d's view: %v\n%s", id, err, raw)
			}
		}
		out[id] = v
	}
	return out
}

// /bug files a petition under Bug Report / Sent from chat with the server's
// view: the character, its place, how it came, and what it was shown before
// (a line queued to it). The player is told its ticket; the GM is told of it,
// the player (no GM) is not told of its own. #bug is the same.
func TestChatReportFilesWithTheServersView(t *testing.T) {
	c, _, db, as, bs := bugWorld(t)
	SendClientMessage(as, "You feel a strange chill.")
	bs.Queue().GetPending()
	chat(t, c, as, "/bug the bat by the well floats in the air")
	if got := sentText(t, as); !strings.Contains(got, "your report is Ticket #1001") {
		t.Fatalf("Ann was told %q", got)
	}
	if got := sentText(t, bs); !strings.Contains(got, "[Petition] Ticket #1001 from Ann (Bug Report / Sent from chat): the bat by the well floats in the air") {
		t.Fatalf("the GM was told %q", got)
	}
	p, ok, err := db.PetitionByID(1001)
	if err != nil || !ok || p.Category != 1 || p.Subcategory != bugSubcategory || !strings.Contains(p.Text, "the bat by the well floats") {
		t.Fatalf("ticket %+v %v %v", p, ok, err)
	}
	v := views(t, db)[1001]
	if v.Via != "chat" || v.Character.Name != "Ann" || v.Character.X != 100 || v.Server.Build == "" {
		t.Fatalf("view %+v", v)
	}
	shown := false
	for _, l := range v.Lines {
		if l.Kind == "line" && l.Text == "You feel a strange chill." && l.Ago <= 0 {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("the line Ann was shown is not in the view: %+v", v.Lines)
	}
	if len(v.Stats) == 0 {
		t.Fatal("no #stats lines in the view")
	}

	// #bug, from a PS2's chat; the limit: a second report at once is refused
	// (the control: the first of this test went in)
	chat(t, c, as, "#bug a second at once")
	if got := sentText(t, as); !strings.Contains(got, reportLimitText) {
		t.Fatalf("a report at once was not refused: %q", got)
	}
	if n := len(views(t, db)); n != 1 {
		t.Fatalf("%d petitions", n)
	}
	// alone, it says how
	chat(t, c, as, "/bug")
	if got := sentText(t, as); !strings.Contains(got, "Type /bug (or #bug)") {
		t.Fatalf("/bug alone: %q", got)
	}
}

// The limit: one a reportGap, reportsPerHour an hour, per account.
func TestReportLimiter(t *testing.T) {
	var l reportLimiter
	at := time.Date(2026, 11, 5, 3, 0, 0, 0, time.UTC)
	if !l.allow(1, at) || l.allow(1, at.Add(reportGap-time.Second)) || !l.allow(2, at) {
		t.Fatal("the gap")
	}
	n := 1
	for i := 1; i < 40; i++ {
		if l.allow(1, at.Add(time.Duration(i)*reportGap)) {
			n++
		}
	}
	if n != reportsPerHour {
		t.Fatalf("%d in an hour", n)
	}
	if !l.allow(1, at.Add(time.Hour+40*reportGap)) {
		t.Fatal("an hour on")
	}
}

// The client's note goes with the next petition its client files, by the
// form, and with that one only; the offer's answer is kept on the session.
func TestBugNoteGoesWithTheNextPetition(t *testing.T) {
	_, h, db, as, _ := bugWorld(t)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, 1)
	as.Queue().GetPending()
	h.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, buf))
	if !as.BugReports() {
		t.Fatal("the offer's answer is not kept")
	}
	if got := sentText(t, as); !strings.Contains(got, reportNoticeF3) {
		t.Fatalf("the answer was not told F3's line: %q", got)
	}
	as.Queue().GetPending()
	h.handleBugReportOffer(as, incoming(protocol.OpcodeBugReportOffer, buf))
	if got := sentText(t, as); strings.Contains(got, reportNoticeF3) {
		t.Fatal("F3's line twice in a session")
	}
	note := "build: dev-abc1234\nplatform: Linux x86_64\x07 bell"
	nb := make([]byte, 8+len(note))
	binary.LittleEndian.PutUint32(nb, 1)
	binary.LittleEndian.PutUint32(nb[4:], uint32(len(note)))
	copy(nb[8:], note)
	h.handleBugReportNote(as, incoming(protocol.OpcodeBugReportNote, nb))
	file := func(text string) {
		got := csSend(t, h, as, 0, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(1)
			w.Write7BitEncodedInt64(13)
			writeWideString(w, text, 1024)
		})
		if len(got) != 1 || got[0][1] != csOK {
			t.Fatalf("filing % X", got)
		}
	}
	file("the guard walks through the wall")
	v := views(t, db)
	if v[1001].Client.Note != "build: dev-abc1234\nplatform: Linux x86_64  bell" || !v[1001].Client.F3 || v[1001].Via != "support" {
		t.Fatalf("first: %+v", v[1001].Client)
	}
	h.limit = reportLimiter{off: true} // the next filing is not refused by the gap
	file("and again")
	if v := views(t, db); v[1002].Client.Note != "" {
		t.Fatalf("the note went with a second petition: %q", v[1002].Client.Note)
	}
	if p, _, _ := db.PetitionByID(1001); !strings.Contains(p.Text, "(Bug Report / Creature or NPC)") {
		t.Fatalf("the topic's name: %q", p.Text)
	}
}

// A note is kept as text: valid UTF-8, a line end and a tab kept, every other
// control character a space, cut at BugNoteMax bytes.
func TestCleanNote(t *testing.T) {
	if got := cleanNote("a\tb\r\nc\x00d\xff"); got != "a\tb\nc d?" {
		t.Fatalf("%q", got)
	}
	if got := cleanNote(strings.Repeat("é", BugNoteMax)); len(got) > BugNoteMax {
		t.Fatalf("%d bytes", len(got))
	}
}

// A petitions table from before the server view gets its column, its rows
// kept.
func TestPetitionsTableGetsTheServerView(t *testing.T) {
	path := t.TempDir() + "/old.db"
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`DROP TABLE petitions`,
		`CREATE TABLE petitions (id INTEGER PRIMARY KEY AUTOINCREMENT, serverid INTEGER NOT NULL, charname TEXT NOT NULL DEFAULT '',
			accountid INTEGER NOT NULL DEFAULT 0, category INTEGER NOT NULL DEFAULT 0, subcategory INTEGER NOT NULL DEFAULT 0,
			filed TEXT NOT NULL, text TEXT NOT NULL DEFAULT '', unread INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'open',
			waiting INTEGER NOT NULL DEFAULT 1, updated TEXT NOT NULL DEFAULT '', replied TEXT NOT NULL DEFAULT '',
			answered TEXT NOT NULL DEFAULT '', answered_by TEXT NOT NULL DEFAULT '', closed TEXT NOT NULL DEFAULT '',
			closed_by TEXT NOT NULL DEFAULT '')`,
		`INSERT INTO petitions (id, serverid, filed, text) VALUES (1001, 7, '2026-11-01T00:00:00Z', 'old')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v, err := db.PetitionServerView(1001)
	if err != nil || v != "" {
		t.Fatalf("%q %v", v, err)
	}
	if p, ok, _ := db.PetitionByID(1001); !ok || p.Text != "old" {
		t.Fatal("the old row")
	}
}

// The transcript keeps what the client is shown (a line, a damage figure),
// not the rest of the traffic, and reads back into lines with who hit whom.
func TestTranscriptReadsBack(t *testing.T) {
	_, h, _, as, _ := bugWorld(t)
	SendErrorMessage(as, "You cannot do that.")
	SendDamageIndication(as, as.PlayerObjectID(), -42, 7)
	queueReliable(as, protocol.OpcodeBugReportOffer, buildBugReportOffer()) // not a line
	lines := h.transcriptLines(as, as.Transcript(time.Now()), time.Now())
	var got []string
	for _, l := range lines {
		got = append(got, l.Kind+": "+l.Text)
	}
	want := "red line: You cannot do that.|damage: you hit object 7 for 42"
	if strings.Join(got, "|") != want {
		t.Fatalf("got %q\nwant %q", strings.Join(got, "|"), want)
	}
}

// The offer is a u32, 1, and then the most characters a petition's text may
// have here: a client from before the second reads the first alone (vanilla's
// "u32 != 0"), and one that reads the second may let its form hold that many.
func TestTheOfferSaysHowLongAPetitionMayBe(t *testing.T) {
	want := []byte{0x0E, 0x7E, 1, 0, 0, 0, 0xFF, 0x07, 0, 0}
	if got := buildBugReportOffer(); !bytes.Equal(got, want) {
		t.Fatalf("offer % X, want % X", got, want)
	}
	if CSTextMost != 2047 {
		t.Fatalf("CSTextMost %d", CSTextMost)
	}
}

// A GM's reports are not counted against the limit (the GM Jibrish files many
// while testing, 2026-11-10): Bob, a GM, sends reportsPerHour+3 at once and
// each is filed. The control: Ann, a player, is refused her second at once.
func TestAGMIsNotHeldToTheReportLimit(t *testing.T) {
	c, _, db, as, bs := bugWorld(t)
	for i := 0; i < reportsPerHour+3; i++ {
		chat(t, c, bs, fmt.Sprintf("/bug the GM's report %d", i))
		if got := sentText(t, bs); !strings.Contains(got, "your report is Ticket #") {
			t.Fatalf("the GM's report %d: told %q", i, got)
		}
	}
	chat(t, c, as, "/bug the first")
	chat(t, c, as, "/bug a second at once")
	if got := sentText(t, as); !strings.Contains(got, reportLimitText) {
		t.Fatalf("the player's second at once: told %q", got)
	}
	if n := len(views(t, db)); n != reportsPerHour+4 {
		t.Fatalf("%d petitions, want %d", n, reportsPerHour+4)
	}
}

// Feature Request (petition 1841): Bug Report's lists offer it to every client
// (a PS2's form too: CLIENT shows the server's lists), not hidden, after every
// bug's topic, by featureSubcategory; the form files it labelled "(Bug Report /
// Feature Request)", /feature and #feature file it from chat, the player told
// it is a feature request and the GM told its topic; the old kinds are as they
// were (a form's Creature or NPC, /bug's Sent from chat). The controls: /bug
// still files 19, /feature alone says how and files nothing.
func TestFeatureRequestFiledApart(t *testing.T) {
	// the lists, as a client reads them
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	writeCSCategories(w)
	r := buffer.NewReader(buf[:w.Position()])
	n, _ := r.Read7BitEncodedInt64()
	var offered []string // Bug Report's topics a client shows
	for k := 0; k < int(n); k++ {
		id, _ := r.Read7BitEncodedInt64()
		r.ReadByte()
		readWide(r)
		m, _ := r.Read7BitEncodedInt64()
		for j := 0; j < int(m); j++ {
			sid, _ := r.Read7BitEncodedInt64()
			hidden, _ := r.ReadByte()
			name, _ := readWide(r)
			if id == 1 && hidden == 0 {
				offered = append(offered, fmt.Sprintf("%d %s", sid, name))
			}
		}
	}
	want := []string{"12 Quest", "13 Creature or NPC", "14 Item, loot or merchant", "15 Spell or ability", "16 Combat",
		"17 Other", "18 Game client", "100 Feature Request"}
	if strings.Join(offered, "|") != strings.Join(want, "|") {
		t.Fatalf("Bug Report offers %q", offered)
	}
	for _, s := range []struct {
		sub  int64
		name string
	}{{10, "Crash"}, {11, "Graphics"}, {12, "Quest"}, {13, "Creature or NPC"}, {18, "Game client"}, {19, "Sent from chat"},
		{featureSubcategory, "Feature Request"}} {
		if c, got := CSNames(1, s.sub); c != "Bug Report" || got != s.name {
			t.Fatalf("CSNames(1, %d) = %q / %q", s.sub, c, got)
		}
	}

	c, h, db, as, bs := bugWorld(t)
	h.limit.off = true // several at once (TestChatReportFilesWithTheServersView holds the limit)
	form := func(sub int64, text string) {
		t.Helper()
		got := csSend(t, h, as, 0, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(1)
			w.Write7BitEncodedInt64(sub)
			writeWideString(w, text, 1024)
		})
		if len(got) != 1 || !bytes.Equal(got[0], []byte{0, csOK}) {
			t.Fatalf("filing %d: % X", sub, got)
		}
	}
	ticket := func(id, sub int64, head, text string) {
		t.Helper()
		p, ok, err := db.PetitionByID(id)
		if err != nil || !ok || p.Category != 1 || p.Subcategory != sub ||
			!strings.HasPrefix(p.Text, "Your request (Bug Report / "+head+") has been received:") || !strings.Contains(p.Text, text) {
			t.Fatalf("ticket %d: %+v %v %v", id, p, ok, err)
		}
	}

	bs.Queue().GetPending()
	form(featureSubcategory, "a /lfg window please")
	ticket(1001, featureSubcategory, "Feature Request", "a /lfg window please")
	if got := sentText(t, bs); !strings.Contains(got, "[Petition] Ticket #1001 from Ann (Bug Report / Feature Request): a /lfg window please") {
		t.Fatalf("the GM was told %q", got)
	}
	form(13, "the guard walks through walls")
	ticket(1002, 13, "Creature or NPC", "the guard walks through walls")

	chat(t, c, as, "/feature a bank in Qeynos")
	if got := sentText(t, as); !strings.Contains(got, "your feature request is Ticket #1003") {
		t.Fatalf("Ann was told %q", got)
	}
	ticket(1003, featureSubcategory, "Feature Request", "a bank in Qeynos")
	chat(t, c, as, "#FEATURE more hair styles")
	ticket(1004, featureSubcategory, "Feature Request", "more hair styles")
	chat(t, c, as, "/bug the gate is shut")
	if got := sentText(t, as); !strings.Contains(got, "your report is Ticket #1005") {
		t.Fatalf("Ann was told %q", got)
	}
	ticket(1005, bugSubcategory, "Sent from chat", "the gate is shut")
	if v := views(t, db); v[1003].Via != "chat" || v[1001].Via != "support" {
		t.Fatalf("via %q %q", v[1003].Via, v[1001].Via)
	}

	chat(t, c, as, "/feature")
	if got := sentText(t, as); !strings.Contains(got, "Type /feature (or #feature)") {
		t.Fatalf("/feature alone: %q", got)
	}
	if n := len(views(t, db)); n != 5 {
		t.Fatalf("%d petitions", n)
	}
}
