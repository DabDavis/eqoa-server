package entity

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// racelabels.go — what a client that can shows for a character of a race of
// racedefs (the user, 2026-11-07: "my race says ogre, but lets make it Hill Giant,
// Titan as MC"; protocol.OpcodeRaceLabels, handlers/racelabels.go). A character
// of such a race passes as a race of CLIENT's on every wire site (race.go), so
// nothing about its items, its camera or the game's tables changes; a client that
// says it can is told, by name, which characters read differently and what their
// race's words are, so it shows those in place of the wire race's. A second race
// needs nothing new in such a client: the words come from its row. A PS2 is sent
// only the offer, which it ignores, and keeps the wire race's word with the race on
// the target box's title line (TargetInfoLines).

// raceLabelMax is the most bytes of a name or short name a label carries: its
// length on the wire is a byte.
const raceLabelMax = 64

// emptyRaceLabels is the list with nothing on it, what a client that has just
// asked holds.
const emptyRaceLabels = "\x00"

// encodeRaceLabels is OpcodeRaceLabelsData's body: a u8 count of races, then each
// race's name and short name, a u16 count of its characters and each one's name
// (each string a u8 byte count and UTF-8). A race with no character is left out.
func encodeRaceLabels(defs []RaceDef, chars []database.RaceCharacter) []byte {
	body := []byte{0}
	str := func(s string) {
		if len(s) > 255 {
			s = s[:255]
		}
		body = append(body, byte(len(s)))
		body = append(body, s...)
	}
	for _, d := range defs {
		var names []string
		for _, c := range chars {
			if c.Race == d.ID {
				names = append(names, c.Name)
				if c.GM { // its channel name carries its tag (statesync.go)
					names = append(names, GMTag+c.Name)
				}
			}
		}
		if len(names) == 0 || body[0] == 255 {
			continue
		}
		if len(names) > 65535 {
			names = names[:65535]
		}
		body[0]++
		str(d.Name)
		str(d.Short)
		body = binary.LittleEndian.AppendUint16(body, uint16(len(names)))
		for _, n := range names {
			str(n)
		}
	}
	return body
}

// raceLabelsMessage is OpcodeRaceLabelsData with body. A long list goes in a
// message of its own size, past a message's own span, and comes fragmented, as
// the who list does.
func raceLabelsMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceLabelsData)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeRaceLabelsData)), body...))
	return msg
}

// SetRaceLabels makes sess's player's client have race labels (on) or stop (off).
// False when sess has no player.
func (w *World) SetRaceLabels(sess *session.Session, on bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return false
	}
	p.RaceLabels, p.raceLabelsSent = on, emptyRaceLabels
	return true
}

// RefreshRaceLabels reads who the labels name and sends the list to each client
// that asked and was last sent another: when a client asks, when a character of
// such a race logs in (its row may have changed since the others were told) and
// after a GM's edit. So a server with no such race sends only the offer, and an
// ordinary character's login costs nothing. Call it without the lock held. It
// reports how many clients were sent the list.
func (w *World) RefreshRaceLabels(db *database.SQLiteDB) int {
	if db == nil || w.SessionManager == nil {
		return 0
	}
	defs := RaceDefs()
	ids := make([]int, len(defs))
	for i, d := range defs {
		ids[i] = d.ID
	}
	chars, err := db.CharactersOfRaces(ids)
	if err != nil {
		log.Printf("RaceLabels: the characters: %v", err)
		return 0
	}
	body := encodeRaceLabels(defs, chars)
	sent := 0
	w.mu.Lock()
	defer w.mu.Unlock()
	w.SessionManager.ForEach(func(s *session.Session) bool {
		p := w.sessionPlayerLocked(s)
		if p == nil || !p.RaceLabels || p.raceLabelsSent == string(body) {
			return true
		}
		p.raceLabelsSent = string(body)
		s.Queue().AddWithSequence(raceLabelsMessage(body))
		sent++
		return true
	})
	if sent > 0 {
		log.Printf("RaceLabels: %d characters' words, %d bytes, to %d clients", len(chars), len(body), sent)
	}
	return sent
}
