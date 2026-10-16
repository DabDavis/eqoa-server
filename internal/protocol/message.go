package protocol

import (
	"errors"
	"sync"
	"time"
)

// Constants for message handling.
const (
	MaxMessageSize     = 0x07FE
	DefaultMessageSize = 0x514
)

// Errors for message handling.
var (
	ErrMessageTooLarge    = errors.New("message size exceeds maximum")
	ErrInvalidMessageType = errors.New("invalid message type")
	ErrInvalidChannel     = errors.New("invalid state channel")
	ErrInvalidStateRef    = errors.New("invalid state reference number")
)

// MessageType represents the type of a network message.
type MessageType byte

const (
	// MessageTypeObjectUpdateChannel0-23 are state update channels.
	MessageTypeObjectUpdateChannel0  MessageType = 0x00
	MessageTypeObjectUpdateChannel1  MessageType = 0x01
	MessageTypeObjectUpdateChannel2  MessageType = 0x02
	MessageTypeObjectUpdateChannel3  MessageType = 0x03
	MessageTypeObjectUpdateChannel4  MessageType = 0x04
	MessageTypeObjectUpdateChannel5  MessageType = 0x05
	MessageTypeObjectUpdateChannel6  MessageType = 0x06
	MessageTypeObjectUpdateChannel7  MessageType = 0x07
	MessageTypeObjectUpdateChannel8  MessageType = 0x08
	MessageTypeObjectUpdateChannel9  MessageType = 0x09
	MessageTypeObjectUpdateChannel10 MessageType = 0x0A
	MessageTypeObjectUpdateChannel11 MessageType = 0x0B
	MessageTypeObjectUpdateChannel12 MessageType = 0x0C
	MessageTypeObjectUpdateChannel13 MessageType = 0x0D
	MessageTypeObjectUpdateChannel14 MessageType = 0x0E
	MessageTypeObjectUpdateChannel15 MessageType = 0x0F
	MessageTypeObjectUpdateChannel16 MessageType = 0x10
	MessageTypeObjectUpdateChannel17 MessageType = 0x11
	MessageTypeObjectUpdateChannel18 MessageType = 0x12
	MessageTypeObjectUpdateChannel19 MessageType = 0x13
	MessageTypeObjectUpdateChannel20 MessageType = 0x14
	MessageTypeObjectUpdateChannel21 MessageType = 0x15
	MessageTypeObjectUpdateChannel22 MessageType = 0x16
	MessageTypeObjectUpdateChannel23 MessageType = 0x17

	// MessageTypeClientUpdate is client actor update (unreliable).
	MessageTypeClientUpdate MessageType = 0x40
	// MessageTypeGroupUpdate is group state update.
	MessageTypeGroupUpdate MessageType = 0x40
	// MessageTypeStatUpdate is stat state update.
	MessageTypeStatUpdate MessageType = 0x42
	// MessageTypeBuffUpdate is buff state update.
	MessageTypeBuffUpdate MessageType = 0x43

	// MessageTypePing is a ping message for keepalive.
	MessageTypePing MessageType = 0xF9
	// MessageTypeSegmentReliable is a segmented reliable message.
	MessageTypeSegmentReliable MessageType = 0xFA
	// MessageTypeReliable is a standard reliable message.
	MessageTypeReliable MessageType = 0xFB
	// MessageTypeUnreliable is an unreliable message.
	MessageTypeUnreliable MessageType = 0xFC
)

// IsReliable returns true if the message type requires reliable delivery.
func (mt MessageType) IsReliable() bool {
	return mt == MessageTypeReliable || mt == MessageTypeSegmentReliable || mt == MessageTypePing
}

// IsStateChannel returns true if the message type is a state channel (0x00-0xF7).
// State channels are used for position updates, object updates, etc.
// Special message types (Ping, Reliable, etc.) are 0xF8+.
func (mt MessageType) IsStateChannel() bool {
	return mt <= 0xF7
}

// Message represents a network message.
type Message struct {
	Type     MessageType
	Opcode   GameOpcode
	Sequence uint16
	XORByte  byte
	Created  time.Time
	LastSent time.Time
	data     []byte
	size     int
}

// messagePool is a pool of Message objects for reuse.
var messagePool = sync.Pool{
	New: func() interface{} {
		return &Message{
			data: make([]byte, DefaultMessageSize),
		}
	},
}

// GetMessage retrieves a Message from the pool.
func GetMessage() *Message {
	msg := messagePool.Get().(*Message)
	msg.Reset()
	return msg
}

// PutMessage returns a Message to the pool.
func PutMessage(msg *Message) {
	if msg == nil {
		return
	}
	// Only return standard-sized messages to the pool
	if cap(msg.data) == DefaultMessageSize {
		messagePool.Put(msg)
	}
}

// NewMessage creates a new message with the given type and opcode.
func NewMessage(msgType MessageType, opcode GameOpcode) *Message {
	msg := GetMessage()
	msg.Type = msgType
	msg.Opcode = opcode
	msg.Created = time.Now()
	msg.LastSent = msg.Created
	return msg
}

// NewSegmentedMessage creates a new message for large data that needs segmentation.
func NewSegmentedMessage(opcode GameOpcode) *Message {
	return &Message{
		Type:     MessageTypeSegmentReliable,
		Opcode:   opcode,
		Created:  time.Now(),
		LastSent: time.Now(),
		data:     make([]byte, 65535),
	}
}

// Reset clears the message for reuse.
func (m *Message) Reset() {
	m.Type = 0
	m.Opcode = 0
	m.Sequence = 0
	m.XORByte = 0
	m.Created = time.Time{}
	m.LastSent = time.Time{}
	m.size = 0
	// Clear data but keep the backing array
	for i := range m.data[:m.size] {
		m.data[i] = 0
	}
}

// Data returns the message data.
func (m *Message) Data() []byte {
	return m.data[:m.size]
}

// SetData copies data into the message.
func (m *Message) SetData(data []byte) {
	if len(data) > cap(m.data) {
		m.data = make([]byte, len(data))
	}
	m.size = copy(m.data, data)
}

// Size returns the data size.
func (m *Message) Size() int {
	return m.size
}

// SetSize sets the data size.
func (m *Message) SetSize(size int) {
	if size > cap(m.data) {
		size = cap(m.data)
	}
	m.size = size
}

// Span returns a writable slice of the message data buffer.
func (m *Message) Span() []byte {
	return m.data
}

// HeaderSize returns the size of the message header for serialization.
// Note: Opcode is now included in the data, not the header (C# reference format).
func (m *Message) HeaderSize() int {
	sizeBytes := 1
	if m.size >= 255 {
		sizeBytes = 3
	}

	switch m.Type {
	case MessageTypeReliable, MessageTypeSegmentReliable, MessageTypePing:
		// type(1) + size(1-3) + seq(2)
		// Note: opcode is in data, not header
		return 1 + sizeBytes + 2
	case MessageTypeUnreliable:
		// type(1) + size(1-3)
		// Note: opcode is in data, not header
		return 1 + sizeBytes
	default:
		// State channel: type(1) + size(1-3) + seq(2) + ref(1)
		return 1 + sizeBytes + 2 + 1
	}
}

// TotalSize returns the total serialized size including header and data.
// For state channels, this accounts for RLE encoding.
func (m *Message) TotalSize() int {
	// State channels need special handling - their data is RLE encoded
	if m.Type.IsStateChannel() && !m.Type.IsReliable() {
		// Pre-compute RLE size for actual wire bytes
		rleData := RunLengthEncode(m.Data())

		// Size field encoding uses ORIGINAL data size (matching C#/Java)
		sizeBytes := 1
		if m.size >= 255 {
			sizeBytes = 3
		}

		// type(1) + size_field(1-3) + seq(2) + ref(1) + RLE_data
		return 1 + sizeBytes + 2 + 1 + len(rleData)
	}

	return m.HeaderSize() + m.size
}

// UpdateTime updates the last sent timestamp.
func (m *Message) UpdateTime() {
	m.LastSent = time.Now()
}
