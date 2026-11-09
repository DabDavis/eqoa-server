package handlers

import (
	"encoding/binary"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// What character creation takes past the game's own, told to a client whose
// create pages can show it (protocol.OpcodeCreationOffer, entity/creation.go;
// world.open_races, world.all_classes).
//
// After a session's first character list, and only while creation takes
// something past the game's, the server offers it (0x7E1E, a u32 1), as the live
// look's offer goes. A client that can answers 1 and is sent the races
// (0x7E1F) at once. A PS2 ignores the offer (CLIENT's dispatcher has no default
// case) and creates from the disc's pages; vanilla without it ignores it too,
// and a server with neither switch on offers nothing. The offer goes after the
// list, so a PS2's list keeps the sequence number it always had.

// creationRacesVersion is OpcodeCreationRaces' first byte.
const creationRacesVersion = 1

// buildCreationOffer is OpcodeCreationOffer from the server: a u32, 1.
func buildCreationOffer() []byte {
	return build(protocol.OpcodeCreationOffer, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendCreationOffer offers the session's client creation's races, once a
// session, while creation takes something past the game's own.
func SendCreationOffer(sess *session.Session) {
	if !entity.Creation().On() {
		return
	}
	if sess.OfferCreationOnce() {
		queueReliable(sess, protocol.OpcodeCreationOffer, buildCreationOffer())
	}
}

// encodeCreationRaces is OpcodeCreationRaces' body (protocol.OpcodeCreationRaces).
func encodeCreationRaces(rules entity.CreationRules, races []entity.CreationRace) []byte {
	flags := byte(0)
	if rules.AllClasses {
		flags |= 1
	}
	if len(races) > 255 {
		races = races[:255]
	}
	b := []byte{creationRacesVersion, flags, byte(len(races))}
	short := func(s string) {
		if len(s) > 255 {
			s = s[:255]
		}
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	for _, r := range races {
		b = append(b, byte(r.ID), byte(r.Like), byte(r.City))
		for _, v := range r.Bonus {
			b = append(b, byte(int8(max(-128, min(127, v)))))
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(r.ClassMask))
		b = binary.LittleEndian.AppendUint32(b, uint32(r.Body[0]))
		b = binary.LittleEndian.AppendUint32(b, uint32(r.Body[1]))
		short(r.Name)
		short(r.Short)
		d := r.Desc
		if len(d) > 4000 {
			d = d[:4000]
		}
		b = binary.LittleEndian.AppendUint16(b, uint16(len(d)))
		b = append(b, d...)
	}
	return b
}

// creationRacesMessage is OpcodeCreationRaces with body: a message of its own
// size, as a long list comes fragmented.
func creationRacesMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeCreationRaces)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeCreationRaces)), body...))
	return msg
}

// CreationHandler takes a client's answer to the offer.
type CreationHandler struct{}

// NewCreationHandler creates creation's handler.
func NewCreationHandler() *CreationHandler { return &CreationHandler{} }

// RegisterHandlers registers it.
func (h *CreationHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeCreationOffer, h.HandleCreationOffer)
}

// HandleCreationOffer takes the client's answer: a u32, nonzero to be sent the
// races (as they stand now), 0 nothing.
func (h *CreationHandler) HandleCreationOffer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("CreationOffer: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if binary.LittleEndian.Uint32(data[:4]) == 0 {
		log.Printf("CreationOffer: %s wants nothing", sess.RemoteAddr())
		return nil
	}
	sess.SetCreationAsked()
	SendCreationRaces(sess)
	return nil
}

// SendCreationRaces sends sess creation's races as they stand now: at its
// client's answer to the offer, and again at a login's world entry
// (resendPregameExtras).
func SendCreationRaces(sess *session.Session) {
	rules, races := entity.Creation(), entity.CreationRaces()
	body := encodeCreationRaces(rules, races)
	sess.Queue().AddWithSequence(creationRacesMessage(body))
	log.Printf("CreationOffer: %s told %d races past the game's (every class for every race: %v), %d bytes",
		sess.RemoteAddr(), len(races), rules.AllClasses, len(body))
}

// NewCharRace is the account page's rule for a race past the game's ten (main.go
// wires it where creation takes such races): whether creation takes race and class
// (entity.CheckCreation) and the body the character wears ("" taken).
func NewCharRace(race, class, sex int) (int64, string) {
	if race <= 9 || race > entity.LastCustomRace {
		return 0, "not a race past the game's"
	}
	if why := entity.CheckCreation(race, class, 0); why != "" {
		return 0, why
	}
	return (&CharacterHandler{}).getModelID(Race(race), Gender(sex)), ""
}
