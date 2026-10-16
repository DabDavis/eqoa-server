package tcp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// teleTestServer builds an account server backed by a scratch database, with one
// account holding one character and a second account holding another. Two
// accounts is the point: the interesting question is not whether teleport works
// but whether it works on somebody else's character.
func teleTestServer(t *testing.T) (*AccountServer, uint32, uint32, string) {
	t.Helper()
	db, err := database.NewSQLite(t.TempDir() + "/tele.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	s := NewAccountServer(db, 0, 4)

	accounts := database.NewSQLiteAccountStore(db)
	mine, err := accounts.Create("mine", "x")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := accounts.Create("theirs", "x")
	if err != nil {
		t.Fatal(err)
	}

	chars := database.NewSQLiteCharacterStore(db)
	mk := func(name string, account int) uint32 {
		c := &database.CharacterRecord{
			CharName: name, AccountID: account, ModelID: 1,
			Level: 1, World: 0, X: 25000, Y: 54, Z: 15000,
		}
		if err := chars.Create(c); err != nil {
			t.Fatal(err)
		}
		return c.ServerID
	}
	myChar := mk("Mine", mine.ID)
	theirChar := mk("Theirs", theirs.ID)

	// A logged-in web session for "mine".
	rec := httptest.NewRecorder()
	token := s.createSession(rec, mine.ID, "mine")

	s.WorldNames = func() []string { return []string{"Tunaria"} }
	s.Destinations = func() []TeleportDest {
		return []TeleportDest{{Name: "freeport", Label: "Freeport", World: 0}}
	}
	s.ResolveDest = func(name string) (float32, float32, float32, byte, string, bool) {
		if name == "freeport" {
			return 25273, 54, 15719, 0, "Freeport", true
		}
		return 0, 0, 0, 0, "", false
	}
	s.ValidatePlace = func(x, y, z float32, world byte) (byte, error) {
		if y <= -1000 {
			return world, errors.New("that is a fall, not a place to stand")
		}
		return world, nil
	}
	return s, myChar, theirChar, token
}

func postTeleport(t *testing.T, s *AccountServer, token, body string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/characters/teleport",
		strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rec := httptest.NewRecorder()
	s.handleAPITeleport(rec, req)

	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON %q: %v", rec.Body.String(), err)
	}
	return out
}

// A serverID is a number a browser can type, so the route has to prove the
// character belongs to the session before it moves anything. Without this, one
// logged-in account could scatter every character on the server.
func TestTeleportRefusesAnotherAccountsCharacter(t *testing.T) {
	s, _, theirChar, token := teleTestServer(t)

	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(theirChar))+`,"dest":"freeport"}`)

	if out["success"] == true {
		t.Fatal("moved a character belonging to another account")
	}

	// And it must not have been written anyway.
	rec, err := database.NewSQLiteCharacterStore(s.db).GetByServerID(theirChar)
	if err != nil {
		t.Fatal(err)
	}
	if rec.X != 25000 || rec.Z != 15000 {
		t.Errorf("the refused character was moved anyway: (%.0f, %.0f)", rec.X, rec.Z)
	}
}

func TestTeleportRequiresLogin(t *testing.T) {
	s, myChar, _, _ := teleTestServer(t)
	out := postTeleport(t, s, "not-a-real-token",
		`{"serverid":`+itoa(int(myChar))+`,"dest":"freeport"}`)
	if out["success"] == true {
		t.Error("teleported without a session")
	}
}

// An offline character is moved by writing the row, and the reply says so —
// "moved" means something different for a row that takes effect at next login
// than for a character standing in the world, and a player who is not told will
// go looking for the wrong one.
func TestTeleportWritesTheRowWhenOffline(t *testing.T) {
	s, myChar, _, token := teleTestServer(t)
	s.TeleportLive = func(uint32, float32, float32, float32, byte) bool { return false }

	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(myChar))+`,"dest":"freeport"}`)

	if out["success"] != true {
		t.Fatalf("teleport failed: %v", out["error"])
	}
	if out["online"] != false {
		t.Error("an offline character was reported as moved in game")
	}
	rec, err := database.NewSQLiteCharacterStore(s.db).GetByServerID(myChar)
	if err != nil {
		t.Fatal(err)
	}
	if int(rec.X) != 25273 || int(rec.Z) != 15719 {
		t.Errorf("row not updated: (%.0f, %.0f)", rec.X, rec.Z)
	}
}

// An ONLINE character must be moved in the world and NOT written to the
// database. Its position lives in memory and is saved on disconnect, so a row
// written underneath it is silently undone by that player's own logout — the
// teleport would appear to work and then quietly reverse itself.
func TestTeleportLeavesTheRowAloneWhenOnline(t *testing.T) {
	s, myChar, _, token := teleTestServer(t)
	var moved bool
	s.TeleportLive = func(id uint32, x, y, z float32, w byte) bool {
		moved = id == myChar
		return moved
	}

	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(myChar))+`,"dest":"freeport"}`)

	if out["success"] != true || out["online"] != true {
		t.Fatalf("expected an in-game move, got %v", out)
	}
	if !moved {
		t.Error("the live path was not asked to move this character")
	}
	rec, err := database.NewSQLiteCharacterStore(s.db).GetByServerID(myChar)
	if err != nil {
		t.Fatal(err)
	}
	if rec.X != 25000 {
		t.Errorf("the row was written for an online character (x=%.0f); the "+
			"player's own logout would overwrite it", rec.X)
	}
}

// Typed coordinates go through the same guards the disconnect path applies. A
// web form can produce a fall by typing, and saving one strands the character at
// the world floor permanently.
func TestTeleportValidatesTypedCoordinates(t *testing.T) {
	s, myChar, _, token := teleTestServer(t)
	s.TeleportLive = func(uint32, float32, float32, float32, byte) bool { return false }

	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(myChar))+`,"x":1,"y":-1000,"z":1,"world":0}`)

	if out["success"] == true {
		t.Fatal("accepted a position at the world floor")
	}
	rec, _ := database.NewSQLiteCharacterStore(s.db).GetByServerID(myChar)
	if rec.Y != 54 {
		t.Errorf("the refused position was written anyway: y=%.0f", rec.Y)
	}
}

func TestTeleportRejectsUnknownDestination(t *testing.T) {
	s, myChar, _, token := teleTestServer(t)
	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(myChar))+`,"dest":"atlantis"}`)
	if out["success"] == true {
		t.Error("accepted a destination the game does not have")
	}
}

// With no game world wired in there is no way to check a placement, so the route
// refuses rather than writing a row it could not validate.
func TestTeleportDisabledWithoutAGameWorld(t *testing.T) {
	s, myChar, _, token := teleTestServer(t)
	s.ResolveDest, s.ValidatePlace = nil, nil

	out := postTeleport(t, s, token,
		`{"serverid":`+itoa(int(myChar))+`,"dest":"freeport"}`)
	if out["success"] == true {
		t.Error("teleported on a server with no world attached")
	}
}
