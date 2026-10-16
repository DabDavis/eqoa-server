package handlers

import (
	"log"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// GuildHandler handles guild opcodes.
// Wire format from PS2 CLIENT decompilation (claude-notes/guild-system-decompiled.md).
type GuildHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
}

func NewGuildHandler(sessionMgr *session.Manager) *GuildHandler {
	return &GuildHandler{sessionManager: sessionMgr}
}

func (h *GuildHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

func (h *GuildHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeGuildCreate, h.HandleGuildCreate)
	r.Register(protocol.OpcodeGuildLeave, h.HandleGuildLeave)
	r.Register(protocol.OpcodeGuildSetMOTD, h.HandleGuildSetMOTD)
	log.Printf("Guild: registered handlers for opcodes 0x%04X, 0x%04X, 0x%04X",
		uint16(protocol.OpcodeGuildCreate), uint16(protocol.OpcodeGuildLeave), uint16(protocol.OpcodeGuildSetMOTD))
}

// HandleGuildCreate creates a new guild.
func (h *GuildHandler) HandleGuildCreate(sess *session.Session, msg *protocol.Message) error {
	log.Printf("Guild: HandleGuildCreate called from %s", sess.RemoteAddr())
	if h.gameWorld == nil {
		return nil
	}
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}
	reader := buffer.NewReader(data)
	nameLen, _ := reader.ReadInt32()
	if nameLen <= 0 || nameLen > 24 {
		return nil
	}
	guildName, err := reader.ReadStringUTF8(int(nameLen))
	if err != nil {
		return nil
	}

	player := h.gameWorld.SessionPlayer(sess)
	if player == nil || player.GuildID != 0 {
		return nil
	}

	guildID := h.gameWorld.CreateGuild(guildName, player)
	if guildID == 0 {
		return nil
	}
	log.Printf("Guild: %s created '%s' (ID=%d)", player.Name, guildName, guildID)
	h.SendGuildInfo(sess, player)
	return nil
}

// HandleGuildLeave removes a player from their guild.
func (h *GuildHandler) HandleGuildLeave(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil || player.GuildID == 0 {
		return nil
	}
	log.Printf("Guild: %s left '%s'", player.Name, player.GuildName)
	h.gameWorld.RemoveFromGuild(player)
	return nil
}

// HandleGuildSetMOTD sets the guild MOTD.
func (h *GuildHandler) HandleGuildSetMOTD(sess *session.Session, msg *protocol.Message) error {
	if h.gameWorld == nil {
		return nil
	}
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}
	reader := buffer.NewReader(data)
	charCount, _ := reader.ReadInt32()
	if charCount <= 0 || charCount > 256 {
		return nil
	}
	motd, _ := reader.ReadStringUnicode(int(charCount))

	player := h.gameWorld.SessionPlayer(sess)
	if player == nil || player.GuildID == 0 {
		return nil
	}

	h.gameWorld.SetGuildMOTD(player.GuildID, motd)
	log.Printf("Guild: %s set MOTD: %s", player.Name, motd)
	h.BroadcastGuildInfo(player.GuildID)
	return nil
}

// SendGuildInfo sends GuildInfo (0x0468) with the shared guild tail wire format.
// PS2 CLIENT reads this at shared guild tail (0x00BD27A4).
// Wire format: guild identity + hall + economy + bank + crest + MOTD + name + power + damage + quest + final.
func (h *GuildHandler) SendGuildInfo(sess *session.Session, player *entity.Player) {
	msg := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeGuildInfo)
	dataBuf := make([]byte, 1024)
	writer := buffer.NewWriter(dataBuf)
	writer.WriteUint16(uint16(protocol.OpcodeGuildInfo))

	writeGuildTail(writer, player)

	data := dataBuf[:writer.Position()]
	msg.SetData(data)
	sess.Queue().AddWithSequence(msg)
}

// BroadcastGuildInfo sends guild info to all online guild members.
func (h *GuildHandler) BroadcastGuildInfo(guildID uint32) {
	if h.gameWorld == nil {
		return
	}
	members := h.gameWorld.GuildMembers(guildID)
	for _, p := range members {
		sess := h.gameWorld.FindSessionByEntityID(p.ServerID)
		if sess != nil {
			h.SendGuildInfo(sess, p)
		}
	}
}

// writeGuildTail writes the shared guild tail format (0x00BD27A4).
// PS2 CLIENT reads ~25 fields in this exact order.
func writeGuildTail(w *buffer.Writer, p *entity.Player) {
	// Guild identity: raw(4,1) × 3
	w.WriteUint32(p.GuildID) // guildID
	w.WriteUint32(0)         // guildFlags
	w.WriteUint32(0)         // guildTimestamp

	// Guild hall: raw(4,1) × 2
	w.WriteInt32(int32(p.GuildHallZone)) // guildHallZoneID (-1 = none)
	w.WriteInt32(int32(p.GuildHallInst)) // guildHallInstanceID

	// Economy: raw(1,1) + raw(4,1)
	w.WriteByte(p.GuildTaxRate)       // taxRate
	w.WriteInt32(int32(p.GuildLevel)) // level

	// Bank: raw(4,1) + zigzag
	w.WriteInt32(int32(p.GuildBankGold)) // bankGold
	w.Write7BitEncodedInt64(0)           // taxCollected (zigzag)

	// XP: raw(4,1)
	w.WriteInt32(int32(p.GuildXP))

	// Crest + MOTD: raw(4,1) + blob(256)
	w.WriteUint32(p.GuildCrestID)
	// MOTD as blob: int32(charCount) + UTF-16LE
	motd := p.GuildMOTD
	w.WriteInt32(int32(len(motd)))
	for _, c := range motd {
		w.WriteUint16(uint16(c))
	}

	// Crest #2 + alliance: raw(4,1) + zigzag
	w.WriteUint32(p.GuildCrestID) // crestID2 (same)
	w.Write7BitEncodedInt64(0)    // allianceID

	// Player speed: raw(4,1) as float32
	w.WriteFloat32(p.Speed)

	// Player title: string(24) + raw(1,1)
	w.WriteInt32(0) // titleLen = 0 (no title)
	w.WriteByte(0)  // titleFlags

	// Guild name: string(24)
	w.WriteInt32(int32(len(p.GuildName)))
	w.WriteStringUTF8Raw(p.GuildName)

	// Power: raw(4,1)
	w.WriteInt32(int32(p.Power))

	// Damage indication: raw(4,1) + zigzag + raw(4,1)
	w.WriteUint32(0)           // dmgSourceEntityID (none)
	w.Write7BitEncodedInt64(0) // dmgAmount (none)
	w.WriteUint32(0)           // dmgType (none)

	// Target + action state: raw(4,1) × 2
	w.WriteUint32(0xFFFFFFFF) // targetEntityID (none)
	w.WriteInt32(0)           // actionState
}
