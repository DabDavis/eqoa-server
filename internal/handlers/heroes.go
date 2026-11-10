package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// heroes.go — the Testing Heroes Vendor (entity/heroes.go is the faction's
// half; petition 1471, the GM Jibrish, 2026-11-10: "Add a vendor in freeport
// near the well and dabrish that allows you to buy items from him with that
// faction. The items should cost 25, 100, 250, 500 and 1000 faction and reduce
// their faction by that amount. If it would take the player below max
// negative, they do not have enough to buy the item.").
//
// world.testing_heroes (SetTestingHeroes: off unless set; the Heirloom Test
// Server sets it) turns him on; off, an NPC of his name has nothing to say.
// Any NPC named Testing Heroes Vendor is him (talked to before quests, as
// Dabrish and the Teleporter); tools/seed_testing_heroes.py stands one by the
// well behind Dabrish and writes his stock, the faction and the items.
//
// His shop is talk boxes, not the merchant window (CLIENT's shows a price in
// tunar and takes it from the purse): a greeting that says what the player
// stands at and may spend (0x0046), then a question listing his stock, each
// "<item>: <cost>" (0x0034 under box mark 0x64000000, beside Call of the
// Hero's 0x63; six answers a box, so more than six wares go five a page with
// "More..."). A pick says what the item is (its heroes_stock row's words, else
// its record's description; that the buyer could not wear it) and asks again;
// Yes buys: the stock row is read again (a price changed since the list asks
// again), the standing above the floor must cover the cost, a Lore item not be
// held already, and the pack have room (the quests' rule, questroom.go's
// packShort); then the cost comes off the standing (entity.HeroesSpend, told as
// a faction's change is), the item is given as a merchant's sale is
// (GiveItemNow, new, with its charges), the standing is kept at once
// (charfactions: an item given and a standing lost with a crash would be a
// free item) and the purchase is logged (the server's log and table
// heroes_purchases). A give that fails after all gives the cost back. A box is
// answered only within heroesReach of the vendor that asked. The words are
// ours.

// HeroesVendorName is his name: an NPC of it is the vendor.
const HeroesVendorName = "Testing Heroes Vendor"

// heroesBoxMark marks his questions' ids.
const heroesBoxMark = 0x64000000

// heroesReach is how near the vendor a box is answered.
const heroesReach = 30

// heroesPage is how many wares a box lists when there are more than a box
// holds (maxDialogOptions): the sixth answer is then "More...".
const heroesPage = maxDialogOptions - 1

var heroesOn atomic.Bool

// SetTestingHeroes sets world.testing_heroes for the vendor.
func SetTestingHeroes(on bool) { heroesOn.Store(on) }

// TestingHeroesOn reports world.testing_heroes.
func TestingHeroesOn() bool { return heroesOn.Load() }

// IsHeroesVendor is whether an NPC of the name is the vendor.
func IsHeroesVendor(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), HeroesVendorName)
}

// IsHeroesBox reports a box the vendor asked.
func IsHeroesBox(id uint32) bool { return id&0xFF000000 == heroesBoxMark }

// His words. A question is 127 characters at most (questAskChars), an answer
// 63 (questAnswerChars), a box 217 (questBoxChars: say cuts a longer one).
const (
	heroesGreeting = "Well met, %s. Every kill that gives you experience earns 1 faction with the Testing Heroes, a raid boss 5. You stand at %d: %d above the bottom, and that is yours to spend."
	heroesAskList  = "Testing Heroes Vendor: 'You have %d to spend. What will you have?'"
	heroesAskBuy   = "Testing Heroes Vendor: 'The %s, for %d of your %d?'"
	heroesYes      = "Yes, I will take it"
	heroesNo       = "No, not now"
	heroesMore     = "More..."
	heroesClosed   = "My ledger is closed today."
	heroesEmpty    = "I have nothing to sell today."
	heroesShort    = "The %s costs %d faction and you have %d to spend: %d more to earn."
	heroesChanged  = "My prices have changed since I showed you. Ask me again."
	heroesTooFar   = "Come back to me to trade."
	heroesFailed   = "I could not hand it over: you keep your faction. Try again."
	heroesBought   = "The %s is yours. You have %d left to spend."
	heroesCantWear = " You could not wear it as you are."
)

// heroesWare is something the vendor sells: its stock row and its record.
type heroesWare struct {
	row database.HeroesStockRow
	pat database.ItemPatternRecord
}

// label is the ware as an answer names it: "<item>: <cost>", the name cut to
// leave the cost in an answer's 63 characters.
func (w heroesWare) label() string {
	tail := fmt.Sprintf(": %d", w.row.Cost)
	return clipRunes(strings.TrimSpace(w.pat.ItemName), questAnswerChars-len(tail)) + tail
}

// The questions he asks.
const (
	heroesStageList = iota + 1 // which ware, a page of them
	heroesStageBuy             // yes or no to one
)

// heroesAsk is his open question to a character: its box, its stage, the
// wares as the box listed them (a pick names what the box showed), the page,
// the ware a Yes buys, and which vendor asked.
type heroesAsk struct {
	box   uint32
	stage int
	wares []heroesWare
	page  int
	ware  heroesWare
	npc   entity.EntityID
}

// heroesKeeper keeps a standing at once (database.SQLiteCharacterStore).
type heroesKeeper interface {
	SetFaction(serverID uint32, faction string, standing int) error
}

// Heroes is the Testing Heroes Vendor's talk.
type Heroes struct {
	gw    *entity.World
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	items *database.ItemPatternStore
	chars heroesKeeper
	now   func() time.Time
	// give hands a bought item over (GiveItemNow); a test's stands in
	give func(sess *session.Session, pat database.ItemPatternRecord) (int, error)

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]heroesAsk // by the character's serverid
}

// NewHeroes is the vendor over the World, the database and the stores; nil
// without them (an NPC of his name then has nothing to say). chars keeps a
// buyer's standing at once (nil: at its logout alone).
func NewHeroes(gw *entity.World, db *database.SQLiteDB, inv *database.InventoryStore, chars heroesKeeper) *Heroes {
	if gw == nil || db == nil || inv == nil {
		return nil
	}
	h := &Heroes{gw: gw, db: db, inv: inv, items: database.NewItemPatternStore(db), chars: chars, now: time.Now,
		asked: map[uint32]heroesAsk{}}
	h.give = func(sess *session.Session, pat database.ItemPatternRecord) (int, error) {
		return GiveItemNow(sess, gw, inv, pat, 1)
	}
	gw.OnPlayerLeaving(func(p *entity.Player) {
		h.mu.Lock()
		delete(h.asked, p.CharServerID)
		h.mu.Unlock()
	})
	return h
}

// say is his line in a dialogue box (0x0046), cut where CLIENT's box ends.
func (h *Heroes) say(sess *session.Session, line string) {
	for _, part := range splitSay(HeroesVendorName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// ask opens a question under a new box, kept as the character's.
func (h *Heroes) ask(sess *session.Session, p *entity.Player, a heroesAsk, question string, answers []string) {
	h.mu.Lock()
	h.seq++
	a.box = heroesBoxMark | (h.seq & 0xFFFFFF)
	h.asked[p.CharServerID] = a
	h.mu.Unlock()
	for i := range answers {
		answers[i] = clipRunes(answers[i], questAnswerChars)
	}
	SendQuestPopupMulti(sess, a.box, clipRunes(question, questAskChars), answers)
}

// stock is what he sells now: the rows of heroes_stock whose records exist,
// the cheapest first.
func (h *Heroes) stock() ([]heroesWare, error) {
	rows, err := h.db.HeroesStock()
	if err != nil {
		return nil, err
	}
	out := make([]heroesWare, 0, len(rows))
	for _, r := range rows {
		pat, err := h.items.GetItemPattern(r.PatternID)
		if err != nil || pat == nil {
			log.Printf("Heroes: heroes_stock row %d sells record %d, which is no item (%v): not offered", r.ID, r.PatternID, err)
			continue
		}
		out = append(out, heroesWare{row: r, pat: *pat})
	}
	return out, nil
}

// Talk is a talk to npc: the vendor's greeting and his stock when npc is him
// and the Testing Heroes are on, else false.
func (h *Heroes) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if h == nil || p == nil || npc == nil || npc.Dead || !IsHeroesVendor(npc.Name) || !TestingHeroesOn() {
		return false
	}
	forgetDialogs(p)
	balance, standing, ok := h.gw.HeroesBalance(sess)
	if !ok {
		h.say(sess, heroesClosed)
		return true
	}
	wares, err := h.stock()
	if err != nil {
		log.Printf("Heroes: the stock: %v", err)
		h.say(sess, heroesClosed)
		return true
	}
	h.say(sess, fmt.Sprintf(heroesGreeting, p.Name, standing, balance))
	if len(wares) == 0 {
		h.say(sess, heroesEmpty)
		return true
	}
	h.list(sess, p, wares, 0, npc.ServerID, balance)
	log.Printf("Heroes: %s talked to the vendor: standing %d, %d to spend, %d wares", p.Name, standing, balance, len(wares))
	return true
}

// list asks which ware, a page of them.
func (h *Heroes) list(sess *session.Session, p *entity.Player, wares []heroesWare, page int, npc entity.EntityID, balance int) {
	shown, more := heroesPageOf(len(wares), page)
	answers := make([]string, 0, maxDialogOptions)
	for _, w := range wares[shown[0]:shown[1]] {
		answers = append(answers, w.label())
	}
	if more {
		answers = append(answers, heroesMore)
	}
	h.ask(sess, p, heroesAsk{stage: heroesStageList, wares: wares, page: page, npc: npc}, fmt.Sprintf(heroesAskList, balance), answers)
}

// heroesPageOf is the wares a page lists ([from, to)) and whether "More..."
// follows them: every ware in one box up to maxDialogOptions, else heroesPage
// a page, the last page's More going round to the first.
func heroesPageOf(n, page int) (shown [2]int, more bool) {
	if n <= maxDialogOptions {
		return [2]int{0, n}, false
	}
	pages := (n + heroesPage - 1) / heroesPage
	page = ((page % pages) + pages) % pages
	return [2]int{page * heroesPage, min((page+1)*heroesPage, n)}, true
}

// Answer is HandleDialogueOption's question for his box: a ware asked about, a
// purchase made. It reports whether the box was his (an old one, or a close,
// does nothing).
func (h *Heroes) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsHeroesBox(box) {
		return false
	}
	if h == nil || p == nil || option == 255 {
		// a close is not the last word: the PS2 sends it just before the
		// real pick when two boxes were open (interact.go)
		return true
	}
	h.mu.Lock()
	a, ok := h.asked[p.CharServerID]
	if ok && a.box == box {
		delete(h.asked, p.CharServerID)
	}
	h.mu.Unlock()
	if !ok || a.box != box || !TestingHeroesOn() {
		return true
	}
	if !h.near(p, a.npc) {
		h.say(sess, heroesTooFar)
		return true
	}
	balance, _, open := h.gw.HeroesBalance(sess)
	if !open {
		h.say(sess, heroesClosed)
		return true
	}
	n := int(option)
	switch a.stage {
	case heroesStageList:
		shown, more := heroesPageOf(len(a.wares), a.page)
		switch {
		case more && n == shown[1]-shown[0]:
			h.list(sess, p, a.wares, a.page+1, a.npc, balance)
		case n < shown[1]-shown[0]:
			h.offer(sess, p, a.wares[shown[0]+n], a.npc, balance)
		}
	case heroesStageBuy:
		if n == 0 {
			h.buy(sess, p, a.ware)
		}
	}
	return true
}

// near is whether p stands within heroesReach of the vendor that asked, in its
// world (the Teleporter's rule).
func (h *Heroes) near(p *entity.Player, id entity.EntityID) bool {
	world, x, _, z, ok := h.gw.NPCSpot(id)
	return ok && world == p.World && (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) <= heroesReach*heroesReach
}

// offer says what a ware is and asks whether to buy it, or says why it cannot
// be bought now.
func (h *Heroes) offer(sess *session.Session, p *entity.Player, w heroesWare, npc entity.EntityID, balance int) {
	if why := h.refusal(sess, w, balance); why != "" {
		h.say(sess, why)
		return
	}
	about := strings.TrimSpace(w.row.Says)
	if about == "" {
		about = strings.TrimSpace(w.pat.ItemDesc)
	}
	if w.pat.EquipSlot > 0 && cannotWear(p, &w.pat) != "" {
		about += heroesCantWear
	}
	if about = strings.TrimSpace(about); about != "" {
		h.say(sess, about)
	}
	name := strings.TrimSpace(w.pat.ItemName)
	h.ask(sess, p, heroesAsk{stage: heroesStageBuy, ware: w, npc: npc}, fmt.Sprintf(heroesAskBuy, name, w.row.Cost, balance),
		[]string{heroesYes, heroesNo})
}

// refusal is why the ware cannot be bought with balance to spend, in his
// words, "" when it can: the cost, a Lore item held already, the pack's room.
func (h *Heroes) refusal(sess *session.Session, w heroesWare, balance int) string {
	name := strings.TrimSpace(w.pat.ItemName)
	if balance < w.row.Cost {
		return fmt.Sprintf(heroesShort, name, w.row.Cost, balance, w.row.Cost-balance)
	}
	l := sessionInvList(sess)
	if l == nil {
		return heroesFailed
	}
	if w.pat.Lore != 0 && l.loreHeld(w.pat.PatternID) {
		return fmt.Sprintf(loreLine, name)
	}
	short := packShort(l, nil, []database.ItemCount{{PatternID: w.pat.PatternID, Count: 1}},
		func(id int) (database.ItemPatternRecord, bool) { return w.pat, id == w.pat.PatternID })
	if short != "" {
		return fmt.Sprintf(questPackFull, short)
	}
	return ""
}

// buy is a Yes to a ware: checked again as it stands now, paid, given, kept
// and logged.
func (h *Heroes) buy(sess *session.Session, p *entity.Player, asked heroesWare) {
	// the row as it is now: a price a GM changed since the list is asked again
	wares, err := h.stock()
	if err != nil {
		log.Printf("Heroes: the stock: %v", err)
		h.say(sess, heroesClosed)
		return
	}
	var w *heroesWare
	for i := range wares {
		if wares[i].row.ID == asked.row.ID {
			w = &wares[i]
		}
	}
	if w == nil || w.row.Cost != asked.row.Cost || w.row.PatternID != asked.row.PatternID {
		h.say(sess, heroesChanged)
		return
	}
	balance, _, open := h.gw.HeroesBalance(sess)
	if !open {
		h.say(sess, heroesClosed)
		return
	}
	name := strings.TrimSpace(w.pat.ItemName)
	if why := h.refusal(sess, *w, balance); why != "" {
		log.Printf("Heroes: %s may not buy %q (%d): %s", p.Name, name, w.row.Cost, why)
		h.say(sess, why)
		return
	}
	before, after, ok := h.gw.HeroesSpend(sess, w.row.Cost)
	if !ok {
		h.say(sess, fmt.Sprintf(heroesShort, name, w.row.Cost, before-entity.FactionMin, w.row.Cost-(before-entity.FactionMin)))
		return
	}
	if n, err := h.give(sess, w.pat); err != nil || n < 1 {
		back, _ := h.gw.HeroesRefund(sess, w.row.Cost)
		log.Printf("Heroes: %s paid %d for %q and it could not be given (%d given, %v): given back, standing %d", p.Name, w.row.Cost, name, n, err, back)
		h.keep(p, back)
		h.say(sess, heroesFailed)
		return
	}
	h.keep(p, after)
	at := h.now()
	log.Printf("Heroes: %s bought %q (record %d) for %d: %s %d -> %d", p.Name, name, w.pat.PatternID, w.row.Cost, entity.HeroesFaction, before, after)
	if err := h.db.LogHeroesPurchase(database.HeroesPurchase{At: at, ServerID: p.CharServerID, CharName: p.Name, AccountID: sess.AccountID(),
		PatternID: w.pat.PatternID, Item: name, Cost: w.row.Cost, StandingBefore: before, StandingAfter: after, Vendor: HeroesVendorName}); err != nil {
		log.Printf("Heroes: %s's purchase of %q not logged in heroes_purchases: %v", p.Name, name, err)
	}
	h.say(sess, fmt.Sprintf(heroesBought, name, after-entity.FactionMin))
}

// keep writes the buyer's standing at once.
func (h *Heroes) keep(p *entity.Player, standing int) {
	if h.chars == nil {
		return
	}
	if err := h.chars.SetFaction(p.CharServerID, entity.HeroesFaction, standing); err != nil {
		log.Printf("Heroes: %s's standing %d not kept: %v (its logout keeps it)", p.Name, standing, err)
	}
}
