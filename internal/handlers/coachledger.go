package handlers

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
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
// read it) is offered every coach town of the world and the ones his coach runs
// to in another (Oggok's Dark Solace), as everyone was before, and anyone at a
// coachman whose town is not on the network every coach town of the world.
// Without a ledger store (no database) the menus are as before.
//
// Every ledger at once (the user, 2026-11-06: "lets add option to Coachmen to
// grant all coaches if they want, but may break immersion"): a coachman on
// the network offers anyone who is not a GM and has not signed every coach
// town's ledger (entity.CoachTownNames) one more entry, last in his menu, or
// alone after his "My horses run to ..." when no town his coach runs to is
// signed. Picked, he says what it does and asks Yes or No, the warning in the
// question (the stables had a traveler sign each ledger at its own stable);
// Yes signs every ledger not yet signed, source coachman-all, and opens his
// menu again, No changes nothing. Only the signing is granted: a coach still
// runs only to its neighbours. Its question goes under box mark 0x57000000,
// beside the dyers' 0x56 (dyer.go names the others).

// coachLedger keeps the ledgers (database.SQLiteCharacterStore).
type coachLedger interface {
	CoachTowns(serverID uint32) (map[string]bool, error)
	SignCoachLedger(serverID uint32, town string, at time.Time) (bool, error)
	SignCoachLedgers(serverID uint32, towns []string, source string, at time.Time) (int, error)
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
// nothing to say, as before: no other coach in his world); every is whether
// he offers to sign every ledger at once (the player has not, and is no GM).
// A stable in a world the client does not hold is offered to no one (a race's
// home, entity/coachhomes.go: holdsWorld), and a coachman of a server's world
// that is no home's runs no coach.
func (h *InteractHandler) coachDests(sess *session.Session, player *entity.Player, npc *entity.NPC) (dests []entity.CoachDest, none string, every bool) {
	town, named := entity.CoachTown(npc)
	if entity.Composed(npc.World) && !named {
		return nil, "", false
	}
	held := func(in []entity.CoachDest) []entity.CoachDest {
		out := in[:0:0]
		for _, d := range in {
			if holdsWorld(sess, d.World) {
				out = append(out, d)
			}
		}
		return out
	}
	all := held(h.gameWorld.GetCoachDestinations(npc.Name, npc.World))
	// a network's neighbours may stand in another world (Oggok's Dark Solace, a race's home)
	runs, routed := entity.CoachMenu(town, named, held(h.gameWorld.GetCoachDestinationsEveryWorld(npc.Name)))
	if !routed {
		return all, "", false
	}
	for _, d := range runs {
		if d.World != npc.World {
			all = append(all, d)
		}
	}
	if h.ledger == nil {
		return all, "", false
	}
	if h.isGM != nil && h.isGM(sess) {
		log.Printf("Coach: %s offers GM %s every coach town (%d)", npc.Name, player.Name, len(all))
		return all, "", false
	}
	signed, err := h.ledger.CoachTowns(player.CharServerID)
	if err != nil {
		log.Printf("Coach: reading %s's ledgers: %v", player.Name, err)
	} else if player.CharServerID != 0 {
		every = len(unsignedLedgers(signed)) > 0
	}
	// the towns his coach runs to, for what he says when none is signed: the
	// races' homes together by their world's name ("the Homelands"), not one
	// by one (twenty-one of them)
	var towns []string
	seen := map[string]bool{}
	for _, d := range runs {
		said := d.Name
		if w, home := entity.CoachHomeWorld(d.Name); home && w != "" {
			said = w
		}
		if !seen[said] {
			seen[said] = true
			towns = append(towns, said)
		}
		if signed[d.Name] {
			dests = append(dests, d)
		}
	}
	log.Printf("Coach: %s (%s) offers %s %d of the %d towns his coach runs to %v (every ledger at once: %v)", npc.Name,
		town, player.Name, len(dests), len(towns), towns, every)
	if len(dests) == 0 {
		return nil, npc.Name + ": " + coachNone(towns), every
	}
	return dests, "", every
}

// coachMenu opens a coachman's menu (coachDests): the towns he offers, then
// his offer to sign every ledger at once when he makes it; with no town, why
// not first. It reports whether he said anything: a coachman with no other
// coach in his world has nothing to say here.
func (h *InteractHandler) coachMenu(sess *session.Session, player *entity.Player, npc *entity.NPC) bool {
	dests, none, every := h.coachDests(sess, player, npc)
	if len(dests) == 0 && none != "" {
		forgetDialogs(player)
		for _, part := range splitSay(none, questBoxChars) {
			h.sendDialogueBox(sess, part)
		}
	}
	if every {
		dests = append(dests, entity.CoachDest{Name: everyLedgerOption, Ledgers: true})
		h.every.offer(player.CharServerID, npc)
	}
	if len(dests) == 0 {
		return none != ""
	}
	forgetDialogs(player)
	player.PendingCoachDests = dests
	player.CoachPage = 0
	h.sendCoachPage(sess, player)
	return true
}

// holdsWorld is whether sess's client can be sent to a stable of world: any
// world it can load (LoadableWorlds), and one of the server's own only when it
// said it built it (a July S2's table has every slot, and nothing in them).
func holdsWorld(sess *session.Session, world uint8) bool {
	if _, own := ServerWorld(world); own || entity.Composed(world) {
		return sess != nil && holds(sess.ServerWorlds(), world)
	}
	return LoadableWorlds(sess)(world)
}

// The words of the offer to sign every ledger at once. everyLedgerOption is
// its entry in the menu, everyLedgerAlone the menu's question when it is the
// only entry; everyLedgerTell what the coachman says it does when it is
// picked; everyLedgerAsk the question (at most questAskChars), with the
// warning, and its answers; everyLedgerDone his word once it is done.
const (
	everyLedgerOption = "Sign every stable's ledger"
	everyLedgerAlone  = "Shall I sign you into every stable's ledger instead?"
	everyLedgerTell   = "I can write your name in every stable's ledger at once. Then any coach will take you on to the towns its horses run to."
	everyLedgerAsk    = "Sign every stable's ledger now? Travelers once signed each at its own stable, so this may break immersion."
	everyLedgerYes    = "Yes, sign them all"
	everyLedgerNo     = "No, I'll sign them as I travel"
	everyLedgerDone   = "Your name is in every stable's ledger now."
)

// coachBoxMark marks a coachman's question's id (0x0034's u32, echoed by
// 0x0035).
const coachBoxMark = 0x57000000

// IsCoachBox reports a box a coachman asked.
func IsCoachBox(id uint32) bool { return id&0xFF000000 == coachBoxMark }

// unsignedLedgers are the coach towns (entity.CoachTownNames) whose ledgers
// are not in signed.
func unsignedLedgers(signed map[string]bool) []string {
	var out []string
	for _, t := range entity.CoachTownNames() {
		if !signed[t] {
			out = append(out, t)
		}
	}
	return out
}

// everyLedgerAsks keeps, by character (CharServerID), the coachman whose
// menu last offered it every ledger at once, and the box of his question
// once it is asked (0 before).
type everyLedgerAsks struct {
	mu   sync.Mutex
	seq  uint32
	open map[uint32]everyLedgerOffer
}

type everyLedgerOffer struct {
	npc *entity.NPC
	box uint32
}

// offer records that npc's menu offers the character every ledger.
func (a *everyLedgerAsks) offer(charID uint32, npc *entity.NPC) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.open == nil || len(a.open) > 20000 {
		a.open = map[uint32]everyLedgerOffer{}
	}
	a.open[charID] = everyLedgerOffer{npc: npc}
}

// ask gives the character's offer its question's box id, and is the
// coachman who made it (nil: none was made).
func (a *everyLedgerAsks) ask(charID uint32) (*entity.NPC, uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.open[charID]
	if !ok {
		return nil, 0
	}
	a.seq++
	o.box = coachBoxMark | (a.seq & 0xFFFFFF)
	a.open[charID] = o
	return o.npc, o.box
}

// answered takes the character's question asked as box, and is its coachman
// (nil: not the question it has open, an old box or none).
func (a *everyLedgerAsks) answered(charID, box uint32) *entity.NPC {
	a.mu.Lock()
	defer a.mu.Unlock()
	o, ok := a.open[charID]
	if !ok || o.box == 0 || o.box != box {
		return nil
	}
	delete(a.open, charID)
	return o.npc
}

// askEveryLedger is the menu's every-ledger entry picked: the coachman says
// what it does and asks Yes or No.
func (h *InteractHandler) askEveryLedger(sess *session.Session, player *entity.Player) {
	npc, box := h.every.ask(player.CharServerID)
	if npc == nil {
		return
	}
	for _, part := range splitSay(npc.Name+": "+everyLedgerTell, questBoxChars) {
		h.sendDialogueBox(sess, part)
	}
	SendQuestPopupMulti(sess, box, clipRunes(everyLedgerAsk, questAskChars), []string{everyLedgerYes, everyLedgerNo})
	log.Printf("Coach: %s asks %s whether to sign every stable's ledger", npc.Name, player.Name)
}

// answerEveryLedger is HandleDialogueOption's question for a coachman's box.
// Yes signs every ledger the character had not (source coachman-all), says
// so and opens his menu again while the player stands by him; No changes
// nothing. It reports whether the box was a coachman's (an old one does
// nothing).
func (h *InteractHandler) answerEveryLedger(sess *session.Session, player *entity.Player, box uint32, option byte) bool {
	if !IsCoachBox(box) {
		return false
	}
	if option == 255 || player == nil {
		return true // a close: the PS2 sends one before the real pick when two boxes were open
	}
	npc := h.every.answered(player.CharServerID, box)
	if npc == nil || h.ledger == nil {
		return true
	}
	if option != 0 {
		log.Printf("Coach: %s chose not to sign every stable's ledger at %s's", player.Name, npc.Name)
		return true
	}
	towns := entity.CoachTownNames()
	n, err := h.ledger.SignCoachLedgers(player.CharServerID, towns, database.CoachLedgersAll, time.Now())
	if err != nil {
		log.Printf("Coach: signing %s into every stable's ledger at %s's: %v", player.Name, npc.Name, err)
		h.sendDialogueBox(sess, npc.Name+": Something went wrong with the ledgers. Try again.")
		return true
	}
	h.sendDialogueBox(sess, npc.Name+": "+everyLedgerDone)
	log.Printf("Coach: %s signed every stable's ledger at %s's (%d of the %d towns new)", player.Name, npc.Name, n, len(towns))
	if !npc.Dead && entity.InRange(player, npc, entity.InteractRange) {
		h.coachMenu(sess, player, npc)
	}
	return true
}
