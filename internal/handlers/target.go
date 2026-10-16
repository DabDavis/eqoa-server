package handlers

import (
	"log"
	"sync"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
)

// TargetHandler handles targeting-related messages.
type TargetHandler struct {
	sessionManager  *session.Manager
	gameWorld       *entity.World
	targetCounters  sync.Map // session addr → *uint32 (server-side target counter per session)
}

// NewTargetHandler creates a new target handler.
func NewTargetHandler(sessionMgr *session.Manager) *TargetHandler {
	return &TargetHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference (called after world init).
func (h *TargetHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers target handlers with the registry.
func (h *TargetHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeTarget, h.HandleTarget)
	r.Register(protocol.OpcodeClassicTargetInfo, h.HandleTarget)
}

// targetInfo holds the data needed for a target info response.
type targetInfo struct {
	name      string
	level     int
	race      int
	class     int
	faceColor byte
	conColor  byte
	objectID  uint32
	counter   uint32
}

// HandleTarget handles a client target request (opcode 0x0760 or 0x0010).
func (h *TargetHandler) HandleTarget(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 8 {
		return nil
	}

	reader := buffer.NewReader(data)

	_, _ = reader.ReadUint32() // client counter (discarded, C# also discards this)
	targetObjectID, _ := reader.ReadUint32()

	// Server-side target counter per session (C# uses _targetCounter++ starting at 1)
	var counterPtr *uint32
	if val, ok := h.targetCounters.Load(sess.RemoteAddr()); ok {
		counterPtr = val.(*uint32)
	} else {
		initial := uint32(0)
		counterPtr = &initial
		h.targetCounters.Store(sess.RemoteAddr(), counterPtr)
	}
	*counterPtr++
	serverCounter := *counterPtr

	log.Printf("Target from %s: targetObjectID=%d serverCounter=%d", sess.RemoteAddr(), targetObjectID, serverCounter)

	if h.gameWorld == nil {
		return nil
	}

	// Look up target entity by runtime objectID.
	targetEntity, targetFound := h.gameWorld.FindEntityByObjectID(sess, targetObjectID)

	if !targetFound {
		log.Printf("Target: entity with objectID %d not found", targetObjectID)
		return nil
	}

	// Get target info via type switch
	info := targetInfo{
		objectID: targetObjectID,
		counter:  serverCounter,
	}

	switch t := targetEntity.(type) {
	case *entity.Player:
		info.name = t.Name
		info.level = t.Level
		info.race = t.Race
		info.class = t.Class
		info.faceColor = 5 // player target — ally (blue face)
	case *entity.NPC:
		info.name = t.Name
		info.level = t.Level
		// NPCs don't have race/class in the player sense
		info.race = 0
		info.class = 0
		// Face color from faction
		switch t.Faction {
		case entity.FactionFriendly:
			info.faceColor = 4 // blue face — amiable
		case entity.FactionNeutral:
			info.faceColor = 2 // neutral face — indifferent
		case entity.FactionHostile:
			info.faceColor = 0 // red face — ready to attack
		default:
			info.faceColor = 3 // default neutral
		}
	default:
		info.name = "???"
		info.level = 1
		info.faceColor = 3
	}

	// Get sender's level for con color calculation
	senderLevel := 1
	player := h.gameWorld.SessionPlayer(sess)
	if player != nil {
		senderLevel = player.Level
	}
	info.conColor = calcConColor(senderLevel, info.level)

	h.sendTargetInfo(sess, &info)

	// Update player's target directly
	if player != nil {
		h.gameWorld.SetTarget(player.ServerID, targetObjectID)
	}

	log.Printf("Target: sent info for '%s' level=%d race=%d class=%d conColor=%d objectID=%d",
		info.name, info.level, info.race, info.class, info.conColor, info.objectID)

	return nil
}

// calcConColor calculates the "consider" color based on level difference.
// Matches C# Entity.Interaction.cs GenerateConColor().
// 0=red, 1=yellow, 2=white, 3=dark blue, 4=light blue, 5=green
func calcConColor(observerLevel, targetLevel int) byte {
	diff := observerLevel - targetLevel // positive = observer is higher
	switch {
	case diff <= -3:
		return 0 // red — target much higher
	case diff <= -1:
		return 1 // yellow — target slightly higher
	case diff == 0:
		return 2 // white — same level
	case diff <= 2:
		return 3 // dark blue — target slightly lower
	default:
		if observerLevel <= 15 {
			if diff > 3 {
				return 5 // green
			}
			return 4 // light blue
		}
		if diff > observerLevel/4 {
			return 5 // green
		}
		return 4 // light blue
	}
}

// sendTargetInfo sends the target info response (261 bytes).
// Uses opcode 0x0010 (ClassicTargetInfo) matching eqoa.live pcap.
// Wire format: opcode(2) + face(1) + con(1) + byte(1, always 1) + zeros(256) = 261 bytes.
func (h *TargetHandler) sendTargetInfo(sess *session.Session, info *targetInfo) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeClassicTargetInfo)
	span := msg.Span()

	// Zero out 261 bytes
	for i := 0; i < 261 && i < len(span); i++ {
		span[i] = 0
	}

	writer := buffer.NewWriter(span)

	// Write opcode 0x0010
	writer.WriteUint16(uint16(protocol.OpcodeClassicTargetInfo))

	// Byte 2: face color (0=hostile red, 2=neutral, 4=friendly blue)
	writer.WriteByte(info.faceColor)
	// Byte 3: con color (0=green, 1=yellow, 2=white)
	writer.WriteByte(info.conColor)
	// Byte 4: always 1 (observed in eqoa.live pcap)
	writer.WriteByte(0x01)

	msg.SetSize(261)

	log.Printf("TargetPacket: face=%d con=%d targetID=%d counter=%d",
		info.faceColor, info.conColor, info.objectID, info.counter)

	sess.Queue().AddWithSequence(msg)
}
