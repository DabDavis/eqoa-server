package voice

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// The lobby without media: sessions joined, rooms made and joined.

// roomH is a client's H for a room's name and password (PROTOCOL.md).
func roomH(name, password string) string {
	sum := sha256.Sum256([]byte("eqoa-voice-room\n" + strings.ToLower(strings.TrimSpace(name)) + "\n" + password))
	return hex.EncodeToString(sum[:])
}

type lobbyClock struct{ t time.Time }

func (c *lobbyClock) now() time.Time { return c.t }

func newLobbyServer(t *testing.T) (*Server, *lobbyClock) {
	t.Helper()
	s, err := New(Config{BindIP: "127.0.0.1", PublicIP: "127.0.0.1",
		RefuseRoom: func(name string) bool { return strings.Contains(strings.ToLower(name), "badword") }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	clk := &lobbyClock{time.Unix(1_800_000_000, 0)}
	s.clock = clk.now
	return s, clk
}

func lobbyOf(s *Server) map[string]Room {
	out := map[string]Room{}
	for _, r := range s.Lobby() {
		out[r.Name] = r
	}
	return out
}

// An open room is made by its first join and joined by the next, the name
// matched without its case and spaces; a locked room refuses a wrong H (and
// none) and takes the right one; the lobby lists both with their members,
// most first.
func TestRoomsOpenAndLocked(t *testing.T) {
	s, clk := newLobbyServer(t)
	var ids []uint32
	for i := 0; i < 4; i++ {
		ids = append(ids, s.Join("P", "", newTestSink()))
	}
	if r, n := s.JoinRoom(ids[0], "  Freeport Docks ", "", false); r != RoomIn || n != "Freeport Docks" {
		t.Fatalf("making an open room: %d %q", r, n)
	}
	if r, n := s.JoinRoom(ids[1], "freeport docks", "", false); r != RoomIn || n != "Freeport Docks" {
		t.Fatalf("joining it: %d %q", r, n)
	}
	h := roomH("Guildhall", "hunter2")
	if r, _ := s.JoinRoom(ids[2], "Guildhall", h, false); r != RoomIn {
		t.Fatalf("making a locked room: %d", r)
	}
	if r, _ := s.JoinRoom(ids[3], "Guildhall", roomH("Guildhall", "hunter3"), false); r != RoomWrongPassword {
		t.Fatalf("a wrong password: %d", r)
	}
	if r, _ := s.JoinRoom(ids[3], "Guildhall", "", false); r != RoomWrongPassword {
		t.Fatalf("no password: %d", r)
	}
	if r, _ := s.JoinRoom(ids[3], "Freeport Docks", h, false); r != RoomWrongPassword {
		t.Fatalf("a password to an open room: %d", r)
	}
	if got := lobbyOf(s)["Guildhall"]; got.Members != 1 || !got.Locked {
		t.Fatalf("refused joins changed the room: %+v", got)
	}
	if r, _ := s.JoinRoom(ids[3], "GUILDHALL", h, false); r != RoomWrongPassword {
		t.Fatalf("the right password within 2 s of a wrong one was checked: %d", r)
	}
	clk.t = clk.t.Add(roomRetry)
	if r, _ := s.JoinRoom(ids[3], "GUILDHALL", h, false); r != RoomIn {
		t.Fatalf("the right password: %d", r)
	}
	lb := s.Lobby()
	if len(lb) != 2 || lb[0] != (Room{"Freeport Docks", 2, false}) && lb[0] != (Room{"Guildhall", 2, true}) {
		t.Fatalf("the lobby %+v", lb)
	}
	byName := lobbyOf(s)
	if byName["Freeport Docks"] != (Room{"Freeport Docks", 2, false}) || byName["Guildhall"] != (Room{"Guildhall", 2, true}) {
		t.Fatalf("the lobby %+v", byName)
	}
	if _, ok := s.LeaveRoom(ids[0]); !ok {
		t.Fatal("leaving")
	}
	if lb := s.Lobby(); lb[0].Name != "Guildhall" || lb[1].Members != 1 {
		t.Fatalf("most members first: %+v", lb)
	}
}

// A room lives while anyone is in it and 60 s after: its lock still holds
// in that minute (a wrong H refused), then the name is free to make anew.
func TestAnEmptyRoomGoesAfterAMinute(t *testing.T) {
	s, clk := newLobbyServer(t)
	a, b := s.Join("A", "", newTestSink()), s.Join("B", "", newTestSink())
	h := roomH("Camp", "pw")
	s.JoinRoom(a, "Camp", h, false)
	s.LeaveRoom(a)
	clk.t = clk.t.Add(59 * time.Second)
	if got, ok := lobbyOf(s)["Camp"]; !ok || got.Members != 0 {
		t.Fatalf("59 s after it emptied: %+v %v", got, ok)
	}
	if r, _ := s.JoinRoom(b, "Camp", "", false); r != RoomWrongPassword {
		t.Fatalf("its lock in its last minute: %d", r)
	}
	clk.t = clk.t.Add(2 * time.Second)
	if _, ok := lobbyOf(s)["Camp"]; ok {
		t.Fatal("61 s after it emptied it is still listed")
	}
	if r, _ := s.JoinRoom(b, "Camp", "", false); r != RoomIn {
		t.Fatalf("the name made anew, open: %d", r)
	}
	if got := lobbyOf(s)["Camp"]; got.Locked || got.Members != 1 {
		t.Fatalf("the new room %+v", got)
	}
	// a session leaving the voice leaves its room
	s.Leave(b, false)
	if got := lobbyOf(s)["Camp"]; got.Members != 0 {
		t.Fatalf("after its one member left the voice: %+v", got)
	}
}

// A name the profanity filter refuses, or of 2 or 25 characters or a
// control character, makes no room; 3 and 24 do (the control).
func TestARoomNameIsChecked(t *testing.T) {
	s, _ := newLobbyServer(t)
	a := s.Join("A", "", newTestSink())
	for _, n := range []string{"the badword room", "ab", strings.Repeat("x", 25), "tab\there", "   "} {
		if r, _ := s.JoinRoom(a, n, "", false); r != RoomRefused {
			t.Errorf("%q: %d, want refused", n, r)
		}
	}
	if len(s.Lobby()) != 0 {
		t.Fatalf("refused names made rooms: %+v", s.Lobby())
	}
	for _, n := range []string{"abc", strings.Repeat("é", 24)} {
		if r, _ := s.JoinRoom(a, n, "", false); r != RoomIn {
			t.Errorf("%q: %d, want in", n, r)
		}
	}
	if r, _ := s.JoinRoom(a, "Fine Room", "nothex", false); r != RoomWrongPassword {
		t.Fatalf("a malformed H: %d", r)
	}
}

// An unlisted room is in no list, its members' neither, and a join by its
// name finds it; a later join's flag changes nothing (a listed room stays
// listed, an unlisted one unlisted).
func TestAnUnlistedRoomIsFoundByNameOnly(t *testing.T) {
	s, _ := newLobbyServer(t)
	a, b, c := s.Join("A", "", newTestSink()), s.Join("B", "", newTestSink()), s.Join("C", "", newTestSink())
	if r, _ := s.JoinRoom(a, "Hidden Cove", "", true); r != RoomIn {
		t.Fatalf("making it: %d", r)
	}
	if _, ok := lobbyOf(s)["Hidden Cove"]; ok {
		t.Fatal("an unlisted room is listed")
	}
	if r, _ := s.JoinRoom(b, "hidden cove", "", false); r != RoomIn {
		t.Fatalf("joining it by name: %d", r)
	}
	if _, ok := lobbyOf(s)["Hidden Cove"]; ok {
		t.Fatal("a later join's flag listed it")
	}
	if r, _ := s.JoinRoom(c, "Open Square", "", false); r != RoomIn {
		t.Fatal(r)
	}
	if r, _ := s.JoinRoom(a, "Open Square", "", true); r != RoomIn {
		t.Fatal(r)
	}
	if got, ok := lobbyOf(s)["Open Square"]; !ok || got.Members != 2 {
		t.Fatalf("a later join's flag unlisted a listed room: %+v %v", got, ok)
	}
}

// A roster: each other session in a channel of its, bit 0 the group, bit 1
// the room, groupmates first; leaving the voice empties its own.
func TestRosters(t *testing.T) {
	s, _ := newLobbyServer(t)
	ka, kb, kc, kd := newTestSink(), newTestSink(), newTestSink(), newTestSink()
	a := s.Join("Asker", "g:1", ka)
	b := s.Join("Zed", "g:1", kb) // a groupmate, listed first though last by name
	c := s.Join("Crier", "g:2", kc)
	d := s.Join("Dancer (demo)", "d:0123456789abcdef0123456789abcdef", kd)
	s.JoinRoom(a, "Tavern", "", false)
	s.JoinRoom(b, "Tavern", "", false)
	s.JoinRoom(c, "Tavern", "", false)
	s.JoinRoom(d, "Tavern", "", false)
	want := []Member{{b, "Zed", 3}, {c, "Crier", 2}, {d, "Dancer (demo)", 2}}
	if got := ka.members(); !equalMembers(got, want) {
		t.Fatalf("the asker's roster %+v, want %+v", got, want)
	}
	s.LeaveRoom(c)
	if got := ka.members(); !equalMembers(got, []Member{{b, "Zed", 3}, {d, "Dancer (demo)", 2}}) {
		t.Fatalf("one gone from the room: %+v", got)
	}
	if got := kc.members(); len(got) != 0 {
		t.Fatalf("out of the room, its own group of one: %+v", got)
	}
	s.SetGroup(b, "")
	if got := ka.members(); !equalMembers(got, []Member{{d, "Dancer (demo)", 2}, {b, "Zed", 2}}) {
		t.Fatalf("the groupmate left the group: %+v", got)
	}
	s.Leave(a, false)
	if got := ka.members(); len(got) != 0 {
		t.Fatalf("its voice off, its roster %+v", got)
	}
}

func equalMembers(a, b []Member) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
