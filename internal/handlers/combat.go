package handlers

import (
	"log"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// CombatHandler handles combat-related messages.
type CombatHandler struct {
	sessionManager *session.Manager
	gameWorld      *entity.World
	inventoryStore *database.InventoryStore
}

// NewCombatHandler creates a new combat handler.
func NewCombatHandler(sessionMgr *session.Manager) *CombatHandler {
	return &CombatHandler{sessionManager: sessionMgr}
}

// SetGameWorld sets the game world reference (called after world init).
func (h *CombatHandler) SetGameWorld(gw *entity.World) {
	h.gameWorld = gw
}

// SetInventoryStore gives the handler the inventory, which decides the swing.
func (h *CombatHandler) SetInventoryStore(inv *database.InventoryStore) {
	h.inventoryStore = inv
}

// RegisterHandlers registers combat handlers with the registry.
func (h *CombatHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeAttack, h.HandleAttack)
	r.Register(protocol.OpcodeSwingOutOfReach, h.HandleSwingOutOfReach)
	for op, action := range emoteActions {
		action := action
		r.Register(op, func(sess *session.Session, msg *protocol.Message) error {
			return h.HandleEmote(sess, action)
		})
	}
}

// emoteActions is CLIENT's PlayEmote (0x00BCFC38 [V]): the bare opcode it
// sends for an emote and the action it plays.
var emoteActions = map[protocol.GameOpcode]byte{
	protocol.OpcodeEmoteWave:  entity.ActionWave,
	protocol.OpcodeEmoteBow:   entity.ActionBow,
	protocol.OpcodeEmotePoint: entity.ActionPoint,
	protocol.OpcodeEmoteCheer: entity.ActionCheer,
}

// HandleEmote shows the sender's emote to the players who can see it
// (entity.ShowEmote, 0x0775).
func (h *CombatHandler) HandleEmote(sess *session.Session, action byte) error {
	if h.gameWorld == nil {
		return nil
	}
	n := h.gameWorld.ShowEmote(sess, action)
	log.Printf("Emote from %s: action %d shown to %d", sess.RemoteAddr(), action, n)
	return nil
}

// HandleAttack is a 0x000F, u32 target: the attack button's press
// (VIClient::ToggleAutoAttack, 0x00BD59E8 [V]), sent once and only when the
// client's own swing is due. It engages the target; the hits come from the
// tick (entity/attack.go), one swing delay apart, for as long as the button
// is up (0x40 byte 36) or the NPC fights back.
func (h *CombatHandler) HandleAttack(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 {
		return nil
	}

	reader := buffer.NewReader(data)
	targetObjectID, _ := reader.ReadUint32()

	log.Printf("Attack from %s: targetObjectID=%d", sess.RemoteAddr(), targetObjectID)

	if h.gameWorld == nil {
		return nil
	}
	player := h.gameWorld.SessionPlayer(sess)
	if player == nil {
		return nil
	}
	h.gameWorld.EngageAttack(sess, targetObjectID, h.swingFor(sess, player), time.Now())
	return nil
}

// HandleSwingOutOfReach is a bare 0x00D8: CLIENT's AutoAttackTick
// (0x00BC76A0 [V]) swung at a target out of its reach and will try again in
// 2 s. The server's own reach test (entity/attack.go) already holds the
// swing; there is nothing to answer, and an unhandled opcode would be logged
// every 2 s for as long as the button is held.
func (h *CombatHandler) HandleSwingOutOfReach(sess *session.Session, msg *protocol.Message) error {
	return nil
}

// swingFor is the swing CLIENT's AutoAttackTick would make with this
// player's gear: which of equip slots 14-20 are worn, and the first worn
// item in 14, 16 or 18 in the client's list order, whose attack type and
// range (the item record's field after the weapon damage) set the reach.
func (h *CombatHandler) swingFor(sess *session.Session, p *entity.Player) entity.Swing {
	var worn [21]bool
	hasItem := false
	var attackType, itemRange int
	if h.inventoryStore != nil {
		rows, err := h.inventoryStore.GetCharacterItems(p.CharServerID)
		if err != nil {
			log.Printf("Attack: inventory for %s: %v", p.Name, err)
		}
		for _, r := range inventoryListOrder(sessionInvList(sess), rows) {
			e := r.EquipLoc
			if e < 14 || e > 20 {
				continue
			}
			worn[e] = true
			if !hasItem && (e == 14 || e == 16 || e == 18) {
				hasItem = true
				attackType, itemRange = r.Pattern.AttackType, r.Pattern.Unk5
			}
		}
	}
	return entity.SwingFor(p.Class, p.Level, worn, hasItem, attackType, itemRange)
}

// inventoryListOrder is the carried items (not the bank) in the order the
// client holds them: the session's list where it has one, else as stored.
func inventoryListOrder(l *invList, rows []database.CharInventoryEntry) []database.CharInventoryEntry {
	var carried []database.CharInventoryEntry
	for _, r := range rows {
		if r.Location == 0 {
			carried = append(carried, r)
		}
	}
	if l == nil {
		return carried
	}
	byID := make(map[int]database.CharInventoryEntry, len(carried))
	for _, r := range carried {
		byID[r.ItemID] = r
	}
	out := make([]database.CharInventoryEntry, 0, len(carried))
	for i := 0; ; i++ {
		rec, ok := l.at(i)
		if !ok {
			break
		}
		if r, ok := byID[rec.itemID]; ok {
			out = append(out, r)
			delete(byID, rec.itemID)
		}
	}
	for _, r := range carried { // anything the list does not name, last
		if _, ok := byID[r.ItemID]; ok {
			out = append(out, r)
		}
	}
	return out
}
