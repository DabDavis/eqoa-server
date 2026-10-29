package handlers

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// A header is written the way VICSMessageHeader's reader takes it: the id
// raw, seven date bytes, the category and subcategory raw, the unread byte.
func TestCSHeaderLayout(t *testing.T) {
	tk := &csTicket{id: 0x01020304, date: time.Date(2026, 10, 19, 21, 5, 3, 0, time.UTC), category: 2, subcategory: 21, unread: true}
	buf := make([]byte, 64)
	w := buffer.NewWriter(buf)
	writeCSHeader(w, tk)
	want := []byte{4, 3, 2, 1, 0xEA, 0x07, 10, 19, 21, 5, 3, 2, 0, 0, 0, 21, 0, 0, 0, 1}
	if got := buf[:w.Position()]; !bytes.Equal(got, want) {
		t.Fatalf("header\n got %v\nwant %v", got, want)
	}
}

// The lists: a zigzag count, then per category a zigzag id, a hidden byte,
// the wide name and its subcategories the same way. The control: the hidden
// category is flagged, the first is not.
func TestCSCategoriesLayout(t *testing.T) {
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	writeCSCategories(w)
	r := buffer.NewReader(buf[:w.Position()])
	n, _ := r.Read7BitEncodedInt64()
	if int(n) != len(csCategories) {
		t.Fatalf("count %d", n)
	}
	for k := 0; k < int(n); k++ {
		id, _ := r.Read7BitEncodedInt64()
		hidden, _ := r.ReadByte()
		name, _ := readWide(r)
		c := csCategories[k]
		if id != c.id || (hidden != 0) != c.hidden || name != c.name {
			t.Fatalf("category %d: %d %d %q", k, id, hidden, name)
		}
		m, _ := r.Read7BitEncodedInt64()
		for j := 0; j < int(m); j++ {
			sid, _ := r.Read7BitEncodedInt64()
			sh, _ := r.ReadByte()
			sname, _ := readWide(r)
			s := c.subs[j]
			if sid != s.id || (sh != 0) != s.hidden || sname != s.name {
				t.Fatalf("category %d sub %d: %d %d %q", k, j, sid, sh, sname)
			}
		}
	}
	if r.Remaining() != 0 {
		t.Fatalf("%d bytes left", r.Remaining())
	}
	if csCategories[0].hidden || !csCategories[len(csCategories)-1].hidden {
		t.Fatal("the control: one hidden category, last")
	}
}

// csWorld is a chat world with Ann (1) and Bob (2) in it and a support handler
// on a fresh petitions table.
func csWorld(t *testing.T) (*chatWorld, *SupportHandler, *database.SQLiteDB, *session.Session, *session.Session) {
	t.Helper()
	c := newChatWorld(t)
	db, err := database.NewSQLite(t.TempDir() + "/cs.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewSupportHandler(c.gw, db)
	h.RegisterHandlers(NewRegistry())
	as, _ := c.join(1, "Ann", 100, 100, 0)
	bs, _ := c.join(2, "Bob", 110, 110, 0)
	return c, h, db, as, bs
}

// csSend handles one 0x0AA1 from s and gives back each 0x07B0 it was sent
// (after the opcode).
func csSend(t *testing.T, h *SupportHandler, s *session.Session, sub byte, fields func(w *buffer.Writer)) [][]byte {
	t.Helper()
	s.Queue().GetPending()
	buf := make([]byte, 4096)
	w := buffer.NewWriter(buf)
	w.WriteByte(sub)
	if fields != nil {
		fields(w)
	}
	if err := h.handleRequest(s, incoming(opcodeCSRequest, buf[:w.Position()])); err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, m := range s.Queue().PeekPending() {
		if d := m.Data(); len(d) >= 4 && opcode(d) == opcodeCSReply {
			out = append(out, d[2:])
		}
	}
	return out
}

// csID is a request's zigzag id field.
func csID(id int64) func(w *buffer.Writer) {
	return func(w *buffer.Writer) { w.Write7BitEncodedInt64(id) }
}

// csInbox is the ids and unread flags the inbox lists.
func csInbox(t *testing.T, h *SupportHandler, s *session.Session) (ids []int64, unread []bool) {
	t.Helper()
	got := csSend(t, h, s, 2, nil)
	if len(got) != 1 || got[0][0] != 2 || got[0][1] != csOK {
		t.Fatalf("inbox % X", got)
	}
	r := buffer.NewReader(got[0][2:])
	r.ReadUint32()
	n, _ := r.Read7BitEncodedInt64()
	for i := 0; i < int(n); i++ {
		id, _ := r.ReadUint32()
		r.ReadBytes(7 + 8)
		u, _ := r.ReadByte()
		ids, unread = append(ids, int64(id)), append(unread, u != 0)
	}
	if r.Remaining() != 0 {
		t.Fatalf("the inbox does not read as CLIENT reads it: %d bytes left", r.Remaining())
	}
	return ids, unread
}

// A petition's life through the game on the table: filed (status 0, ticket
// 1001, unread, the request acknowledged in its text), opened (its text, read),
// replied to (waiting on a GM again), answered by a GM (unread again; a login
// is told), closed by a GM: then opening, replying to and deleting it are all
// status 8 (the live client's "addressed"), and it is listed for a week. A
// deleted petition is no longer the player's (3), and another character's is
// not found (3).
func TestPetitionsThroughTheGame(t *testing.T) {
	c, h, db, as, bs := csWorld(t)
	file := func(text string) [][]byte {
		return csSend(t, h, as, 0, func(w *buffer.Writer) {
			w.Write7BitEncodedInt64(2)
			w.Write7BitEncodedInt64(20)
			writeWideString(w, text, 1024)
		})
	}
	if got := file("I am stuck in a wall"); len(got) != 1 || !bytes.Equal(got[0], []byte{0, csOK}) {
		t.Fatalf("filing: % X", got)
	}
	ids, unread := csInbox(t, h, as)
	if len(ids) != 1 || ids[0] != 1001 || !unread[0] {
		t.Fatalf("inbox %v %v", ids, unread)
	}
	open := func(s *session.Session, id int64) (byte, string) {
		got := csSend(t, h, s, 3, csID(id))
		if len(got) != 1 || got[0][0] != 3 {
			t.Fatalf("open % X", got)
		}
		if got[0][1] != csOK {
			return got[0][1], ""
		}
		text, _ := readWideLine(got[0][2:])
		return csOK, text
	}
	if st, text := open(as, 1001); st != csOK || text != "Your request (Stuck / In terrain) has been received:\n\nI am stuck in a wall" {
		t.Fatalf("open: %d %q", st, text)
	}
	if _, unread = csInbox(t, h, as); unread[0] {
		t.Error("still unread after opening")
	}
	if st, _ := open(bs, 1001); st != csNotFound {
		t.Errorf("Bob opening Ann's: %d", st)
	}
	if got := csSend(t, h, as, 1, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(1001)
		writeWideString(w, "Near the Freeport gate", 1024)
	}); len(got) != 1 || !bytes.Equal(got[0], []byte{1, csOK}) {
		t.Fatalf("reply % X", got)
	}
	p, _, _ := db.PetitionByID(1001)
	if !strings.HasSuffix(p.Text, "\n\nAnn: Near the Freeport gate") || !p.Waiting {
		t.Errorf("after the reply: %+v", p)
	}

	gm := database.GMEdit{AccountID: 9, Username: "gm", Via: "web"}
	if _, _, err := db.AnswerPetition(1001, "GM Jib", "Moved you out.", gm, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, unread = csInbox(t, h, as); !unread[0] {
		t.Error("an answer not unread")
	}
	as.Queue().GetPending()
	c.gw.PlayerEntered(as)
	if said := texts(heard(t, as)); !hasLine(said, PetitionAnsweredText(1001)) {
		t.Errorf("a login with an answer unread heard %q", said)
	}
	if st, text := open(as, 1001); st != csOK || !strings.HasSuffix(text, "\n\nGM Jib: Moved you out.") {
		t.Errorf("the answer reads %d %q", st, text)
	}
	if _, _, err := db.ClosePetition(1001, gm, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st, _ := open(as, 1001); st != csAddressed {
		t.Errorf("opening a closed one: %d", st)
	}
	if got := csSend(t, h, as, 1, func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(1001)
		writeWideString(w, "still stuck", 1024)
	}); len(got) != 1 || !bytes.Equal(got[0], []byte{1, csAddressed}) {
		t.Errorf("replying to a closed one: % X", got)
	}
	if got := csSend(t, h, as, 4, csID(1001)); len(got) != 1 || !bytes.Equal(got[0], []byte{4, csAddressed, 0xD2, 0x0F}) {
		t.Errorf("deleting a closed one: % X", got)
	}
	if ids, _ = csInbox(t, h, as); len(ids) != 1 {
		t.Errorf("a closed one leaves the inbox at once: %v", ids)
	}
	if _, err := db.Exec(`UPDATE petitions SET closed = ? WHERE id = 1001`,
		time.Now().Add(-database.PetitionClosedListed-time.Hour).UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if ids, _ = csInbox(t, h, as); len(ids) != 0 {
		t.Errorf("a week after it closed it is still listed: %v", ids)
	}

	file("second")
	if got := csSend(t, h, as, 4, csID(1002)); len(got) != 1 || !bytes.Equal(got[0], []byte{4, csOK, 0xD4, 0x0F}) {
		t.Fatalf("deleting an open one: % X", got)
	}
	if p, _, _ := db.PetitionByID(1002); p.Status != database.PetitionDeleted {
		t.Errorf("a deleted one is kept as %q", p.Status)
	}
	if st, _ := open(as, 1002); st != csNotFound {
		t.Errorf("opening a deleted one: %d", st)
	}
}

// A character with database.PetitionsListed petitions listed is refused a
// new one (status 2), as CLIENT refuses itself at 100 rows; one deleted makes
// room. The control: another character files.
func TestPetitionsListedCap(t *testing.T) {
	_, h, db, as, bs := csWorld(t)
	for i := 0; i < database.PetitionsListed; i++ {
		if _, err := db.FilePetition(database.Petition{ServerID: 1, CharName: "Ann", Text: "x"}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	req := func(w *buffer.Writer) {
		w.Write7BitEncodedInt64(1)
		w.Write7BitEncodedInt64(10)
		writeWideString(w, "one more", 1024)
	}
	if got := csSend(t, h, as, 0, req); len(got) != 1 || !bytes.Equal(got[0], []byte{0, csTooMany}) {
		t.Fatalf("the 101st: % X", got)
	}
	if got := csSend(t, h, bs, 0, req); len(got) != 1 || !bytes.Equal(got[0], []byte{0, csOK}) {
		t.Errorf("the control, Bob: % X", got)
	}
	csSend(t, h, as, 4, csID(1001))
	if got := csSend(t, h, as, 0, req); len(got) != 1 || !bytes.Equal(got[0], []byte{0, csOK}) {
		t.Errorf("after a delete: % X", got)
	}
}

// A ticket longer than CLIENT keeps is sent by its end, the latest said,
// after a line saying the start is not shown; a short one whole.
func TestPetitionShownByItsEnd(t *testing.T) {
	if got := csShown("short"); got != "short" {
		t.Fatalf("short: %q", got)
	}
	long := strings.Repeat("a", csShownMax) + "THE END"
	got := csShown(long)
	if n := len(utf16.Encode([]rune(got))); n > csShownMax || !strings.HasSuffix(got, "THE END") ||
		!strings.HasPrefix(got, "(the start of this ticket is not shown)") {
		t.Fatalf("long: %d units, %q...%q", n, got[:50], got[len(got)-10:])
	}
}
