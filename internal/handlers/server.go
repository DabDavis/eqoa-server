package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"unicode/utf16"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// ServerInfo holds game server information.
type ServerInfo struct {
	Name        string
	Recommended byte
	EndpointID  uint16
	Port        uint16
	IP          net.IP
	Language    byte
}

// ServerHandler handles server list related messages.
type ServerHandler struct {
	servers []ServerInfo
}

// NewServerHandler creates a new server handler with the default server list:
// one shard. eqoa.live sends six identical "Marr's Fist" entries and Sandstorm
// four, behind a RAW count byte that CLIENT reads as zigzag (VIShardInfo::Read,
// 0x00BFC208: ReadSigned), so the byte 2 this server used to send with two
// entries reached a PS2 as ONE. One entry with a zigzag count is the same byte
// and the same first entry, without the second one a PS2 never read.
//
// Config's shards replace it (ShardList, Registry.SetServers).
func NewServerHandler(serverIP string) *ServerHandler {
	ip := net.ParseIP(serverIP)
	return &ServerHandler{
		servers: []ServerInfo{
			{Name: DefaultShardName, Recommended: 1, EndpointID: GameServerEndpoint, Port: 10070, IP: ip, Language: 0},
		},
	}
}

// DefaultShardName is the one shard's name when config names none.
const DefaultShardName = "Marr's Fist"

// maxShardName is the most characters CLIENT keeps of a shard's name
// (VIElem_BFC798::m_name, 32 wide with its terminator; ReadString cuts there).
const maxShardName = 31

// shardNameRoom is the widest a shard's name may draw and stay whole on one
// line of CLIENT's server list: VIWndServerSelect's name column is cols[1] -
// cols[0] - 16 = 245 (cols 58, 319, 460 of 640), and VIUIWindow::TextBox
// breaks a line once its text reaches 8 short of that, at the last space,
// with an ellipsis on a one-line box (flag 2). Vanilla's TextBox is the same.
// So "Live Server - No Wipes Ever" (253) showed as "Live Server - No Wipes
// ..." (the GM Jibwiz's petition 1923).
const shardNameRoom = 245 - 8

// arialUIWidths is each printable ASCII glyph's width in ARIALUI.CSF (the
// UI's font, 0x7020; ' ' through '~'), read off the disc's file.
var arialUIWidths = [95]uint8{5, 5, 7, 10, 10, 15, 12, 4, 6, 6, 7, 10, 5, 6, 5, 5, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 5, 5,
	10, 10, 10, 10, 17, 12, 12, 13, 13, 12, 11, 13, 13, 6, 9, 12, 10, 14, 13, 13, 12, 13, 13, 12, 11, 13, 12, 16, 12, 12, 11, 5,
	5, 5, 9, 9, 6, 10, 10, 9, 10, 10, 5, 10, 10, 4, 4, 9, 4, 14, 10, 10, 10, 10, 6, 9, 5, 10, 9, 13, 9, 9, 9, 6, 5, 6, 10}

// ShardNameWidth is how wide CLIENT draws a name in the list (GetStringWidth:
// each glyph's width and one); a character past ASCII counts as the widest.
func ShardNameWidth(name string) int {
	w := 0
	for _, r := range name {
		if r >= ' ' && r <= '~' {
			w += int(arialUIWidths[r-' ']) + 1
		} else {
			w += 18
		}
	}
	return w
}

// ShardNameFits is whether a name draws whole on one line of the list
// (shardNameRoom); a longer one is still listed, and the server says so.
func ShardNameFits(name string) bool { return ShardNameWidth(name) < shardNameRoom }

// ShardList is config's shards as the list's entries, with what is wrong with
// any of them; none configured, this server alone under the default name.
// Every entry's endpoint is the game server's (CLIENT connects to it whole,
// ConnectShard; HandleAuthenticate tells a game connection by it), so a shard
// differs from another by its address alone.
func ShardList(shards []config.ShardConfig, serverIP string, gamePort int) ([]ServerInfo, []error) {
	if len(shards) == 0 {
		shards = []config.ShardConfig{{Name: DefaultShardName, Port: gamePort, Recommended: true}}
	}
	var out []ServerInfo
	var bad []error
	seen := map[string]bool{}
	for _, c := range shards {
		ipText := c.IP
		if ipText == "" {
			ipText = serverIP
		}
		ip := net.ParseIP(ipText)
		switch {
		case c.Name == "" || len(utf16.Encode([]rune(c.Name))) > maxShardName:
			bad = append(bad, fmt.Errorf("shard %q: a name is 1 to %d characters", c.Name, maxShardName))
			continue
		case seen[c.Name]:
			// the window finds its selection and the home shard by name
			bad = append(bad, fmt.Errorf("shard %q: two shards of one name", c.Name))
			continue
		case c.Port <= 0 || c.Port > 65535:
			bad = append(bad, fmt.Errorf("shard %q: port %d", c.Name, c.Port))
			continue
		case ip == nil || ip.To4() == nil:
			bad = append(bad, fmt.Errorf("shard %q: address %q is not IPv4 (DRDP's are four bytes)", c.Name, ipText))
			continue
		}
		seen[c.Name] = true
		info := ServerInfo{Name: c.Name, EndpointID: GameServerEndpoint, Port: uint16(c.Port), IP: ip}
		if c.Recommended {
			info.Recommended = 1
		}
		out = append(out, info)
	}
	return out, bad
}

// RegisterHandlers registers server handlers with the registry.
func (h *ServerHandler) RegisterHandlers(r *Registry) {
	// Server list is typically sent after authentication, not requested
}

// SendServerList sends the game server list to a session.
// Format matches C# ServerListManager.ReadConfig (ServerListManager.cs lines 90-124).
func (h *ServerHandler) SendServerList(sess *session.Session) {
	log.Printf("Sending server list to %s (%d servers)", sess.RemoteAddr(), len(h.servers))

	// Use Unreliable message type like C#
	msg := protocol.NewMessage(protocol.MessageTypeUnreliable, protocol.OpcodeGameServers)
	writer := buffer.NewWriter(msg.Span())

	// Write opcode in data (C# reference format)
	writer.WriteUint16(uint16(protocol.OpcodeGameServers))

	// The count is ZIGZAG: CLIENT's VIShardInfo::Read takes it with
	// ReadSigned (VIMemStreamArray.h). A raw byte N reads as N/2 entries, and
	// one entry as -1, so nothing at all.
	writer.Write7BitEncodedInt64(int64(len(h.servers)))

	for _, srv := range h.servers {
		// Server name: WriteString(Encoding.Unicode, name)
		// C# writes int32 LE char count, then UTF-16LE bytes (no null terminator)
		name := utf16.Encode([]rune(srv.Name))
		writer.WriteUint32(uint32(len(name)))
		for _, u := range name {
			writer.WriteUint16(u)
		}

		// Recommended (1 byte)
		writer.WriteByte(srv.Recommended)

		// EndpointID (2 bytes LE)
		writer.WriteUint16(srv.EndpointID)

		// Port (2 bytes LE)
		writer.WriteUint16(srv.Port)

		// IP (4 bytes, reversed from network byte order - matching C# Array.Reverse)
		ip := srv.IP.To4()
		if ip != nil {
			writer.WriteByte(ip[3])
			writer.WriteByte(ip[2])
			writer.WriteByte(ip[1])
			writer.WriteByte(ip[0])
		} else {
			writer.WriteUint32(0)
		}

		// Language (1 byte)
		writer.WriteByte(srv.Language)
	}

	msg.SetSize(writer.Position())
	sess.Queue().Add(msg)

	log.Printf("Server list message built: %d bytes", writer.Position())
}

// writeUTF16String writes a length-prefixed UTF-16LE string.
func (h *ServerHandler) writeUTF16String(writer *buffer.Writer, s string) {
	// Convert to UTF-16LE
	utf16 := make([]byte, len(s)*2)
	for i, r := range s {
		binary.LittleEndian.PutUint16(utf16[i*2:], uint16(r))
	}

	// Write length (7-bit encoded, in characters not bytes)
	writer.Write7BitEncodedUInt64(uint64(len(s)))

	// Write UTF-16LE bytes
	writer.WriteBytes(utf16)
}

// writeUTF16StringNullTerm writes a null-terminated UTF-16LE string.
func (h *ServerHandler) writeUTF16StringNullTerm(writer *buffer.Writer, s string) {
	// Write string as UTF-16LE
	writer.WriteStringUnicodeRaw(s)
	// Null terminator (2 bytes for UTF-16)
	writer.WriteUint16(0)
}

// SetServers updates the server list.
func (h *ServerHandler) SetServers(servers []ServerInfo) {
	h.servers = servers
}

// AddServer adds a server to the list.
func (h *ServerHandler) AddServer(srv ServerInfo) {
	h.servers = append(h.servers, srv)
}
