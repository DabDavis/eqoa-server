package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// TargetHandler handles targeting-related messages.
type TargetHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
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
	r.Register(protocol.OpcodeAssist, h.HandleAssistTarget)
	r.Register(protocol.OpcodeAssistTarget, h.HandleAssistLeader)
}

// HandleAssistTarget is 0x00CB, Assist Target (DoAction 0x0B0E0006: the
// quick command |22, the start menu's): a raw u32, the sender's target. The
// answer is a 0x007B with whom that target has targeted, as the sender knows
// it, or -1 for none (entity/assist.go).
func (h *TargetHandler) HandleAssistTarget(sess *session.Session, msg *protocol.Message) error {
	r := newClientReader(msg)
	id := r.u32()
	if r.err || h.gameWorld == nil {
		return nil
	}
	if to, ok := h.gameWorld.AssistTarget(sess, id); ok {
		SendAssistTarget(sess, to)
	}
	return nil
}

// HandleAssistLeader is 0x007B, Assist Leader (DoAction 0x0B090000, |21):
// the same u32, the sender's own target, which it does not need. The answer
// is a 0x007B with the group leader's target, or -1.
func (h *TargetHandler) HandleAssistLeader(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	if to, ok := h.gameWorld.AssistLeader(sess); ok {
		SendAssistTarget(sess, to)
	}
	return nil
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
	counter   uint32 // the request's token, echoed
	// a player's title (its Master Class, shown in place of its class) and
	// guild (shown under its name): World.TargetInfoLines
	title, guild string
}

// HandleTarget handles a client target request (opcode 0x0760 or 0x0010).
func (h *TargetHandler) HandleTarget(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 8 {
		return nil
	}

	reader := buffer.NewReader(data)

	// CLIENT's request is SendOpIntInt(16, token, target): the token comes
	// back last in the reply, and Msg0010 keeps the reply's colours only when
	// it matches.
	token, _ := reader.ReadUint32()
	targetObjectID, _ := reader.ReadUint32()

	log.Printf("Target from %s: targetObjectID=%d token=%d", sess.RemoteAddr(), targetObjectID, token)

	if h.gameWorld == nil {
		return nil
	}

	// CLIENT turns auto-attack off on every target change, and a 0x0010 is
	// sent on every one (-1 included): the fight on the old target ends here.
	h.gameWorld.StopAttackOnRetarget(sess, targetObjectID)

	// The target is what the client names, found or not (-1 is none, a group
	// stand-in a member out of sight): Assist Target and Assist Leader read
	// it. Letting a target go (ClearTarget) sends nothing, so it can outlive
	// the client's.
	player := h.gameWorld.SessionPlayer(sess)
	if player != nil {
		h.gameWorld.SetTarget(player.ServerID, targetObjectID)
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
		counter:  token,
	}

	switch t := targetEntity.(type) {
	case *entity.Player:
		info.name = t.Name
		info.level = t.Level
		info.race = t.Race
		info.class = t.Class
		info.faceColor = 5 // player target — ally (blue face)
		info.title, info.guild = h.gameWorld.TargetInfoLines(t)
	case *entity.NPC:
		info.name = t.Name
		info.level = t.Level
		// NPCs don't have race/class in the player sense
		info.race = 0
		info.class = 0
		// The face is the creature's to this player (entity/aggro.go): red
		// for one that attacks it on sight or fights it, as Sandstorm's; for
		// an NPC of a faction (npcfaction) the player's standing with it;
		// a pet's the players' (entity/pet.go)
		info.faceColor = h.gameWorld.TargetFace(player, t)
	default:
		info.name = "???"
		info.level = 1
		info.faceColor = 3
	}

	// Get sender's level for con color calculation
	senderLevel := 1
	if player != nil {
		senderLevel = player.Level
	}
	info.conColor = calcConColor(senderLevel, info.level)

	h.sendTargetInfo(sess, &info)

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

// sendTargetInfo answers a 0x0010 request (buildClassicTargetInfo): face,
// con, a byte retail always sends as 1, the two 64-unit wide lines (a
// player's title and guild, empty for anything else), and the request's
// token. Retail's replies stop at 261 bytes, before the token, which leaves
// a PS2 comparing whatever its stack held; with the token they are 265 and
// the colours apply every time.
func (h *TargetHandler) sendTargetInfo(sess *session.Session, info *targetInfo) {
	queueReliable(sess, protocol.OpcodeClassicTargetInfo,
		buildClassicTargetInfo(info.counter, info.faceColor, info.conColor, 0x01, info.title, info.guild))
	log.Printf("TargetPacket: face=%d con=%d targetID=%d token=%d",
		info.faceColor, info.conColor, info.objectID, info.counter)
}
