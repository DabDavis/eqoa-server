package database

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func petitionDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLite(t.TempDir() + "/petitions.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// The GMs' query: open by default, waiting (filed or replied to since the last
// answer), closed, deleted, all; one character's; a search of the name or the
// text with LIKE's wildcards taken as text; the counts of every status. The
// control: an unknown status is refused.
func TestPetitionQuery(t *testing.T) {
	db := petitionDB(t)
	now := time.Now()
	gm := GMEdit{AccountID: 1, Username: "gm", Via: "web"}
	file := func(id uint32, name, text string) Petition {
		p, err := db.FilePetition(Petition{ServerID: id, CharName: name, Text: text}, now)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := file(1, "Ann", "stuck under Freeport")
	b := file(2, "Bob", "100% stuck_here")
	c := file(1, "Ann", "a bug")
	if a.ID != 1001 || b.ID != 1002 || c.ID != 1003 {
		t.Fatalf("numbers %d %d %d", a.ID, b.ID, c.ID)
	}
	if _, _, err := db.AnswerPetition(b.ID, "", "Look again.", gm, now); err != nil {
		t.Fatal(err)
	}
	if err := db.PlayerDelete(1, c.ID, now); err != nil {
		t.Fatal(err)
	}
	ids := func(q PetitionQuery) []int64 {
		t.Helper()
		l, err := db.Petitions(q)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, p := range l.Petitions {
			out = append(out, p.ID)
		}
		return out
	}
	for _, c := range []struct {
		q    PetitionQuery
		want string
	}{
		{PetitionQuery{}, "[1002 1001]"},
		{PetitionQuery{Status: "waiting"}, "[1001]"},
		{PetitionQuery{Status: PetitionDeleted}, "[1003]"},
		{PetitionQuery{Status: "all"}, "[1003 1002 1001]"},
		{PetitionQuery{Status: "all", ServerID: 1}, "[1003 1001]"},
		{PetitionQuery{Status: "all", Search: "FREEPORT"}, "[1001]"},
		{PetitionQuery{Status: "all", Search: "bob"}, "[1002]"},
		{PetitionQuery{Status: "all", Search: "%"}, "[1002]"},
		{PetitionQuery{Status: "all", Search: "k_h"}, "[1002]"},
		{PetitionQuery{Status: "all", Limit: 1, Offset: 1}, "[1002]"},
	} {
		if got := ids(c.q); fmt.Sprint(got) != c.want {
			t.Errorf("%+v: %v, want %s", c.q, got, c.want)
		}
	}
	l, _ := db.Petitions(PetitionQuery{})
	if l.Counts[PetitionOpen] != 2 || l.Counts[PetitionDeleted] != 1 || l.Counts["waiting"] != 1 || l.Matching != 2 {
		t.Errorf("counts %v, matching %d", l.Counts, l.Matching)
	}
	if _, err := db.Petitions(PetitionQuery{Status: "pending"}); err == nil {
		t.Error("an unknown status taken")
	}
}

// Feature requests (Bug Report / Feature Request, petition 1841) listed apart
// from the rest: feature alone, other all but them, all (and "") every one.
// The controls: another category's subcategory 100 and Bug Report's other
// topics are not feature requests; an unknown kind is refused.
func TestPetitionKindFilter(t *testing.T) {
	db := petitionDB(t)
	now := time.Now()
	for _, p := range []Petition{
		{ServerID: 1, CharName: "Ann", Category: FeatureRequestCategory, Subcategory: FeatureRequestSubcategory, Text: "a /lfg window"},
		{ServerID: 1, CharName: "Ann", Category: FeatureRequestCategory, Subcategory: 12, Text: "a quest bug"},
		{ServerID: 2, CharName: "Bob", Category: 2, Subcategory: FeatureRequestSubcategory, Text: "stuck"},
		{ServerID: 2, CharName: "Bob", Category: FeatureRequestCategory, Subcategory: FeatureRequestSubcategory, Text: "ability quests coloured"},
	} {
		if _, err := db.FilePetition(p, now); err != nil {
			t.Fatal(err)
		}
	}
	for kind, want := range map[string]string{"feature": "[1004 1001]", "other": "[1003 1002]", "all": "[1004 1003 1002 1001]", "": "[1004 1003 1002 1001]"} {
		l, err := db.Petitions(PetitionQuery{Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, p := range l.Petitions {
			got = append(got, p.ID)
		}
		if fmt.Sprint(got) != want || l.Matching != len(got) {
			t.Errorf("kind %q: %v (matching %d), want %s", kind, got, l.Matching, want)
		}
	}
	if l, _ := db.Petitions(PetitionQuery{Kind: "feature", Search: "lfg"}); len(l.Petitions) != 1 {
		t.Errorf("feature and a search: %d", len(l.Petitions))
	}
	if _, err := db.Petitions(PetitionQuery{Kind: "bugs"}); err == nil {
		t.Error("an unknown kind taken")
	}
}

// The bounds and the refusals: a player's request on another character's
// petition or a deleted one is ErrPetitionNotFound, on a closed one
// ErrPetitionClosed; a reply past PetitionMaxText is ErrPetitionTooLong; a GM's
// answer to a closed one, an empty or too long answer, and a close of a
// closed one are refused; a close with the answer unread needs force. Text is
// kept with its line ends and without other control characters.
func TestPetitionRefusals(t *testing.T) {
	db := petitionDB(t)
	now := time.Now()
	gm := GMEdit{AccountID: 1, Username: "gm", Via: "web"}
	p, err := db.FilePetition(Petition{ServerID: 1, CharName: "Ann", Text: "line one\r\nline\x00 two\x1b "}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Text != "line one\nline two" {
		t.Errorf("kept as %q", p.Text)
	}
	if err := db.PlayerReply(2, p.ID, "Bob", "mine?", now); !errors.Is(err, ErrPetitionNotFound) {
		t.Errorf("another character's: %v", err)
	}
	if err := db.PlayerReply(1, p.ID, "Ann", strings.Repeat("x", PetitionMaxText), now); !errors.Is(err, ErrPetitionTooLong) {
		t.Errorf("a reply past the bound: %v", err)
	}
	for _, bad := range []string{"", strings.Repeat("y", PetitionMaxAnswer+1)} {
		if _, _, err := db.AnswerPetition(p.ID, "", bad, gm, now); err == nil {
			t.Errorf("an answer of %d characters taken", len(bad))
		}
	}
	if _, _, err := db.AnswerPetition(p.ID, strings.Repeat("n", PetitionMaxSigned+1), "ok", gm, now); err == nil {
		t.Error("a signature past the bound taken")
	}
	ans, audit, err := db.AnswerPetition(p.ID, "  ", "Fixed.", gm, now)
	if err != nil || audit == 0 || !strings.HasSuffix(ans.Text, "\n\nGM: Fixed.") {
		t.Fatalf("answered %+v %d %v", ans, audit, err)
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM gm_audit WHERE tbl = 'petitions' AND action = 'update'`).Scan(&n)
	if n != 1 {
		t.Errorf("%d audit lines", n)
	}
	if _, _, err := db.ClosePetition(p.ID, gm, false, now); err == nil {
		t.Fatal("closed with the answer unread")
	}
	if _, _, err := db.ClosePetition(p.ID, gm, true, now); err != nil {
		t.Fatalf("force: %v", err)
	}
	if _, _, err := db.ClosePetition(p.ID, gm, true, now); err == nil {
		t.Error("closed twice")
	}
	if _, _, err := db.AnswerPetition(p.ID, "", "more", gm, now); err == nil {
		t.Error("a closed one answered")
	}
	for name, err := range map[string]error{
		"reply":  db.PlayerReply(1, p.ID, "Ann", "?", now),
		"delete": db.PlayerDelete(1, p.ID, now),
	} {
		if !errors.Is(err, ErrPetitionClosed) {
			t.Errorf("%s of a closed one: %v", name, err)
		}
	}
	if got, err := db.PlayerRead(1, p.ID); !errors.Is(err, ErrPetitionClosed) || got.Text == "" {
		t.Errorf("read of a closed one: %v, its text with it %q", err, got.Text)
	}
	if err := PetitionProblem("answered", ""); err == nil {
		t.Error("a status the table lacks")
	}
	if err := PetitionProblem(PetitionClosed, "ok"); err != nil {
		t.Errorf("a good row: %v", err)
	}
}

// The inbox lists PetitionsListed of a character's petitions, fewer than the
// 100 rows with which CLIENT refuses to file (CheckFull): the most recently
// changed, oldest first, so a GM's answer to an old one is listed. Filing goes
// on past them, up to PetitionsKept. The control: another character's are not
// listed.
func TestTheInboxListsTheMostRecentlyChanged(t *testing.T) {
	db := petitionDB(t)
	at := time.Date(2026, 11, 10, 4, 0, 0, 0, time.UTC)
	gm := GMEdit{AccountID: 1, Username: "gm", Via: "web"}
	for i := 0; i < 120; i++ {
		if _, err := db.FilePetition(Petition{ServerID: 1, CharName: "Ann", Text: fmt.Sprintf("ticket %d", i)}, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("ticket %d: %v", i, err)
		}
	}
	if _, err := db.FilePetition(Petition{ServerID: 2, CharName: "Bob", Text: "his"}, at); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AnswerPetition(1001, "", "Fixed.", gm, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	l, err := db.PlayerPetitions(1, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != PetitionsListed || PetitionsListed >= 100 {
		t.Fatalf("%d listed (PetitionsListed %d): CLIENT files none with 100 held", len(l), PetitionsListed)
	}
	if l[0].ID != 1001 || l[1].ID != 1023 || l[len(l)-1].ID != 1120 {
		t.Fatalf("listed %d, %d .. %d: want the answered 1001, then 1023 .. 1120", l[0].ID, l[1].ID, l[len(l)-1].ID)
	}
	for _, p := range l {
		if p.ServerID != 1 {
			t.Fatalf("another character's %d listed", p.ID)
		}
	}
}
