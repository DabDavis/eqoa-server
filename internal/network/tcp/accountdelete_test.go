package tcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// The account page never deletes a character still in the game (InWorld: the
// user, 2026-11-08, "add the delete guard for characters in another window"):
// refused with the reason, the row kept. The control: once out of it, deleted.
func TestTheAccountPageNeverDeletesACharacterInTheWorld(t *testing.T) {
	s, mine, _, token := teleTestServer(t)
	chars := database.NewSQLiteCharacterStore(s.db)
	inWorld := true
	s.InWorld = func(serverID uint32) bool { return serverID == mine && inWorld }
	post := func() map[string]interface{} {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/characters/delete", strings.NewReader(`{"id":`+itoa(int(mine))+`}`))
		req.AddCookie(&http.Cookie{Name: "session", Value: token})
		rec := httptest.NewRecorder()
		s.handleAPIDeleteCharacter(rec, req)
		var out map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
		}
		return out
	}
	out := post()
	if out["success"] == true || !strings.Contains(out["error"].(string), "still in the world") {
		t.Fatalf("in the world: %v", out)
	}
	if c, err := chars.GetByServerID(mine); err != nil || c == nil {
		t.Fatalf("the row went: %v %v", c, err)
	}
	inWorld = false
	if out = post(); out["success"] != true {
		t.Fatalf("out of the world: %v", out)
	}
	if c, err := chars.GetByServerID(mine); err == nil && c != nil {
		t.Fatal("out of the world: not deleted")
	}
}
