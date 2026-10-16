package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/pkg/pkg/buffer"
)

const MaxPacketSize = 0x530

type TestClient struct {
	conn       *net.UDPConn
	serverAddr *net.UDPAddr
	clientID   uint16
	serverID   uint16
	instanceID uint32
	hasInstance bool
	packetSeq   uint16
	messageSeq  uint16
	lastRecvPacket uint16
	lastRecvMsg    uint16
}

func main() {
	serverAddr := flag.String("server", "127.0.0.1:10070", "Server address")
	charServerID := flag.Uint("char", 49870, "Character server_id to select")
	flag.Parse()

	client, err := NewTestClient(*serverAddr)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	log.Println("=== EQOA Test Client ===")
	log.Printf("Connecting to %s", *serverAddr)

	// Step 1: Initial connection to login server (0xFFFE)
	log.Println("\n--- Step 1: Initial Connection ---")
	if err := client.SendInitialConnection(); err != nil {
		log.Fatalf("Initial connection failed: %v", err)
	}
	if err := client.ReceiveAndPrint("Initial response"); err != nil {
		log.Fatalf("Failed to receive: %v", err)
	}

	// Step 2: Send authentication (opcode 0x0904)
	log.Println("\n--- Step 2: Authentication ---")
	if err := client.SendAuthentication(); err != nil {
		log.Fatalf("Auth failed: %v", err)
	}
	if err := client.ReceiveAndPrint("Server list"); err != nil {
		log.Fatalf("Failed to receive: %v", err)
	}

	// Step 3: Connect to game server (0xB991)
	log.Println("\n--- Step 3: Game Server Connection ---")
	client.serverID = 0xB991
	client.packetSeq = 0
	client.messageSeq = 0
	if err := client.SendGameServerAuth(); err != nil {
		log.Fatalf("Game auth failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := client.ReceiveAndPrint(fmt.Sprintf("Game response %d", i+1)); err != nil {
			break
		}
	}

	// Step 4: Select character
	log.Println("\n--- Step 4: Character Selection ---")
	if err := client.SendCharacterSelect(uint32(*charServerID)); err != nil {
		log.Fatalf("Char select failed: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := client.ReceiveAndPrint(fmt.Sprintf("MemoryDump %d", i+1)); err != nil {
			break
		}
	}

	// Step 5: Enter world (opcode 0x14)
	log.Println("\n--- Step 5: Enter World ---")
	if err := client.SendEnterWorld(); err != nil {
		log.Fatalf("Enter world failed: %v", err)
	}

	// Receive state updates
	log.Println("\n--- Receiving State Updates ---")
	for i := 0; i < 10; i++ {
		if err := client.ReceiveAndPrint(fmt.Sprintf("Update %d", i+1)); err != nil {
			log.Printf("Receive error: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	log.Println("\n=== Test Complete ===")
}

func NewTestClient(serverAddr string) (*TestClient, error) {
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return &TestClient{
		conn:       conn,
		serverAddr: addr,
		clientID:   0x7384,
		serverID:   0xFFFE,
		instanceID: 0x73FD6058,
	}, nil
}

func (c *TestClient) Close() { c.conn.Close() }

func (c *TestClient) SendInitialConnection() error {
	buf := make([]byte, MaxPacketSize)
	writer := buffer.NewWriter(buf)
	writer.WriteUint16(c.clientID)
	writer.WriteUint16(c.serverID)
	flags := uint64(protocol.SegmentFlagNewInstance) | uint64(protocol.SegmentFlagHasInstance) | uint64(protocol.SegmentFlagClientInitiated)
	bodySize := 7
	writer.Write7BitEncodedUInt64(flags | uint64(bodySize))
	writer.WriteUint32(c.instanceID)
	c.hasInstance = true
	writer.WriteByte(byte(protocol.BodyFlagSessionAck | protocol.BodyFlagRdpMessage))
	writer.WriteUint32(c.instanceID)
	c.packetSeq++
	writer.WriteUint16(c.packetSeq)
	packet := protocol.AppendCRC(writer.Bytes())
	log.Printf("TX: %x", packet)
	_, err := c.conn.Write(packet)
	return err
}

func (c *TestClient) SendAuthentication() error {
	return c.sendReliableMessage(protocol.OpcodeAuthenticate, []byte{})
}

func (c *TestClient) SendGameServerAuth() error {
	buf := make([]byte, MaxPacketSize)
	writer := buffer.NewWriter(buf)
	writer.WriteUint16(c.clientID)
	writer.WriteUint16(c.serverID)
	flags := uint64(protocol.SegmentFlagNewInstance) | uint64(protocol.SegmentFlagHasInstance) | uint64(protocol.SegmentFlagClientInitiated)
	bodySize := 7
	writer.Write7BitEncodedUInt64(flags | uint64(bodySize))
	writer.WriteUint32(c.instanceID)
	writer.WriteByte(byte(protocol.BodyFlagSessionAck | protocol.BodyFlagRdpMessage))
	writer.WriteUint32(c.instanceID)
	c.packetSeq++
	writer.WriteUint16(c.packetSeq)
	packet := protocol.AppendCRC(writer.Bytes())
	log.Printf("TX: %x", packet)
	_, err := c.conn.Write(packet)
	return err
}

func (c *TestClient) SendCharacterSelect(serverID uint32) error {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, serverID)
	return c.sendReliableMessage(protocol.OpcodeSelectedChar, data)
}

func (c *TestClient) SendEnterWorld() error {
	return c.sendReliableMessage(protocol.OpcodeCharacterInWorld, []byte{})
}

func (c *TestClient) sendReliableMessage(opcode protocol.GameOpcode, payload []byte) error {
	buf := make([]byte, MaxPacketSize)
	writer := buffer.NewWriter(buf)
	writer.WriteUint16(c.clientID)
	writer.WriteUint16(c.serverID)
	msgSize := 2 + len(payload)
	bodySize := 1 + 4 + 2 + 1 + 1 + 2 + 2 + len(payload)
	flags := uint64(protocol.SegmentFlagHasInstance) | uint64(protocol.SegmentFlagClientInitiated)
	writer.Write7BitEncodedUInt64(flags | uint64(bodySize))
	writer.WriteUint32(c.instanceID)
	writer.WriteByte(byte(protocol.BodyFlagSessionAck | protocol.BodyFlagRdpMessage))
	writer.WriteUint32(c.instanceID)
	c.packetSeq++
	writer.WriteUint16(c.packetSeq)
	writer.WriteByte(byte(protocol.MessageTypeReliable))
	writer.WriteByte(byte(msgSize))
	c.messageSeq++
	writer.WriteUint16(c.messageSeq)
	writer.WriteUint16(uint16(opcode))
	if len(payload) > 0 {
		writer.WriteBytes(payload)
	}
	packet := protocol.AppendCRC(writer.Bytes())
	log.Printf("TX opcode=0x%04X: %x", opcode, packet)
	_, err := c.conn.Write(packet)
	return err
}

func (c *TestClient) ReceiveAndPrint(label string) error {
	buf := make([]byte, MaxPacketSize)
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.conn.Read(buf)
	if err != nil {
		return err
	}
	log.Printf("\n=== %s (%d bytes) ===", label, n)
	log.Printf("RX: %x", buf[:n])
	c.parsePacket(buf[:n])
	c.sendAck()
	return nil
}

func (c *TestClient) parsePacket(data []byte) {
	if len(data) < 9 {
		return
	}
	if !protocol.ValidateCRC(data) {
		log.Printf("WARNING: Invalid CRC!")
	}
	reader := buffer.NewReader(data)
	serverEndpoint, _ := reader.ReadUint16()
	clientEndpoint, _ := reader.ReadUint16()
	log.Printf("Endpoints: server=0x%04X client=0x%04X", serverEndpoint, clientEndpoint)
	if serverEndpoint != 0 && serverEndpoint != 0xFFFE {
		c.serverID = serverEndpoint
	}
	flagsLen, _ := reader.Read7BitEncodedUInt64()
	flags := protocol.SegmentHeaderFlags(flagsLen & uint64(protocol.SegmentHeaderFlagsMask))
	size := int(flagsLen & protocol.SegmentHeaderSizeMask)
	log.Printf("Header: flags=0x%X size=%d", flags, size)
	if flags&protocol.SegmentFlagHasInstance != 0 {
		inst, _ := reader.ReadUint32()
		log.Printf("Instance: 0x%X", inst)
		c.instanceID = inst
		c.hasInstance = true
	}
	if size == 0 {
		return
	}
	bodyFlags, _ := reader.ReadByte()
	log.Printf("BodyFlags: 0x%02X", bodyFlags)
	if bodyFlags&byte(protocol.BodyFlagSessionAck) != 0 {
		sessAck, _ := reader.ReadUint32()
		log.Printf("SessionAck: 0x%X", sessAck)
	}
	arrival, _ := reader.ReadUint16()
	c.lastRecvPacket = arrival
	log.Printf("Arrival: %d", arrival)
	if bodyFlags&byte(protocol.BodyFlagFlushAck) != 0 {
		flushAck, _ := reader.ReadUint16()
		log.Printf("FlushAck: %d", flushAck)
	}
	if bodyFlags&byte(protocol.BodyFlagGuaranteedAck) != 0 {
		guaAck, _ := reader.ReadUint16()
		c.lastRecvMsg = guaAck
		log.Printf("GuaranteedAck: %d", guaAck)
	}
	if bodyFlags&byte(protocol.BodyFlagClientUpdateAck) != 0 {
		for {
			ch, _ := reader.ReadByte()
			if ch == 0xF8 {
				break
			}
			ack, _ := reader.ReadUint16()
			log.Printf("StateAck: ch=0x%02X ack=%d", ch, ack)
		}
	}
	// Parse messages
	for reader.Remaining() > 4 {
		msgType, err := reader.ReadByte()
		if err != nil {
			break
		}
		msgSize, err := reader.ReadByte()
		if err != nil {
			break
		}
		if msgSize&0x80 != 0 {
			reader.ReadByte()
		}
		log.Printf("Msg: type=0x%02X size=%d", msgType, msgSize&0x7F)
		switch protocol.MessageType(msgType) {
		case protocol.MessageTypeReliable, protocol.MessageTypePing:
			seq, _ := reader.ReadUint16()
			log.Printf("  Seq: %d", seq)
			if msgType != byte(protocol.MessageTypePing) && msgSize >= 2 {
				opcode, _ := reader.ReadUint16()
				log.Printf("  Opcode: 0x%04X (%s)", opcode, protocol.GameOpcode(opcode).String())
				remaining := int(msgSize) - 2
				if remaining > 0 && remaining < 500 {
					payload, _ := reader.ReadBytes(remaining)
					if len(payload) > 30 {
						log.Printf("  Data: %x...", payload[:30])
					} else {
						log.Printf("  Data: %x", payload)
					}
				}
			}
		case protocol.MessageType(0x00), protocol.MessageType(0x01), protocol.MessageType(0x02):
			seq, _ := reader.ReadUint16()
			refNum, _ := reader.ReadByte()
			log.Printf("  StateChannel %d: seq=%d ref=%d", msgType, seq, refNum)
			var rleData []byte
			for reader.Remaining() > 4 {
				b, _ := reader.ReadByte()
				rleData = append(rleData, b)
				if b == 0x00 {
					break
				}
			}
			log.Printf("  RLE: %x", rleData)
			decoded := decodeRLE(rleData)
			if len(decoded) > 0 {
				log.Printf("  Decoded (%d bytes): %x", len(decoded), decoded[:min(40, len(decoded))])
				parseEntity(decoded)
			}
		default:
			if msgSize > 0 && int(msgSize) < reader.Remaining() {
				reader.ReadBytes(int(msgSize))
			}
		}
	}
}

func decodeRLE(data []byte) []byte {
	result := make([]byte, 0, len(data)*2)
	i := 0
	for i < len(data) {
		ctrl := data[i]
		i++
		if ctrl == 0 {
			break
		}
		copyCount := int(ctrl & 0x7F)
		if i >= len(data) {
			break
		}
		zeroCount := int(data[i])
		i++
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

func parseEntity(data []byte) {
	if len(data) < 20 {
		return
	}
	log.Printf("  Entity: isActive=%d", data[0])
	if len(data) >= 5 {
		objID := binary.LittleEndian.Uint32(data[1:5])
		log.Printf("  Entity: objID=%d (0x%X)", objID, objID)
	}
	if len(data) >= 32 {
		modelID := binary.LittleEndian.Uint32(data[28:32])
		log.Printf("  Entity: modelID=0x%X", modelID)
	}
	if len(data) >= 180 {
		log.Printf("  Entity: level=%d", data[179])
	}
	if len(data) >= 185 {
		log.Printf("  Entity: race=%d class=%d", data[183], data[184])
	}
	if len(data) >= 178 {
		var name string
		for _, b := range data[155:min(178, len(data))] {
			if b == 0 {
				break
			}
			name += string(b)
		}
		if name != "" {
			log.Printf("  Entity: name='%s'", name)
		}
	}
}

func (c *TestClient) sendAck() error {
	buf := make([]byte, MaxPacketSize)
	writer := buffer.NewWriter(buf)
	writer.WriteUint16(c.clientID)
	writer.WriteUint16(c.serverID)
	bodySize := 1 + 4 + 2 + 2 + 2
	flags := uint64(protocol.SegmentFlagHasInstance) | uint64(protocol.SegmentFlagClientInitiated)
	writer.Write7BitEncodedUInt64(flags | uint64(bodySize))
	writer.WriteUint32(c.instanceID)
	writer.WriteByte(byte(protocol.BodyFlagSessionAck | protocol.BodyFlagRdpMessage | protocol.BodyFlagFlushAck | protocol.BodyFlagGuaranteedAck))
	writer.WriteUint32(c.instanceID)
	c.packetSeq++
	writer.WriteUint16(c.packetSeq)
	writer.WriteUint16(c.lastRecvPacket)
	writer.WriteUint16(c.lastRecvMsg)
	packet := protocol.AppendCRC(writer.Bytes())
	_, err := c.conn.Write(packet)
	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
