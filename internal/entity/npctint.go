package entity

import (
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// npctint.go — a tint on the bodies of the NPCs of a name, for a client that
// draws one (the server owner, 2026-11-10: Yhafiran, "the black and green
// oriental dragon", wears the disc's only green oriental dragon, CHAR.ESF
// 0xB5C8A651, which the Plane of Disease's twins wear too; no black and green
// body exists, so "vanilla draws Yhafiran tinted toward black and green").
//
// Table npctint (database/npctint.go) names a colour for an NPC name, any NPC's;
// the world holds the rows read at the start and after a GM's edit (the GM
// panel's NPC body tints). A client that draws tints answers the offer at its
// world entry (handlers/npctint.go) and is sent them (protocol.OpcodeNPCTints),
// and again whenever they change: the whole set, by name, so every spawn of the
// name, a respawn and a GM's #spawn copy are tinted alike with nothing sent as
// they come into view. A PS2 never answers and is sent none: it draws the
// body's own colours, as every client of a server without the table does.

// NPCTintsMost is the most tints one message carries (about 6 KB at the
// longest names), the first in the keys' order.
const NPCTintsMost = 200

// npcTintNameMost is the most bytes of a name the entity channel carries
// (statesync.go: 116-139), so a client matches no more of one.
const npcTintNameMost = 24

// npcTintState is the world's tints and the messages that tell them.
type npcTintState struct {
	mu    sync.Mutex
	tints []database.NPCTint
	body  map[uint8][]byte // OpcodeNPCTints's body for tints, by layout; nil until made
}

// NPCTintsLayoutMost is the newest layout of OpcodeNPCTints: 2, each name's
// colour followed by its glow (petition 1893).
const NPCTintsLayoutMost = 2

// TintWireName is a tint's name as OpcodeNPCTints carries it: the key cut to
// the bytes the entity channel carries a name in, keyed again (a trailing space
// the cut left).
func TintWireName(key string) string {
	if len(key) > npcTintNameMost {
		key = key[:npcTintNameMost]
	}
	return database.NPCClassKey(key)
}

// EncodeNPCTints is OpcodeNPCTints's body: a u16 count, then each tint's name
// (TintWireName: a u8 byte count and UTF-8) and its u32 colour, at most
// NPCTintsMost of them. Two names that cut to one are sent once, the first's.
func EncodeNPCTints(tints []database.NPCTint) []byte {
	return EncodeNPCTintsLayout(tints, 1)
}

// EncodeNPCTintsLayout is OpcodeNPCTints's body in a layout: 1 as
// EncodeNPCTints (the names whose colour tints, a glow alone left out); 2 the
// names whose colour tints or whose glow lights, each its u32 colour then its
// u32 glow (0xRRGGBBAA, 0 none; the GM Jibrish's petition 1893: Mr. Moogie's
// body in a soft red glow).
func EncodeNPCTintsLayout(tints []database.NPCTint, layout uint8) []byte {
	seen := make(map[string]bool, len(tints))
	var list []database.NPCTint
	for _, t := range tints {
		n := TintWireName(t.Name)
		shows := database.NPCTintChanges(t.Color)
		if layout >= 2 {
			shows = t.Shows()
		}
		if n == "" || seen[n] || !shows {
			continue
		}
		seen[n] = true
		list = append(list, database.NPCTint{Name: n, Color: t.Color, Glow: t.Glow})
		if len(list) == NPCTintsMost {
			break
		}
	}
	b := binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(list)*(1+npcTintNameMost+8)), uint16(len(list)))
	for _, t := range list {
		b = wireStr(b, t.Name)
		b = binary.LittleEndian.AppendUint32(b, t.Color)
		if layout >= 2 {
			b = binary.LittleEndian.AppendUint32(b, t.Glow)
		}
	}
	return b
}

// NPCTintsMessage is OpcodeNPCTints with body.
func NPCTintsMessage(body []byte) *protocol.Message {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeNPCTints)
	msg.SetData(append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(body)), uint16(protocol.OpcodeNPCTints)), body...))
	return msg
}

// messageLocked is the body that tells the tints in a layout, and whether it
// tells none. Must be called with s.mu held.
func (s *npcTintState) messageLocked(layout uint8) ([]byte, bool) {
	layout = min(max(layout, 1), NPCTintsLayoutMost)
	if s.body == nil {
		s.body = make(map[uint8][]byte, NPCTintsLayoutMost)
	}
	b, ok := s.body[layout]
	if !ok {
		b = EncodeNPCTintsLayout(s.tints, layout)
		s.body[layout] = b
	}
	return b, binary.LittleEndian.Uint16(b) == 0
}

// SetNPCTints sets the NPCs' body tints (table npctint, as database.NPCTints
// reads it) and tells every client that asked for them where that changes what
// it holds. It reports how many it told.
func (w *World) SetNPCTints(tints []database.NPCTint) int {
	w.npcTints.mu.Lock()
	w.npcTints.tints = append([]database.NPCTint(nil), tints...)
	w.npcTints.body = nil
	w.npcTints.mu.Unlock()
	return w.RefreshNPCTints()
}

// LoadNPCTints reads table npctint into the world: at the start and after a
// GM's edit (the GM panel's live step). Call it without the database held. It
// reports how many names it read and how many clients it told.
func (w *World) LoadNPCTints(db *database.SQLiteDB) (names, told int, err error) {
	tints, err := db.NPCTints()
	if err != nil {
		return 0, 0, fmt.Errorf("npctint: %w", err)
	}
	return len(tints), w.SetNPCTints(tints), nil
}

// NPCTints is the tints the world holds, in the keys' order.
func (w *World) NPCTints() []database.NPCTint {
	w.npcTints.mu.Lock()
	defer w.npcTints.mu.Unlock()
	return append([]database.NPCTint(nil), w.npcTints.tints...)
}

// RefreshNPCTints sends the tints to every session that asked for them (with
// only, to those alone) where they differ from what it was last sent
// (session.TakeNPCTints), and reports how many it sent them to. A session that
// never asked (a PS2) is sent nothing.
func (w *World) RefreshNPCTints(only ...*session.Session) int {
	if w == nil {
		return 0
	}
	// held throughout, so no refresh sends a set older than one sent before it
	// (a client's answer while a GM's edit goes out)
	w.npcTints.mu.Lock()
	defer w.npcTints.mu.Unlock()
	sent, names := 0, 0
	send := func(s *session.Session) bool {
		if s == nil || s.NPCTintsLayout() == 0 {
			return true
		}
		body, empty := w.npcTints.messageLocked(s.NPCTintsLayout())
		if s.TakeNPCTints(string(body), empty) {
			s.Queue().AddWithSequence(NPCTintsMessage(body))
			sent++
			names = max(names, int(binary.LittleEndian.Uint16(body)))
		}
		return true
	}
	if len(only) > 0 {
		for _, s := range only {
			send(s)
		}
	} else if w.SessionManager != nil {
		w.SessionManager.ForEach(send)
	}
	if sent > 0 {
		log.Printf("NPCTints: %d names to %d clients", names, sent)
	}
	return sent
}

// NPCTintSummary says what the tints hold, for the log at the start.
func (w *World) NPCTintSummary() string {
	tints := w.NPCTints()
	if len(tints) == 0 {
		return "body tints: none (npctint)"
	}
	names := make([]string, 0, min(len(tints), 8))
	for i, t := range tints {
		if i == 8 {
			names = append(names, "...")
			break
		}
		if t.Glow != 0 {
			names = append(names, fmt.Sprintf("%s %08X glow %08X", t.Name, t.Color, t.Glow))
		} else {
			names = append(names, fmt.Sprintf("%s %08X", t.Name, t.Color))
		}
	}
	line := fmt.Sprintf("body tints: %d names in npctint (%s), drawn by the clients that ask", len(tints), strings.Join(names, ", "))
	if len(tints) > NPCTintsMost {
		line += fmt.Sprintf("; only the first %d are sent", NPCTintsMost)
	}
	return line
}
