package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The stables' ledgers (the user, 2026-10-30: "our coach ledger lets any
// player coach anywhere, lets make it unlocked by talking to the coachman and
// also only coach to connecting"; entity/coachnetwork.go has the live game's
// rule and its words). Talking to a town's coachman signs his ledger, kept
// in charcoaches, before anything else he says: every class's level 2 quest
// step at a coachman is that signing ("Your name is in my ledger."). His
// menu then offers only the towns his coach runs to whose ledgers the
// character has signed; with none, he says so (coachNone) where the menu
// would be. A GM (acctlevel 1 or more, read at each talk as the # commands
// read it) is offered every coach town of the world, as everyone was before,
// and so is anyone at a coachman whose town is not on the network (the
// Rathe's Coachman Dilga, the only coach of his world). Without a ledger
// store (no database) the menus are as before.

// coachLedger keeps the ledgers (database.SQLiteCharacterStore).
type coachLedger interface {
	CoachTowns(serverID uint32) (map[string]bool, error)
	SignCoachLedger(serverID uint32, town string, at time.Time) (bool, error)
}

// ledgerSigned is what a coachman says as he signs a new name into his ledger
// (Sandstorm's Coachman Ronks: "All you need to do is talk to the coachman of
// the stable and he will add you to his ledger. I'll add you to mine now.").
const ledgerSigned = "I'll add you to my ledger now."

// coachNone is a coachman's answer when his coach runs to no town whose
// ledger the player has signed: the towns it runs to (the stops of this
// world), and Ronks's words for why ("Without that record, they cannot offer
// you passage.").
func coachNone(towns []string) string {
	why := " Without that record, I cannot offer you passage."
	if len(towns) == 0 {
		return "You are in the ledger of no stable my horses run to." + why
	}
	return "My horses run to " + joinAnd(towns) + ", but you are in none of their stables' ledgers." + why
}

// joinAnd lists names as a sentence does: "A", "A and B", "A, B and C".
func joinAnd(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// signLedger signs the coachman's ledger for the player, if it is a town's
// and the player had not, and says so in chat as a spiritmaster's bind is
// said (answerBind).
func (h *InteractHandler) signLedger(sess *session.Session, player *entity.Player, npc *entity.NPC) {
	if h.ledger == nil || npc.Dead || player.CharServerID == 0 {
		return
	}
	town, named := entity.CoachTown(npc)
	if !named {
		return
	}
	signed, err := h.ledger.SignCoachLedger(player.CharServerID, town, time.Now())
	if err != nil {
		log.Printf("Coach: signing %s into %s's ledger (%s): %v", player.Name, npc.Name, town, err)
		return
	}
	if signed {
		sess.Queue().AddWithSequence(buildClientMessage(npc.Name + ": " + ledgerSigned))
		log.Printf("Coach: %s signed %s's ledger (%s)", player.Name, npc.Name, town)
	}
}

// coachDests is what the coachman's menu offers the player (dests), or, when
// it offers nothing, what he says instead (none; "" for a coachman who has
// nothing to say, as before: no other coach in his world).
func (h *InteractHandler) coachDests(sess *session.Session, player *entity.Player, npc *entity.NPC) (dests []entity.CoachDest, none string) {
	all := h.gameWorld.GetCoachDestinations(npc.Name, npc.World)
	town, named := entity.CoachTown(npc)
	runs, routed := entity.CoachMenu(town, named, all)
	if !routed || h.ledger == nil {
		return all, ""
	}
	if h.isGM != nil && h.isGM(sess) {
		log.Printf("Coach: %s offers GM %s every coach town (%d)", npc.Name, player.Name, len(all))
		return all, ""
	}
	signed, err := h.ledger.CoachTowns(player.CharServerID)
	if err != nil {
		log.Printf("Coach: reading %s's ledgers: %v", player.Name, err)
	}
	var towns []string
	seen := map[string]bool{}
	for _, d := range runs {
		if !seen[d.Name] {
			seen[d.Name] = true
			towns = append(towns, d.Name)
		}
		if signed[d.Name] {
			dests = append(dests, d)
		}
	}
	log.Printf("Coach: %s (%s) offers %s %d of the %d towns his coach runs to %v", npc.Name, town, player.Name,
		len(dests), len(towns), towns)
	if len(dests) == 0 {
		return nil, npc.Name + ": " + coachNone(towns)
	}
	return dests, ""
}
