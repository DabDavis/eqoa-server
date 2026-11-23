package entity

import (
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Heirloom petition 2817 (2026-11-23 23:24 keep restart): a character whose
// serverID is an NPC row's id is given a free entity id, and a later login of
// the character whose serverID that free id is took the first for its own
// ghost and removed it. The first session kept its Player, but its dump
// (PlayerSnapshot) and its moves (ApplyClientUpdate), looked up by id, were
// the second character's: its client was sent the other character's dump, in
// its world, and drove it. Now each keeps its own Player under its own id.
func TestALoginNeverTakesAnotherCharactersFreeID(t *testing.T) {
	w := NewWorld()
	sm := session.NewManager()
	w.SessionManager = sm
	open := func(port int, inst uint32, acct int) *session.Session {
		s := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(198, 51, 100, 9), Port: port}, inst, inst, uint16(inst), 2, true)
		s.SetAccountID(acct)
		s.SetInGame(true)
		s.SetCharacterInWorld(true)
		return s
	}
	const rowID = 2533
	w.NPCs[rowID] = &NPC{ServerID: rowID, Name: "a creature on a low row", HP: 100, MaxHP: 100, Level: 10, X: 100, Z: 100}

	a := open(6401, 41, 7)
	pa := w.CreatePlayer(a, &database.CharacterRecord{ServerID: rowID, CharName: "Firstchar", AccountID: 7, Level: 22,
		World: 0, X: 12635, Y: 107, Z: 18708})
	if pa == nil || pa.ServerID == rowID {
		t.Fatalf("first: %+v, want a free id past the NPC row", pa)
	}
	freeID := pa.ServerID

	b := open(6402, 42, 8)
	pb := w.CreatePlayer(b, &database.CharacterRecord{ServerID: uint32(freeID), CharName: "Secondchar", AccountID: 8, Level: 4,
		World: 10, X: 6828, Y: 110, Z: 11624})
	if pb == nil {
		t.Fatal("second: no player")
	}
	if got := w.Players[freeID]; got != pa {
		name := "no one"
		if got != nil {
			name = got.Name
		}
		t.Fatalf("id %d is %s's, want the first character still on it", freeID, name)
	}
	if pb.ServerID == freeID || w.Players[pb.ServerID] != pb {
		t.Fatalf("second on id %d, want its own id", pb.ServerID)
	}
	if w.SessionPlayer(a) != pa || w.SessionPlayer(b) != pb {
		t.Fatal("sessions' players swapped")
	}
	// the first's dump is its own: its world and place
	if snap, ok := w.PlayerSnapshot(w.SessionPlayer(a).ServerID); !ok || snap.Name != "Firstchar" || snap.World != 0 {
		t.Fatalf("first's dump: %q world %d (%v)", snap.Name, snap.World, ok)
	}

	// the control: a character on its own serverID logging in again still
	// replaces its ghost
	c := open(6403, 43, 9)
	pc := w.CreatePlayer(c, &database.CharacterRecord{ServerID: 77, CharName: "Thirdchar", AccountID: 9, Level: 4})
	c2 := open(6404, 44, 9)
	pc2 := w.CreatePlayer(c2, &database.CharacterRecord{ServerID: 77, CharName: "Thirdchar", AccountID: 9, Level: 4})
	if pc.ServerID != 77 || pc2.ServerID != 77 || w.Players[77] != pc2 {
		t.Fatalf("relogin: %d then %d, id 77 not the second login's", pc.ServerID, pc2.ServerID)
	}
}
