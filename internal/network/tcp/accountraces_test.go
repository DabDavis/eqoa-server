package tcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The account page makes a character of a race past the game's ten only where
// creation takes it (main.go wires handlers.NewCharRace when world.open_races or
// world.all_classes is on): in the body the rule names; refused otherwise, and
// refused by the rule's own word. A race of the game's is the page's own, as before.
func TestTheAccountPageTakesExtraRacesByItsRule(t *testing.T) {
	db, err := database.NewSQLite(t.TempDir() + "/races.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewAccountServer(db, 0, 4)
	acct, err := database.NewSQLiteAccountStore(db).Create("maker", "x")
	if err != nil {
		t.Fatal(err)
	}
	token := s.createSession(httptest.NewRecorder(), acct.ID, "maker")
	create := func(name string, race, class int) map[string]interface{} {
		t.Helper()
		body := fmt.Sprintf(`{"name":%q,"race":%d,"class":%d,"gender":1}`, name, race, class)
		req := httptest.NewRequest(http.MethodPost, "/api/characters/create", strings.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "session", Value: token})
		rec := httptest.NewRecorder()
		s.handleAPICreateCharacter(rec, req)
		var out map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s: %v (%s)", name, err, rec.Body.String())
		}
		return out
	}
	chars := database.NewSQLiteCharacterStore(db)
	if out := create("Nokitty", 12, 0); out["success"] != false {
		t.Fatalf("no rule: a race past ten was made: %v", out)
	}
	s.SetExtraRaces(func(race, class, sex int) (int64, string) {
		if race == 12 && class != 13 {
			return 0x22176258 + int64(sex), ""
		}
		return 0, "not this one"
	})
	if out := create("Kitty", 12, 0); out["success"] != true {
		t.Fatalf("with the rule: %v", out)
	}
	if c, err := chars.GetByName("Kitty"); err != nil || c.Race != 12 || c.ModelID != 0x22176259 {
		t.Fatalf("the cat: %+v %v", c, err)
	}
	if out := create("Wizkitty", 12, 13); out["success"] != false {
		t.Fatalf("the rule refused it, the page made it: %v", out)
	}
	// the control: a race of the game's is the page's own
	if out := create("Plainman", 0, 13); out["success"] != true {
		t.Fatalf("a human: %v", out)
	}
	if c, _ := chars.GetByName("Plainman"); c == nil || c.ModelID != 1893243078 {
		t.Fatalf("the human's body: %+v", c)
	}
}
