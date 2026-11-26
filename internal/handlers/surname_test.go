package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// surnameRig is the chat world's four (Anna, Cora, Bert, Dora: accounts 1 to 4,
// Anna a GM) on rows of a database, with the surnames handler RegisterDefaultHandlers makes.
func surnameRig(t *testing.T) (*portalRig, *Surnames, *database.SQLiteDB) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/surname.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := newPortalRig(t)
	for _, c := range []struct {
		id   int
		name string
	}{{1, "Anna"}, {2, "Cora"}, {3, "Bert"}, {4, "Dora"}, {9, "Smith"}} {
		if _, err := db.Exec(`INSERT INTO characters (serverid, charName, accountid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle, faceoption, sex)
			VALUES (?, ?, ?, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0)`, c.id, c.name, c.id); err != nil {
			t.Fatal(err)
		}
	}
	s := NewSurnames(r.gw, db, r.sm)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	was := surnamesH.Load()
	surnamesH.Store(s)
	t.Cleanup(func() { surnamesH.Store(was) })
	for i, sess := range []*session.Session{r.anna, r.cora, r.bert, r.dora} {
		sess.SetAccountID(i + 1)
	}
	r.h.SetAccessLevels(levels{1: 1})
	r.pa.Level, r.pc.Level, r.pb.Level = 30, 30, 10
	r.drain()
	return r, s, db
}

// surnamesSent is the pairs of every OpcodeSurnames queued to s, name=surname.
func surnamesSent(t *testing.T, s *session.Session) []string {
	t.Helper()
	var out []string
	for _, d := range queued(s, protocol.OpcodeSurnames) {
		r := buffer.NewReader(d[2:])
		if l, _ := r.ReadByte(); l != surnameLayout {
			t.Fatalf("layout %d", l)
		}
		n, _ := r.ReadUint16()
		for range n {
			a, _ := r.ReadByte()
			name, _ := r.ReadBytes(int(a))
			b, _ := r.ReadByte()
			sur, _ := r.ReadBytes(int(b))
			out = append(out, string(name)+"="+string(sur))
		}
		if r.Remaining() != 0 {
			t.Fatalf("%d bytes past the pairs", r.Remaining())
		}
	}
	return out
}

// Live petition 5648: /surname from level 20, the pets' and mercs' name rules,
// kept in characters.surname, after the name in /who, told to the clients that
// answered the offer (the list at the answer, then each change); changed only
// after a week, cleared by its player (the wait running on) or by a GM's
// #surname clear (any character's, the wait with it). The controls: level 10
// is refused; a filtered word, a character's name, two words and the
// character's own name are refused and nothing is kept; a client that never
// answered is sent nothing.
func TestSurnames(t *testing.T) {
	r, s, db := surnameRig(t)
	surname := func(id uint32) string {
		t.Helper()
		v, _, err := db.CharacterSurname(id)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	said := func(sess *session.Session) string { return strings.Join(r.msgs(sess), "|") }

	// Cora's client draws the surnames; Dora's never answered
	if err := s.HandleAnswer(r.cora, incoming(protocol.OpcodeSurnameOffer, []byte{1})); err != nil {
		t.Fatal(err)
	}
	if got := surnamesSent(t, r.cora); len(got) != 0 {
		t.Errorf("no one has a surname: the list %q", got)
	}
	r.drain()

	// Bert at level 10
	r.say(r.bert, "/surname Stoneheart")
	if got := said(r.bert); !strings.Contains(got, "from level 20") || surname(3) != "" {
		t.Errorf("level 10: %q, kept %q", got, surname(3))
	}

	// refusals
	for _, c := range []struct{ asked, want string }{
		{"Smith", "is a character's name"},
		{"Anna", "is a character's name"},
		{"Cora", "your own name"},
		{"Van Dorn", "not a name"},
		{"X1", "not a name"},
	} {
		r.say(r.cora, "/surname "+c.asked)
		if got := said(r.cora); !strings.Contains(got, c.want) {
			t.Errorf("/surname %s: %q, want %q", c.asked, got, c.want)
		}
	}
	if surname(2) != "" {
		t.Fatalf("a refused surname kept: %q", surname(2))
	}

	// taken, cased, told
	r.drain()
	r.say(r.cora, "/surname stoneheart")
	if got := said(r.cora); !strings.Contains(got, "You are now Cora Stoneheart.") {
		t.Errorf("taken: %q", got)
	}
	if surname(2) != "Stoneheart" {
		t.Fatalf("kept %q", surname(2))
	}
	r.drain()
	// /who: after the name, everyone's
	r.say(r.dora, "/who")
	if got := said(r.dora); !strings.Contains(got, "Cora Stoneheart: level 30") {
		t.Errorf("/who: %q", got)
	}

	// told at once to the client that draws them (Cora's own), not to Dora's
	r.say(r.cora, "/surname stoneheart")
	if got := said(r.cora); !strings.Contains(got, "is Stoneheart already") {
		t.Errorf("the same again: %q", got)
	}

	// a week's wait for a change
	r.say(r.cora, "/surname Ironhand")
	if got := said(r.cora); !strings.Contains(got, "again in 7 days") || surname(2) != "Stoneheart" {
		t.Errorf("a change at once: %q, kept %q", got, surname(2))
	}
	s.now = func() time.Time { return time.Unix(1_800_000_000, 0).Add(surnameEvery) }
	r.say(r.cora, "/t surname Ironhand") // a PS2's spelling
	if surname(2) != "Ironhand" {
		t.Fatalf("after the week: %q (%q)", surname(2), said(r.cora))
	}

	// a client that answers now is sent the list
	r.drain()
	if err := s.HandleAnswer(r.dora, incoming(protocol.OpcodeSurnameOffer, []byte{1})); err != nil {
		t.Fatal(err)
	}
	if got := surnamesSent(t, r.dora); len(got) != 1 || got[0] != "Cora=Ironhand" {
		t.Errorf("Dora's list %q", got)
	}

	// cleared by its player: told, and the wait runs on
	r.drain()
	r.say(r.cora, "/surname clear")
	if surname(2) != "" {
		t.Fatalf("cleared: %q", surname(2))
	}
	if got := surnamesSent(t, r.dora); len(got) != 1 || got[0] != "Cora=" {
		t.Errorf("the clear told %q", got)
	}
	r.say(r.cora, "/surname Ironhand")
	if got := said(r.cora); !strings.Contains(got, "again in 7 days") {
		t.Errorf("a new one after a clear: %q", got)
	}

	// a GM's clear of an offline character's, and its wait with it
	if err := db.SetSurname(9, "Grimm", time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	r.say(r.cora, "/surname clear Smith") // not a GM: her own (none)
	if v, _, _ := db.CharacterSurname(9); v != "Grimm" {
		t.Fatalf("a player cleared another's: %q", v)
	}
	r.drain()
	r.say(r.anna, "#surname clear smith")
	if got := said(r.anna); !strings.Contains(got, `Smith's surname "Grimm" is cleared`) {
		t.Errorf("the GM's clear: %q", got)
	}
	if v, at, _ := db.CharacterSurname(9); v != "" || !at.IsZero() {
		t.Errorf("after the GM's clear: %q at %v", v, at)
	}
	// and of an online one: told
	s.now = func() time.Time { return time.Unix(1_800_000_000, 0).Add(3 * surnameEvery) }
	r.say(r.cora, "/surname Ironhand")
	r.drain()
	r.say(r.anna, "#surname clear Cora")
	if got := said(r.cora); !strings.Contains(got, "A GM cleared your surname Ironhand.") {
		t.Errorf("Cora told %q", got)
	}
	r.say(r.cora, "/surname Oakshield")
	if surname(2) != "Oakshield" {
		t.Errorf("after a GM's clear a new one at once: %q", surname(2))
	}
}

// The columns are made once, read back, and a character's own write; none for a
// character that is not.
func TestSurnameColumns(t *testing.T) {
	_, _, db := surnameRig(t)
	at := time.Unix(1_800_000_123, 0)
	if err := db.SetSurname(2, "Oakshield", at); err != nil {
		t.Fatal(err)
	}
	if v, got, err := db.CharacterSurname(2); err != nil || v != "Oakshield" || !got.Equal(at) {
		t.Errorf("read back %q %v %v", v, got, err)
	}
	if err := db.SetSurname(77, "Nobody", at); err == nil {
		t.Error("a surname for no character")
	}
}
