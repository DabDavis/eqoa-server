package handlers

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// gemsetter.go — Hogarth Bartholomew sets the Gems of Khalimet in the Khal weapons and takes
// them out again (Heirloom petition 2614: a player finished quest "Gnadriin", whose last step
// at him says "Khalimet's gems fit the warlords' weapons. For 10,000 tunar I will set one, for
// 2000 take it out", and nothing answered). What a set gem makes is the live game's, from its
// films (work board 682): a weapon named by the gem's colour ("Green Bloodplague"), the plain
// weapon with the gem's stats, the White gem raising its damage from 700 to 900; a gem comes out
// again whole, the weapon back to its plain record, and fits any class's Khal weapon. Which
// record is which is table gemsetting (database/gemsetting.go), written by
// tools/harbingers_20261122.py.
//
// Talked to by a character that has finished "Gnadriin" (GemSetterQuest) while the table has
// rows, he asks what to do: set a gem (a plain Khal weapon carried in the pack, then a Gem of
// Khalimet carried there, then the price) or take one out (a coloured Khal weapon carried, then
// the price). Yes does it: the copies asked about must still be carried, unworn and with no
// enchanter's aura (a copy's aura would go with it), the purse hold the price, a Lore record to
// come not be held already and the pack have room for what comes once what goes has gone; then
// the weapon and the gem (or the coloured weapon) are taken, the price, and the new record (or
// the plain weapon and the gem) given. A give that fails after all gives the price and what was
// taken back. Before "Gnadriin", or with no rows, he is the quest's Hogarth alone (the talk
// goes on to his lines). His questions go under box mark gemSetterBoxMark; a box is answered
// only within gemSetterReach of him. The words are ours.

// GemSetterName is the setter's name: an NPC of it sets gems.
const GemSetterName = "Hogarth Bartholomew"

// GemSetterQuest is the quest a character must have finished for him to set gems: "Gnadriin"
// (EQOA-Data Misc/harbingers-of-khalimet/p5), whose last step is at him.
const GemSetterQuest = 101280

// The prices, in tunar: the quest's words.
const (
	GemSetPrice    = 10000
	GemRemovePrice = 2000
)

// gemSetterBoxMark marks the setter's questions' ids.
const gemSetterBoxMark = 0x74000000

// gemSetterReach is how near him a box is answered.
const gemSetterReach = 30

// IsGemSetterBox reports a box the setter asked.
func IsGemSetterBox(id uint32) bool { return id&0xFF000000 == gemSetterBoxMark }

// IsGemSetter is whether an NPC of the name is the setter.
func IsGemSetter(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), GemSetterName)
}

// The setter's questions, in the order they come.
const (
	gemsWhat    = iota // set or take out
	gemsWeapon         // which plain weapon (set) or coloured weapon (take out)
	gemsGem            // which gem (set)
	gemsConfirm        // the price: yes or no
)

// The first question's answers.
const (
	gemsAnswerSet    = "Set a gem in a weapon"
	gemsAnswerRemove = "Take a gem out of a weapon"
	gemsAnswerYes    = "Yes"
	gemsAnswerNo     = "No"
)

// gemsHeld is a copy carried: its charinventory row, and what it is.
type gemsHeld struct {
	itemID int
	row    database.CharInventoryEntry
	is     database.GemSettingRow
}

// gemsAsk is a question asked.
type gemsAsk struct {
	box    uint32
	stage  int
	remove bool
	page   int
	npc    entity.EntityID
	choice []gemsHeld // the weapons or gems offered (gemsWeapon, gemsGem)
	weapon gemsHeld   // the weapon chosen
	gem    gemsHeld   // the gem chosen (set)
}

// GemSetter is Hogarth's gem setting.
type GemSetter struct {
	gw    *entity.World
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	items *database.ItemPatternStore
	// finished is the quests a character has finished (Quests().FinishedBy); a test's stands in
	finished func(serverID uint32) map[int]bool
	// give hands a record over (GiveItemNow); a test's stands in
	give func(sess *session.Session, pat database.ItemPatternRecord) (int, error)

	mu    sync.Mutex
	seq   uint32
	asked map[uint32]gemsAsk // by the character's serverid
}

// NewGemSetter is the setter over the World and the database; nil without them (an NPC of
// his name then talks as any other).
func NewGemSetter(gw *entity.World, db *database.SQLiteDB) *GemSetter {
	if gw == nil || db == nil {
		return nil
	}
	inv := database.NewInventoryStore(db)
	g := &GemSetter{gw: gw, db: db, inv: inv, items: database.NewItemPatternStore(db), asked: map[uint32]gemsAsk{},
		finished: func(id uint32) map[int]bool { return Quests().FinishedBy(id) }}
	g.give = func(sess *session.Session, pat database.ItemPatternRecord) (int, error) {
		return GiveItemNow(sess, gw, inv, pat, 1)
	}
	gw.OnPlayerLeaving(func(p *entity.Player) {
		g.mu.Lock()
		delete(g.asked, p.CharServerID)
		g.mu.Unlock()
	})
	return g
}

func (g *GemSetter) say(sess *session.Session, line string) {
	for _, part := range splitSay(GemSetterName+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (g *GemSetter) ask(sess *session.Session, p *entity.Player, a gemsAsk, question string, answers []string) {
	g.mu.Lock()
	g.seq++
	a.box = gemSetterBoxMark | (g.seq & 0xFFFFFF)
	g.asked[p.CharServerID] = a
	g.mu.Unlock()
	for i := range answers {
		answers[i] = clipRunes(answers[i], questAnswerChars)
	}
	SendQuestPopupMulti(sess, a.box, clipRunes(question, questAskChars), answers)
}

// table is gemsetting by record, and the canonical record of each (kind, family, colour).
func (g *GemSetter) table() (map[int]database.GemSettingRow, map[string]int, error) {
	rows, err := g.db.GemSettings()
	if err != nil {
		return nil, nil, err
	}
	by := make(map[int]database.GemSettingRow, len(rows))
	canon := map[string]int{}
	for _, r := range rows {
		by[r.PatternID] = r
		if r.Canonical {
			if _, ok := canon[gemsKey(r.Kind, r.Family, r.Colour)]; !ok {
				canon[gemsKey(r.Kind, r.Family, r.Colour)] = r.PatternID
			}
		}
	}
	return by, canon, nil
}

func gemsKey(kind, family, colour string) string { return kind + "|" + family + "|" + colour }

// carried is what of the table the character carries in its pack, unworn, by kind: the plain
// weapons, the coloured weapons and the gems, each in its pack's order.
func (g *GemSetter) carried(p *entity.Player, by map[int]database.GemSettingRow) (plain, coloured, gems []gemsHeld, err error) {
	rows, err := g.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, r := range rows {
		is, ok := by[r.Pattern.PatternID]
		if !ok || r.Location != database.LocPack || r.EquipLoc >= 0 || r.StackLeft < 1 {
			continue
		}
		h := gemsHeld{itemID: r.ItemID, row: r, is: is}
		switch {
		case is.Kind == database.GemKindGem:
			gems = append(gems, h)
		case is.Colour == "":
			plain = append(plain, h)
		default:
			coloured = append(coloured, h)
		}
	}
	return plain, coloured, gems, nil
}

// Talk is a talk to npc: his question when npc is him, the character has finished
// GemSetterQuest and the table has rows; else false (his quest's lines and his own go on).
func (g *GemSetter) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if g == nil || p == nil || npc == nil || npc.Dead || !IsGemSetter(npc.Name) {
		return false
	}
	if !g.finished(p.CharServerID)[GemSetterQuest] {
		return false
	}
	by, _, err := g.table()
	if err != nil {
		log.Printf("GemSetter: the table: %v", err)
		return false
	}
	if len(by) == 0 {
		return false
	}
	forgetDialogs(p)
	g.say(sess, fmt.Sprintf("Khalimet's gems fit the warlords' weapons. I set one for %s and take one out for %s; "+
		"the gem comes out whole. Carry the weapon and the gem in your pack, not in your hands.",
		tunarText(GemSetPrice), tunarText(GemRemovePrice)))
	g.ask(sess, p, gemsAsk{stage: gemsWhat, npc: npc.ServerID}, "What shall I do?", []string{gemsAnswerSet, gemsAnswerRemove})
	log.Printf("GemSetter: %s talked to %s", p.Name, GemSetterName)
	return true
}

// Answer is HandleDialogueOption's question for a box of his. It reports whether the box was
// his (an old one does nothing).
func (g *GemSetter) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsGemSetterBox(box) {
		return false
	}
	if g == nil || p == nil || option == 255 {
		// a close is not the last word: the PS2 sends it just before the real pick when two
		// boxes were open (interact.go)
		return true
	}
	g.mu.Lock()
	a, ok := g.asked[p.CharServerID]
	if ok && a.box == box {
		delete(g.asked, p.CharServerID)
	}
	g.mu.Unlock()
	if !ok || a.box != box {
		return true
	}
	if !g.near(p, a.npc) {
		g.say(sess, "Come closer; I cannot work from over there.")
		return true
	}
	n := int(option)
	switch a.stage {
	case gemsWhat:
		switch n {
		case 0:
			g.askWeapon(sess, p, gemsAsk{npc: a.npc})
		case 1:
			g.askWeapon(sess, p, gemsAsk{npc: a.npc, remove: true})
		}
	case gemsWeapon, gemsGem:
		labels := gemsLabels(a.choice)
		shown, first, paged := enchantPage(labels, a.page, dialogRoom(sess))
		if paged && n == len(shown)-1 {
			a.page++
			g.askPage(sess, p, a)
			return true
		}
		i := first + n
		if n >= len(shown) || i >= len(a.choice) {
			return true
		}
		if a.stage == gemsWeapon {
			a.weapon = a.choice[i]
			if a.remove {
				g.askConfirm(sess, p, a)
			} else {
				g.askGem(sess, p, a)
			}
			return true
		}
		a.gem = a.choice[i]
		g.askConfirm(sess, p, a)
	case gemsConfirm:
		if n == 0 {
			g.work(sess, p, a)
		} else {
			g.say(sess, "As you like.")
		}
	}
	return true
}

// near is whether p stands within gemSetterReach of him, in his world.
func (g *GemSetter) near(p *entity.Player, id entity.EntityID) bool {
	world, x, _, z, ok := g.gw.NPCSpot(id)
	return ok && world == p.World && (x-p.X)*(x-p.X)+(z-p.Z)*(z-p.Z) <= gemSetterReach*gemSetterReach
}

func gemsName(h gemsHeld) string { return strings.TrimSpace(h.row.Pattern.ItemName) }

// gemsLabels is the answers for the copies offered: their names, a second of a name numbered.
func gemsLabels(hs []gemsHeld) []string {
	out := make([]string, len(hs))
	seen := map[string]int{}
	for i, h := range hs {
		name := gemsName(h)
		seen[name]++
		if seen[name] > 1 {
			name = fmt.Sprintf("%s (%d)", name, seen[name])
		}
		out[i] = name
	}
	return out
}

func (g *GemSetter) askPage(sess *session.Session, p *entity.Player, a gemsAsk) {
	shown, _, _ := enchantPage(gemsLabels(a.choice), a.page, dialogRoom(sess))
	q := "Which weapon?"
	switch {
	case a.stage == gemsGem:
		q = "Which gem shall I set?"
	case a.remove:
		q = "Which weapon's gem shall I take out?"
	}
	g.ask(sess, p, a, q, shown)
}

// askWeapon offers the weapons carried that the work fits: plain ones to set a gem in, coloured
// ones to take a gem out of.
func (g *GemSetter) askWeapon(sess *session.Session, p *entity.Player, a gemsAsk) {
	by, _, err := g.table()
	if err != nil {
		log.Printf("GemSetter: the table: %v", err)
		g.say(sess, "Something is wrong with my tools. Come back later.")
		return
	}
	plain, coloured, gems, err := g.carried(p, by)
	if err != nil {
		log.Printf("GemSetter: %s's pack: %v", p.Name, err)
		g.say(sess, "Something is wrong with my tools. Come back later.")
		return
	}
	a.stage, a.page = gemsWeapon, 0
	if a.remove {
		if len(coloured) == 0 {
			g.say(sess, "You carry no Khal weapon with a gem in it. Carry it in your pack, not in your hands.")
			return
		}
		a.choice = coloured
	} else {
		switch {
		case len(plain) == 0:
			g.say(sess, "You carry no plain Khal weapon. Carry it in your pack, not in your hands; one with a gem in it must have that gem taken out first.")
			return
		case len(gems) == 0:
			g.say(sess, "You carry no Gem of Khalimet.")
			return
		}
		a.choice = plain
	}
	g.askPage(sess, p, a)
}

// askGem offers the gems carried, one of each colour.
func (g *GemSetter) askGem(sess *session.Session, p *entity.Player, a gemsAsk) {
	by, _, err := g.table()
	if err != nil {
		log.Printf("GemSetter: the table: %v", err)
		g.say(sess, "Something is wrong with my tools. Come back later.")
		return
	}
	_, _, gems, err := g.carried(p, by)
	if err != nil {
		log.Printf("GemSetter: %s's pack: %v", p.Name, err)
		g.say(sess, "Something is wrong with my tools. Come back later.")
		return
	}
	var one []gemsHeld
	seen := map[string]bool{}
	for _, h := range gems {
		if !seen[h.is.Colour] {
			seen[h.is.Colour] = true
			one = append(one, h)
		}
	}
	if len(one) == 0 {
		g.say(sess, "You carry no Gem of Khalimet.")
		return
	}
	sort.SliceStable(one, func(i, j int) bool { return gemsName(one[i]) < gemsName(one[j]) })
	a.stage, a.page, a.choice = gemsGem, 0, one
	g.askPage(sess, p, a)
}

func (g *GemSetter) askConfirm(sess *session.Session, p *entity.Player, a gemsAsk) {
	a.stage = gemsConfirm
	q := fmt.Sprintf("Set the %s in your %s for %s?", gemsName(a.gem), gemsName(a.weapon), tunarText(GemSetPrice))
	if a.remove {
		q = fmt.Sprintf("Take the gem out of your %s for %s?", gemsName(a.weapon), tunarText(GemRemovePrice))
	}
	g.ask(sess, p, a, q, []string{gemsAnswerYes, gemsAnswerNo})
}

// stillCarried is the copy as the pack holds it now: the same record, unworn, false when gone.
func (g *GemSetter) stillCarried(p *entity.Player, h gemsHeld) (database.CharInventoryEntry, bool) {
	r, err := g.inv.GetCharItem(p.CharServerID, h.itemID)
	if err != nil || r.Pattern.PatternID != h.row.Pattern.PatternID || r.Location != database.LocPack || r.EquipLoc >= 0 || r.StackLeft < 1 {
		return database.CharInventoryEntry{}, false
	}
	return r, true
}

// work does what was asked, everything checked again first.
func (g *GemSetter) work(sess *session.Session, p *entity.Player, a gemsAsk) {
	by, canon, err := g.table()
	if err != nil {
		log.Printf("GemSetter: the table: %v", err)
		g.say(sess, "Something is wrong with my tools. Come back later.")
		return
	}
	price := GemSetPrice
	if a.remove {
		price = GemRemovePrice
	}
	takes := []gemsHeld{a.weapon}
	if !a.remove {
		takes = append(takes, a.gem)
	}
	var taken []database.CharInventoryEntry
	for _, h := range takes {
		r, ok := g.stillCarried(p, h)
		if !ok {
			g.say(sess, fmt.Sprintf("You no longer carry the %s in your pack.", gemsName(h)))
			return
		}
		if r.AuraTag != "" {
			g.say(sess, fmt.Sprintf("Your %s carries an enchanter's aura, which my work would break. Have a Weapon Enchanter take it off first.", gemsName(h)))
			return
		}
		if _, ok := by[r.Pattern.PatternID]; !ok {
			g.say(sess, "Something is wrong with my tools. Come back later.")
			return
		}
		taken = append(taken, r)
	}
	// what comes: the coloured record, or the plain weapon and the gem
	var gives []int
	w := by[a.weapon.row.Pattern.PatternID]
	if a.remove {
		gives = []int{canon[gemsKey(database.GemKindWeapon, w.Family, "")], canon[gemsKey(database.GemKindGem, "", w.Colour)]}
	} else {
		gem := by[a.gem.row.Pattern.PatternID]
		if w.Colour != "" || gem.Kind != database.GemKindGem {
			g.say(sess, "Something is wrong with my tools. Come back later.")
			return
		}
		gives = []int{canon[gemsKey(database.GemKindWeapon, w.Family, gem.Colour)]}
	}
	var pats []database.ItemPatternRecord
	for _, id := range gives {
		var pat *database.ItemPatternRecord
		if id > 0 {
			pat, err = g.items.GetItemPattern(id)
		}
		if id == 0 || err != nil || pat == nil {
			log.Printf("GemSetter: %s: no record to give for %q (%v; gemsetting lacks a canonical row)", p.Name, gemsName(a.weapon), err)
			g.say(sess, "I cannot do that work on that weapon. Ask a guide.")
			return
		}
		pats = append(pats, *pat)
	}
	if p.Tunar < price {
		g.say(sess, fmt.Sprintf("My price is %s, and you carry %s.", tunarText(price), tunarText(p.Tunar)))
		return
	}
	l := sessionInvList(sess)
	if l == nil {
		g.say(sess, "Come back when you are settled.")
		return
	}
	takenIDs := map[int]bool{}
	for _, r := range taken {
		takenIDs[r.Pattern.PatternID] = true
	}
	var room []invGive
	for _, pat := range pats {
		// a Lore record held already (and not among what goes) cannot come
		if pat.Lore != 0 && !takenIDs[pat.PatternID] && l.loreHeld(pat.PatternID) {
			g.say(sess, fmt.Sprintf("You can hold only one %s, and you hold one already.", strings.TrimSpace(pat.ItemName)))
			return
		}
		room = append(room, invGive{patternID: pat.PatternID, n: 1, maxStack: pat.MaxStack})
	}
	var tk []invTake
	for _, r := range taken {
		tk = append(tk, invTake{itemID: r.ItemID, n: 1})
	}
	if short := l.roomFor(tk, room); len(short) > 0 {
		g.say(sess, "Your pack has no room for what I would hand you. Make room and come back.")
		return
	}
	purse, ok := g.gw.TakeTunar(p.CharServerID, price)
	if !ok {
		g.say(sess, fmt.Sprintf("My price is %s, and you carry less.", tunarText(price)))
		return
	}
	g.keepPurse(sess, p, purse)
	for _, r := range taken {
		takeStacks(g.inv, sess, p, "GemSetter", []database.CharInventoryEntry{r}, 1)
	}
	var given []string
	for _, pat := range pats {
		n, err := g.give(sess, pat)
		if err != nil || n < 1 {
			// put it all back: the price, what was taken, less what was given
			log.Printf("GemSetter: %s: %q could not be given (%d, %v): the work undone", p.Name, pat.ItemName, n, err)
			if back, ok := g.gw.TakeTunar(p.CharServerID, -price); ok {
				g.keepPurse(sess, p, back)
			}
			for _, r := range taken {
				if _, err := g.give(sess, r.Pattern); err != nil {
					log.Printf("GemSetter: %s: %q could not be given back: %v", p.Name, r.Pattern.ItemName, err)
				}
			}
			g.say(sess, "Something slipped. Here is all you gave me; try again.")
			return
		}
		given = append(given, strings.TrimSpace(pat.ItemName))
	}
	economy.Spent(economy.GemSetting, p.CharServerID, p.Name, price)
	var gave []string
	for _, r := range taken {
		gave = append(gave, strings.TrimSpace(r.Pattern.ItemName))
	}
	if a.remove {
		g.say(sess, fmt.Sprintf("There: the gem is out, whole. Your %s and your %s.", given[0], given[1]))
	} else {
		g.say(sess, fmt.Sprintf("There: your %s.", given[0]))
	}
	log.Printf("GemSetter: %s gave %s and %d tunar for %s (purse %d)", p.Name, strings.Join(gave, " + "), price, strings.Join(given, " + "), purse)
}

// keepPurse writes the purse and sends it.
func (g *GemSetter) keepPurse(sess *session.Session, p *entity.Player, purse int) {
	if err := g.inv.SetPurse(p.CharServerID, purse); err != nil {
		log.Printf("GemSetter: %s's purse %d: %v", p.Name, purse, err)
	}
	if sess != nil {
		queueReliable(sess, protocol.OpcodePlayerTunar, tunarPayload(purse))
	}
}
