package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// keeper.go — the Heirloom Keeper (work board 1927 section 8): any NPC of the name, beside the
// Rehabilitators and Dabrish on the Heirloom Test Server, who takes Heirloom Marks (marks.go):
//
//   - a. the Ring of Guises: rank 1 gives the ring (record "Ring of Guises", NO TRADE) and its first
//     guise, each next rank unlocks the next guise of ringGuises, at 100 + 25 x (rank - 1) marks
//     (3,675 for all 14); "Change my guise" picks among those unlocked, free. Used, the ring lands
//     the guise's illusion on its user as that spell lands on its caster, for 30 minutes, whatever
//     the user's class or level, resting a minute between uses;
//   - b. an Heirloom Rank (heirloomrank.go): 150, 300, 600, 700, 900, 1000;
//   - c. a Mastery Tome, 100 marks: used by a level 60 not reliving, 49,117,200 experience (150
//     even-con level 60 kills) through World.AwardXP, so a boon and a reward multiply it and a debt
//     takes its half; refused below 60, while reliving, or at the mastery cap.
//
// Each purchase is asked again (Yes / No) and done in one transaction (database.SpendMarks).

// HeirloomKeeperName is the Keeper's name.
const HeirloomKeeperName = "Heirloom Keeper"

// keeperBoxMark marks the Keeper's questions.
const keeperBoxMark = 0x76000000

// The records (by name: the data tool makes them, 966020 and 966021 where free).
const (
	RingOfGuisesName = "Ring of Guises"
	MasteryTomeName  = "Mastery Tome"
)

// The figures (work 1927 8).
const (
	ringFirstPrice = 100
	ringPriceStep  = 25
	tomePrice      = 100
	tomeXP         = 49_117_200
	ringLasts      = 30 * time.Minute
	ringRest       = time.Minute
)

// heirloomRankPrices are the ranks' prices, rank 1 first.
var heirloomRankPrices = [HeirloomRankMost]int{150, 300, 600, 700, 900, 1000}

// ringGuises are the ring's guises by rank, the enchanter's illusions' spell ids.
var ringGuises = []int{700005, 700035, 700066, 700102, 700152, 700193, 545, 554, 565, 575, 584, 585, 700717, 700789}

// ringPrice is rank's price.
func ringPrice(rank int) int { return ringFirstPrice + ringPriceStep*(rank-1) }

// The Keeper's menu answers.
const (
	keeperRing = iota + 1
	keeperRingAgain
	keeperGuise
	keeperRank
	keeperTome
)

// The Keeper's questions.
const (
	keeperAskMenu = iota + 1
	keeperAskConfirm
	keeperAskGuise
)

// The Keeper's words (ours).
const (
	keeperQuestion  = "You hold %s Heirloom Marks. What will you have?"
	keeperYes       = "Yes, buy it."
	keeperNo        = "Not now."
	keeperShort     = "That costs %s Heirloom Marks, and you hold %s."
	keeperDone      = "It is yours."
	keeperTrouble   = "My ledger will not balance just now. Come back in a little while."
	keeperClosed    = "I keep nothing on this server."
	keeperRingFull  = "Your ring holds every guise I know."
	keeperRankFull  = "Your heirlooms have risen as far as they can."
	keeperNoRing    = "Buy the Ring of Guises first."
	keeperPackFull  = "Make room in your pack first."
	keeperAskGuises = "Which guise shall your ring wear?"
	keeperGuiseSet  = "Your ring now wears the guise of %s."
	ringNoGuise     = "The Ring of Guises holds no guise yet."
	ringRests       = "The Ring of Guises is not ready yet."
	tomeNot60       = "The tome's lessons are for one at the height of its strength, level 60."
	tomeReliving    = "Return to your full strength first."
	tomeCapped      = "You have nothing more to learn from it."
)

// IsHeirloomKeeper is whether an NPC is the Keeper.
func IsHeirloomKeeper(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), HeirloomKeeperName)
}

// keeperTalk is a talk to npc: the Keeper's menu when npc is the Keeper, else false.
func (m *HeirloomMarks) keeperTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || !IsHeirloomKeeper(npc.Name) {
		return false
	}
	if m == nil || !HeirloomMarksOn() {
		marksSay(sess, HeirloomKeeperName, keeperClosed)
		return true
	}
	m.keeperMenu(sess, p, npc.ServerID)
	return true
}

// keeperMenu asks what to buy, each answer its price.
func (m *HeirloomMarks) keeperMenu(sess *session.Session, p *entity.Player, npc entity.EntityID) {
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil {
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	var labels []string
	var opts []int
	if row.RingRank < len(ringGuises) {
		next := row.RingRank + 1
		labels = append(labels, fmt.Sprintf("Ring of Guises, rank %d (%s): %s marks", next, m.guiseName(ringGuises[next-1]), commas(ringPrice(next))))
		opts = append(opts, keeperRing)
	}
	if row.RingRank > 0 && !m.holds(p.CharServerID, m.recordNamed(RingOfGuisesName)) {
		labels = append(labels, "A new Ring of Guises: free")
		opts = append(opts, keeperRingAgain)
	}
	if row.RingRank > 1 {
		labels = append(labels, "Change my guise: free")
		opts = append(opts, keeperGuise)
	}
	if row.HeirloomRank < HeirloomRankMost {
		next := row.HeirloomRank + 1
		labels = append(labels, fmt.Sprintf("Heirloom Rank %d (heirlooms to %d): %s marks", next, heirloomTopLevel+next, commas(heirloomRankPrices[next-1])))
		opts = append(opts, keeperRank)
	}
	labels = append(labels, fmt.Sprintf("Mastery Tome: %d marks", tomePrice))
	opts = append(opts, keeperTome)
	m.ask(sess, p, keeperBoxMark, marksBox{kind: keeperAskMenu, npc: npc, opts: opts},
		fmt.Sprintf(keeperQuestion, commas(row.Marks)), labels)
}

// answerKeeper is HandleDialogueOption's question for the Keeper's box. It reports whether the box
// was his.
func (m *HeirloomMarks) answerKeeper(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if box&0xFF000000 != keeperBoxMark {
		return false
	}
	if m == nil || option == 255 {
		return true
	}
	b, ok := m.answered(p, box)
	if !ok {
		return true
	}
	if !HeirloomMarksOn() {
		marksSay(sess, HeirloomKeeperName, keeperClosed)
		return true
	}
	i := int(option)
	switch b.kind {
	case keeperAskMenu:
		if i >= len(b.opts) {
			return true
		}
		switch what := b.opts[i]; what {
		case keeperGuise:
			m.askGuise(sess, p, b.npc)
		case keeperRingAgain:
			m.giveRing(sess, p)
		default:
			m.askBuy(sess, p, b.npc, what)
		}
	case keeperAskConfirm:
		if i == 0 && len(b.opts) == 1 {
			m.buy(sess, p, b.opts[0])
		} else {
			marksSay(sess, HeirloomKeeperName, rehabAsYouWish)
		}
	case keeperAskGuise:
		if i < len(b.opts) {
			m.setGuise(sess, p, b.opts[i])
		}
	}
	return true
}

// askBuy asks again before a purchase.
func (m *HeirloomMarks) askBuy(sess *session.Session, p *entity.Player, npc entity.EntityID, what int) {
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil {
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	label, price, why := m.offer(row, what)
	if why != "" {
		marksSay(sess, HeirloomKeeperName, why)
		return
	}
	if price > row.Marks {
		marksSay(sess, HeirloomKeeperName, fmt.Sprintf(keeperShort, commas(price), commas(row.Marks)))
		return
	}
	m.ask(sess, p, keeperBoxMark, marksBox{kind: keeperAskConfirm, npc: npc, opts: []int{what}},
		fmt.Sprintf("%s for %s of your %s Heirloom Marks?", label, commas(price), commas(row.Marks)), []string{keeperYes, keeperNo})
}

// offer is what a menu answer buys for row now: its words, its price, or why not.
func (m *HeirloomMarks) offer(row database.CharMarks, what int) (string, int, string) {
	switch what {
	case keeperRing:
		if row.RingRank >= len(ringGuises) {
			return "", 0, keeperRingFull
		}
		next := row.RingRank + 1
		if next == 1 {
			return fmt.Sprintf("The Ring of Guises with the guise of %s", m.guiseName(ringGuises[0])), ringPrice(1), ""
		}
		return fmt.Sprintf("Rank %d of the Ring of Guises, the guise of %s", next, m.guiseName(ringGuises[next-1])), ringPrice(next), ""
	case keeperRank:
		if row.HeirloomRank >= HeirloomRankMost {
			return "", 0, keeperRankFull
		}
		next := row.HeirloomRank + 1
		return fmt.Sprintf("Heirloom Rank %d, your heirlooms to level %d", next, heirloomTopLevel+next), heirloomRankPrices[next-1], ""
	case keeperTome:
		return "A Mastery Tome", tomePrice, ""
	}
	return "", 0, keeperTrouble
}

// buy is a purchase confirmed: the marks taken and the thing given, or nothing.
func (m *HeirloomMarks) buy(sess *session.Session, p *entity.Player, what int) {
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil {
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	label, price, why := m.offer(row, what)
	if why != "" {
		marksSay(sess, HeirloomKeeperName, why)
		return
	}
	// an item first finds room: the ring at rank 1, a tome
	var item *database.ItemPatternRecord
	if what == keeperTome || (what == keeperRing && row.RingRank == 0) {
		name := MasteryTomeName
		if what == keeperRing {
			name = RingOfGuisesName
		}
		id := m.recordNamed(name)
		if id == 0 {
			log.Printf("Marks: no record named %q", name)
			marksSay(sess, HeirloomKeeperName, keeperTrouble)
			return
		}
		rec, err := m.items.GetItemPattern(id)
		if err != nil || rec == nil {
			marksSay(sess, HeirloomKeeperName, keeperTrouble)
			return
		}
		item = rec
	}
	after, err := m.db.SpendMarks(p.CharServerID, price, label, func(c *database.CharMarks) error {
		switch what {
		case keeperRing:
			c.RingRank++
			if c.RingGuise == 0 || c.RingRank == 1 {
				c.RingGuise = ringGuises[0]
			}
		case keeperRank:
			c.HeirloomRank++
		}
		return nil
	}, m.now())
	if errors.Is(err, database.ErrNotEnoughMarks) {
		marksSay(sess, HeirloomKeeperName, fmt.Sprintf(keeperShort, commas(price), commas(after.Marks)))
		return
	}
	if err != nil {
		log.Printf("Marks: %s's purchase %q: %v", p.Name, label, err)
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	log.Printf("Marks: %s -%d source=spend ref=%q (%d left)", p.Name, price, label, after.Marks)
	if item != nil {
		if n, err := GiveItemNow(sess, m.gw, m.inv, *item, 1); err != nil || n < 1 {
			// the marks are spent: the ring is given again for nothing ("A new Ring of Guises"), a
			// tome is handed back as its marks
			log.Printf("Marks: %s's %q not given (%v): its pack", p.Name, item.ItemName, err)
			if what == keeperTome {
				m.db.SpendMarks(p.CharServerID, -tomePrice, "a tome not given: its marks back", nil, m.now())
			}
			marksSay(sess, HeirloomKeeperName, keeperPackFull)
			return
		}
	}
	if what == keeperRank {
		m.heir.SetRank(p.CharServerID, after.HeirloomRank)
		if m.refresh != nil {
			go m.refresh(p) // the worn heirlooms' figures at once, as at a level up
		}
	}
	marksSay(sess, HeirloomKeeperName, keeperDone)
	SendClientMessage(sess, fmt.Sprintf("You spend %s Heirloom Marks: %s. %s left.", commas(price), label, commas(after.Marks)))
}

// giveRing gives a character of rank 1 or more that holds no ring a new one.
func (m *HeirloomMarks) giveRing(sess *session.Session, p *entity.Player) {
	id := m.recordNamed(RingOfGuisesName)
	if id == 0 || m.holds(p.CharServerID, id) {
		return
	}
	rec, err := m.items.GetItemPattern(id)
	if err != nil || rec == nil {
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	if n, err := GiveItemNow(sess, m.gw, m.inv, *rec, 1); err != nil || n < 1 {
		marksSay(sess, HeirloomKeeperName, keeperPackFull)
		return
	}
	marksSay(sess, HeirloomKeeperName, keeperDone)
}

// holds is whether a character carries a record anywhere it keeps items.
func (m *HeirloomMarks) holds(serverID uint32, id int) bool {
	if id == 0 || m.inv == nil {
		return false
	}
	items, err := m.inv.GetCharacterItems(serverID)
	if err != nil {
		return true // not known: offer nothing free
	}
	for _, it := range items {
		if it.Pattern.PatternID == id {
			return true
		}
	}
	return false
}

// askGuise asks which unlocked guise the ring wears.
func (m *HeirloomMarks) askGuise(sess *session.Session, p *entity.Player, npc entity.EntityID) {
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil || row.RingRank == 0 {
		marksSay(sess, HeirloomKeeperName, keeperNoRing)
		return
	}
	unlocked := ringGuises[:min(row.RingRank, len(ringGuises))]
	// CLIENT's box holds maxDialogOptions: the newest guises first past that
	var opts []int
	var labels []string
	for i := len(unlocked) - 1; i >= 0 && len(opts) < maxDialogOptions; i-- {
		opts = append(opts, unlocked[i])
		labels = append(labels, m.guiseName(unlocked[i]))
	}
	m.ask(sess, p, keeperBoxMark, marksBox{kind: keeperAskGuise, npc: npc, opts: opts}, keeperAskGuises, labels)
}

// setGuise sets the ring's guise, when unlocked.
func (m *HeirloomMarks) setGuise(sess *session.Session, p *entity.Player, spell int) {
	if !m.guiseUnlocked(p.CharServerID, spell) {
		return
	}
	if _, err := m.db.SpendMarks(p.CharServerID, 0, "", func(c *database.CharMarks) error {
		c.RingGuise = spell
		return nil
	}, m.now()); err != nil {
		marksSay(sess, HeirloomKeeperName, keeperTrouble)
		return
	}
	marksSay(sess, HeirloomKeeperName, fmt.Sprintf(keeperGuiseSet, m.guiseName(spell)))
}

// guiseUnlocked is whether a character's ring rank has unlocked a guise.
func (m *HeirloomMarks) guiseUnlocked(serverID uint32, spell int) bool {
	row, err := m.db.CharMarksOf(serverID)
	if err != nil {
		return false
	}
	for i := 0; i < min(row.RingRank, len(ringGuises)); i++ {
		if ringGuises[i] == spell {
			return true
		}
	}
	return false
}

// IsRingOfGuises and IsMasteryTome are the two records' items.
func IsRingOfGuises(p *database.ItemPatternRecord) bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.ItemName), RingOfGuisesName)
}

func IsMasteryTome(p *database.ItemPatternRecord) bool {
	return p != nil && strings.EqualFold(strings.TrimSpace(p.ItemName), MasteryTomeName)
}

// ringCast is the ring's use: the guise picked as a cast of its spell, an item's (no power, no
// lockout, any class and level), lasting ringLasts; ok false with the line to say.
func (m *HeirloomMarks) ringCast(p *entity.Player) (entity.CastSpell, string) {
	if m == nil || !HeirloomMarksOn() {
		return entity.CastSpell{}, marksOffLine
	}
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil || row.RingRank == 0 || row.RingGuise == 0 || !m.guiseUnlocked(p.CharServerID, row.RingGuise) {
		return entity.CastSpell{}, ringNoGuise
	}
	m.mu.Lock()
	last, used := m.ringAt[p.CharServerID]
	m.mu.Unlock()
	if used && m.now().Before(last.Add(ringRest)) {
		return entity.CastSpell{}, ringRests
	}
	sp, err := m.spells.SpellPattern(row.RingGuise)
	if err != nil || sp == nil {
		return entity.CastSpell{}, ringNoGuise
	}
	fx, _, err := m.spells.SpellEffectRow(row.RingGuise)
	if err != nil {
		return entity.CastSpell{}, ringNoGuise
	}
	spell := castSpellRow(&database.CharSpellEntry{Pattern: *sp}, fx)
	spell.PowerCost, spell.ItemCast = 0, true
	spell.Def.Duration = ringLasts
	return spell, ""
}

// ringUsed notes a use, for its rest.
func (m *HeirloomMarks) ringUsed(serverID uint32) {
	m.mu.Lock()
	m.ringAt[serverID] = m.now()
	m.mu.Unlock()
}

// tomeRefusal is why p may not read a tome, "" when it may.
func tomeRefusal(p *entity.Player, mastery entity.Mastery) string {
	switch {
	case p.Reliving():
		return tomeReliving
	case p.Level < entity.MaxLevel:
		return tomeNot60
	case mastery.Points+mastery.Spent >= entity.MasteryCap:
		return tomeCapped
	}
	return ""
}

// useRing is the Ring of Guises' Use (equip.go).
func (h *EquipHandler) useRing(sess *session.Session, player *entity.Player, target uint32) {
	spell, why := h.marks.ringCast(player)
	if why != "" {
		SendErrorMessage(sess, why)
		return
	}
	if h.gameWorld != nil && h.gameWorld.StartCast(sess, spell, target, time.Now()) {
		h.marks.ringUsed(player.CharServerID)
		log.Printf("Marks: %s uses the Ring of Guises: %q", player.Name, spell.Name)
	}
}

// useTome is a Mastery Tome's Use (equip.go): its experience, one off the stack.
func (h *EquipHandler) useTome(sess *session.Session, player *entity.Player, idx int, row *database.CharInventoryEntry) {
	if h.marks == nil || !HeirloomMarksOn() || h.gameWorld == nil {
		SendErrorMessage(sess, marksOffLine)
		return
	}
	m, _ := h.gameWorld.MasteryOf(player.ServerID)
	if why := tomeRefusal(player, m); why != "" {
		SendErrorMessage(sess, why)
		return
	}
	if err := h.takeOne(sess, player, idx, row); err != nil {
		log.Printf("Marks: %s's tome: %v", player.Name, err)
		return
	}
	queueReliable(sess, protocol.OpcodeRemoveInvItem, removeInvItemPayload(idx, 0, 1))
	log.Printf("Marks: %s reads a Mastery Tome: %d experience", player.Name, tomeXP)
	h.gameWorld.AwardXP(sess, player, tomeXP)
}
