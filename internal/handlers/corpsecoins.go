package handlers

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A corpse's tunar, as Sandstorm hands it over (questrec16,
// claude-notes/quests-sandstorm-20261025.md: a level 2 Orc Pawn's corpse,
// opened by 0x0015, answered 0x0052, the purse, and "You receive 2 tunar from
// the corpse." before its items): the coins go to whoever first opens a
// creature's corpse after its death, at once, and the corpse holds only its
// items after. How much is ours, as that one corpse is all that was recorded:
// a roll from the creature's level to twice it, the pawn's 2 at level 2 its
// floor (the user: "add the corpse tunar drop like sandstorm too"). A pet's
// corpse and a training dummy's carry none.

// corpseCoinLine is Sandstorm's line for them.
const corpseCoinLine = "You receive %d tunar from the corpse."

// corpseCoins is the tunar a creature's corpse carries, rolled when it is
// first opened (roll is rand.Intn, a test's stand-in).
func corpseCoins(npc *entity.NPC, roll func(int) int) int {
	if npc.Owner != 0 || npc.Dummy != entity.NotDummy || npc.Level <= 0 || npc.DuelSpawn() {
		return 0 // a Metal Moogie's gives its tribute alone (entity/metalmoogie.go)
	}
	if roll == nil {
		roll = rand.Intn
	}
	return npc.Level + roll(npc.Level+1)
}

// corpseSplitLine is a group member's line for its share of the coins of a
// corpse another member opened (ours, after EverQuest's "as your split").
const corpseSplitLine = "You receive %d tunar as your split."

// giveCorpseCoins puts n tunar in the opener's purse, kept, the purse sent
// (0x0052) and the line said, as Sandstorm's; with its group about it, shared
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
