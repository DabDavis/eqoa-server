package entity

import (
	"bytes"
	"net"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// socialPair is two players in a world, each with a session the manager
// knows (sessionOfLocked walks the manager).
func socialPair(t *testing.T) (*World, *session.Session, *session.Session) {
	w := NewWorld()
	w.SessionManager = session.NewManager()
	mk := func(port int, id uint32, name string) *session.Session {
		s := w.SessionManager.CreateSession(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}, uint32(port), uint32(port), 1, 1, true)
		w.CreatePlayer(s, &database.CharacterRecord{ServerID: id, CharName: name, Level: 3, TClass: 6, Race: 2, X: 100, Y: 54, Z: 100})
		return s
	}
	return w, mk(20001, 7, "Asker"), mk(20002, 40, "Friend")
}

// sent is each message queued on a session since the last call: the opcode
// and the bytes after it.
func sent(s *session.Session) [][]byte {
	var out [][]byte
	for _, m := range s.Queue().GetPending() {
		out = append(out, append([]byte(nil), m.Data()...))
	}
	return out
}

func TestBuddyRequestAcceptAndRefuse(t *testing.T) {
	w, a, b := socialPair(t)
	// a name nobody has: 0x0771 type 5 back to the asker
	w.BuddyAdd(a, "Nobody")
	got := sent(a)
	if len(got) != 1 || !bytes.Equal(got[0][:3], []byte{0x71, 0x07, 5}) {
		t.Fatalf("an unknown name: %x", got)
	}
	// the request goes to the player named, any case, with the asker's id
	w.BuddyAdd(a, "friend")
	got = sent(b)
	want := []byte{0x6F, 0x07, 7, 0, 0, 0, 5, 0, 0, 0, 'A', 's', 'k', 'e', 'r'}
	if len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("the request: %x, want %x", got, want)
	}
	// an answer to a request that was never made does nothing
	w.BuddyAnswer(a, 40, true, 0)
	if n := len(sent(a)) + len(sent(b)); n != 0 {
		t.Fatalf("an unasked answer sent %d messages", n)
	}
	// accepted: each gets the other's record, the asker word 557's 0x0770
	w.BuddyAnswer(b, 7, true, 0)
	ga, gb := sent(a), sent(b)
	if len(ga) != 2 || len(gb) != 1 {
		t.Fatalf("after the accept: %x / %x", ga, gb)
	}
	// zigzag 40, "Friend", zigzag class 6, race 2, level 3, online, no guild
	rec := []byte{0xD5, 0x03, 80, 6, 0, 0, 0, 'F', 'r', 'i', 'e', 'n', 'd', 12, 4, 6, 1, 0, 0, 0, 0}
	if !bytes.Equal(ga[0], rec) || ga[1][0] != 0x70 {
		t.Errorf("the asker got %x, want %x then 0x0770", ga, rec)
	}
	if gb[0][2] != 14 {
		t.Errorf("the friend's record is not the asker's (id byte %d)", gb[0][2])
	}
	// the list again at a world entry
	if n := w.SendBuddyList(a); n != 1 || !bytes.Equal(sent(a)[0], rec) {
		t.Errorf("the list at the world entry: %d", n)
	}
	// a second request refused as busy: 0x0771 with the reason, the name
	w.BuddyAdd(a, "Friend")
	sent(b)
	w.BuddyAnswer(b, 7, false, 1)
	got = sent(a)
	if len(got) != 1 || !bytes.Equal(got[0][:3], []byte{0x71, 0x07, 1}) {
		t.Errorf("the refusal: %x", got)
	}
	// removed: 0x03D6 with the zigzag id, and gone from the next list
	w.BuddyRemove(a, 40)
	if got = sent(a); len(got) != 1 || !bytes.Equal(got[0], []byte{0xD6, 0x03, 80}) {
		t.Errorf("the removal: %x", got)
	}
	if n := w.SendBuddyList(a); n != 0 {
		t.Errorf("%d buddies left", n)
	}
}

func TestIgnoreAddAndRemove(t *testing.T) {
	w, a, _ := socialPair(t)
	w.IgnoreAdd(a, "friend") // online: the name as the player has it
	w.IgnoreAdd(a, "FRIEND") // twice: nothing
	w.IgnoreAdd(a, "asker")  // oneself: nothing
	w.IgnoreAdd(a, "Gone")   // offline: kept as typed
	got := sent(a)
	if len(got) != 2 || !bytes.Equal(got[0], []byte{0x03, 0x10, 6, 0, 0, 0, 'F', 'r', 'i', 'e', 'n', 'd', 1, 0, 0, 0}) || got[1][len(got[1])-4] != 2 {
		t.Fatalf("adds: %x", got)
	}
	names, ids := w.IgnoreList(a)
	if len(names) != 2 || names[1] != "Gone" || ids[1] != 2 {
		t.Errorf("the list: %v %v", names, ids)
	}
	w.IgnoreRemove(a, 9) // not there: nothing
	w.IgnoreRemove(a, 1)
	if got = sent(a); len(got) != 1 || !bytes.Equal(got[0], []byte{0x04, 0x10, 1, 0, 0, 0}) {
		t.Errorf("the removal: %x", got)
	}
	if names, _ = w.IgnoreList(a); len(names) != 1 || names[0] != "Gone" {
		t.Errorf("left: %v", names)
	}
}
