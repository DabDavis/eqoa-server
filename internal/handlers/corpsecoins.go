package handlers

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A corpse's tunar, as the reference server hands it over (questrec16,
// claude-notes/quests-sandstorm-20261025.md: a level 2 Orc Pawn's corpse,
// opened by 0x0015, answered 0x0052, the purse, and "You receive 2 tunar from
// the corpse." before its items): the coins go to whoever first opens a
// creature's corpse after its death, at once, and the corpse holds only its
// items after. How much is the live game's (work board 658, a GM's decision from
// live-era footage of 2012: a level 5 highwayman's corpse held 1, level 14-17
// creatures' 2-4; "very little tunar dropped"; wildlife printed no tunar line):
// an ordinary creature's corpse holds 1 + 0..level/4, an animal's (entity's
// wildlife test, the one that decides wandering) none, and a named creature's
// the amount its npccoins row gives (database/npccoins.go), whatever it is. A
// pet's corpse, a training dummy's, a duel spawn's and a corpse of nothing carry
// none, a row or not.

// corpseCoinLine is the reference server's line for them.
const corpseCoinLine = "You receive %d tunar from the corpse."

// OrdinaryCorpseCoins is an ordinary creature's corpse tunar at that level: 1 +
// roll(level/4 + 1) (roll is rand.Intn, a test's stand-in), the level at least 1.
// Pickpocketing takes the same (entity/workbookmore.go).
func OrdinaryCorpseCoins(level int, roll func(int) int) int {
	if roll == nil {
		roll = rand.Intn
	}
	return 1 + roll(max(level, 1)/4+1)
}

// corpseCoins is the tunar a creature's corpse carries, rolled when it is
// first opened (roll is rand.Intn, a test's stand-in): named is its npccoins
// row (ok false: none), wild whether it is wildlife.
func corpseCoins(npc *entity.NPC, roll func(int) int, named coinRange, wild bool) int {
	if npc.Owner != 0 || npc.Dummy != entity.NotDummy || npc.Level <= 0 || npc.DuelSpawn() || npc.CorpseOfNothing() {
		return 0 // a Metal Moogie's gives its tribute alone (entity/metalmoogie.go)
	}
	if roll == nil {
		roll = rand.Intn
	}
	if named.ok {
		return named.lo + roll(named.hi-named.lo+1)
	}
	if wild {
		return 0
	}
	return OrdinaryCorpseCoins(npc.Level, roll)
}

// coinRange is a creature's npccoins row: coins_min..coins_max.
type coinRange struct {
	lo, hi int
	ok     bool
}

// corpseCoinsOf is npc's corpse tunar with its npccoins row and its wildlife
// verdict looked up (the row read now: a GM's edit counts for the next corpse).
func (h *NPCWindowHandler) corpseCoinsOf(npc *entity.NPC, roll func(int) int) int {
	var named coinRange
	if h.inv != nil {
		lo, hi, ok, err := h.inv.NPCCoins(npc.Name)
		if err != nil {
			log.Printf("Loot: %q's npccoins row: %v", npc.Name, err)
		}
		named = coinRange{lo, hi, ok}
	}
	wild := false
	if !named.ok && h.isWild != nil {
		wild = h.isWild(npc)
	}
	return corpseCoins(npc, roll, named, wild)
}

// corpseSplitLine is a group member's line for its share of the coins of a
// corpse another member opened (ours, after EverQuest's "as your split").
const corpseSplitLine = "You receive %d tunar as your split."

// giveCorpseCoins puts n tunar in the opener's purse, kept, the purse sent
// (0x0052) and the line said, as the reference server's; with its group about it, shared
// (petition 1228: "During live, the entire group shared a portion of it"):
// each member who would share the kill (entity.CoinSharers) an equal part,
// the odd coins the opener's, each told in its own line.
func (h *NPCWindowHandler) giveCorpseCoins(sess *session.Session, npc *entity.NPC, n int) {
	if n <= 0 || h.playerOf == nil {
		return
	}
	player := h.playerOf(sess)
	if player == nil {
		return
	}
	var sharers []*entity.Player
	var sessions []*session.Session
	if h.gameWorld != nil {
		sharers, sessions = h.gameWorld.CoinSharers(player, npc)
	}
	each := n
	if len(sharers) > 1 {
		each = n / len(sharers)
		for i, m := range sharers[1:] {
			if each > 0 {
				h.giveCoinSplit(sessions[i+1], m, npc, each)
			}
		}
		n -= each * (len(sharers) - 1)
	}
	player.Tunar += n
	economy.Made(economy.Loot, player.CharServerID, player.Name, n)
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	if !sess.LootMessagesOff() { // the client's Loot Messages option (faction.go)
		sess.Queue().AddWithSequence(buildClientMessage(fmt.Sprintf(corpseCoinLine, n)))
	}
	log.Printf("Loot: %s took %d tunar from %q's corpse (%d now)", player.Name, n, npc.Name, player.Tunar)
}

// giveCoinSplit is a member's share of a corpse's coins another opened: its
// purse paid under the World's lock (it is another session's player), kept,
// sent and said.
func (h *NPCWindowHandler) giveCoinSplit(ms *session.Session, m *entity.Player, npc *entity.NPC, n int) {
	purse, ok := h.gameWorld.TakeTunar(m.CharServerID, -n)
	if !ok {
		return
	}
	economy.Made(economy.LootSplit, m.CharServerID, m.Name, n)
	if h.inv != nil {
		if err := h.inv.SetPurse(m.CharServerID, purse); err != nil {
			log.Printf("tunar: saving %s's purse %d: %v", m.Name, purse, err)
		}
	}
	if ms != nil {
		queueReliable(ms, protocol.OpcodePlayerTunar, tunarPayload(purse))
		if !ms.LootMessagesOff() {
			ms.Queue().AddWithSequence(buildClientMessage(fmt.Sprintf(corpseSplitLine, n)))
		}
	}
	log.Printf("Loot: %s took a split of %d tunar from %q's corpse (%d now)", m.Name, n, npc.Name, purse)
}
