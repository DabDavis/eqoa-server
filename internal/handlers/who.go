package handlers

import (
	"log"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
)

// WhoHandler handles who list requests.
type WhoHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

// NewWhoHandler creates a new who handler.
func NewWhoHandler(sessionMgr *session.Manager) *WhoHandler {
	return &WhoHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference.
func (h *WhoHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers who handlers with the registry.
func (h *WhoHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeWhoList, h.HandleWhoList)
}

// HandleWhoList handles the who list request (0x0E01).
// Client sends: byte temp1 (unused), byte temp2 (filter mode, echoed back)
// Server responds with 0x0E02 listing all online players except the requester.
func (h *WhoHandler) HandleWhoList(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		return nil
	}

	reader := buffer.NewReader(data)
	_, _ = reader.ReadByte()       // temp1 (unused)
	temp2, _ := reader.ReadByte()  // filter/mode — echoed in response

	log.Printf("WhoList request from %s (temp2=%d)", sess.RemoteAddr(), temp2)

	if h.gameWorld == nil {
		return nil
	}

	sender := h.gameWorld.SessionPlayer(sess)
	if sender == nil {
		return nil
	}

	// Collect all players except sender
	type playerInfo struct {
		Race  int
		Class int
		Level int
		Name  string
	}
	var players []playerInfo

	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		players = append(players, playerInfo{
			Race:  p.Race,
			Class: p.Class,
			Level: p.Level,
			Name:  p.Name,
		})
	})

	count := int64(len(players))

	// Build response: opcode + byte(1) + temp2 + 7BitEncodedInt64(count) + per-player data
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWhoListResponse)
	writer := buffer.NewWriter(resp.Span())

	writer.WriteUint16(uint16(protocol.OpcodeWhoListResponse))
	writer.WriteByte(1)     // constant
	writer.WriteByte(temp2) // echo filter mode
	writer.Write7BitEncodedInt64(count)

	for _, p := range players {
		writer.WriteByte(byte(p.Race))
		writer.WriteByte(byte(p.Class))
		writer.WriteByte(byte(p.Level))
		writer.WriteStringUTF8(p.Name)
		writer.WriteUint32(0) // objectID — not used for cross-zone targeting
	}

	resp.SetSize(writer.Position())
	sess.Queue().AddWithSequence(resp)

	log.Printf("WhoList: sent %d players to %s", count, sender.Name)
	return nil
}
