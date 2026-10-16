package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
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

// HandleWhoList handles the who list request (0x0E01), VIClientPart38334::Open
// (0x00C03038 [A]): a u8 1, then a ZIGZAG token, one more per request. The
// answer, 0x0E02 (Recv0E02 0x00C03168 [A]), is a u8 1, the same token as a
// zigzag — a page whose token is not the last request's is dropped — then a
// zigzag count and the players. The token was read and echoed as one raw
// byte, which is the zigzag only below 64: from the 64th request on the
// client dropped every answer.
func (h *WhoHandler) HandleWhoList(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 2 {
		return nil
	}

	reader := buffer.NewReader(data)
	_, _ = reader.ReadByte() // 1
	token, err := reader.Read7BitEncodedInt64()
	if err != nil {
		return nil
	}

	log.Printf("WhoList request from %s (token=%d)", sess.RemoteAddr(), token)

	if h.gameWorld == nil {
		return nil
	}

	sender := h.gameWorld.SessionPlayer(sess)
	if sender == nil {
		return nil
	}

	// Every player in the world, the sender included. The row's last word is
	// the id the requester knows the player by: CLIENT's InviteToGroup takes
	// the row as a VIInviteRec, whose +0x1C is the id it sends back in 0x0625.
	// Collected first: IDSeenBy takes the world's lock.
	var all []*entity.Player
	h.gameWorld.ForEachPlayer(func(p *entity.Player) {
		all = append(all, p)
	})
	var players []whoEntry
	for _, p := range all {
		players = append(players, whoEntry{
			Race:  p.Race,
			Class: p.Class,
			Level: p.Level,
			Name:  p.Name,
			ID:    h.gameWorld.IDSeenBy(sess, p),
		})
	}

	count := int64(len(players))

	sess.Queue().AddWithSequence(whoListResponse(token, players))

	log.Printf("WhoList: sent %d players to %s", count, sender.Name)
	return nil
}

// whoEntry is one line of the who list.
type whoEntry struct {
	Race  int
	Class int
	Level int
	Name  string
	ID    uint32
}

// whoListResponse is 0x0E02: a u8 1, the request's token and the count as
// zigzags, then per player a u8 race, class and level, the name (u32 length
// and bytes, ReadString's) and a u32 the client does not use. It is built in
// a buffer of its own size: the message's own span is 2,046 bytes, which a
// list of a hundred-odd players overflows (Sandstorm's runs to 4 KB and comes
// fragmented, which AddWithSequence does).
func whoListResponse(token int64, players []whoEntry) *protocol.Message {
	size := 2 + 1 + 10 + 10
	for _, p := range players {
		size += 3 + 4 + len(p.Name) + 4
	}
	buf := make([]byte, size)
	writer := buffer.NewWriter(buf)
	writer.WriteUint16(uint16(protocol.OpcodeWhoListResponse))
	writer.WriteByte(1)
	writer.Write7BitEncodedInt64(token)
	writer.Write7BitEncodedInt64(int64(len(players)))
	for _, p := range players {
		writer.WriteByte(byte(p.Race))
		writer.WriteByte(byte(p.Class))
		writer.WriteByte(byte(p.Level))
		writer.WriteStringUTF8(p.Name)
		writer.WriteUint32(p.ID)
	}
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWhoListResponse)
	resp.SetData(buf[:writer.Position()])
	return resp
}
