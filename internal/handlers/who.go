package handlers

import (
	"fmt"
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

	// Every player the list shows (whoShows), the sender included. The row's last word is
	// the id the requester knows the player by: CLIENT's InviteToGroup takes
	// the row as a VIInviteRec, whose +0x1C is the id it sends back in 0x0625.
	// Collected first: IDSeenBy takes the world's lock.
	all := whoPlayers(h.gameWorld, sender)
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

	count := len(players)

	sess.Queue().AddWithSequence(whoListResponse(token, players))
	// How many are online, the owner's ask (2026-11-20: "put players online in who menu in
	// game"). VIWndWhoList (CLIENT 0x00CA2A60) draws a fixed title (word 730), the four
	// headings, the rows and a legend: nothing the server fills but the rows, and a row of
	// its own would be selectable (Tell, Invite, Mail) and sorted among the players by S2's
	// patch and by vanilla. So the count is a system line: OpenPanel35 sets the chat to its
	// fixed mode (VIWndGame, SetMode(1): five lines under the window), where a PS2 shows it
	// beside the list, vanilla in its chat box, each request (Triangle's refresh too) its own.
	sess.Queue().AddWithSequence(buildClientMessage(whoOnlineLine(count)))

	log.Printf("WhoList: sent %d players to %s", count, sender.Name)
	return nil
}

// whoPlayers is everyone in the world a who list shows viewer (whoShows), the viewer
// included. Collected before anything else takes the world's lock (IDSeenBy does).
func whoPlayers(w *entity.World, viewer *entity.Player) []*entity.Player {
	var all []*entity.Player
	w.ForEachPlayer(func(p *entity.Player) {
		if whoShows(viewer, p) {
			all = append(all, p)
		}
	})
	return all
}

// whoShows is whether a who list (the window's rows and count, /who's lines) shows p to
// viewer: a GM hidden by Toggle Invis (GMHidden) is left out of anyone's but a GM's and its
// own, so the list does not give away a GM the world does not show.
func whoShows(viewer, p *entity.Player) bool {
	return !p.GMHidden || p == viewer || (viewer != nil && viewer.GM)
}

// whoOnlineLine is the who window's count: the players of this shard's world the list
// shows. Shards are separate servers sharing nothing at run time, so it is this one's alone.
func whoOnlineLine(n int) string {
	if n == 1 {
		return "1 player online."
	}
	return fmt.Sprintf("%d players online.", n)
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
// list of a hundred-odd players overflows (the reference server's runs to 4 KB and comes
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
		writer.WriteByte(byte(entity.WireRace(p.Race)))
		writer.WriteByte(byte(p.Class))
		writer.WriteByte(byte(p.Level))
		writer.WriteStringUTF8(p.Name)
		writer.WriteUint32(p.ID)
	}
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeWhoListResponse)
	resp.SetData(buf[:writer.Position()])
	return resp
}
