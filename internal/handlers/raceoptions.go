package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A custom race's own character options, for a client whose create pages and
// world can show them (protocol.OpcodeRaceOptions, entity/raceoptions.go).
//
// After a session's first character list the server offers them (a u32, 1),
// after the live look's and creation's offers, so a PS2's list keeps its sequence
// number; a server where no race has options offers nothing. A client that can answers 1 and is sent each race's options
// (OpcodeRaceOptionsData, once a session) and the looks the characters chose
// (OpcodeRaceLooks), and from then on the looks again whenever they change: a
// creation, a deletion, a GM's edit, a keep restart. A creation of such a client
// carries its choices after 0x002B's own fields (raceOptionsBlock), which the
// server holds to the race's options. A PS2 ignores the offer and never sends the
// choices; a body chosen is the character's model, which a PS2 draws as any.

// buildRaceOptionsOffer is OpcodeRaceOptions from the server: a u32, 1.
func buildRaceOptionsOffer() []byte {
	return build(protocol.OpcodeRaceOptions, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendRaceOptionsOffer offers the session's client the races' options, once a
// session, on a server where any race has some.
func SendRaceOptionsOffer(sess *session.Session) {
	if entity.HasRaceOptions() && sess.OfferRaceOptionsOnce() {
		queueReliable(sess, protocol.OpcodeRaceOptions, buildRaceOptionsOffer())
	}
}

// RaceOptionsHandler takes a client's answer to the offer.
type RaceOptionsHandler struct {
	world *entity.World
	db    *database.SQLiteDB
}

// NewRaceOptionsHandler creates the race options' handler.
func NewRaceOptionsHandler(w *entity.World, db *database.SQLiteDB) *RaceOptionsHandler {
	return &RaceOptionsHandler{world: w, db: db}
}

// RegisterHandlers registers it.
func (h *RaceOptionsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeRaceOptions, h.HandleRaceOptions)
}

// HandleRaceOptions makes the client's answer the session's: a u32, nonzero to
// be sent the races' options and the looks, 0 to stop. On, both go out at once.
func (h *RaceOptionsHandler) HandleRaceOptions(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("RaceOptions: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	sess.SetRaceLooks(on)
	log.Printf("RaceOptions: %s %s", sess.RemoteAddr(), map[bool]string{true: "shows them", false: "stopped"}[on])
	if on {
		entity.SendRaceOptions(sess)
		h.world.RefreshRaceLooks(h.db, sess)
	}
	return nil
}

// resendPregameExtras sends a client that asked for them at character select
// creation's races, the races' options and the looks again, at a login's world
// entry (SendMemoryDump). They went out on the lobby's connection after its
// answers, and a client that selects its character before they have all come
// loses the rest with that connection (it closes it at the promotion), its
// world then drawing no race's look, option body or held items: vanilla's
// `eqoa play` selects a second after its list, and on a loaded box a Wisp
// showed as a plain human (2026-11-09). A PS2 asked for none and is sent none.
func (h *CharacterHandler) resendPregameExtras(sess *session.Session) {
	if sess.CreationAsked() {
		SendCreationRaces(sess)
	}
	if !sess.RaceLooks() {
		return
	}
	sess.RaceDataAgain()
	entity.SendRaceOptions(sess)
	if h.gameWorld != nil && h.store != nil && h.store.DB() != nil {
		h.gameWorld.RefreshRaceLooks(h.store.DB(), sess)
	}
}

// raceOptionsMark opens the choices a client of the race options writes after
// 0x002B's own fields: 'R', then a u8 version (1), a u8 count and each choice,
// its row's key (a u8 byte count and UTF-8) and a u8 step.
const (
	raceOptionsMark    = 'R'
	raceOptionsVersion = 1
)

// raceOptionsBlock reads the choices after a 0x002B's own fields (rest, the
// bytes left). present is whether a block of choices is there at all: bytes that
// do not open with the mark are none, as from a PS2 or any client that was not
// offered them, and read as before. ok is false for a block that is cut short or
// of another version.
func raceOptionsBlock(rest []byte) (choices []entity.OptionChoice, present, ok bool) {
	if len(rest) == 0 || rest[0] != raceOptionsMark {
		return nil, false, true
	}
	if len(rest) < 3 || rest[1] != raceOptionsVersion {
		return nil, true, false
	}
	n, at := int(rest[2]), 3
	out := make([]entity.OptionChoice, 0, n)
	for range n {
		if at >= len(rest) {
			return nil, true, false
		}
		k := int(rest[at])
		at++
		if at+k+1 > len(rest) {
			return nil, true, false
		}
		out = append(out, entity.OptionChoice{Key: string(rest[at : at+k]), Step: int(rest[at+k])})
		at += k + 1
	}
	return out, true, at == len(rest)
}
