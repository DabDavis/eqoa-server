package handlers

import (
	"encoding/binary"
	"log"
	"net"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
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

// NewServerHandler creates a new server handler with default servers.
// Format matches eqoa.live which sends 6 identical "Marr's Fist" servers
func NewServerHandler(serverIP string) *ServerHandler {
	ip := net.ParseIP(serverIP)
	return &ServerHandler{
		servers: []ServerInfo{
			{Name: "Marr's Fist", Recommended: 1, EndpointID: 0xB991, Port: 10070, IP: ip, Language: 0},
			{Name: "Marr's Fist", Recommended: 0, EndpointID: 0xB991, Port: 10070, IP: ip, Language: 0},
		},
	}
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

	// Write server count as raw byte (from eqoa.live packet capture format)
	writer.WriteByte(byte(len(h.servers)))

	for _, srv := range h.servers {
		// Server name: WriteString(Encoding.Unicode, name)
		// C# writes int32 LE char count, then UTF-16LE bytes (no null terminator)
		writer.WriteUint32(uint32(len(srv.Name)))
		for _, r := range srv.Name {
			writer.WriteUint16(uint16(r))
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
