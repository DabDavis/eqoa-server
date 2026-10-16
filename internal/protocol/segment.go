package protocol

import (
	"github.com/eqoa/pkg/pkg/buffer"
)

// SegmentHeaderFlags contains flags for the segment header.
type SegmentHeaderFlags uint32

const (
	// SegmentFlagNewInstance indicates a new session instance.
	SegmentFlagNewInstance SegmentHeaderFlags = 0x00080000
	// SegmentFlagHasInstance indicates the packet contains an instance ID.
	SegmentFlagHasInstance SegmentHeaderFlags = 0x00002000
	// SegmentFlagIsRemote indicates a remote endpoint.
	SegmentFlagIsRemote SegmentHeaderFlags = 0x00000800
	// SegmentFlagResetConnection indicates connection reset/termination.
	SegmentFlagResetConnection SegmentHeaderFlags = 0x00010000
	// SegmentFlagData0x40000 is an unknown flag.
	SegmentFlagData0x40000 SegmentHeaderFlags = 0x00040000
	// SegmentFlagDidServerInitiate indicates server initiated the session.
	SegmentFlagDidServerInitiate SegmentHeaderFlags = 0x00004000
	// SegmentFlagClientInitiated indicates client initiated the session.
	SegmentFlagClientInitiated SegmentHeaderFlags = 0x00001000

	// SegmentHeaderFlagsMask masks off the size bits from flags.
	SegmentHeaderFlagsMask SegmentHeaderFlags = 0xFFFFF800
	// SegmentHeaderSizeMask masks the size bits.
	SegmentHeaderSizeMask uint64 = 0x7FF
)

// SegmentHeader represents the header portion of a network segment.
type SegmentHeader struct {
	Flags          SegmentHeaderFlags
	Size           int
	Instance       uint32
	Data0x40000    int
	SrcAddr        uint32
	LocalEndpoint  uint16
	RemoteEndpoint uint32
}

// Read parses a segment header from the buffer reader.
func (h *SegmentHeader) Read(reader *buffer.Reader, localEndpoint, remoteEndpoint uint16) error {
	// Read combined flags and length as 7-bit encoded value
	flagsLen, err := reader.Read7BitEncodedUInt64()
	if err != nil {
		return err
	}

	h.Flags = SegmentHeaderFlags(flagsLen & uint64(SegmentHeaderFlagsMask))
	h.Size = int(flagsLen & SegmentHeaderSizeMask)
	h.LocalEndpoint = localEndpoint

	// Read instance ID if present
	if h.Flags&SegmentFlagHasInstance != 0 {
		h.Instance, err = reader.ReadUint32()
		if err != nil {
			return err
		}
	}

	// Read reset connection data
	if h.Flags&SegmentFlagResetConnection != 0 {
		val, err := reader.ReadUint32()
		if err != nil {
			return err
		}
		// Store as uint32 in LocalEndpoint context (used for connection reset)
		_ = val
	}

	// Determine remote endpoint
	if h.Flags&SegmentFlagIsRemote == 0 {
		h.RemoteEndpoint = uint32(remoteEndpoint)
	} else {
		val, err := reader.Read7BitEncodedInt64()
		if err != nil {
			return err
		}
		h.RemoteEndpoint = uint32(val)
	}

	return nil
}

// Write serializes the segment header to a buffer writer.
func (h *SegmentHeader) Write(writer *buffer.Writer, hasInstance, didServerInitiate bool) error {
	// Build flags value
	flags := uint64(0)

	if h.Flags&SegmentFlagNewInstance != 0 {
		flags |= uint64(SegmentFlagNewInstance)
	}
	if didServerInitiate {
		flags |= uint64(SegmentFlagDidServerInitiate)
	} else {
		flags |= uint64(SegmentFlagClientInitiated)
	}
	if hasInstance {
		flags |= uint64(SegmentFlagHasInstance)
	}

	// Combine flags with size
	flagsLen := flags | uint64(h.Size)

	return writer.Write7BitEncodedUInt64(flagsLen)
}

// SegmentBodyFlags contains flags for the segment body.
type SegmentBodyFlags byte

const (
	// BodyFlagFlushAck indicates flush acknowledgment present.
	BodyFlagFlushAck SegmentBodyFlags = 0x01
	// BodyFlagGuaranteedAck indicates guaranteed delivery ack present.
	BodyFlagGuaranteedAck SegmentBodyFlags = 0x02
	// BodyFlagUnknownPacked is an unknown flag.
	BodyFlagUnknownPacked SegmentBodyFlags = 0x04
	// BodyFlagGuaranteedAckMask indicates ack mask present.
	BodyFlagGuaranteedAckMask SegmentBodyFlags = 0x08
	// BodyFlagClientUpdateAck indicates client state update acks present.
	BodyFlagClientUpdateAck SegmentBodyFlags = 0x10
	// BodyFlagRdpMessage indicates RDP message present.
	BodyFlagRdpMessage SegmentBodyFlags = 0x20
	// BodyFlagSessionAck indicates session acknowledgment present.
	BodyFlagSessionAck SegmentBodyFlags = 0x40
)

// StateAck represents an acknowledgment for a state channel.
type StateAck struct {
	Channel byte
	Ack     uint16
}

// SegmentBody represents the body portion of a network segment.
type SegmentBody struct {
	Flags              SegmentBodyFlags
	InstanceLocal      int32
	InstanceLocalOK    bool
	ArrivalPositive    bool
	ArrivalNumber      uint16
	FlushAck           uint16
	UnknownPacked      int64
	GuaranteedAckMaybe bool
	GuaranteedAck      uint16
	GuaranteedAckMask  int64
	StateAcks          []StateAck
	Messages           []*Message
}

// NewSegmentBody creates a new empty segment body.
func NewSegmentBody() *SegmentBody {
	return &SegmentBody{
		StateAcks: make([]StateAck, 0, 24),
		Messages:  make([]*Message, 0, 8),
	}
}

// Read parses the segment body from the buffer reader.
func (b *SegmentBody) Read(reader *buffer.Reader, data []byte, header *SegmentHeader) error {
	bodyStart := reader.Position()

	// Read body flags
	flags, err := reader.ReadByte()
	if err != nil {
		return err
	}
	b.Flags = SegmentBodyFlags(flags)

	// Read session ack if present
	if b.Flags&BodyFlagSessionAck != 0 {
		val, err := reader.ReadInt32()
		if err != nil {
			return err
		}
		b.InstanceLocal = val
	}

	// Always read arrival number
	b.ArrivalPositive = true
	b.ArrivalNumber, err = reader.ReadUint16()
	if err != nil {
		return err
	}

	// Read flush ack if present
	if b.Flags&BodyFlagFlushAck != 0 {
		b.FlushAck, err = reader.ReadUint16()
		if err != nil {
			return err
		}

		// Read unknown packed value if present
		if b.Flags&BodyFlagUnknownPacked != 0 {
			b.UnknownPacked, err = reader.Read7BitEncodedInt64()
			if err != nil {
				return err
			}
		}
	}

	// Read guaranteed ack if present
	if b.Flags&BodyFlagGuaranteedAck != 0 {
		b.GuaranteedAck, err = reader.ReadUint16()
		if err != nil {
			return err
		}

		// Read guaranteed ack mask if present
		if b.Flags&BodyFlagGuaranteedAckMask != 0 {
			b.GuaranteedAckMask, err = reader.Read7BitEncodedInt64()
			if err != nil {
				return err
			}
		}
	}

	// Read state channel acks if present
	if b.Flags&BodyFlagClientUpdateAck != 0 {
		for {
			channel, err := reader.ReadByte()
			if err != nil {
				return err
			}

			// 0xF8 indicates end of state acks
			if channel == 0xF8 {
				break
			}

			// Invalid channel
			if channel > 0xF8 {
				return ErrInvalidChannel
			}

			ack, err := reader.ReadUint16()
			if err != nil {
				return err
			}

			b.StateAcks = append(b.StateAcks, StateAck{
				Channel: channel,
				Ack:     ack,
			})
		}
	}

	// Read messages
	for {
		// Check if we've read all data in the segment
		if reader.Position()-bodyStart >= header.Size {
			break
		}

		msgType, err := reader.ReadByte()
		if err != nil {
			return err
		}

		size, err := reader.ReadSize()
		if err != nil {
			return err
		}

		if size > MaxMessageSize {
			return ErrMessageTooLarge
		}

		mt := MessageType(msgType)

		// Handle reliable messages
		if mt == MessageTypeReliable || mt == MessageTypePing || mt == MessageTypeSegmentReliable {
			seqNum, err := reader.ReadUint16()
			if err != nil {
				return err
			}

			var opcode GameOpcode
			if mt != MessageTypePing {
				op, err := reader.ReadUint16()
				if err != nil {
					return err
				}
				opcode = GameOpcode(op)
				size -= 2 // Remove opcode from data size
			}

			// Read message data
			msgData, err := reader.ReadBytes(size)
			if err != nil {
				return err
			}

			msg := GetMessage()
			msg.Type = mt
			msg.Opcode = opcode
			msg.Sequence = seqNum
			msg.SetData(msgData)
			b.Messages = append(b.Messages, msg)
			continue
		}

		// Handle unreliable messages
		if mt == MessageTypeUnreliable {
			op, err := reader.ReadUint16()
			if err != nil {
				return err
			}
			size -= 2 // Remove opcode from data size

			msgData, err := reader.ReadBytes(size)
			if err != nil {
				return err
			}

			msg := GetMessage()
			msg.Type = mt
			msg.Opcode = GameOpcode(op)
			msg.Sequence = 0
			msg.SetData(msgData)
			b.Messages = append(b.Messages, msg)
			continue
		}

		// Handle state channel messages (0x00-0xF7)
		// These are position/state updates from the client
		// Format: type(1) + size(var) + seqNum(2) + refNum(1) + RLE_data(until 0x00 terminator)
		// The 'size' field is the DECODED size, not the encoded size!
		if msgType <= 0xF7 {
			remaining := reader.Remaining()

			// Need at least 3 bytes for seqNum + refNum
			if remaining < 3 {
				break
			}

			seqNum, _ := reader.ReadUint16()
			refNum, _ := reader.ReadByte()

			// Read and decode RLE data directly from the reader.
			// Two control byte formats (matching Java SegmentBody.runLengthDecode):
			//   High bit set (b & 0x80): copyCount = b & 0x7F, zeroCount = next_byte
			//   High bit clear:          copyCount = b >> 4,    zeroCount = b & 0x0F
			var decoded []byte
			for reader.Remaining() > 0 {
				b, err := reader.ReadByte()
				if err != nil || b == 0x00 {
					break
				}

				var copyCount, zeroCount int
				if b&0x80 != 0 {
					copyCount = int(b & 0x7F)
					z, _ := reader.ReadByte()
					zeroCount = int(z)
				} else {
					copyCount = int(b >> 4)
					zeroCount = int(b & 0x0F)
				}

				for j := 0; j < zeroCount; j++ {
					decoded = append(decoded, 0)
				}
				if copyCount > 0 && reader.Remaining() >= copyCount {
					data, _ := reader.ReadBytes(copyCount)
					decoded = append(decoded, data...)
				}
			}

			msg := GetMessage()
			msg.Type = mt
			msg.Sequence = seqNum
			msg.XORByte = refNum
			msg.SetData(decoded)
			b.Messages = append(b.Messages, msg)
			continue
		}

		// Invalid message type
		return ErrInvalidMessageType
	}

	return nil
}

// Reset clears the segment body for reuse.
func (b *SegmentBody) Reset() {
	b.Flags = 0
	b.InstanceLocal = 0
	b.InstanceLocalOK = false
	b.ArrivalPositive = false
	b.ArrivalNumber = 0
	b.FlushAck = 0
	b.UnknownPacked = 0
	b.GuaranteedAckMaybe = false
	b.GuaranteedAck = 0
	b.GuaranteedAckMask = 0
	b.StateAcks = b.StateAcks[:0]

	// Return messages to pool
	for _, msg := range b.Messages {
		PutMessage(msg)
	}
	b.Messages = b.Messages[:0]
}

// runLengthDecode decodes run-length encoded data matching Java eqoa.live format.
// Two control byte formats:
//
//	High bit set (b & 0x80): copyCount = b & 0x7F, zeroCount = next_byte
//	High bit clear:          copyCount = b >> 4,    zeroCount = b & 0x0F
//
// Zeroes are inserted first, then bytes are copied. Terminated by 0x00.
func runLengthDecode(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}

	result := make([]byte, 0, len(data)*2)

	i := 0
	for i < len(data) {
		b := data[i]
		i++

		if b == 0x00 {
			break
		}

		var copyCount, zeroCount int
		if b&0x80 != 0 {
			copyCount = int(b & 0x7F)
			if i >= len(data) {
				break
			}
			zeroCount = int(data[i])
			i++
		} else {
			copyCount = int(b >> 4)
			zeroCount = int(b & 0x0F)
		}

		for j := 0; j < zeroCount; j++ {
			result = append(result, 0)
		}
		for j := 0; j < copyCount && i < len(data); j++ {
			result = append(result, data[i])
			i++
		}
	}

	return result
}

// RunLengthEncode encodes data using run-length encoding matching Java eqoa.live format.
// Format: (copyCount|0x80), zeroCount, [copyBytes]... repeated, terminated by 0x00
// From each position: count leading zeroes, then count bytes to copy (allowing isolated zeros).
func RunLengthEncode(data []byte) []byte {
	if len(data) == 0 {
		return []byte{0x00} // Just terminator
	}

	result := make([]byte, 0, len(data)+10)
	length := len(data)
	i := 0

	for i < length {
		// Count leading zeroes from position i
		j := i
		for j < length && data[j] == 0 && j-i < 0x7F {
			j++
		}
		zeroes := j - i

		// Count bytes to copy (including isolated zeros followed by non-zero)
		// Java: (buf.getByte(j) != 0 || j == end-1 || buf.getByte(j+1) != 0)
		for j < length && (data[j] != 0 || j == length-1 || data[j+1] != 0) && j-i-zeroes < 0x7F {
			j++
		}
		copyCount := j - zeroes - i

		if copyCount == 0 && zeroes == 0 {
			break
		}

		// Write: (copyCount|0x80), zeroCount, [copyBytes]
		result = append(result, byte(copyCount|0x80))
		result = append(result, byte(zeroes))
		if copyCount > 0 {
			result = append(result, data[i+zeroes:i+zeroes+copyCount]...)
		}

		i += copyCount + zeroes
	}

	// Terminator
	result = append(result, 0x00)

	return result
}
