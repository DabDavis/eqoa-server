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

// optionRaces is every race with options a client that answered the offer
// answer is told, by id: each racedefs row whose own body has rows, and the
// froglok while creation offers it (a server that does not has no froglok of its
// own to dress); answered 2 or more, the game's races' too (their skin shades on
// a server that offers them: skinshades.go; answered 3, the barbarians' tattoo:
// tattoos.go).
func optionRaces(answer uint32) []int {
	var out []int
	if answer >= 2 {
		for race := range 10 {
			if len(RaceOptionsFor(race, answer)) > 0 {
				out = append(out, race)
			}
		}
	}
	if Creation().Races && len(RaceOptionsFor(raceFroglok, answer)) > 0 {
		out = append(out, raceFroglok)
	}
	for _, d := range RaceDefs() {
		if len(RaceOptionsFor(d.ID, answer)) > 0 {
			out = append(out, d.ID)
		}
	}
	sort.Ints(out)
	return out
}

// RaceOptionsFor is race's rows a client that answered the offer answer is
// told: every row but those that need a later answer (OptionRow.Answer).
func RaceOptionsFor(race int, answer uint32) []OptionRow {
	var out []OptionRow
	for _, r := range RaceOptions(race) {
		if r.Answer <= answer {
			out = append(out, r)
		}
	}
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
	return len(optionRaces(raceAnswerNewest)) > 0
}

// EncodeRaceOptions is OpcodeRaceOptionsData's body: every race's options
// but the game's races' (a client that answered the offer 1).
func EncodeRaceOptions() []byte {
	return EncodeRaceOptionsFor(false)
}

// EncodeRaceOptionsFor is OpcodeRaceOptionsData's body: every race's options,
// with games the game's races' too (a client that answered the offer 2 shows
// them under the four; one that answered 1 would show them instead).
func EncodeRaceOptionsFor(games bool) []byte {
	return EncodeRaceOptionsAt(map[bool]uint32{false: 1, true: 2}[games])
}

// raceAnswerNewest is the latest answer to the offer this server knows: 3, a
// client that draws the barbarians' tattoo (vanilla since 2026-11-12).
const raceAnswerNewest = 3

// EncodeRaceOptionsAt is OpcodeRaceOptionsData's body for a client that
// answered the offer answer: every race's rows it is told (RaceOptionsFor).
func EncodeRaceOptionsAt(answer uint32) []byte {
	races := optionRaces(answer)
	if len(races) > 255 {
		races = races[:255]
	}
	b := []byte{raceOptionsVersion, byte(len(races))}
	for _, race := range races {
		rows := RaceOptionsFor(race, answer)
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
// another kind or with no part of the body lays nothing. In the options' row
// order, as the appearance page lays a look (vanilla's look_of_steps), whatever
// order the rows were kept in: a client lays the parts in the order sent.
func lookParts(race int, body uint32, looks []database.CharLook) (kinds []OptionKind, values []uint32, parts []OptionPart) {
	for _, r := range RaceOptions(race) {
		if r.Kind == KindBody {
			continue
		}
		for _, l := range looks {
			if r.Key != l.Part || int(r.Kind) != l.Kind {
				continue
			}
			for _, p := range r.PartsOf(body) {
				kinds, values, parts = append(kinds, r.Kind), append(values, l.Value), append(parts, p)
			}
			break
		}
	}
	return kinds, values, parts
}

// EncodeRaceLooks is OpcodeRaceLooks' body: each character's look on its own
// body, a GM's under both its names; a character whose rows lay nothing is left
// out.
func EncodeRaceLooks(chars []database.LookCharacter) []byte {
	return encodeRaceLooks(chars, nil)
}

// encodeRaceLooks is EncodeRaceLooks with the Wisps of mortal (by name) in a
// mortal form (wispform.go): their wisp row lays nothing, so a client draws the
// body they show whole.
func encodeRaceLooks(chars []database.LookCharacter, mortal map[string]bool) []byte {
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
		looks := c.Looks
		if mortal[c.Name] {
			looks = withoutRow(withoutRow(looks, wispRowKey), wispColourKey)
		}
		k, v, p := lookParts(c.Race, body, looks)
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
	body := EncodeRaceOptionsAt(sess.RaceLooksLevel())
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
	if w == nil {
		return w.sendRaceLooks(chars, only...)
	}
	l := &w.wispLooks
	l.send.Lock()
	defer l.send.Unlock()
	l.mu.Lock()
	l.chars, l.have = chars, true
	l.mu.Unlock()
	return w.sendRaceLooks(chars, only...)
}

// sendRaceLooks sends the looks of chars (the Wisps in a mortal form without
// their wisp: wispform.go) to each session that asked and was last sent
// another (only: those alone), reporting how many.
func (w *World) sendRaceLooks(chars []database.LookCharacter, only ...*session.Session) int {
	var mortal map[string]bool
	if w != nil {
		mortal = w.wispLooks.mortalNames()
	}
	body := encodeRaceLooks(chars, mortal)
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

// withoutRow is looks less the rows of part key.
func withoutRow(looks []database.CharLook, key string) []database.CharLook {
	out := make([]database.CharLook, 0, len(looks))
	for _, l := range looks {
		if l.Part != key {
			out = append(out, l)
		}
	}
	return out
}
