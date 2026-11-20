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

// enchanter.go — the Weapon Enchanters (the owner, 2026-11-16: "lets add an
// weapon enchanter next to equipment dyer. will stack up to 2 auras on either
// weapon, tags it with the aura tags"; "the weapon enchanter will cost
// christmas currency for our christmas event"). Any NPC named "Weapon
// Enchanter" is one (tools/place_weapon_enchanters_20261116.py stands one
// beside each Equipment Dyer). Talked to, it asks which held weapon (the copy
// in the main hand or the off hand), then which of its two auras (or both
// off), then an aura of its catalog (enchanterAuras) and a colour (its own,
// or one of the dyers' families and shades: database.DyeFamilies), and writes
// that COPY's own aura (table instanceaura, database/instanceaura.go): it
// shows on whoever holds that copy, through trades, the bank and the auction
// house, and the copy is tagged with its auras' names ("Aura: Virtuous,
// Twinkling"), which every client shows with the item, a PS2 too. The auras
// themselves are drawn by vanilla alone (its first aura by a vanilla that
// draws hand auras, its second by one that draws second auras); a PS2 draws
// none, and the enchanter says so.
//
// The price (world.enchanter_price, default 1) is that many of the season's
// token (world.enchanter_token, default "Holly Token", found by name, else
// world.enchanter_token_id) for each aura put on, from the pack; taking an
// aura off is free. The Enchanter is always open, the season or not: tokens
// kept after the season can still be spent (a token record the database does
// not hold yet: it says it cannot enchant until the token exists, unless the
// price is 0, free). Its questions go under box mark 0x65000000 (0x56 is the
// dyers').

// enchanterBoxMark marks an enchanter's question's id.
const enchanterBoxMark = 0x65000000

// IsEnchanterBox reports a box an enchanter asked.
func IsEnchanterBox(id uint32) bool { return id&0xFF000000 == enchanterBoxMark }

// WeaponEnchanterName is the enchanters' name.
const WeaponEnchanterName = "Weapon Enchanter"

// IsWeaponEnchanter is whether an NPC of the name is an enchanter.
func IsWeaponEnchanter(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), WeaponEnchanterName)
}

// enchanterAura is one aura of the catalog: its tag word, the aura (a
// colour 0: the source's own), whether a colour may be chosen, and its line.
type enchanterAura struct {
	Tag     string
	A       database.ItemAura
	Colours bool
	Line    string
}

// enchanterAuras is the catalog: the named looks of /aura (gmaura.go) and the
// auras GMs gave tonight, each with its tag.
var enchanterAuras = []enchanterAura{
	{"Virtuous", database.ItemAura{Kind: database.AuraEpic, Source: 0x88EC092D}, true, "the Sword of Virtue's glow, sheen and sparkles"},
	{"Freezing", database.ItemAura{Kind: database.AuraParticles, Source: 0x6C151A60}, true, "ice dripping off the blade"},
	{"Tainted", database.ItemAura{Kind: database.AuraEpic, Source: 0x5FD4AE7C}, true, "the Sickle of Avhi Escron's olive glow and green mist"},
	{"Twinkling", database.ItemAura{Kind: database.AuraParticles, Source: 0x1D875ECD, Nodes: 100}, true, "a star's twinkle at the tip"},
	{"Druid Glow", database.ItemAura{Kind: database.AuraEpic, Source: 0xCBF4135A, Nodes: 45}, true, "the Oak Staff of Eilerawdl's amber glow and sparkles"},
	{"Shadowed", database.ItemAura{Kind: database.AuraParticles, Color: 0x000000FF}, false, "a dark, smoky shadow"},
	{"Energized", database.ItemAura{Kind: database.AuraEnergy, Source: 0x2EC56029}, true, "an energy skin streaming over it"},
}

// enchanterNoAura is the catalog's last answer: the aura chosen taken off.
const enchanterNoAura = "No aura (take it off)"

// The enchanter's settings (world.enchanter_*; SetEnchanterPrice).
var (
	enchanterPrice   atomic.Int64
	enchanterTokenID atomic.Int64
	enchanterToken   atomic.Pointer[string]
)

// DefaultEnchanterToken is the season's token by name (the Christmas event's).
const DefaultEnchanterToken = "Holly Token"

func init() {
	enchanterPrice.Store(1)
	t := DefaultEnchanterToken
	enchanterToken.Store(&t)
}

// SetEnchanterPrice sets the price of one aura: n of the token named (""
// the default), else of record id (0 none); n 0 free, below 0 the default 1.
func SetEnchanterPrice(n int, token string, id int) {
	if n < 0 {
		n = 1
	}
	if strings.TrimSpace(token) == "" {
		token = DefaultEnchanterToken
	}
	token = strings.TrimSpace(token)
	enchanterPrice.Store(int64(n))
	enchanterToken.Store(&token)
	enchanterTokenID.Store(int64(id))
}

// The enchanter's questions, in the order they come.
const (
	encHand   = iota // which held weapon
	encWhich         // which of its two auras, or both off
	encAura          // enchanterAuras (paged)
	encFamily        // its own colour, or a family of colours (paged)
	encShade         // a family's shades
	encTry           // its try-on: buy it or not
)

// enchantTryOn is how long a try-on shows before its question is answered.
const enchantTryOn = 30 * time.Second

// The try-on's answers (encTry).
const (
	enchantBuy   = "Buy it"
	enchantNoBuy = "No, take it off"
)

// enchanterAsk is a question asked.
type enchanterAsk struct {
	id      uint32
	stage   int
	hand    int // entity.AuraMainHand, AuraOffHand
	itemID  int // the copy, as it was held when asked
	second  bool
	aura    int    // enchanterAuras' index
	colour  uint32 // the try-on's colour (encTry)
	page    int
	family  int
	speaker string
	npc     *entity.NPC // the enchanter asked: the colour picker's anchor (dyepicker.go)
	picker  bool        // its colours' question ended with "Pick any colour..."
}

// Enchanter keeps each character's question in hand.
type Enchanter struct {
	gw    *entity.World
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]enchanterAsk
}

// NewEnchanter makes the enchanters over the database.
func NewEnchanter(gw *entity.World, db *database.SQLiteDB) *Enchanter {
	e := &Enchanter{gw: gw, db: db, asked: map[uint32]enchanterAsk{}}
	if db != nil {
		e.inv = database.NewInventoryStore(db)
	}
	return e
}

func (e *Enchanter) say(sess *session.Session, speaker, line string) {
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (e *Enchanter) ask(sess *session.Session, p *entity.Player, a enchanterAsk, question string, answers []string) {
	e.mu.Lock()
	e.seq++
	a.id = enchanterBoxMark | (e.seq & 0xFFFFFF)
	e.asked[p.CharServerID] = a
	e.mu.Unlock()
	SendQuestPopupMulti(sess, a.id, clipRunes(question, questAskChars), answers)
}

// enchantPage is page of all in a box of room answers: room-1 of them and
// "More" while more are left (the last page's "More" goes back to the first).
func enchantPage(all []string, page, room int) (shown []string, first int, paged bool) {
	if room < 2 || len(all) <= room {
		return all, 0, false
	}
	per := room - 1
	pages := (len(all) + per - 1) / per
	page %= pages
	first = page * per
	end := min(first+per, len(all))
	return append(append([]string(nil), all[first:end]...), "More..."), first, true
}

// Talk is an enchanter talked to. It reports whether the NPC was one.
func (e *Enchanter) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if e == nil || p == nil || npc == nil || npc.Dead || !IsWeaponEnchanter(npc.Name) {
		return false
	}
	speaker := strings.TrimSpace(npc.Name)
	forgetDialogs(p)
	closeDyePicker(sess, p)
	if e.db == nil {
		e.say(sess, speaker, "My runes are cold today.")
		return true
	}
	price := e.priceWords()
	e.say(sess, speaker, "I lay up to two auras on a weapon you hold, and the weapon carries their names wherever it goes. "+
		price+" Only the newest game client draws an aura (the second needs the very newest); a PlayStation 2 shows the names alone.")
	e.askHand(sess, p, speaker, npc)
	return true
}

// priceWords says the price.
func (e *Enchanter) priceWords() string {
	n := enchanterPrice.Load()
	if n == 0 {
		return "My work is free."
	}
	return fmt.Sprintf("Each aura costs %s; taking one off is free.", reagentWords(*enchanterToken.Load(), int(n)))
}

// held is the copies the player holds, by hand.
func heldCopies(p *entity.Player) (hands []int, ids []int) {
	for _, h := range []int{entity.AuraMainHand, entity.AuraOffHand} {
		if p.WornMask&(1<<h) != 0 && p.WornItems[h] > 0 {
			hands, ids = append(hands, h), append(ids, p.WornItems[h])
		}
	}
	return hands, ids
}

func (e *Enchanter) itemName(p *entity.Player, id int) string {
	if e.inv == nil {
		return "your weapon"
	}
	r, err := e.inv.GetCharItem(p.CharServerID, id)
	if err != nil {
		return "your weapon"
	}
	return r.Pattern.ItemName
}

func (e *Enchanter) askHand(sess *session.Session, p *entity.Player, speaker string, npc *entity.NPC) {
	hands, ids := heldCopies(p)
	switch len(hands) {
	case 0:
		e.say(sess, speaker, "Hold the weapon you want enchanted, in either hand, and speak to me again.")
		return
	case 1:
		e.askWhich(sess, p, enchanterAsk{speaker: speaker, hand: hands[0], itemID: ids[0], npc: npc})
		return
	}
	answers := make([]string, len(hands))
	for i, h := range hands {
		answers[i] = clipRunes(map[int]string{entity.AuraMainHand: "Main hand: ", entity.AuraOffHand: "Off hand: "}[h]+e.itemName(p, ids[i]), 40)
	}
	e.ask(sess, p, enchanterAsk{speaker: speaker, stage: encHand, npc: npc}, "Which weapon shall I enchant?", answers)
}

// lendsNothing is whether aura i of the catalog would change nothing on a held
// item of model: one whose aura's source is that very look (Heirloom petition
// 2472: Energized, the old energy shield's skin, bought for the Tower of Woe,
// which IS the old energy shield: vanilla lends a look or particles from a
// source only to an item that is not it, aura.rs's lend_hand and
// hand_emitters).
func lendsNothing(i int, model int64) bool {
	if i < 0 || i >= len(enchanterAuras) {
		return false
	}
	src := enchanterAuras[i].A.Source
	return src != 0 && uint32(model) == src
}

// heldLook is the model of the copy itemID the player holds (0: unknown).
func (e *Enchanter) heldLook(p *entity.Player, itemID int) int64 {
	if e.inv == nil {
		return 0
	}
	r, err := e.inv.GetCharItem(p.CharServerID, itemID)
	if err != nil {
		return 0
	}
	return r.Pattern.Model
}

// sameLookLine is what the enchanter says of an aura that would lend nothing.
func sameLookLine(i int, item string) string {
	return fmt.Sprintf("Your %s is the very look %s comes from, so it would not show on it. Choose another aura.",
		item, enchanterAuras[i].Tag)
}

// auraName is a copy's aura by its catalog name, "none", or "an aura" (a GM's).
func auraName(a database.ItemAura) string {
	if !a.SecondShows() {
		return "none"
	}
	if i := enchanterAuraOf(a); i >= 0 {
		return enchanterAuras[i].Tag
	}
	return "a GM's aura"
}

// enchanterAuraOf is the catalog's aura a copy's aura is (its kind and
// source; the colour and along are a player's to choose), -1 none.
func enchanterAuraOf(a database.ItemAura) int {
	for i, c := range enchanterAuras {
		if c.A.Kind == a.Kind && c.A.Source == a.Source && (c.Colours || c.A.Color == a.Color) {
			return i
		}
	}
	return -1
}

func (e *Enchanter) askWhich(sess *session.Session, p *entity.Player, a enchanterAsk) {
	row, _, err := e.db.InstanceAuraOf(a.itemID)
	if err != nil {
		log.Printf("Enchanter: %s's item %d: %v", p.Name, a.itemID, err)
		e.say(sess, a.speaker, "Something went wrong with my runes. Try again.")
		return
	}
	a.stage = encWhich
	e.ask(sess, p, a, fmt.Sprintf("Which aura of the %s?", e.itemName(p, a.itemID)), []string{
		"First aura: " + auraName(row.First), "Second aura: " + auraName(row.Second), "Take both auras off"})
}

func auraAnswers() []string {
	out := make([]string, 0, len(enchanterAuras)+1)
	for _, c := range enchanterAuras {
		out = append(out, c.Tag)
	}
	return append(out, enchanterNoAura)
}

// colourAnswers is the colours' question's answers: its own and the dyers'
// families; for a client with the picker (dyepicker.go) its own and "Pick any
// colour..." alone, the palette left to the picker (the owner, 2026-11-18:
// "lets remove the color presets from the dyer simplify with the rgb picker").
func colourAnswers(picker bool) []string {
	out := []string{"Its own colours"}
	if picker {
		return append(out, DyePickLabel)
	}
	for _, f := range database.DyeFamilies {
		out = append(out, f.Label)
	}
	if picker {
		out = append(out, DyePickLabel)
	}
	return out
}

// Answer is HandleDialogueOption's question for an enchanter's box. It
// reports whether the box was an enchanter's (an old one does nothing).
func (e *Enchanter) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsEnchanterBox(box) {
		return false
	}
	if e == nil || p == nil {
		return true
	}
	if option == 255 { // a close: a try-on asked is taken off
		e.mu.Lock()
		a, ok := e.asked[p.CharServerID]
		if ok && a.id == box {
			delete(e.asked, p.CharServerID)
		}
		e.mu.Unlock()
		if ok && a.id == box && a.stage == encTry {
			database.EndInstanceAuraPreview(a.itemID)
		}
		return true
	}
	e.mu.Lock()
	a, ok := e.asked[p.CharServerID]
	if ok && a.id == box {
		delete(e.asked, p.CharServerID)
	}
	e.mu.Unlock()
	if !ok || a.id != box {
		return true
	}
	n := int(option)
	room := dialogRoom(sess)
	switch a.stage {
	case encHand:
		hands, ids := heldCopies(p)
		if n < len(hands) {
			e.askWhich(sess, p, enchanterAsk{speaker: a.speaker, hand: hands[n], itemID: ids[n], npc: a.npc})
		}
	case encWhich:
		switch n {
		case 0, 1:
			a.second, a.stage, a.page = n == 1, encAura, 0
			shown, _, _ := enchantPage(auraAnswers(), 0, room)
			e.ask(sess, p, a, "Which aura?", shown)
		case 2:
			e.enchant(sess, p, a, -1, 0, true)
		}
	case encAura:
		all := auraAnswers()
		shown, first, paged := enchantPage(all, a.page, room)
		if paged && n == len(shown)-1 {
			a.page++
			shown, _, _ = enchantPage(all, a.page, room)
			e.ask(sess, p, a, "Which aura?", shown)
			return true
		}
		i := first + n
		switch {
		case n >= len(shown) || i >= len(all):
		case i == len(enchanterAuras):
			e.enchant(sess, p, a, -1, 0, false) // this aura off
		case lendsNothing(i, e.heldLook(p, a.itemID)):
			e.say(sess, a.speaker, sameLookLine(i, e.itemName(p, a.itemID)))
		case !enchanterAuras[i].Colours:
			e.tryOn(sess, p, a, i, enchanterAuras[i].A.Color)
		default:
			a.aura, a.stage, a.page = i, encFamily, 0
			a.picker = sess.DyePicker() >= 1
			shown, _, _ := enchantPage(colourAnswers(a.picker), 0, room)
			e.ask(sess, p, a, fmt.Sprintf("Which colour for %s?", enchanterAuras[i].Tag), shown)
		}
	case encFamily:
		all := colourAnswers(a.picker)
		shown, first, paged := enchantPage(all, a.page, room)
		if paged && n == len(shown)-1 {
			a.page++
			shown, _, _ = enchantPage(all, a.page, room)
			e.ask(sess, p, a, fmt.Sprintf("Which colour for %s?", enchanterAuras[a.aura].Tag), shown)
			return true
		}
		i := first + n
		switch {
		case n >= len(shown) || i >= len(all):
		case i == 0:
			e.tryOn(sess, p, a, a.aura, 0) // its own colours
		case a.picker && i == len(all)-1:
			e.openPicker(sess, p, a)
		default:
			a.family, a.stage = i-1, encShade
			cs := database.DyeFamilies[a.family].Colours
			answers := make([]string, len(cs))
			for k, c := range cs {
				answers[k] = c.Label
			}
			e.ask(sess, p, a, fmt.Sprintf("Which colour for %s?", enchanterAuras[a.aura].Tag), answers)
		}
	case encShade:
		if cs := database.DyeFamilies[a.family].Colours; n < len(cs) {
			e.tryOn(sess, p, a, a.aura, cs[n].RGBA)
		}
	case encTry:
		database.EndInstanceAuraPreview(a.itemID)
		if n == 0 {
			e.enchant(sess, p, a, a.aura, a.colour, false)
		} else {
			e.say(sess, a.speaker, "As you wish: it is as it was.")
		}
	}
	return true
}

// tryOn shows the aura chosen on the held copy for enchantTryOn, to everyone
// who sees it (as it would be bought: the copy's other aura kept), and asks
// whether to buy it (petition 2450). Its answer, or a close, takes it off;
// unanswered, it goes by itself.
func (e *Enchanter) tryOn(sess *session.Session, p *entity.Player, a enchanterAsk, aura int, colour uint32) {
	if p.WornItems[a.hand] != a.itemID || p.WornMask&(1<<a.hand) == 0 {
		e.say(sess, a.speaker, "You are no longer holding that weapon. Hold it and speak to me again.")
		return
	}
	if lendsNothing(aura, e.heldLook(p, a.itemID)) {
		e.say(sess, a.speaker, sameLookLine(aura, e.itemName(p, a.itemID)))
		return
	}
	row, _, err := e.db.InstanceAuraOf(a.itemID)
	if err != nil {
		log.Printf("Enchanter: %s's item %d: %v", p.Name, a.itemID, err)
		e.say(sess, a.speaker, "Something went wrong with my runes. Try again.")
		return
	}
	row.ItemID = a.itemID
	put := enchanterAuras[aura].A
	put.Color = colour
	if a.second {
		row.Second = put
	} else {
		row.First = put
	}
	database.PreviewInstanceAura(row, enchantTryOn)
	a.stage, a.aura, a.colour = encTry, aura, colour
	price := ""
	if n := int(enchanterPrice.Load()); n > 0 {
		_, name := e.tokenPattern()
		price = " for " + reagentWords(name, n)
	}
	e.ask(sess, p, a, fmt.Sprintf("Here is %s on your weapon for a moment. Buy it%s?", enchanterAuras[aura].Tag, price), []string{enchantBuy, enchantNoBuy})
	log.Printf("Enchanter: %s tries %s on item %d (colour %08X)", p.Name, enchanterAuras[aura].Tag, a.itemID, colour)
}

// EnchantTag is a copy's tag from its two auras: each one's catalog name
// (one not of the catalog, a GM's, keeps its word of the old tag, by its
// place), joined "First, Second"; none "".
func EnchantTag(row database.InstanceAura, old string) string {
	oldWords := strings.Split(old, ", ")
	var words []string
	for k, a := range []database.ItemAura{row.First, row.Second} {
		if !a.SecondShows() {
			continue
		}
		if i := enchanterAuraOf(a); i >= 0 {
			words = append(words, enchanterAuras[i].Tag)
		} else if k < len(oldWords) && strings.TrimSpace(oldWords[k]) != "" {
			words = append(words, strings.TrimSpace(oldWords[k]))
		}
	}
	t := strings.Join(words, ", ")
	if !database.ValidInstanceTag(t) {
		t = clipRunes(t, database.InstanceTagMost)
		t = strings.TrimRight(t, ", ")
	}
	return t
}

// tokenPattern is the token's record (0: the database holds none).
func (e *Enchanter) tokenPattern() (int, string) {
	name := *enchanterToken.Load()
	var id int
	if err := e.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? COLLATE NOCASE ORDER BY patternid LIMIT 1`, name).Scan(&id); err == nil {
		return id, name
	}
	if fb := int(enchanterTokenID.Load()); fb > 0 {
		var n string
		if err := e.db.QueryRow(`SELECT itemname FROM itempattern WHERE patternid = ?`, fb).Scan(&n); err == nil {
			return fb, n
		}
	}
	return 0, name
}

// enchant writes the copy's chosen aura (aura -1: off; both: both off) in
// colour, the price paid for an aura put on.
func (e *Enchanter) enchant(sess *session.Session, p *entity.Player, a enchanterAsk, aura int, colour uint32, both bool) {
	if p.WornItems[a.hand] != a.itemID || p.WornMask&(1<<a.hand) == 0 {
		e.say(sess, a.speaker, "You are no longer holding that weapon. Hold it and speak to me again.")
		return
	}
	row, _, err := e.db.InstanceAuraOf(a.itemID)
	if err != nil {
		log.Printf("Enchanter: %s's item %d: %v", p.Name, a.itemID, err)
		e.say(sess, a.speaker, "Something went wrong with my runes. Try again.")
		return
	}
	if aura >= 0 && lendsNothing(aura, e.heldLook(p, a.itemID)) { // the picker's way here too
		e.say(sess, a.speaker, sameLookLine(aura, e.itemName(p, a.itemID)))
		return
	}
	oldTag := row.Tag
	row.ItemID = a.itemID
	var put database.ItemAura
	if aura >= 0 {
		put = enchanterAuras[aura].A
		put.Color = colour
	}
	switch {
	case both:
		row.First, row.Second = database.ItemAura{}, database.ItemAura{}
	case a.second:
		row.Second = put
	default:
		row.First = put
	}
	row.Tag = EnchantTag(row, oldTag)
	row.Source = "enchanter"
	// the price, for an aura put on
	price := int(enchanterPrice.Load())
	var held []database.CharInventoryEntry
	if aura >= 0 && price > 0 {
		tok, name := e.tokenPattern()
		if tok == 0 {
			e.say(sess, a.speaker, fmt.Sprintf("I work for %s, and none can be had yet. Come back when the season brings them.", reagentWords(name, price)))
			return
		}
		items, err := e.inv.GetCharacterItems(p.CharServerID)
		if err != nil {
			log.Printf("Enchanter: %s's pack: %v", p.Name, err)
			e.say(sess, a.speaker, "Something went wrong with my runes. Try again.")
			return
		}
		have := 0
		for _, it := range items {
			if it.Pattern.PatternID == tok && it.Location == database.LocPack && it.EquipLoc < 0 && it.StackLeft > 0 {
				held = append(held, it)
				have += it.StackLeft
			}
		}
		if have < price {
			e.say(sess, a.speaker, fmt.Sprintf("An aura costs %s, and you carry %d.", reagentWords(name, price), have))
			return
		}
	}
	who := database.GMEdit{AccountID: sess.AccountID(), Username: p.Name, Via: "game"}
	if addr := sess.RemoteAddr(); addr != nil {
		who.IP = addr.IP.String()
	}
	detail := fmt.Sprintf("the Weapon Enchanter: %s's %s (item %d): %s", p.Name, auraPieceNames[a.hand], a.itemID, instanceAuraWords(row))
	if err := e.db.SetInstanceAuraAudited(row, who, detail); err != nil {
		log.Printf("Enchanter: %s's item %d: %v", p.Name, a.itemID, err)
		e.say(sess, a.speaker, "Something went wrong with my runes. Try again.")
		return
	}
	if len(held) > 0 {
		takeStacks(e.inv, sess, p, "Enchanter", held, price)
	}
	switch {
	case both:
		e.say(sess, a.speaker, "There: its auras are gone.")
	case aura < 0:
		e.say(sess, a.speaker, "There: that aura is gone.")
	default:
		e.say(sess, a.speaker, fmt.Sprintf("There: %s, %s. Its name shows with it from your next login.", enchanterAuras[aura].Tag, enchanterAuras[aura].Line))
	}
	log.Printf("Enchanter: %s's %s (item %d): %s", p.Name, auraPieceNames[a.hand], a.itemID, instanceAuraWords(row))
}

// openPicker opens the client's colour picker (dyepicker.go) for the aura
// chosen: its colour now where the copy wears that aura, previewed on the
// hand; a colour picked is enchanted as a palette's colour is (the price
// paid, the weapon still held).
func (e *Enchanter) openPicker(sess *session.Session, p *entity.Player, a enchanterAsk) {
	c := enchanterAuras[a.aura]
	place := uint8(auraPlaceMainHand)
	if a.hand != entity.AuraMainHand {
		place = auraPlaceOffHand
	}
	if a.second {
		place += auraPlaceMainHand2 - auraPlaceMainHand
	}
	o := dyePickerOpen{target: DyePickAura, title: "Colour of " + c.Tag, slots: []uint8{place},
		aura: dyeAuraPreview{kind: c.A.Kind, source: c.A.Source, nodes: c.A.Nodes}}
	if row, _, err := e.db.InstanceAuraOf(a.itemID); err == nil {
		was := row.First
		if a.second {
			was = row.Second
		}
		if enchanterAuraOf(was) == a.aura {
			o.current = was.Color
		}
	}
	what := fmt.Sprintf("%s's %s (item %d) %s", p.Name, auraPieceNames[a.hand], a.itemID, c.Tag)
	openDyePicker(sess, p, a.npc, a.speaker, what, o, func(sess *session.Session, p *entity.Player, rgba uint32) {
		e.enchant(sess, p, a, a.aura, rgba, false)
	})
}
