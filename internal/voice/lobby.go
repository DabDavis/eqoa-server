package voice

import (
	"crypto/subtle"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The lobby: rooms anyone makes, by name (the user's "a free voice chat
// lobby, anyone can make a room"). A room is a channel, "r:" + its key (the
// name trimmed and case-folded), beside a session's group; one room a
// session. The first to join a name that has no room makes it, open, or
// locked with the H its client brought (a hash of the name and a password
// the server never sees), and unlisted when it asked; a later join must
// bring the same H. A room lives while anyone is in it and roomGrace after.

const (
	RoomIn            = 0 // in the room
	RoomWrongPassword = 1
	RoomRefused       = 2 // the name: its length, its characters, the profanity filter
	RoomLeft          = 3

	roomNameMin = 3
	roomNameMax = 24
	roomGrace   = 60 * time.Second
	lobbyMax    = 200

	// roomRetry is how long a session that brought a wrong H waits before
	// its next join to a locked room is checked at all: a guess every 2 s
	roomRetry = 2 * time.Second
)

// room is one room of the lobby.
type room struct {
	name       string // as its maker spelled it
	key        string
	hash       string // the lock's H; "" open (never logged)
	unlisted   bool
	members    int
	emptySince time.Time
}

// Room is a room as the lobby lists it.
type Room struct {
	Name    string
	Members int
	Locked  bool
}

// RoomName is a room's name as a join gives it, trimmed, and false when it
// is refused: 3 to 24 printable characters.
func RoomName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if !utf8.ValidString(name) || n < roomNameMin || n > roomNameMax {
		return name, false
	}
	for _, r := range name {
		if !unicode.IsPrint(r) {
			return name, false
		}
	}
	return name, true
}

// roomHash is a join's H in lower case, false when it is neither empty nor
// 64 hex digits.
func roomHash(h string) (string, bool) {
	if h == "" {
		return "", true
	}
	if len(h) != 64 {
		return "", false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return "", false
		}
	}
	return strings.ToLower(h), true
}

// JoinRoom puts a session in the room of that name, making it (open, or
// locked with hash; unlisted as asked) when there is none: its result
// (RoomIn, RoomWrongPassword, RoomRefused) and the room's name as it is
// spelled. A session in another room leaves it.
func (s *Server) JoinRoom(id uint32, name, hash string, unlisted bool) (byte, string) {
	name, ok := RoomName(name)
	if !ok || s.cfg.RefuseRoom != nil && s.cfg.RefuseRoom(name) {
		return RoomRefused, name
	}
	h, ok := roomHash(hash)
	if !ok {
		return RoomWrongPassword, name
	}
	key := strings.ToLower(name)
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peers[id]
	if p == nil {
		return RoomRefused, name
	}
	s.expireRoomsLocked(now)
	r := s.rooms[key]
	if r != nil && p.room == r {
		return RoomIn, r.name
	}
	if r != nil && r.hash != "" && now.Sub(p.wrongAt) < roomRetry {
		return RoomWrongPassword, r.name
	}
	if r != nil && subtle.ConstantTimeCompare([]byte(r.hash), []byte(h)) != 1 {
		p.wrongAt = now
		return RoomWrongPassword, r.name
	}
	if r == nil {
		r = &room{name: name, key: key, hash: h, unlisted: unlisted}
		s.rooms[key] = r
	}
	s.leaveRoomLocked(p, now)
	p.room = r
	r.members++
	p.ch[1] = "r:" + key
	s.rostersLocked()
	return RoomIn, r.name
}

// LeaveRoom takes a session out of its room: the room's name, false when it
// was in none.
func (s *Server) LeaveRoom(id uint32) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peers[id]
	if p == nil || p.room == nil {
		return "", false
	}
	name := p.room.name
	s.leaveRoomLocked(p, s.clock())
	s.rostersLocked()
	return name, true
}

// leaveRoomLocked takes p out of its room (it stays roomGrace once empty);
// the caller sends the rosters.
func (s *Server) leaveRoomLocked(p *peer, now time.Time) {
	r := p.room
	if r == nil {
		return
	}
	p.room = nil
	p.ch[1] = ""
	if r.members--; r.members <= 0 {
		r.members, r.emptySince = 0, now
	}
	if p.send&2 != 0 {
		p.send &^= 2 // its room's push to talk ends with the room
	}
}

// expireRoomsLocked lets each room that has been empty roomGrace go.
func (s *Server) expireRoomsLocked(now time.Time) {
	for k, r := range s.rooms {
		if r.members == 0 && now.Sub(r.emptySince) >= roomGrace {
			delete(s.rooms, k)
		}
	}
}

// Lobby lists the rooms with anyone in them (an unlisted one never), most
// members first, at most 200. An empty room's minute keeps its name and lock
// for those coming back, out of the list: a room of "0 people" read as a
// dead one (F3 pass 18, the voice window's two-client run).
func (s *Server) Lobby() []Room {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireRoomsLocked(s.clock())
	out := make([]Room, 0, len(s.rooms))
	for _, r := range s.rooms {
		if !r.unlisted && r.members > 0 {
			out = append(out, Room{Name: r.name, Members: r.members, Locked: r.hash != ""})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Members != out[j].Members {
			return out[i].Members > out[j].Members
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if len(out) > lobbyMax {
		out = out[:lobbyMax]
	}
	return out
}
