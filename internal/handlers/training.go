package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
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
// CLIENT's trainer (TrainStats, viwndtrainer.cpp) sends 7 zigzag values, the
// points added to each stat (STR STA AGI DEX WIS INT CHA), and waits (its mode
// 4) for the answer: 0x001D with the points spent, which it takes off what it
// shows (Msg001D: m_p40 -= z) before it lets the player choose again.
func (h *TrainingHandler) HandleUpdateTrainingPoints(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 7 {
		return nil
	}

	reader := buffer.NewReader(data)

	var adds [7]int64
	for i := range adds {
		adds[i], _ = reader.Read7BitEncodedInt64()
	}
	strAdd, staAdd, agiAdd, dexAdd, wisAdd, intAdd, chaAdd := adds[0], adds[1], adds[2], adds[3], adds[4], adds[5], adds[6]

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

	base := [7]int{player.BaseStr, player.BaseSta, player.BaseAgi, player.BaseDex, player.BaseWis, player.BaseInt, player.BaseCha}
	if why := trainRefusal(adds, player.UnusedTP); why != "" {
		// nothing spent: the trainer goes back to choosing with its points,
		// where without an answer it waited (mode 4) until it was closed
		log.Printf("TrainingPoints: %s refused: %s", player.Name, why)
		SendTrainingPointsSpent(sess, 0)
		return nil
	}
	// each stat's base up to its own cap, the masteries' raises in it
	// (entity/mastery.go): the client is told the highest of them, so a
	// stat of a lower one may be asked past it, and keeps those points
	caps := player.StatCaps()
	fit, held := entity.FitTraining(base, caps, adds)
	if heldBack := heldLine(held, caps); heldBack != "" {
		log.Printf("TrainingPoints: %s held back: %s", player.Name, heldBack)
		SendErrorMessage(sess, heldBack)
	}
	adds = fit
	strAdd, staAdd, agiAdd, dexAdd, wisAdd, intAdd, chaAdd = adds[0], adds[1], adds[2], adds[3], adds[4], adds[5], adds[6]
	total = int(strAdd + staAdd + agiAdd + dexAdd + wisAdd + intAdd + chaAdd)
	if total == 0 {
		SendTrainingPointsSpent(sess, 0)
		return nil
	}

	// Apply stat increases: training raises the base stats (the stat
	// channel's base half), and the stats, the maxima and the full hit
	// points and power follow from them with the worn items' (gear.go)
	player.BaseStr += int(strAdd)
	player.BaseSta += int(staAdd)
	player.BaseAgi += int(agiAdd)
	player.BaseDex += int(dexAdd)
	player.BaseWis += int(wisAdd)
	player.BaseInt += int(intAdd)
	player.BaseCha += int(chaAdd)
	player.UnusedTP -= total
	player.RecalcStats(true)

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

	// the points spent, which the trainer takes off what it shows
	SendTrainingPointsSpent(sess, int32(total))

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

// trainRefusal is why a training request is not paid at all, "" when it is,
// by CLIENT's own trainer's rules (VIWndTrainer::Adjust, VIClient::TrainStats):
// no stat lowered, something to spend, and no more than the points left. Where
// a stat's base stops is its cap, raises and all (entity.FitTraining), as
// CLIENT's trainer stops it at m_p44 (GetP44Total(-1)), which the client is
// told is the highest of the seven (entity.Player.TrainerCap). A stat whose
// shown value is held at its full cap by worn items still takes points into
// its base, as the client lets it: they count once the items stop taking it
// past the cap.
func trainRefusal(adds [7]int64, unused int) string {
	total := int64(0)
	for i, a := range adds {
		if a < 0 {
			return fmt.Sprintf("stat %d lowered by %d", i, -a)
		}
		total += a
	}
	switch {
	case total <= 0:
		return "nothing to spend"
	case total > int64(unused):
		return fmt.Sprintf("spends %d with %d left", total, unused)
	}
	return ""
}

// heldLine says which stats a training request asked past their caps
// (held: the points each was held back by, entity.FitTraining) and that
// those points stay unspent; "" when none was.
func heldLine(held [7]int64, caps [7]int) string {
	var parts []string
	n := int64(0)
	for i, v := range held {
		if v > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", trainStatNames[i], caps[i]))
			n += v
		}
	}
	if n == 0 {
		return ""
	}
	unspent := "training points stay"
	if n == 1 {
		unspent = "training point stays"
	}
	return fmt.Sprintf("A stat cannot be trained past its maximum (%s): %d %s unspent.", strings.Join(parts, ", "), n, unspent)
}

// trainStatNames are the trainer's seven stats, STR..CHA.
var trainStatNames = [7]string{"STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA"}
