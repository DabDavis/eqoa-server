package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// FactionHandler handles faction-related messages.
type FactionHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

// NewFactionHandler creates a new faction handler.
func NewFactionHandler(sessionMgr *session.Manager) *FactionHandler {
	return &FactionHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference.
func (h *FactionHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers faction handlers with the registry.
func (h *FactionHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeClientFaction, h.HandleClientFaction)
	r.Register(protocol.OpcodeFactionMessages, h.HandleFactionMessages)
	r.Register(protocol.OpcodeLootMessages, h.HandleLootMessages)
}

// HandleFactionMessages is the client's Faction Messages option (0x00C9):
// byte(state) + byte(changed), state 0 show, 1 hide, sent at the world entry
// (changed 0) and when the option changes (1). The client filters no line
// itself: a session that hides them is sent none (entity/faction.go).
func (h *FactionHandler) HandleFactionMessages(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 {
		return nil
	}
	sess.SetFactionMessagesOff(data[0] != 0)
	log.Printf("FactionMessages from %s: %x (lines %s)", sess.RemoteAddr(), data, map[bool]string{false: "shown", true: "hidden"}[data[0] != 0])
	return nil
}

// HandleLootMessages handles the loot message toggle (0x00CA).
// Client sends this on world entry: byte(state) + byte(0). state: 0=show, 1=hide.
func (h *FactionHandler) HandleLootMessages(sess *session.Session, msg *protocol.Message) error {
	log.Printf("LootMessages from %s: %x", sess.RemoteAddr(), msg.Data())
	return nil
}

// HandleClientFaction answers a faction page request, 0x0AC4 (func_00BEA6B8
// [A]): a u8 0x66, a zigzag token (one more per opening of the window), a
// zigzag page. The client asks for page 0, then the next on each answer, and
// stops when page+1 reaches the total (claude-notes/go-client-egress-pass-
// 20261016/senders.md). So EVERY request is answered — a page with nothing on
// it, or past the end, with a count of 0 — or the client's paging stops there.
// The answer, 0x07E2: a u8 0x77, the token, the page, the total pages and the
// count, all zigzag, then per faction a wide name and a zigzag standing.
//
// The pages are the character's own standings (entity.World.PlayerFactions),
// ten a page in the eqoa.live capture's order: standing high to low, then
// name. The page was once read and echoed as one raw byte, which is its
// zigzag only below 64.
func (h *FactionHandler) HandleClientFaction(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 3 {
		return nil
	}

	reader := buffer.NewReader(data)
	marker, _ := reader.ReadByte()
	if marker != 0x66 {
		log.Printf("ClientFaction: unexpected marker 0x%02X", marker)
		return nil
	}
	token, err := reader.Read7BitEncodedInt64()
	if err != nil {
		return nil
	}
	page, err := reader.Read7BitEncodedInt64()
	if err != nil {
		return nil
	}

	log.Printf("ClientFaction from %s: token=%d page=%d", sess.RemoteAddr(), token, page)

	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeFactionPage)
	resp.SetData(factionPagePayload(h.standings(sess), token, page))
	sess.Queue().AddWithSequence(resp)
	return nil
}

// standings is the faction window's list for the session: its player's own
// standings, or the starting ones with no world.
func (h *FactionHandler) standings(sess *session.Session) []entity.FactionStanding {
	if h.gameWorld != nil {
		return h.gameWorld.PlayerFactions(sess)
	}
	out := make([]entity.FactionStanding, len(database.DefaultFactions))
	for i, f := range database.DefaultFactions {
		out[i] = entity.FactionStanding{Name: f.Name, Standing: f.Standing}
	}
	return out
}

// factionsPerPage is how many factions a page holds: the capture's ten.
const factionsPerPage = 10

// factionPageCount is how many pages a list of standings fills.
func factionPageCount(list []entity.FactionStanding) int64 {
	return int64((len(list) + factionsPerPage - 1) / factionsPerPage)
}

// factionPagePayload is page `page` of a list of standings, as 0x07E2.
func factionPagePayload(list []entity.FactionStanding, token, page int64) []byte {
	var entries []entity.FactionStanding
	if pages := factionPageCount(list); page >= 0 && page < pages {
		from := int(page) * factionsPerPage
		entries = list[from:min(from+factionsPerPage, len(list))]
	}
	size := 2 + 1 + 3*10 + 10
	for _, f := range entries {
		size += 4 + 2*len([]rune(f.Name)) + 10
	}
	buf := make([]byte, size)
	w := buffer.NewWriter(buf)
	w.WriteUint16(uint16(protocol.OpcodeFactionPage))
	w.WriteByte(0x77)
	w.Write7BitEncodedInt64(token)
	w.Write7BitEncodedInt64(page)
	w.Write7BitEncodedInt64(factionPageCount(list))
	w.Write7BitEncodedInt64(int64(len(entries)))
	for _, f := range entries {
		writeWideString(w, f.Name, 128)
		w.Write7BitEncodedInt64(int64(f.Standing))
	}
	return buf[:w.Position()]
}
