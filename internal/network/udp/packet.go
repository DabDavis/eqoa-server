package udp

import (
	"errors"
	"fmt"
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/protocol"
)

var (
	ErrPacketTooShort = errors.New("packet too short")
	ErrInvalidCRC     = errors.New("invalid packet CRC")
)

// Packet represents a parsed UDP packet.
type Packet struct {
	LocalEndpoint  uint16
	RemoteEndpoint uint16
	Header         *protocol.SegmentHeader
	Body           *protocol.SegmentBody
	RawData        []byte
}

// GetLocalEndpoint returns the client's endpoint ID.
func (p *Packet) GetLocalEndpoint() uint16 {
	return p.LocalEndpoint
}

// GetRemoteEndpoint returns the remote endpoint ID.
func (p *Packet) GetRemoteEndpoint() uint16 {
	return p.RemoteEndpoint
}

// GetInstanceID returns the instance ID from the packet header.
func (p *Packet) GetInstanceID() uint32 {
	if p.Header != nil {
		return p.Header.Instance
	}
	return 0
}

// IsNewConnection returns true if this is a new connection packet.
func (p *Packet) IsNewConnection() bool {
	if p.Header != nil {
		return p.Header.Flags&protocol.SegmentFlagNewInstance != 0
	}
	return false
}

// ParsePacket parses a raw UDP packet.
func ParsePacket(data []byte) (*Packet, error) {
	// Minimum packet size: 2 (local) + 2 (remote) + 1 (header) + 4 (CRC)
	if len(data) < 9 {
		return nil, ErrPacketTooShort
	}

	// Validate CRC
	if !protocol.ValidateCRC(data) {
		return nil, ErrInvalidCRC
	}

	reader := buffer.NewReader(data)

	// Read endpoints
	localEndpoint, err := reader.ReadUint16()
	if err != nil {
		return nil, fmt.Errorf("failed to read local endpoint: %w", err)
	}

	remoteEndpoint, err := reader.ReadUint16()
	if err != nil {
		return nil, fmt.Errorf("failed to read remote endpoint: %w", err)
	}

	// Parse segment header
	header := &protocol.SegmentHeader{}
	if err := header.Read(reader, localEndpoint, remoteEndpoint); err != nil {
		return nil, fmt.Errorf("failed to read segment header: %w", err)
	}

	log.Printf("ParsePacket: local=0x%04X remote=0x%04X flags=0x%X size=%d instance=0x%X",
		localEndpoint, remoteEndpoint, header.Flags, header.Size, header.Instance)

	// Check for reset connection - but only if it's ONLY the reset flag without other session flags
	// Flag 0x10000 combined with HasInstance (0x2000) is NOT a reset - it's a session establishment
	isReset := header.Flags&protocol.SegmentFlagResetConnection != 0 &&
		header.Flags&protocol.SegmentFlagHasInstance == 0
	if isReset {
		log.Printf("ParsePacket: ResetConnection flag detected (pure reset), returning packet with nil body")
		return &Packet{
			LocalEndpoint:  localEndpoint,
			RemoteEndpoint: remoteEndpoint,
			Header:         header,
			Body:           nil,
			RawData:        data,
		}, nil
	}

	// For packets with size=0 but HasInstance, create empty body (ACK-only packet)
	if header.Size == 0 {
		log.Printf("ParsePacket: size=0 packet (ACK-only), creating empty body")
		return &Packet{
			LocalEndpoint:  localEndpoint,
			RemoteEndpoint: remoteEndpoint,
			Header:         header,
			Body:           protocol.NewSegmentBody(),
			RawData:        data,
		}, nil
	}

	// Parse segment body
	body := protocol.NewSegmentBody()
	if err := body.Read(reader, data, header); err != nil {
		return nil, fmt.Errorf("failed to read segment body: %w", err)
	}

	return &Packet{
		LocalEndpoint:  localEndpoint,
		RemoteEndpoint: remoteEndpoint,
		Header:         header,
		Body:           body,
		RawData:        data,
	}, nil
}

// PacketBuilder helps construct outgoing packets.
type PacketBuilder struct {
	writer *buffer.Writer
	buf    []byte
}

// NewPacketBuilder creates a new packet builder.
func NewPacketBuilder() *PacketBuilder {
	buf := make([]byte, MaxPacketSize)
	return &PacketBuilder{
		writer: buffer.NewWriter(buf),
		buf:    buf,
	}
}

// Reset resets the builder for a new packet.
func (b *PacketBuilder) Reset() {
	b.writer.Reset()
}

// WriteEndpoints writes the local and remote endpoint IDs.
func (b *PacketBuilder) WriteEndpoints(serverID, clientID uint16) error {
	if err := b.writer.WriteUint16(serverID); err != nil {
		return err
	}
	return b.writer.WriteUint16(clientID)
}

// WriteSegmentHeader writes the segment header.
func (b *PacketBuilder) WriteSegmentHeader(header *protocol.SegmentHeader, hasInstance, didServerInitiate bool) error {
	return header.Write(b.writer, hasInstance, didServerInitiate)
}

// WriteInstance writes the instance ID.
func (b *PacketBuilder) WriteInstance(instanceID uint32) error {
	return b.writer.WriteUint32(instanceID)
}

// WriteSessionID writes a 7-bit encoded session ID.
func (b *PacketBuilder) WriteSessionID(sessionID uint32) error {
	return b.writer.Write7BitEncodedUInt64(uint64(sessionID))
}

// WriteBodyFlags writes the segment body flags.
func (b *PacketBuilder) WriteBodyFlags(flags protocol.SegmentBodyFlags) error {
	return b.writer.WriteByte(byte(flags))
}

// WriteSessionAck writes a session acknowledgment.
func (b *PacketBuilder) WriteSessionAck(instanceID uint32) error {
	return b.writer.WriteUint32(instanceID)
}

// WriteRDPReport writes the RDP report (packet sequence, acks).
func (b *PacketBuilder) WriteRDPReport(packetSeq, lastRecvPacket, lastRecvMsg uint16, includeAcks bool) error {
	if err := b.writer.WriteUint16(packetSeq); err != nil {
		return err
	}
	if includeAcks {
		if err := b.writer.WriteUint16(lastRecvPacket); err != nil {
			return err
		}
		return b.writer.WriteUint16(lastRecvMsg)
	}
	return nil
}

// WriteStateAck writes a state channel acknowledgment.
func (b *PacketBuilder) WriteStateAck(channel byte, seqNum uint16) error {
	if err := b.writer.WriteByte(channel); err != nil {
		return err
	}
	return b.writer.WriteUint16(seqNum)
}

// WriteStateAckEnd writes the state ack terminator.
func (b *PacketBuilder) WriteStateAckEnd() error {
	return b.writer.WriteByte(0xF8)
}

// WriteMessage writes a message to the packet.
// Note: The opcode should be included at the start of the message data by the caller.
// This matches the C# reference implementation where each message handler writes
// the opcode to the message span before the payload data.
func (b *PacketBuilder) WriteMessage(msg *protocol.Message, seqNum uint16) error {
	// Checkpoint position so we can rollback on partial write failure
	checkpoint := b.writer.Position()

	// Write message type
	if err := b.writer.WriteByte(byte(msg.Type)); err != nil {
		b.writer.SetPosition(checkpoint)
		return err
	}

	// Handle state channel messages (0x00-0xF7) - need to RLE encode first to get correct size
	if msg.Type.IsStateChannel() && !msg.Type.IsReliable() {
		// RLE encode the data first
		rleData := protocol.RunLengthEncode(msg.Data())

		log.Printf("WriteMessage: type=0x%02X (state channel) seq=%d xorByte=%d rawSize=%d rleSize=%d",
			msg.Type, seqNum, msg.XORByte, msg.Size(), len(rleData))

		// Write ORIGINAL data size (matching C# and Java reference implementations).
		// Both write the pre-RLE data size here, NOT the encoded content size.
		// The PS2 client uses this value for buffer allocation during RLE decode.
		if err := b.writer.WriteSize(msg.Size()); err != nil {
			b.writer.SetPosition(checkpoint)
			return err
		}

		// Write seqNum + refNum + RLE data
		if err := b.writer.WriteUint16(seqNum); err != nil {
			b.writer.SetPosition(checkpoint)
			return err
		}
		if err := b.writer.WriteByte(msg.XORByte); err != nil {
			b.writer.SetPosition(checkpoint)
			return err
		}
		if err := b.writer.WriteBytes(rleData); err != nil {
			b.writer.SetPosition(checkpoint)
			return err
		}
		return nil
	}

	// Debug: log the size being written (cast opcode to uint16 to ensure numeric format)
	log.Printf("WriteMessage: type=0x%02X opcode=0x%04X (%s) seq=%d size=%d dataHex=%x",
		msg.Type, uint16(msg.Opcode), msg.Opcode.String(), seqNum, msg.Size(), msg.Data()[:min(20, msg.Size())])

	// Write size (data already includes opcode for reliable messages)
	if err := b.writer.WriteSize(msg.Size()); err != nil {
		b.writer.SetPosition(checkpoint)
		return err
	}

	// Write sequence number for reliable messages
	if msg.Type.IsReliable() {
		if err := b.writer.WriteUint16(seqNum); err != nil {
			b.writer.SetPosition(checkpoint)
			return err
		}
	}

	// Write message data (which includes opcode at the start)
	if err := b.writer.WriteBytes(msg.Data()); err != nil {
		b.writer.SetPosition(checkpoint)
		return err
	}
	return nil
}

// Finish completes the packet by appending the CRC.
func (b *PacketBuilder) Finish() []byte {
	data := b.writer.Bytes()
	return protocol.AppendCRC(data)
}

// Position returns the current write position.
func (b *PacketBuilder) Position() int {
	return b.writer.Position()
}

// Remaining returns the remaining buffer space.
func (b *PacketBuilder) Remaining() int {
	return b.writer.Remaining()
}

// PatchSegmentHeaderSize rewrites the segment header varint at the given position
// with the correct body size. The varint length is determined by the flags and is
// the same for any body size (0-2047), so the patch is always the same byte count
// as the placeholder.
func (b *PacketBuilder) PatchSegmentHeaderSize(pos int, flags uint64, bodySize int) {
	val := flags | uint64(bodySize)
	i := pos
	for val >= 0x80 {
		b.buf[i] = byte(val&0x7F) | 0x80
		val >>= 7
		i++
	}
	b.buf[i] = byte(val)
}
