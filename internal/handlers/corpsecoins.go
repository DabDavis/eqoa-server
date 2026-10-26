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
	if npc.Owner != 0 || npc.Dummy != entity.NotDummy || npc.Level <= 0 {
		return 0
	}
	if roll == nil {
		roll = rand.Intn
	}
	return npc.Level + roll(npc.Level+1)
}

// giveCorpseCoins puts n tunar in the opener's purse: kept, the purse sent
// (0x0052) and the line said, as Sandstorm's.
func (h *NPCWindowHandler) giveCorpseCoins(sess *session.Session, npc *entity.NPC, n int) {
	if n <= 0 || h.playerOf == nil {
		return
	}
	player := h.playerOf(sess)
	if player == nil {
		return
	}
	player.Tunar += n
	h.saveTunar(player)
	queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(player.Tunar))
	sess.Queue().AddWithSequence(buildClientMessage(fmt.Sprintf(corpseCoinLine, n)))
	log.Printf("Loot: %s took %d tunar from %q's corpse (%d now)", player.Name, n, npc.Name, player.Tunar)
}
