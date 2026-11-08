package entity

import (
	"encoding/binary"
	"log"
	"math"
	"sort"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// raceoptions_wire.go — the race options and the looks as a client that asked is
// told them (protocol.OpcodeRaceOptionsData, protocol.OpcodeRaceLooks; the formats
// are there). raceoptions.go has what they are.

// raceOptionsVersion is both messages' first byte.
const raceOptionsVersion = 1

// optionRaces is every race with options, by id: each racedefs row whose own
// body has rows, and the froglok while creation offers it (a server that does
// not has no froglok of its own to dress).
func optionRaces() []int {
	var out []int
	if Creation().Races && len(RaceOptions(raceFroglok)) > 0 {
		out = append(out, raceFroglok)
	}
	for _, d := range RaceDefs() {
		if len(RaceOptions(d.ID)) > 0 {
			out = append(out, d.ID)
		}
	}
	sort.Ints(out)
	return out
}

// wireStr appends s as a u8 byte count and its bytes, cut at 255.
func wireStr(b []byte, s string) []byte {
	if len(s) > 255 {
		s = s[:255]
	}
	return append(append(b, byte(len(s))), s...)
}

// wirePart appends a part's entries and rects.
func wirePart(b []byte, p OptionPart) []byte {
	n := min(len(p.Entries), 255)
	b = append(b, byte(n))
	b = append(b, p.Entries[:n]...)
	r := min(len(p.Rects), 255)
	b = append(b, byte(r))
	for _, rc := range p.Rects[:r] {
		b = append(b, rc[:]...)
	}
	return b
}

// wireSteps appends a u8 count and each step.
func wireSteps(b []byte, steps []OptionStep) []byte {
	n := min(len(steps), 255)
	b = append(b, byte(n))
	for _, s := range steps[:n] {
		b = wireStr(b, s.Name)
		b = binary.LittleEndian.AppendUint32(b, s.Value)
		b = binary.LittleEndian.AppendUint16(b, uint16(max(0, min(65535, int(math.Round(float64(s.Size)*1000))))))
	}
	return b
}

// HasRaceOptions reports whether any race here has options: a server with none
// offers nothing, so its lists go as they always did.
func HasRaceOptions() bool {
	return len(optionRaces()) > 0
}

// EncodeRaceOptions is OpcodeRaceOptionsData's body: every race's options.
func EncodeRaceOptions() []byte {
	races := optionRaces()
	if len(races) > 255 {
		races = races[:255]
	}
	b := []byte{raceOptionsVersion, byte(len(races))}
	for _, race := range races {
		rows := RaceOptions(race)
		if len(rows) > 255 {
			rows = rows[:255]
		}
		b = append(b, byte(race), byte(len(rows)))
		for _, r := range rows {
			b = wireStr(b, r.Key)
			b = wireStr(b, r.Label)
			b = append(b, byte(r.Kind))
			parts := r.Parts
			if len(parts) > 255 {
				parts = parts[:255]
			}
			b = append(b, byte(len(parts)))
			for _, p := range parts {
				b = binary.LittleEndian.AppendUint32(b, p.Body)
				b = wirePart(b, p)
			}
			b = wireSteps(b, r.Male)
			if r.Female != nil {
				b = wireSteps(b, r.Female)
			} else {
				b = append(b, 0)
			}
		}
	}
	return b
}

// lookParts is what a character's charlook rows lay on its body: for each row
// its race's options have a colour or texture row of, that row's parts of the
// body, each with the row's kind and the look's value; a row of another key, of
// another kind or with no part of the body lays nothing.
func lookParts(race int, body uint32, looks []database.CharLook) (kinds []OptionKind, values []uint32, parts []OptionPart) {
	rows := RaceOptions(race)
	for _, l := range looks {
		for _, r := range rows {
			if r.Key != l.Part || int(r.Kind) != l.Kind || r.Kind == KindBody {
				continue
			}
			for _, p := range r.PartsOf(body) {
				kinds, values, parts = append(kinds, r.Kind), append(values, l.Value), append(parts, p)
			}
		}
	}
	return kinds, values, parts
}

// EncodeRaceLooks is OpcodeRaceLooks' body: each character's look on its own
// body, a GM's under both its names; a character whose rows lay nothing is left
// out.
func EncodeRaceLooks(chars []database.LookCharacter) []byte {
	type entry struct {
		name string
		body uint32
		k    []OptionKind
		v    []uint32
		p    []OptionPart
	}
	var list []entry
	for _, c := range chars {
		body := uint32(c.ModelID)
		k, v, p := lookParts(c.Race, body, c.Looks)
		if len(p) == 0 {
			continue
		}
		list = append(list, entry{c.Name, body, k, v, p})
		if c.GM { // its channel name carries the tag (statesync.go)
			list = append(list, entry{GMTag + c.Name, body, k, v, p})
		}
	}
	if len(list) > 65535 {
		list = list[:65535]
	}
	b := binary.LittleEndian.AppendUint16([]byte{raceOptionsVersion}, uint16(len(list)))
	for _, e := range list {
		b = wireStr(b, e.name)
		b = binary.LittleEndian.AppendUint32(b, e.body)
		n := min(len(e.p), 255)
		b = append(b, byte(n))
		for i := range n {
			b = append(b, byte(e.k[i]))
			b = binary.LittleEndian.AppendUint32(b, e.v[i])
			b = wirePart(b, e.p[i])
		}
	}
	return b
}

// RaceLooksMessage is OpcodeRaceLooks with body, a message of its own size: a
// long list comes fragmented, as the race labels' does.
func RaceLooksMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceLooks)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeRaceLooks)), body...))
	return msg
}

// RaceOptionsMessage is OpcodeRaceOptionsData with body.
func RaceOptionsMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeRaceOptionsData)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeRaceOptionsData)), body...))
	return msg
}

// SendRaceOptions sends sess the races' options, once a session that asked
// (handlers/raceoptions.go), and reports whether it did.
func SendRaceOptions(sess *session.Session) bool {
	if !sess.TakeRaceCatalogue() {
		return false
	}
	body := EncodeRaceOptions()
	sess.Queue().AddWithSequence(RaceOptionsMessage(body))
	log.Printf("RaceOptions: %s told %d races' options, %d bytes", sess.RemoteAddr(), int(body[1]), len(body))
	return true
}

// RefreshRaceLooks reads the looks the characters chose and sends the list to
// each session that asked and was last sent another (sess: that one alone):
// when a client asks, after a creation or a deletion that changes it, after a
// GM's edit and at a keep restart's resume. It reports how many were sent it.
// Call it without the world's lock held.
func (w *World) RefreshRaceLooks(db *database.SQLiteDB, only ...*session.Session) int {
	if db == nil {
		return 0
	}
	chars, err := db.LookCharacters()
	if err != nil {
		log.Printf("RaceLooks: the characters: %v", err)
		return 0
	}
	body := EncodeRaceLooks(chars)
	sent := 0
	send := func(s *session.Session) bool {
		if s.TakeRaceLooks(string(body)) {
			s.Queue().AddWithSequence(RaceLooksMessage(body))
			sent++
		}
		return true
	}
	if len(only) > 0 {
		for _, s := range only {
			send(s)
		}
	} else if w != nil && w.SessionManager != nil {
		w.SessionManager.ForEach(send)
	}
	if sent > 0 {
		log.Printf("RaceLooks: %d characters' looks, %d bytes, to %d clients", binary.LittleEndian.Uint16(body[1:3]), len(body), sent)
	}
	return sent
}
