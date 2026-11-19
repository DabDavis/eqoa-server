package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// metalmoogie.go — Mr. Moogie's challenge and his favor, as his talk offers them (moogie.go; the GM
// a GM's petitions 1379 and 1380, by a GM): "I'd like to prove my worth" stands a Metal Moogie up
// to duel the player (entity/metalmoogie.go), and a Moogie Tribute, the one item Metal Moogie's corpse
// gives, handed back to Mr. Moogie lands his Duelist's Favor on its giver: half again as big for an hour
// or until death (the spell's record, "Self, Increase Size by 50%, 60min": entity's EffectSize). The
// item and the spell are the data's, found by their names (entity.MoogieTributeName,
// entity.DuelistsFavorName); without the spell his box offers no hand-in, and the tribute stays in the
// pack. The words are ours.

// The turn-in's words: the answer in his box, and what he says.
const (
	moogieProveOpt   = "I'd like to prove my worth" // a GM's words
	moogieTributeOpt = "I bring you a Moogie Tribute"
	moogieFavored    = "Mr. Moogie says, 'A worthy duelist at last! Wear my favor proudly, %s, and stand tall.'"
	moogieNoFavor    = "Mr. Moogie says, 'I have no favor to give today. Keep your tribute.'"
	moogieNoTribute  = "Mr. Moogie says, 'You carry no tribute of mine.'"
	// the tribute's second question while Metal Moogie V2 is on the server (a GM's petition
	// 1441: "if a player has a token from defeating metal moogie that can instead choose to use it to
	// summon Metal Moogie V2"): its uses, and what he says when it cannot be taken
	moogieTributeQuestion = "Mr. Moogie: 'A tribute! Shall I give you my favor for it, or will you spend it to face my metal twin remade?'"
	moogieFavorOpt        = "I would have your favor"
	moogieV2Opt           = "Summon Metal Moogie V2"
	moogieV2Untaken       = "Mr. Moogie says, 'Your tribute slipped away, %s. My twin waits for another.'"
)

// MoogieFavor is what the turn-in needs: the pack, and the spell records.
type MoogieFavor struct {
	inv    *database.InventoryStore
	spells *database.SpellbookStore
}

// NewMoogieFavor is the turn-in over the pack and the spell records.
func NewMoogieFavor(inv *database.InventoryStore, spells *database.SpellbookStore) *MoogieFavor {
	if inv == nil || spells == nil {
		return nil
	}
	return &MoogieFavor{inv: inv, spells: spells}
}

// tribute is the player's first Moogie Tribute in the pack (not banked, not worn), ok false for none.
func (f *MoogieFavor) tribute(p *entity.Player) (database.CharInventoryEntry, bool) {
	if f == nil || p == nil {
		return database.CharInventoryEntry{}, false
	}
	rows, err := f.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Moogie: %s's inventory: %v", p.Name, err)
		return database.CharInventoryEntry{}, false
	}
	for _, r := range rows {
		if r.Location == 0 && r.EquipLoc < 0 && r.StackLeft > 0 &&
			strings.EqualFold(strings.TrimSpace(r.Pattern.ItemName), entity.MoogieTributeName) {
			return r, true
		}
	}
	return database.CharInventoryEntry{}, false
}

// favor is the Duelist's Favor's record as the World lands it, ok false when the server has none.
func (f *MoogieFavor) favor() (entity.CastSpell, bool) {
	if f == nil {
		return entity.CastSpell{}, false
	}
	id, ok, err := f.spells.SpellIDByName(entity.DuelistsFavorName)
	if err != nil || !ok {
		if err != nil {
			log.Printf("Moogie: %s: %v", entity.DuelistsFavorName, err)
		}
		return entity.CastSpell{}, false
	}
	sp, err := f.spells.SpellPattern(id)
	if err != nil || sp == nil {
		log.Printf("Moogie: %s (%d): %v", entity.DuelistsFavorName, id, err)
		return entity.CastSpell{}, false
	}
	row, found, err := f.spells.SpellEffectRow(id)
	if err != nil {
		log.Printf("Moogie: %s's effects (%d): %v", entity.DuelistsFavorName, id, err)
		return entity.CastSpell{}, false
	}
	if !found {
		row = database.SpellEffectRow{}
	}
	return entity.SpellOfRow(sp, 0, row.Line, row.Effects, row.FX), true
}

// offers reports whether his box offers the hand-in: the player holds a tribute and the favor is there.
func (f *MoogieFavor) offers(p *entity.Player) bool {
	if _, ok := f.tribute(p); !ok {
		return false
	}
	_, ok := f.favor()
	return ok
}

// take takes one tribute out of the player's pack, kept and shown (0x003D, as a quest's hand-in takes
// an item). It reports whether one went.
func (f *MoogieFavor) take(sess *session.Session, p *entity.Player, r database.CharInventoryEntry) bool {
	var err error
	if r.StackLeft > 1 {
		err = f.inv.SetItemStack(p.CharServerID, r.ItemID, r.StackLeft-1)
	} else {
		err = f.inv.DeleteCharItem(p.CharServerID, r.ItemID)
	}
	if err != nil {
		log.Printf("Moogie: taking %s's %q: %v", p.Name, r.Pattern.ItemName, err)
		return false
	}
	if l := sessionInvList(sess); l != nil {
		if idx := l.indexOfItem(r.ItemID); idx >= 0 && l.took(idx, 1) {
			queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
		}
	}
	return true
}

// v2Offered reports whether his box offers p Metal Moogie V2's duel for a tribute: p holds one and V2's
// original is on the server (entity's MetalMoogieV2Here).
func (h *InteractHandler) v2Offered(p *entity.Player) bool {
	if h.gameWorld == nil || !h.gameWorld.MetalMoogieV2Here() {
		return false
	}
	_, ok := h.favor.tribute(p)
	return ok
}

// moogieTributeAnswers is the tribute's second question's answers for p: his favor while the server has
// it, V2's duel, nothing.
func (h *InteractHandler) moogieTributeAnswers(p *entity.Player) moogieChoices {
	var c moogieChoices
	if _, ok := h.favor.favor(); ok {
		c.labels, c.does = append(c.labels, moogieFavorOpt), append(c.does, moogieDoFavor)
	}
	c.labels, c.does = append(c.labels, moogieV2Opt, moogieNoOpt), append(c.does, moogieDoV2, moogieDoNothing)
	return c
}

// moogieV2 is V2's duel asked for a tribute (petition 1441): Metal Moogie V2 rises as Metal Moogie does
// (entity's MetalMoogieV2Challenge, every rule of the duel his), and once he stands one tribute is taken
// from the pack; refused (another's duel, his original gone, no tribute) nothing is taken. A tribute that
// cannot be taken after he rose ends his duel at once, so none is had for nothing.
func (h *InteractHandler) moogieV2(sess *session.Session, p *entity.Player, moogie entity.EntityID) {
	f := h.favor
	r, ok := f.tribute(p)
	if !ok {
		SendClientMessage(sess, moogieNoTribute)
		return
	}
	line, rose := h.gameWorld.MetalMoogieV2Challenge(sess, moogie, time.Now())
	SendClientMessage(sess, line)
	if !rose {
		return
	}
	if !f.take(sess, p, r) {
		h.gameWorld.EndMetalDuelOf(sess, "the tribute could not be taken")
		SendClientMessage(sess, fmt.Sprintf(moogieV2Untaken, p.Name))
		return
	}
	log.Printf("Moogie: %s spends a %s on %s's duel", p.Name, r.Pattern.ItemName, entity.MetalMoogieV2Name)
}

// handIn is the hand-in answered: the tribute taken and the favor landed, else said why not.
func (h *InteractHandler) moogieHandIn(sess *session.Session, p *entity.Player) {
	f := h.favor
	r, ok := f.tribute(p)
	if !ok {
		SendClientMessage(sess, moogieNoTribute)
		return
	}
	spell, ok := f.favor()
	if !ok {
		SendClientMessage(sess, moogieNoFavor)
		return
	}
	if !f.take(sess, p, r) {
		SendClientMessage(sess, moogieNoFavor)
		return
	}
	if !h.gameWorld.GiveDuelistsFavor(sess, spell, time.Now()) {
		log.Printf("Moogie: %s's tribute taken, but %q landed nothing", p.Name, spell.Name)
	}
	log.Printf("Moogie: %s hands Mr. Moogie a %s and has his %s", p.Name, r.Pattern.ItemName, spell.Name)
	SendClientMessage(sess, fmt.Sprintf(moogieFavored, p.Name))
}

// metalTalk is a talk to a Metal Moogie standing (another's: the challenger's client fights him, never
// talks), answered in a line; false for any other NPC.
func (h *InteractHandler) metalTalk(sess *session.Session, npc *entity.NPC) bool {
	if h.gameWorld == nil {
		return false
	}
	line, ok := h.gameWorld.MetalMoogieTalk(npc)
	if !ok {
		return false
	}
	SendColoredChat(sess, line, 255, 64, 64)
	return true
}

// The Metal Moogie chain's parts (work board #688, a GM's): each version's corpse gives one, and
// Mr. Moogie takes it for a reward or for the next version's duel. His box adds an answer for each part
// held in the pack (moogieAnswers); its second question has the reward, the next duel (while that
// version's original is on the server) and No; the head has no question, the reward at once. The reward
// goes into the pack and the part is taken (0x003C, as the tribute is) after it; the next duel takes the part
// only once he stands (refused, nothing taken), as V2's does the tribute. Each part allows one turn-in:
// the part is the flag (lore, no trade), taken by either answer. The words are a GM's.
type moogiePart struct {
	item      string // the part (entity.MetalLegsName ...)
	answer    string // its answer in his box
	question  string // its second question, "" none: the reward at once
	rewardOpt string
	nextOpt   string
	reward    string // the reward's item, by its name, and how many
	count     int
	next      string // the next version's name, "" none
	thanks    string // what he says at the reward (%s the player's name)
}

// moogieParts are the parts in the chain's order.
var moogieParts = []moogiePart{
	{item: entity.MetalLegsName, answer: "I bring you Metal Moogie's Legs",
		question:  "Mr. Moogie: 'My twin's legs! Snowballs for your trouble, or will you face him rebuilt, stronger still?'",
		rewardOpt: "I'll take the snowballs", nextOpt: "Summon Metal Moogie V3", reward: "Snowball", count: 99,
		next: entity.MetalMoogieV3Name, thanks: moogieFairTrade},
	{item: entity.MetalTorsoName, answer: "I bring you Metal Moogie's Torso",
		question:  "Mr. Moogie: 'His torso! A draught of my insight, or his next rebuild?'",
		rewardOpt: "I'll take the potion", nextOpt: "Summon Metal Moogie V4", reward: "Potion of Moogie's Insight", count: 1,
		next: entity.MetalMoogieV4Name, thanks: moogieFairTrade},
	{item: entity.MetalArmsName, answer: "I bring you Metal Moogie's Arms",
		question:  "Mr. Moogie: 'His arms! A deputy's draught, or his final form?'",
		rewardOpt: "I'll take the draught", nextOpt: "Summon Metal Moogie V5", reward: "Deputy's Draught", count: 1,
		next: entity.MetalMoogieV5Name, thanks: moogieFairTrade},
	{item: entity.MetalHeadName, answer: "I bring you Metal Moogie's Head", reward: "Moogie Winding Stone", count: 1,
		thanks: "Mr. Moogie says, 'You are worthy, %s. Truly worthy. Take this stone: my twin will follow you, small and loyal, for as long as you like.'"},
}

// The parts' other words: his thanks at a reward (a GM's), the No of a part's question, and ours:
// the one answer for every hand-in when they do not fit his box with the rest (CLIENT's box holds
// maxDialogOptions), its question, a full pack, a reward the server has not, a part slipped away.
const (
	moogieFairTrade   = "Mr. Moogie says, 'A fair trade, %s.'"
	moogiePartNo      = "No"
	moogieTrophiesOpt = "I bring you Metal Moogie's parts"
	moogieTrophiesAsk = "Mr. Moogie: 'What have you brought me of my twin?'"
	moogiePackFull    = "Mr. Moogie says, 'Make room in your pack first, %s.'"
	moogieNoReward    = "Mr. Moogie says, 'I have nothing to give for that today. Keep it.'"
	moogieNoPart      = "Mr. Moogie says, 'You carry no such part of my twin.'"
	moogiePartUntaken = "Mr. Moogie says, 'Your part slipped away, %s. My twin waits for another.'"
)

// held is the player's first row of the item named in the pack (not banked, not worn), ok false for none.
func (f *MoogieFavor) held(p *entity.Player, name string) (database.CharInventoryEntry, bool) {
	if f == nil || p == nil {
		return database.CharInventoryEntry{}, false
	}
	rows, err := f.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		log.Printf("Moogie: %s's inventory: %v", p.Name, err)
		return database.CharInventoryEntry{}, false
	}
	for _, r := range rows {
		if r.Location == 0 && r.EquipLoc < 0 && r.StackLeft > 0 && strings.EqualFold(strings.TrimSpace(r.Pattern.ItemName), name) {
			return r, true
		}
	}
	return database.CharInventoryEntry{}, false
}

// partsHeld is the parts p holds, by their index in moogieParts, in the chain's order.
func (h *InteractHandler) partsHeld(p *entity.Player) []int {
	var out []int
	for i := range moogieParts {
		if _, ok := h.favor.held(p, moogieParts[i].item); ok {
			out = append(out, i)
		}
	}
	return out
}

// partAnswers is a part's second question's answers: its reward, the next version's duel while his original
// is on the server, No.
func (h *InteractHandler) partAnswers(i int) moogieChoices {
	part := &moogieParts[i]
	c := moogieChoices{labels: []string{part.rewardOpt}, does: []moogieDo{moogieDoPartReward + moogieDo(i)}}
	if part.next != "" && h.gameWorld != nil && h.gameWorld.MetalMoogieVersionHere(part.next) {
		c.labels, c.does = append(c.labels, part.nextOpt), append(c.does, moogieDoPartNext+moogieDo(i))
	}
	c.labels, c.does = append(c.labels, moogiePartNo), append(c.does, moogieDoNothing)
	return c
}

// trophyAnswers is the hand-ins' own box (moogieTrophiesOpt): the tribute's while it is offered, each part
// held, No.
func (h *InteractHandler) trophyAnswers(p *entity.Player) moogieChoices {
	var c moogieChoices
	if h.favor.offers(p) || h.v2Offered(p) {
		c.labels, c.does = append(c.labels, moogieTributeOpt), append(c.does, moogieDoTribute)
	}
	for _, i := range h.partsHeld(p) {
		c.labels, c.does = append(c.labels, moogieParts[i].answer), append(c.does, moogieDoPart+moogieDo(i))
	}
	c.labels, c.does = append(c.labels, moogiePartNo), append(c.does, moogieDoNothing)
	return c
}

// moogiePartAnswered is one of a part's answers (does is moogieDoPart, moogieDoPartReward or moogieDoPartNext
// and the part's index): the part's question (the head: its reward at once), its reward, the next version's
// duel. It reports false for any other answer.
func (h *InteractHandler) moogiePartAnswered(sess *session.Session, p *entity.Player, moogie entity.EntityID, do moogieDo) bool {
	switch {
	case do >= moogieDoPart && int(do-moogieDoPart) < len(moogieParts):
		i := int(do - moogieDoPart)
		if moogieParts[i].question == "" {
			h.partReward(sess, p, i)
			return true
		}
		if _, ok := h.favor.held(p, moogieParts[i].item); !ok {
			SendClientMessage(sess, moogieNoPart)
			return true
		}
		h.moogieAsk(sess, p, moogie, moogieParts[i].question, h.partAnswers(i))
	case do >= moogieDoPartReward && int(do-moogieDoPartReward) < len(moogieParts):
		h.partReward(sess, p, int(do-moogieDoPartReward))
	case do >= moogieDoPartNext && int(do-moogieDoPartNext) < len(moogieParts):
		h.partNext(sess, p, moogie, int(do-moogieDoPartNext))
	default:
		return false
	}
	return true
}

// partReward is a part handed in for its reward: the reward into the pack, then the part taken, and his
// thanks; a pack with no room for it keeps the part, told so.
func (h *InteractHandler) partReward(sess *session.Session, p *entity.Player, i int) {
	part := &moogieParts[i]
	f := h.favor
	r, ok := f.held(p, part.item)
	if !ok {
		SendClientMessage(sess, moogieNoPart)
		return
	}
	pattern, found, err := f.inv.ItemPatternByName(part.reward)
	if err != nil || !found {
		if err != nil {
			log.Printf("Moogie: %q: %v", part.reward, err)
		}
		SendClientMessage(sess, moogieNoReward)
		return
	}
	given, err := GiveItemNow(sess, h.gameWorld, f.inv, pattern, part.count)
	if err != nil {
		log.Printf("Moogie: giving %s %d of %q: %v", p.Name, part.count, part.reward, err)
	}
	if given == 0 {
		SendColoredChat(sess, fmt.Sprintf(moogiePackFull, p.Name), 255, 63, 63)
		return
	}
	if !f.take(sess, p, r) {
		log.Printf("Moogie: %s has %d of %q, and the %s could not be taken", p.Name, given, part.reward, part.item)
	}
	log.Printf("Moogie: %s hands Mr. Moogie the %s for %d of %q", p.Name, part.item, given, part.reward)
	SendClientMessage(sess, fmt.Sprintf(part.thanks, p.Name))
}

// partNext is a part spent on the next version's duel: he rises as V2 does for a tribute (entity's
// MetalMoogieVersionChallenge, every rule of the duel his), and the part is taken only once he stands;
// refused (another's duel, his original gone), nothing is taken. A part that cannot be taken after he rose
// ends his duel at once.
func (h *InteractHandler) partNext(sess *session.Session, p *entity.Player, moogie entity.EntityID, i int) {
	part := &moogieParts[i]
	f := h.favor
	r, ok := f.held(p, part.item)
	if !ok || part.next == "" {
		SendClientMessage(sess, moogieNoPart)
		return
	}
	line, rose := h.gameWorld.MetalMoogieVersionChallenge(sess, moogie, part.next, time.Now())
	SendClientMessage(sess, line)
	if !rose {
		return
	}
	if !f.take(sess, p, r) {
		h.gameWorld.EndMetalDuelOf(sess, "the part could not be taken")
		SendClientMessage(sess, fmt.Sprintf(moogiePartUntaken, p.Name))
		return
	}
	log.Printf("Moogie: %s spends the %s on %s's duel", p.Name, part.item, part.next)
}
