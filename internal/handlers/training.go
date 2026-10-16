package handlers

import (
	"log"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/pkg/pkg/buffer"
)

// TrainingHandler handles training point spend/remove requests.
type TrainingHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	charStore      *database.SQLiteCharacterStore
}

// NewTrainingHandler creates a new training handler.
func NewTrainingHandler(sessionMgr *session.Manager, charStore *database.SQLiteCharacterStore) *TrainingHandler {
	return &TrainingHandler{sessionManager: sessionMgr, charStore: charStore}
}

// SetGameWorld sets the game world reference.
func (h *TrainingHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// RegisterHandlers registers training handlers with the registry.
func (h *TrainingHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeUpdateTrainingPoints, h.HandleUpdateTrainingPoints)
}

// HandleUpdateTrainingPoints handles spending training points (0x001D).
// Client sends 7 x 7BitEncodedInt64 values (one per stat to add).
// Server responds with 0x001D + total spent (positive = remove from client's remaining).
func (h *TrainingHandler) HandleUpdateTrainingPoints(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 7 {
		return nil
	}

	reader := buffer.NewReader(data)

	strAdd, _ := reader.Read7BitEncodedInt64()
	staAdd, _ := reader.Read7BitEncodedInt64()
	agiAdd, _ := reader.Read7BitEncodedInt64()
	dexAdd, _ := reader.Read7BitEncodedInt64()
	wisAdd, _ := reader.Read7BitEncodedInt64()
	intAdd, _ := reader.Read7BitEncodedInt64()
	chaAdd, _ := reader.Read7BitEncodedInt64()

	total := int(strAdd + staAdd + agiAdd + dexAdd + wisAdd + intAdd + chaAdd)

	log.Printf("TrainingPoints from %s: STR+%d STA+%d AGI+%d DEX+%d WIS+%d INT+%d CHA+%d (total=%d)",
		sess.RemoteAddr(), strAdd, staAdd, agiAdd, dexAdd, wisAdd, intAdd, chaAdd, total)

	if h.gameWorld == nil {
		return nil
	}

	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}

	// Validate: total must be positive and not exceed remaining TP
	if total <= 0 || total > player.UnusedTP {
		log.Printf("TrainingPoints: %s tried to spend %d but only has %d remaining",
			player.Name, total, player.UnusedTP)
		return nil
	}

	// Apply stat increases
	player.Str += int(strAdd)
	player.Sta += int(staAdd)
	player.Agi += int(agiAdd)
	player.Dex += int(dexAdd)
	player.Wis += int(wisAdd)
	player.Int += int(intAdd)
	player.Cha += int(chaAdd)
	player.UnusedTP -= total

	// Recalculate HP and Power from new stats
	player.MaxHP = entity.CalculateMaxHP(player.Level, player.Class, player.Sta)
	player.MaxPower = entity.CalculateMaxPower(player.Level, player.Class,
		player.Str, player.Sta, player.Agi, player.Dex, player.Wis, player.Int, player.Cha)
	player.HP = player.MaxHP
	player.Power = player.MaxPower

	log.Printf("TrainingPoints: %s spent %d TP (remaining=%d) stats=[%d,%d,%d,%d,%d,%d,%d] HP=%d Power=%d",
		player.Name, total, player.UnusedTP,
		player.Str, player.Sta, player.Agi, player.Dex, player.Wis, player.Int, player.Cha,
		player.MaxHP, player.MaxPower)

	// Persist to database
	if h.charStore != nil {
		record, err := h.charStore.GetByServerID(player.CharServerID)
		if err == nil && record != nil {
			record.TPStrength += int(strAdd)
			record.TPStamina += int(staAdd)
			record.TPAgility += int(agiAdd)
			record.TPDexterity += int(dexAdd)
			record.TPWisdom += int(wisAdd)
			record.TPIntel += int(intAdd)
			record.TPCharisma += int(chaAdd)
			record.UnusedTP -= total
			record.CurrentHP = player.MaxHP
			record.CurrentPower = player.MaxPower
			if err := h.charStore.Update(record); err != nil {
				log.Printf("TrainingPoints: failed to persist for %s: %v", player.Name, err)
			}
		}
	}

	// Send response: opcode + total spent
	resp := protocol.NewMessage(protocol.MessageTypeReliable, protocol.OpcodeUpdateTrainingPoints)
	writer := buffer.NewWriter(resp.Span())
	writer.WriteUint16(uint16(protocol.OpcodeUpdateTrainingPoints))
	writer.Write7BitEncodedInt64(int64(total))
	resp.SetSize(writer.Position())
	sess.Queue().AddWithSequence(resp)

	// Immediately push updated stat channel so the UI refreshes
	conn := sess.Connection()
	if conn != nil {
		statChannel := conn.GetClientStatUpdate()
		if statChannel != nil {
			statData := entity.SerializeStatUpdate(player)
			statChannel.SetData(statData)
		}
	}

	return nil
}
