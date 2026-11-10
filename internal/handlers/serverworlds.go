package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// The server's own worlds: each made of the disc's own zones where they stand
// on the disc (internal/worlds' Compose, cmd/worldbuild), built by the
// operator from their own disc into worlds/ (worldN.ESF, and worldN.json, the
// recipe as built). The server reads the built recipes at its start
// (LoadServerWorlds) and offers them, after a session's first character list,
// to a client that can build the same world off its player's disc
// (protocol.OpcodeServerWorlds; eqoa-rust-client-vanilla): it answers 1, is
// sent the recipes (protocol.OpcodeServerWorldsData), builds each, checks it
// against the built file's CRC, and says which it holds. Those are the worlds
// LoadableWorlds takes for that session: the travel, the GMs' moves and a
// login go there for it alone. A PS2 ignores the offer and is never sent into
// one (its table has no such world; a PS2 with our hard-disk loader could be
// served the file over /worlds/, which no live server does). Nothing of the
// disc goes over the wire: a world is named by its zones (a world file of the
// disc and an index) and checked by CRCs.

// OpcodeServerWorldsData's first byte: version 1, a world named by its
// zones; version 2, each zone followed by its extras (the disc's art placed
// in it, internal/worlds/extras.go), sent only when a world has any, so a
// client that reads version 1 alone still builds a world with none (a world
// with extras it could not build the same anyway: it is told nothing it can
// read, and holds nothing).
const (
	serverWorldsVersion       = 1
	serverWorldsExtrasVersion = 2
)

var (
	serverWorldsMu sync.RWMutex
	serverWorlds   = map[uint8]worlds.Built{}
)

// SetServerWorlds makes built the server's own worlds (a test's, or
// LoadServerWorlds').
func SetServerWorlds(built []worlds.Built) {
	m := make(map[uint8]worlds.Built, len(built))
	for _, b := range built {
		m[uint8(b.World)] = b
	}
	serverWorldsMu.Lock()
	serverWorlds = m
	serverWorldsMu.Unlock()
}

// ServerWorld is the server's own world in slot world, when it has one.
func ServerWorld(world uint8) (worlds.Built, bool) {
	serverWorldsMu.RLock()
	defer serverWorldsMu.RUnlock()
	b, ok := serverWorlds[world]
	return b, ok
}

// ServerWorldList is the server's own worlds, by slot.
func ServerWorldList() []worlds.Built {
	serverWorldsMu.RLock()
	defer serverWorldsMu.RUnlock()
	out := make([]worlds.Built, 0, len(serverWorlds))
	for _, b := range serverWorlds {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].World < out[j].World })
	return out
}

// LoadServerWorlds reads the built recipes in the worlds folder (worldN.json)
// of the slots given (world.server_worlds; past the disc's, below WorldCount)
// whose world file is there with the size the recipe says, makes them the
// server's own worlds and names their slots (worldNames), and says what it
// found. A recipe whose file is missing or of another size is passed over,
// loudly: a client told to build it would build a world the server does not
// have.
func LoadServerWorlds(configured string, slots []int) []worlds.Built {
	dir := worlds.Dir(configured)
	var out []worlds.Built
	for _, slot := range slots {
		if slot < discWorlds || slot >= WorldCount {
			log.Printf("Server worlds: slot %d is not one past the disc's (6 to %d); not offered", slot, WorldCount-1)
			continue
		}
		meta := filepath.Join(dir, worlds.BuiltName(slot))
		if _, err := os.Stat(meta); err != nil {
			continue
		}
		b, err := worlds.ReadBuilt(meta)
		if err != nil {
			log.Printf("Server worlds: %v; not offered", err)
			continue
		}
		if b.World != slot {
			log.Printf("Server worlds: %s names world %d; not offered", meta, b.World)
			continue
		}
		st, err := os.Stat(filepath.Join(dir, worlds.Name(slot)))
		if err != nil || st.Size() != b.Size {
			log.Printf("Server worlds: %s is not the %d bytes %s was built as (%v); not offered", worlds.Name(slot), b.Size, meta, err)
			continue
		}
		out = append(out, b)
	}
	SetServerWorlds(out)
	for _, b := range out {
		nameServerWorld(b)
		log.Printf("Server worlds: world %d %q, %d zones, %d bytes (CRC %08x), offered to clients that build it", b.World, b.Name, len(b.Parts), b.Size, b.CRC)
	}
	return out
}

// nameServerWorld names a server world's slot for GM output and the panel.
func nameServerWorld(b worlds.Built) {
	if b.World >= 0 && b.World < WorldCount && b.Name != "" {
		worldNames[b.World] = b.Name
	}
}

// buildServerWorldsOffer is OpcodeServerWorlds from the server: a u32, 1.
func buildServerWorldsOffer() []byte {
	return build(protocol.OpcodeServerWorlds, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendServerWorldsOffer offers the session's client the server's own worlds,
// once a session, while the server has any.
func SendServerWorldsOffer(sess *session.Session) {
	if len(ServerWorldList()) == 0 {
		return
	}
	if sess.OfferServerWorldsOnce() {
		queueReliable(sess, protocol.OpcodeServerWorlds, buildServerWorldsOffer())
	}
}

// encodeServerWorlds is OpcodeServerWorldsData's body (protocol.OpcodeServerWorldsData).
// Version 2 adds after each zone its extras: a u16 count, the u8 count of the
// places their art comes from and each (its file's name, a u16 zone), then
// each extra (a u8 place, 0xFF the zone's own; the sprite's u32 DictID; where
// it stands, its heading, pitch and roll and its scale, f32s; its static
// light, RGBA), as the zone's ZoneActor takes them.
func encodeServerWorlds(list []worlds.Built) []byte {
	if len(list) > 255 {
		list = list[:255]
	}
	version := byte(serverWorldsVersion)
	for _, w := range list {
		for _, z := range w.Zones {
			if len(z.Extras) > 0 {
				version = serverWorldsExtrasVersion
			}
		}
	}
	b := []byte{version, byte(len(list))}
	str := func(s string) {
		if len(s) > 255 {
			s = s[:255]
		}
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	for _, w := range list {
		b = append(b, byte(w.World))
		str(w.Name)
		b = binary.LittleEndian.AppendUint32(b, uint32(w.Size))
		b = binary.LittleEndian.AppendUint32(b, w.CRC)
		parts := w.Parts
		if len(parts) > 255 {
			parts = parts[:255]
		}
		b = append(b, byte(len(parts)))
		for i, p := range parts {
			str(p.From)
			b = binary.LittleEndian.AppendUint16(b, uint16(p.Zone))
			b = binary.LittleEndian.AppendUint32(b, uint32(p.Size))
			b = binary.LittleEndian.AppendUint32(b, p.CRC)
			b = append(b, byte(p.Col), byte(p.Row))
			name, race := "", 0
			if i < len(w.Zones) {
				name, race = w.Zones[i].Name, w.Zones[i].Race
			}
			str(name)
			b = append(b, byte(race))
			if version >= serverWorldsExtrasVersion {
				var extras []worlds.Extra
				if i < len(w.Zones) {
					extras = w.Zones[i].Extras
				}
				b = appendExtras(b, extras)
			}
		}
	}
	return b
}

// appendExtras is a zone's extras as version 2 has them.
func appendExtras(b []byte, extras []worlds.Extra) []byte {
	type place struct {
		from string
		zone int
	}
	var places []place
	index := map[place]int{}
	for _, e := range extras {
		if p := (place{strings.ToUpper(e.From), e.Zone}); e.From != "" {
			if _, ok := index[p]; !ok {
				index[p] = len(places)
				places = append(places, p)
			}
		}
	}
	b = binary.LittleEndian.AppendUint16(b, uint16(len(extras)))
	b = append(b, byte(len(places)))
	for _, p := range places {
		b = appendStr8(b, p.from)
		b = binary.LittleEndian.AppendUint16(b, uint16(p.zone))
	}
	for _, e := range extras {
		at := byte(0xFF)
		if e.From != "" {
			at = byte(index[place{strings.ToUpper(e.From), e.Zone}])
		}
		b = append(b, at)
		b = binary.LittleEndian.AppendUint32(b, uint32(e.Sprite))
		for _, v := range [7]float32{e.At[0], e.At[1], e.At[2], e.HPR[0], e.HPR[1], e.HPR[2], e.ActorScale()} {
			b = binary.LittleEndian.AppendUint32(b, math.Float32bits(v))
		}
		l := e.ActorLight()
		b = append(b, l[:]...)
	}
	return b
}

// appendStr8 is a string as the list has them: a u8 length, its bytes.
func appendStr8(b []byte, s string) []byte {
	if len(s) > 255 {
		s = s[:255]
	}
	b = append(b, byte(len(s)))
	return append(b, s...)
}

// serverWorldsMessage is OpcodeServerWorldsData with body, a message of its
// own size (a long list comes fragmented).
func serverWorldsMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeServerWorldsData)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeServerWorldsData)), body...))
	return msg
}

// ServerWorldsHandler takes a client's answer to the offer, and what it holds.
type ServerWorldsHandler struct{}

// NewServerWorldsHandler creates the server worlds' handler.
func NewServerWorldsHandler() *ServerWorldsHandler { return &ServerWorldsHandler{} }

// RegisterHandlers registers it.
func (h *ServerWorldsHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeServerWorlds, h.HandleOffer)
	r.Register(protocol.OpcodeServerWorldsData, h.HandleHeld)
}

// HandleOffer takes the client's answer: a u32, nonzero to be sent the
// worlds (as they stand now), 0 nothing.
func (h *ServerWorldsHandler) HandleOffer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		log.Printf("ServerWorlds: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if binary.LittleEndian.Uint32(data[:4]) == 0 {
		return nil
	}
	list := ServerWorldList()
	body := encodeServerWorlds(list)
	sess.Queue().AddWithSequence(serverWorldsMessage(body))
	log.Printf("ServerWorlds: %s told %d worlds of the server's own, %d bytes", sess.RemoteAddr(), len(list), len(body))
	return nil
}

// HandleHeld takes the worlds the client holds: a u8 count, then each slot.
// A slot the server has no world in is left out.
func (h *ServerWorldsHandler) HandleHeld(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || len(data) < 1+int(data[0]) {
		log.Printf("ServerWorlds: a held list of %d bytes from %s", len(data), sess.RemoteAddr())
		return nil
	}
	var ids []uint8
	for _, id := range data[1 : 1+int(data[0])] {
		if _, ok := ServerWorld(id); ok {
			ids = append(ids, id)
		}
	}
	sess.SetServerWorlds(ids)
	log.Printf("ServerWorlds: %s holds %v of the server's own worlds", sess.RemoteAddr(), ids)
	return nil
}

// serverWorldLabel is a server world's name and slot, for a refusal.
func serverWorldLabel(world uint8) string {
	if b, ok := ServerWorld(world); ok && b.Name != "" {
		return fmt.Sprintf("%s (world %d)", b.Name, world)
	}
	return fmt.Sprintf("world %d", world)
}
