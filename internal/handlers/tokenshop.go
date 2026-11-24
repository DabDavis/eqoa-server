package handlers

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// tokenshop.go — shops paid in an item, not in tunar: the Toy Shoppe first
// (the owner, 2026-11-16: "add Toy Shoppe close to the tree by Weapon
// Enchanter. The NPC will sell our toys like barrel of monkeys, snowgun, etc.
// We will make more like Santa's Elf illusion the gnome with red/green
// clothing etc."). Toymaker Tinsel, by Freeport's Christmas tree and its
// Weapon Enchanter, of the Christmas season (npcseason: gone with the tree),
// sells the toys for Holly Tokens (tools/toy_shoppe_20261116.py).
//
// Every NPC whose name has rows in table tokenshop (database/tokenshop.go) is
// a shop: a GM adds a ware, or a shop, with a row. CLIENT's merchant window
// shows a price in tunar and takes it from the purse, so a token shop is talk
// boxes, as the Testing Heroes Vendor's (heroes.go): a greeting that says what
// the player carries of the shop's token, a question listing the stock, each
// "<item>: <price>" (0x0034 under box mark 0x66000000; more than a box holds
// go a page at a time with "More...", dialogroom.go), a pick shown (an item
// link to a client that takes links, else its figures in words) and said of
// (its row's words, else its record's description), then Yes or No. Yes buys:
// the row is read again (a price changed since asks again), the pack must
// carry the price in the token (its bank's do not count), a Lore item must
// not be held already and the pack must have room; the item is given as a
// merchant's sale is (GiveItemNow), then the tokens taken (takeStacks: the
// oldest stacks first), and the purchase kept in tokenshop_purchases. A box
// is answered only within tokenShopReach of the shop that asked. The words
// are ours.
//
// A row's token is an item by name ("" the season's, DefaultEnchanterToken:
// Holly Token), so a shop may sell in several; the greeting counts the first
// ware's.
//
// A row's token "faction:<name>" is paid in standing with that faction, not in
// an item (Live petition 5575, a player, 2026-11-24: "The stroyteller Mercants
// should sale items for Storyteller faction"; the Storyteller's points were
// "redeemable for special rewards", the players' guide of 2010): what may be
// spent is the standing above factionShopFloor, a purchase takes its price off
// the standing (entity.SpendFaction, told as a faction's change is) before the
// item is given and gives it back if the item cannot be, and the standing is
// kept at once (charfactions, as the Testing Heroes Vendor's: an item given and
// a standing lost with a crash would be a free item). A faction table factions
// lacks closes the shop. The purchase is logged with its token as written.

// factionTokenPrefix starts a row's token paid in a faction's standing.
const factionTokenPrefix = "faction:"

// factionShopFloor is the standing a purchase in faction never goes below: what
// is spent is the points earned above it.
const factionShopFloor = 0

// factionOfToken is the faction a row's token names ("faction:<name>"), "" for
// an item's token.
func factionOfToken(token string) string {
	t := strings.TrimSpace(token)
	if len(t) > len(factionTokenPrefix) && strings.EqualFold(t[:len(factionTokenPrefix)], factionTokenPrefix) {
		return strings.TrimSpace(t[len(factionTokenPrefix):])
	}
	return ""
}

// tokenShopBoxMark marks a token shop's questions' ids.
const tokenShopBoxMark = 0x66000000

// IsTokenShopBox reports a box a token shop asked.
func IsTokenShopBox(id uint32) bool { return id&0xFF000000 == tokenShopBoxMark }

// tokenShopReach is how near the shop a box is answered.
const tokenShopReach = 30

// tokenShopsTTL is how long the shops' names are kept before table tokenshop
// is read again (a GM's new shop counts within it).
const tokenShopsTTL = 30 * time.Second

// The shops' words. A question is 127 characters at most (questAskChars), an
// answer 63 (questAnswerChars), a box 217 (questBoxChars: say cuts a longer one).
const (
	tokenShopGreeting  = "Welcome, %s! Everything here is paid in %s, and you carry %d."
	tokenShopAskList   = "%s: 'You carry %d %s. What will you have?'"
	tokenShopAskBuy    = "%s: '%s, for %s?'"
	tokenShopYes       = "Yes, I will take it"
	tokenShopNo        = "No, not now"
	tokenShopMore      = "More..."
	tokenShopClosed    = "My shop is closed today."
	tokenShopEmpty     = "My shelves are bare today. Come back soon!"
	tokenShopShort     = "%s costs %s, and you carry %d: %d more to earn."
	tokenShopNoToken   = "I take %s, and none can be had yet. Come back when the season brings them."
	tokenShopChanged   = "My prices have changed since I showed you. Ask me again."
	tokenShopTooFar    = "Come back to me to trade."
	tokenShopFailed    = "I could not hand it over: you keep your tokens. Try again."
	tokenShopBought    = "%s is yours! You have %s left."
	tokenShopCantWear  = " You could not wear it as you are."
	tokenShopShowLinks = "%s: %s, %s. Click its name to see it."
	tokenShopShowPlain = "%s: %s, %s: %s"
	// a shop paid in a faction's standing (factionTokenPrefix)
	factionShopGreeting = "Welcome, %s! I trade for standing with %s: you have %d to spend, and a purchase takes it off."
	factionShopAskList  = "%s: 'You have %d to spend with %s. What will you have?'"
	factionShopShort    = "%s costs %s, and you have %d to spend: %d more to earn."
	factionShopFailed   = "I could not hand it over: you keep your standing. Try again."
)

// tokenWare is something a shop sells: its stock row, its record and its
// token's record.
type tokenWare struct {
	row     database.TokenShopRow
	pat     database.ItemPatternRecord
	token   int    // the token's record (0: the database holds none, or a faction's)
	tname   string // the token's name (a faction's: the faction)
	faction string // the faction it is paid in ("": an item, token)
	marks   bool   // paid from the character's Heirloom Marks balance
}

// priceWords is a ware's price in words: "36 Holly Tokens", "125 faction".
func (w tokenWare) priceWords(n int) string {
	if w.faction != "" {
		return fmt.Sprintf("%d faction", n)
	}
	return reagentWords(w.tname, n)
}

// tokenName is a row's token by name ("" the season's).
func tokenName(t string) string {
	if t = strings.TrimSpace(t); t != "" {
		return t
	}
	return DefaultEnchanterToken
}

// label is the ware as an answer names it: "<item>: <price>", the name cut to
// leave the price in an answer's 63 characters.
func (w tokenWare) label() string {
	tail := fmt.Sprintf(": %d", w.row.Price)
	return clipRunes(strings.TrimSpace(w.pat.ItemName), questAnswerChars-len(tail)) + tail
}

// theItem is an item's name as a sentence begins with it: "The Barrel of
// Monkeys", but a name of someone's as it is ("Santa's Elf").
func theItem(name string) string {
	name = strings.TrimSpace(name)
	first, _, _ := strings.Cut(name, " ")
	if strings.HasSuffix(first, "'s") || strings.HasPrefix(strings.ToLower(name), "the ") {
		return name
	}
	return "The " + name
}

// The questions a shop asks.
const (
	tokenStageList = iota + 1 // which ware, a page of them
	tokenStageBuy             // yes or no to one
)

// tokenAsk is a shop's open question to a character.
type tokenAsk struct {
	box     uint32
	stage   int
	wares   []tokenWare
	page    int
	room    int
	ware    tokenWare
	npc     entity.EntityID
	speaker string
}

// TokenShop is the token shops' talk.
type TokenShop struct {
	gw    *entity.World
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	items *database.ItemPatternStore
	// links are the item links a ware is shown by (itemlinks.go); nil: its
	// figures in words alone
	links *ItemLinks
	now   func() time.Time
	// give hands a bought item over (GiveItemNow); held is what the pack
	// carries of a token, take takes n of it (takeStacks); a test's stand in
	give func(sess *session.Session, pat database.ItemPatternRecord) (int, error)
	held func(sess *session.Session, p *entity.Player, token int) ([]database.CharInventoryEntry, int, error)
	take func(sess *session.Session, p *entity.Player, held []database.CharInventoryEntry, n int)
	// near is whether the player stands by the shop that asked (a test's stands in)
	near func(p *entity.Player, id entity.EntityID) bool
	// chars keeps a standing spent at once (database.SQLiteCharacterStore; nil:
	// at its logout alone)
	chars heroesKeeper

	mu      sync.Mutex
	seq     uint32
	asked   map[uint32]tokenAsk // by the character's serverid
	shops   map[string]bool
	shopsAt time.Time
}

// NewTokenShop is the shops over the World, the database and the stores; nil
// without them (no NPC is then a shop).
func NewTokenShop(gw *entity.World, db *database.SQLiteDB, inv *database.InventoryStore) *TokenShop {
	if gw == nil || db == nil || inv == nil {
		return nil
	}
	t := &TokenShop{gw: gw, db: db, inv: inv, items: database.NewItemPatternStore(db), now: time.Now,
		asked: map[uint32]tokenAsk{}}
	t.give = func(sess *session.Session, pat database.ItemPatternRecord) (int, error) {
		return GiveItemNow(sess, gw, inv, pat, 1)
	}
	t.held = func(_ *session.Session, p *entity.Player, token int) ([]database.CharInventoryEntry, int, error) {
		return packStacksOf(inv, p, token)
	}
	t.take = func(sess *session.Session, p *entity.Player, held []database.CharInventoryEntry, n int) {
		takeStacks(inv, sess, p, "TokenShop", held, n)
	}
	t.near = func(p *entity.Player, id entity.EntityID) bool {
		world, x, _, z, ok := gw.NPCSpot(id)
		return ok && world == p.World && (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) <= tokenShopReach*tokenShopReach
	}
	gw.OnPlayerLeaving(func(p *entity.Player) {
		t.mu.Lock()
		delete(t.asked, p.CharServerID)
		t.mu.Unlock()
	})
	return t
}

// packStacksOf is the stacks of an item the player's pack carries (worn
// nowhere; its bank's do not count), oldest first, and how many in all.
func packStacksOf(inv *database.InventoryStore, p *entity.Player, pattern int) ([]database.CharInventoryEntry, int, error) {
	items, err := inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, 0, err
	}
	var held []database.CharInventoryEntry
	have := 0
	for _, it := range items {
		if it.Pattern.PatternID == pattern && it.Location == database.LocPack && it.EquipLoc < 0 && it.StackLeft > 0 {
			held = append(held, it)
			have += it.StackLeft
		}
	}
	return held, have, nil
}

// IsShop is whether an NPC of the name is a token shop (table tokenshop has
// a row offered for it), the names kept tokenShopsTTL.
func (t *TokenShop) IsShop(name string) bool {
	if t == nil {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.shops == nil || t.now().Sub(t.shopsAt) >= tokenShopsTTL {
		shops, err := t.db.TokenShops()
		if err != nil {
			log.Printf("TokenShop: the shops: %v", err)
			return t.shops[key]
		}
		t.shops, t.shopsAt = shops, t.now()
	}
	return t.shops[key]
}

func (t *TokenShop) say(sess *session.Session, speaker, line string) {
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (t *TokenShop) ask(sess *session.Session, p *entity.Player, a tokenAsk, question string, answers []string) {
	t.mu.Lock()
	t.seq++
	a.box = tokenShopBoxMark | (t.seq & 0xFFFFFF)
	t.asked[p.CharServerID] = a
	t.mu.Unlock()
	for i := range answers {
		answers[i] = clipRunes(answers[i], questAnswerChars)
	}
	SendQuestPopupMulti(sess, a.box, clipRunes(question, questAskChars), answers)
}

// tokenOf is a token's record by its name (0: the database holds none), the
// enchanter's way: the first record of the name.
func (t *TokenShop) tokenOf(name string, cache map[string]int) int {
	if id, ok := cache[name]; ok {
		return id
	}
	var id int
	if err := t.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE ORDER BY patternid LIMIT 1`, name).Scan(&id); err != nil {
		id = 0
	}
	cache[name] = id
	return id
}

// stock is what the shop of the name sells now: its rows whose records exist.
func (t *TokenShop) stock(shop string) ([]tokenWare, error) {
	rows, err := t.db.TokenShopStock(shop)
	if err != nil {
		return nil, err
	}
	tokens := map[string]int{}
	out := make([]tokenWare, 0, len(rows))
	for _, r := range rows {
		pat, err := t.items.GetItemPattern(r.PatternID)
		if err != nil || pat == nil {
			log.Printf("TokenShop: tokenshop row %d (%s) sells record %d, which is no item (%v): not offered", r.ID, r.Shop, r.PatternID, err)
			continue
		}
		if strings.EqualFold(strings.TrimSpace(r.Token), database.HeirloomMarksShopToken) {
			if HeirloomMarksOn() && pat.MaxStack == 1 {
				out = append(out, tokenWare{row: r, pat: *pat, tname: "Heirloom Mark", marks: true})
			}
			continue
		}
		if f := factionOfToken(r.Token); f != "" {
			out = append(out, tokenWare{row: r, pat: *pat, tname: f, faction: f})
			continue
		}
		name := tokenName(r.Token)
		out = append(out, tokenWare{row: r, pat: *pat, token: t.tokenOf(name, tokens), tname: name})
	}
	return out, nil
}

// Talk is a talk to npc: its greeting and its stock when npc is a token shop,
// else false.
func (t *TokenShop) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if t == nil || p == nil || npc == nil || npc.Dead || !t.IsShop(npc.Name) {
		return false
	}
	speaker := strings.TrimSpace(npc.Name)
	forgetDialogs(p)
	wares, err := t.stock(speaker)
	if err != nil {
		log.Printf("TokenShop: %s's stock: %v", speaker, err)
		t.say(sess, speaker, tokenShopClosed)
		return true
	}
	if len(wares) == 0 {
		t.say(sess, speaker, tokenShopEmpty)
		return true
	}
	first := wares[0]
	if first.faction != "" && !t.gw.FactionKnown(first.faction) {
		log.Printf("TokenShop: %s sells for %q, which table factions lacks: closed", speaker, first.faction)
		t.say(sess, speaker, tokenShopClosed)
		return true
	}
	have := t.have(sess, p, first)
	if first.faction != "" {
		t.say(sess, speaker, fmt.Sprintf(factionShopGreeting, p.Name, first.faction, have))
	} else {
		t.say(sess, speaker, fmt.Sprintf(tokenShopGreeting, p.Name, tokensWord(first.tname), have))
	}
	t.list(sess, p, wares, 0, npc.ServerID, speaker, have, first)
	log.Printf("TokenShop: %s talked to %s: %d %s carried, %d wares", p.Name, speaker, have, first.tname, len(wares))
	return true
}

// have is what the player has to pay a ware's price with: the token's count
// in its pack, or its standing above factionShopFloor with the ware's faction.
func (t *TokenShop) have(sess *session.Session, p *entity.Player, w tokenWare) int {
	if w.marks {
		row, err := t.db.CharMarksOf(p.CharServerID)
		if err != nil {
			log.Printf("TokenShop: %s's marks: %v", p.Name, err)
			return 0
		}
		return row.Marks
	}
	if w.faction != "" {
		s, ok := t.gw.Standing(sess, w.faction)
		if !ok {
			return 0
		}
		return max(s-factionShopFloor, 0)
	}
	if w.token == 0 {
		return 0
	}
	_, n, err := t.held(sess, p, w.token)
	if err != nil {
		return 0
	}
	return n
}

// tokensWord is a token's name made plural ("Holly Tokens"), as reagentWords does.
func tokensWord(name string) string {
	w := reagentWords(name, 2)
	return strings.TrimPrefix(w, "2 ")
}

// list asks which ware, a page of them.
func (t *TokenShop) list(sess *session.Session, p *entity.Player, wares []tokenWare, page int, npc entity.EntityID, speaker string, have int, first tokenWare) {
	room := dialogRoom(sess)
	shown, more := heroesPageOf(len(wares), page, room)
	answers := make([]string, 0, room)
	for _, w := range wares[shown[0]:shown[1]] {
		answers = append(answers, w.label())
	}
	if more {
		answers = append(answers, tokenShopMore)
	}
	question := fmt.Sprintf(tokenShopAskList, speaker, have, tokensWord(first.tname))
	if first.faction != "" {
		question = fmt.Sprintf(factionShopAskList, speaker, have, first.faction)
	}
	t.ask(sess, p, tokenAsk{stage: tokenStageList, wares: wares, page: page, room: room, npc: npc, speaker: speaker},
		question, answers)
}

// Answer is HandleDialogueOption's question for a shop's box: a ware asked
// about, a purchase made. It reports whether the box was a shop's (an old one,
// or a close, does nothing).
func (t *TokenShop) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsTokenShopBox(box) {
		return false
	}
	if t == nil || p == nil || option == 255 {
		return true // a close (the PS2 sends one before the real pick when two boxes were open)
	}
	t.mu.Lock()
	a, ok := t.asked[p.CharServerID]
	if ok && a.box == box {
		delete(t.asked, p.CharServerID)
	}
	t.mu.Unlock()
	if !ok || a.box != box {
		return true
	}
	if !t.near(p, a.npc) {
		t.say(sess, a.speaker, tokenShopTooFar)
		return true
	}
	n := int(option)
	switch a.stage {
	case tokenStageList:
		shown, more := heroesPageOf(len(a.wares), a.page, max(a.room, maxDialogOptions))
		switch {
		case more && n == shown[1]-shown[0]:
			t.list(sess, p, a.wares, a.page+1, a.npc, a.speaker, t.have(sess, p, a.wares[0]), a.wares[0])
		case n < shown[1]-shown[0]:
			t.offer(sess, p, a.wares[shown[0]+n], a.npc, a.speaker)
		}
	case tokenStageBuy:
		if n == 0 {
			t.buy(sess, p, a.ware, a.speaker)
		}
	}
	return true
}

// offer shows a ware and asks whether to buy it, or says why it cannot be
// bought now.
func (t *TokenShop) offer(sess *session.Session, p *entity.Player, w tokenWare, npc entity.EntityID, speaker string) {
	t.show(sess, w, speaker)
	if why, _, _ := t.refusal(sess, p, w); why != "" {
		t.say(sess, speaker, why)
		return
	}
	about := strings.TrimSpace(w.row.Says)
	if about == "" {
		about = strings.TrimSpace(w.pat.ItemDesc)
	}
	if w.pat.EquipSlot > 0 && cannotWear(p, &w.pat) != "" {
		about += tokenShopCantWear
	}
	if about = strings.TrimSpace(about); about != "" {
		t.say(sess, speaker, about)
	}
	t.ask(sess, p, tokenAsk{stage: tokenStageBuy, ware: w, npc: npc, speaker: speaker},
		fmt.Sprintf(tokenShopAskBuy, speaker, theItem(w.pat.ItemName), w.priceWords(w.row.Price)),
		[]string{tokenShopYes, tokenShopNo})
}

// show is a ware picked: its name as an item link to a client that takes
// links, else its figures in words (heroesStats).
func (t *TokenShop) show(sess *session.Session, w tokenWare, speaker string) {
	name := strings.TrimSpace(w.pat.ItemName)
	price := w.priceWords(w.row.Price)
	if t.links != nil && sess.ItemLinks() && name != "" {
		rec := wireRecord(database.CharInventoryEntry{StackLeft: 1, RemainHP: w.pat.MaxHP, Pattern: w.pat})
		linkedLine(t.links, speaker, tokenShopShowLinks, speaker, lootItem(rec, 1), price).client(sess)
		return
	}
	SendClientMessage(sess, fmt.Sprintf(tokenShopShowPlain, speaker, name, price, heroesStats(&w.pat)))
}

// refusal is why the ware cannot be bought, in the shop's words, "" when it
// can, with the token's stacks the pack carries and how many: the token
// itself, the price, a Lore item held already, the pack's room.
func (t *TokenShop) refusal(sess *session.Session, p *entity.Player, w tokenWare) (string, []database.CharInventoryEntry, int) {
	name := strings.TrimSpace(w.pat.ItemName)
	var held []database.CharInventoryEntry
	var have int
	if w.marks {
		if !HeirloomMarksOn() {
			return tokenShopClosed, nil, 0
		}
		if have = t.have(sess, p, w); have < w.row.Price {
			return fmt.Sprintf(tokenShopShort, theItem(name), w.priceWords(w.row.Price), have, w.row.Price-have), nil, have
		}
	} else if w.faction != "" {
		if !t.gw.FactionKnown(w.faction) {
			return tokenShopClosed, nil, 0
		}
		if have = t.have(sess, p, w); have < w.row.Price {
			return fmt.Sprintf(factionShopShort, theItem(name), w.priceWords(w.row.Price), have, w.row.Price-have), nil, have
		}
	} else {
		if w.token == 0 {
			return fmt.Sprintf(tokenShopNoToken, tokensWord(w.tname)), nil, 0
		}
		var err error
		held, have, err = t.held(sess, p, w.token)
		if err != nil {
			log.Printf("TokenShop: %s's pack: %v", p.Name, err)
			return tokenShopFailed, nil, 0
		}
		if have < w.row.Price {
			return fmt.Sprintf(tokenShopShort, theItem(name), reagentWords(w.tname, w.row.Price), have, w.row.Price-have), held, have
		}
	}
	l := sessionInvList(sess)
	if l == nil {
		return tokenShopFailed, held, have
	}
	if w.pat.Lore != 0 && l.loreHeld(w.pat.PatternID) {
		return fmt.Sprintf(loreLine, name), held, have
	}
	short := packShort(l, nil, []database.ItemCount{{PatternID: w.pat.PatternID, Count: 1}},
		func(id int) (database.ItemPatternRecord, bool) { return w.pat, id == w.pat.PatternID })
	if short != "" {
		return fmt.Sprintf(questPackFull, short), held, have
	}
	return "", held, have
}

// buy is a Yes to a ware: checked again as it stands now, given, paid and
// logged.
func (t *TokenShop) buy(sess *session.Session, p *entity.Player, asked tokenWare, speaker string) {
	wares, err := t.stock(speaker)
	if err != nil {
		log.Printf("TokenShop: %s's stock: %v", speaker, err)
		t.say(sess, speaker, tokenShopClosed)
		return
	}
	var w *tokenWare
	for i := range wares {
		if wares[i].row.ID == asked.row.ID {
			w = &wares[i]
		}
	}
	if w == nil || w.row.Price != asked.row.Price || w.row.PatternID != asked.row.PatternID || w.token != asked.token || w.faction != asked.faction || w.marks != asked.marks {
		t.say(sess, speaker, tokenShopChanged)
		return
	}
	name := strings.TrimSpace(w.pat.ItemName)
	why, held, have := t.refusal(sess, p, *w)
	if why != "" {
		log.Printf("TokenShop: %s may not buy %q from %s (%d %s): %s", p.Name, name, speaker, w.row.Price, w.tname, why)
		t.say(sess, speaker, why)
		return
	}
	if w.marks {
		t.buyForMarks(sess, p, *w, speaker)
		return
	}
	if w.faction != "" {
		t.buyForFaction(sess, p, *w, speaker)
		return
	}
	if n, err := t.give(sess, w.pat); err != nil || n < 1 {
		log.Printf("TokenShop: %s's %q from %s could not be given (%d given, %v): nothing taken", p.Name, name, speaker, n, err)
		t.say(sess, speaker, tokenShopFailed)
		return
	}
	t.take(sess, p, held, w.row.Price)
	log.Printf("TokenShop: %s bought %q (record %d) from %s for %d %s (%d carried)", p.Name, name, w.pat.PatternID, speaker, w.row.Price, w.tname, have)
	if err := t.db.LogTokenShopPurchase(database.TokenShopPurchase{At: t.now(), ServerID: p.CharServerID, CharName: p.Name,
		AccountID: sess.AccountID(), Shop: speaker, PatternID: w.pat.PatternID, Item: name, Price: w.row.Price, Token: w.tname}); err != nil {
		log.Printf("TokenShop: %s's purchase of %q not logged in tokenshop_purchases: %v", p.Name, name, err)
	}
	t.say(sess, speaker, fmt.Sprintf(tokenShopBought, theItem(name), reagentWords(w.tname, have-w.row.Price)))
}

func (t *TokenShop) buyForMarks(sess *session.Session, p *entity.Player, w tokenWare, speaker string) {
	item, left, err := t.inv.BuyChargedMarkShopItem(w.row.ID, database.TokenShopPurchase{
		At: t.now(), ServerID: p.CharServerID, CharName: p.Name, AccountID: sess.AccountID(),
		Shop: w.row.Shop, PatternID: w.pat.PatternID, Item: w.pat.ItemName, Price: w.row.Price, Token: w.row.Token,
	})
	name := strings.TrimSpace(w.pat.ItemName)
	switch {
	case errors.Is(err, database.ErrMarkShopStockChanged):
		t.say(sess, speaker, tokenShopChanged)
	case errors.Is(err, database.ErrNotEnoughMarks):
		t.say(sess, speaker, fmt.Sprintf(tokenShopShort, theItem(name), w.priceWords(w.row.Price), left, w.row.Price-left))
	case err != nil:
		log.Printf("TokenShop: %s's purchase of %q for marks: %v", p.Name, name, err)
		t.say(sess, speaker, "I could not hand it over. You keep your Heirloom Marks.")
	default:
		item.Pattern = w.pat
		key, _ := sessionInvList(sess).add(item.ItemID, w.pat.PatternID, item.StackLeft, w.pat.MaxStack)
		out := wireRecord(item)
		queueReliable(sess, protocol.OpcodeAddInvItem, addInvItemPayload(&out, key, readsCapes(sess)))
		t.say(sess, speaker, fmt.Sprintf(tokenShopBought, theItem(name), reagentWords(w.tname, left)))
	}
}

// buyForFaction is a Yes to a ware paid in standing, its refusal checked: the
// price spent, the item given (the price given back if it cannot be), the
// standing kept at once and the purchase logged.
func (t *TokenShop) buyForFaction(sess *session.Session, p *entity.Player, w tokenWare, speaker string) {
	name := strings.TrimSpace(w.pat.ItemName)
	before, after, ok := t.gw.SpendFaction(sess, w.faction, factionShopFloor, w.row.Price)
	if !ok {
		have := max(before-factionShopFloor, 0)
		t.say(sess, speaker, fmt.Sprintf(factionShopShort, theItem(name), w.priceWords(w.row.Price), have, max(w.row.Price-have, 0)))
		return
	}
	if n, err := t.give(sess, w.pat); err != nil || n < 1 {
		back, _ := t.gw.RefundFaction(sess, w.faction, w.row.Price)
		log.Printf("TokenShop: %s paid %d %s for %q from %s and it could not be given (%d given, %v): given back, standing %d",
			p.Name, w.row.Price, w.faction, name, speaker, n, err, back)
		t.keep(p, w.faction, back)
		t.say(sess, speaker, factionShopFailed)
		return
	}
	t.keep(p, w.faction, after)
	log.Printf("TokenShop: %s bought %q (record %d) from %s for %d: %s %d -> %d", p.Name, name, w.pat.PatternID, speaker, w.row.Price, w.faction, before, after)
	if err := t.db.LogTokenShopPurchase(database.TokenShopPurchase{At: t.now(), ServerID: p.CharServerID, CharName: p.Name,
		AccountID: sess.AccountID(), Shop: speaker, PatternID: w.pat.PatternID, Item: name, Price: w.row.Price, Token: factionTokenPrefix + w.faction}); err != nil {
		log.Printf("TokenShop: %s's purchase of %q not logged in tokenshop_purchases: %v", p.Name, name, err)
	}
	t.say(sess, speaker, fmt.Sprintf(tokenShopBought, theItem(name), fmt.Sprintf("%d to spend", max(after-factionShopFloor, 0))))
}

// keep writes a standing spent at once.
func (t *TokenShop) keep(p *entity.Player, faction string, standing int) {
	if t.chars == nil {
		return
	}
	if err := t.chars.SetFaction(p.CharServerID, faction, standing); err != nil {
		log.Printf("TokenShop: %s's standing %d with %s not kept: %v (its logout keeps it)", p.Name, standing, faction, err)
	}
}
