package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// dyer.go — the Armor Dyers (the user, 2026-11-06: "also add a Armor dyer NPC
// nearby the hierloom npc also"; then "lets add more armor dye colors"). Talked
// to, a dyer asks what to dye (all the armour, one piece of armour, the robe,
// shield or a weapon, or every dye taken off), then a family of colours and a
// colour of it (database.DyeFamilies: 35 and no dye), each question a box of
// six answers at most, CLIENT's room for them (maxDialogOptions: until the
// colours came, a list of twelve and one of twenty-one were cut to their first
// six), and dyes it at once, for nothing: table chardyes (database.SetCharDye, a row edit with
// its gm_audit line as #dye's, so the GM panel lists and reverts it) and the
// character where it stands (World.SetDyes). Dyes are drawn by a client of
// the live game's look (vanilla, entity/livelook.go); a PS2 never sees them,
// and the dyer says so. Any NPC named "Armor Dyer" is one
// (tools/seed_heirlooms.py stands one beside each Heirloom Outfitter). Its
// questions go under box mark 0x56000000 (beside the quests' 0x51, the
// triggers' 0x52, the duels' 0x53, the rehabilitators' 0x54, the campfire
// advisors' 0x55); CLIENT's answer box shows three of its six answers a page,
// with More and Back. On a server with capes (world.capes; capes.go) a dyer
// dyes the cape as well (the user, 2026-11-09: "also allow dying"): "All my
// armor" takes it in, it is the second of "Robe, cape, shield or weapons", and
// "My cape", the first question's third answer, dyes it alone; its dye
// (chardyes' DyeCape) is the colour the clients that draw capes are sent it in.
// Without capes the questions are what they always were. The trim (the owner,
// 2026-11-13: "for the plate armor, there is gold trim, but currently is muted
// due to the colors, lets add trim color dyeing also") is the last answer of the
// first question, "Armor trim", after "Take every dye off" (so More leaves the
// cursor on the trim, never on taking every dye off), and asks which: all the
// armour's trim or a piece's: the chest's (a robe's top takes it), the bracers'
// and gloves' together, the legs' (a robe's skirt takes it), the boots', the
// helm's: six answers, CLIENT's box's most (a GM's #dye names each slot alone).
// Every set has trim in its art now, not the plate's gold alone (the owner,
// 2026-11-13: "armor trim for cloth isnt showing, also make sure leather,
// chain, etc. all sets can be trimmed"; the client finds each texture's trim,
// claude-notes/armor-trim-all-sets-20261113.md); a piece whose art has none
// (the plate's gloves, helms 4 and 6, ...) shows nothing of its trim's dye. A
// trim's dye (chardyes' trim slots) is drawn by a client that draws this
// server's armour (entity/livelook.go); "Take every dye off" takes it off too.

// dyerBoxMark marks a dyer's question's id.
const dyerBoxMark = 0x56000000

// IsDyerBox reports a box a dyer asked.
func IsDyerBox(id uint32) bool { return id&0xFF000000 == dyerBoxMark }

// EquipmentDyerName is the dyers' name (the owner, 2026-11-15: "lets rename
// armor dyer npc to equipment dyer"); one still named "Armor Dyer" is one too.
const EquipmentDyerName = "Equipment Dyer"

// IsArmorDyer is whether an NPC of the name is a dyer.
func IsArmorDyer(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "equipment dyer" || n == "armor dyer" || n == "armour dyer"
}

// dyerChoice is an answer to what to dye: the slots it dyes, or a further
// question (sub), or neither: every dye off.
type dyerChoice struct {
	label string
	slots []int
	sub   []dyerChoice
}

func one(label string, slot int) dyerChoice { return dyerChoice{label: label, slots: []int{slot}} }

// dyerTrim is "Armor trim": every armour slot's trim, or one piece's.
var dyerTrim = dyerChoice{label: "Armor trim", sub: []dyerChoice{
	{label: "All armor trim", slots: []int{database.DyeChestTrim, database.DyeBracerTrim, database.DyeGlovesTrim,
		database.DyeLegsTrim, database.DyeBootsTrim, database.DyeHelmTrim}},
	one("Chest or robe top trim", database.DyeChestTrim),
	{label: "Bracers and gloves trim", slots: []int{database.DyeBracerTrim, database.DyeGlovesTrim}},
	one("Legs or robe skirt trim", database.DyeLegsTrim), one("Boots trim", database.DyeBootsTrim),
	one("Helm trim", database.DyeHelmTrim)}}

// The first question, one category an answer (the owner, 2026-11-15: "lets
// rename armor dyer npc to equipment dyer, also organize for easy parsing"):
// the armour whole, a piece of it, its trim, the robe (cape) and shield, the
// weapons, and taking every dye off, last. Six answers, CLIENT's box's most;
// a PS2 shows three a page, so More leaves its cursor on the second page's
// second line, Weapons, never on taking every dye off.
var dyerArmorPiece = dyerChoice{label: "One armor piece", sub: []dyerChoice{one("Chest", database.DyeChest),
	one("Bracers", database.DyeBracer), one("Gloves", database.DyeGloves), one("Legs", database.DyeLegs),
	one("Boots", database.DyeBoots), one("Helm", database.DyeHelm)}}

var dyerWeapons = dyerChoice{label: "Weapons", sub: []dyerChoice{
	{label: "Both weapons", slots: []int{database.DyePrimary, database.DyeSecondary}},
	one("Primary weapon", database.DyePrimary), one("Secondary weapon", database.DyeSecondary)}}

var dyerOff = dyerChoice{label: "Remove all dyes"}

var dyerChoices = []dyerChoice{
	{label: "All armor", slots: []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs,
		database.DyeBoots, database.DyeHelm, database.DyeRobe}},
	dyerArmorPiece,
	dyerTrim,
	{label: "Robe and shield", sub: []dyerChoice{one("Robe", database.DyeRobe), one("Shield", database.DyeShield)}},
	dyerWeapons,
	dyerOff,
}

// dyerChoicesCapes are dyerChoices on a server with capes: the cape in "All
// armor" and beside the robe and shield.
var dyerChoicesCapes = []dyerChoice{
	{label: "All armor", slots: []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs,
		database.DyeBoots, database.DyeHelm, database.DyeRobe, database.DyeCape}},
	dyerArmorPiece,
	dyerTrim,
	{label: "Robe, cape and shield", sub: []dyerChoice{one("Robe", database.DyeRobe), one("Cape", database.DyeCape),
		one("Shield", database.DyeShield)}},
	dyerWeapons,
	dyerOff,
}

// dyerMenu is what a dyer asks on this server: dyerChoicesCapes where capes
// are worn (world.capes), else dyerChoices.
func dyerMenu() []dyerChoice {
	if entity.CapesOn() {
		return dyerChoicesCapes
	}
	return dyerChoices
}

// dyerMenuIn is dyerMenu for a box of room answers (dialogRoom): in CLIENT's
// six it is dyerMenu itself; in a bigger box (a client that said it shows
// more, dialogroom.go) each category's own question is put in its place, in
// order, while the whole still fits (the owner, 2026-11-15: "lets increase the
// dialog box ... to stack higher"): in twelve, the six pieces of armour and
// the robe and shield stand in the first question beside All armor, Armor
// trim, Weapons and Remove all dyes. A category that would not fit keeps its
// question.
func dyerMenuIn(room int) []dyerChoice {
	menu := dyerMenu()
	if room <= len(menu) {
		return menu
	}
	out := append([]dyerChoice(nil), menu...)
	for i := 0; i < len(out); i++ {
		c := out[i]
		if c.sub == nil || len(out)-1+len(c.sub) > room {
			continue
		}
		out = append(out[:i:i], append(append([]dyerChoice(nil), c.sub...), out[i+1:]...)...)
		i += len(c.sub) - 1
	}
	return out
}

// dyerShades is a family's colours as the dyer offers them: the first
// family's with "No dye" after them (the dye of what was chosen taken off).
func dyerShades(family int) []database.DyeColour {
	out := database.DyeFamilies[family].Colours
	if family == 0 {
		out = append(append([]database.DyeColour(nil), out...), database.DyeColour{Label: "No dye"})
	}
	return out
}

// The dyer's questions, in the order they come.
const (
	askWhat   = iota // dyerChoices
	askWhich         // dyerChoices[parent].sub
	askFamily        // database.DyeFamilies
	askShade         // dyerShades(family)
)

// Dyer keeps each character's question in hand.
type Dyer struct {
	gw    *entity.World
	dyes  dyeStore
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]dyerAsk
}

// dyerAsk is a question asked: its box, which question (askWhat...), what
// was chosen before it (the choice its sub-question is of, the slots and
// their name, the family) and who asked it.
type dyerAsk struct {
	id      uint32
	stage   int
	parent  int
	slots   []int
	what    string
	family  int
	room    int // the answers its client's box holds (dialogRoom): which first question it was asked
	speaker string
	npc     *entity.NPC // the dyer asked (nil: the genie's): the colour picker's anchor (dyepicker.go)
	picker  bool        // its families' question ended with "Pick any colour..." (dyepicker.go)
}

// NewDyer makes the dyers over a dye store.
func NewDyer(gw *entity.World, dyes dyeStore) *Dyer {
	return &Dyer{gw: gw, dyes: dyes, asked: map[uint32]dyerAsk{}}
}

func (d *Dyer) say(sess *session.Session, speaker, line string) {
	for _, part := range splitSay(speaker+": "+line, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

func (d *Dyer) ask(sess *session.Session, p *entity.Player, a dyerAsk, question string, answers []string) {
	d.mu.Lock()
	d.seq++
	a.id = dyerBoxMark | (d.seq & 0xFFFFFF)
	d.asked[p.CharServerID] = a
	d.mu.Unlock()
	SendQuestPopupMulti(sess, a.id, clipRunes(question, questAskChars), answers)
}

// Talk is a dyer talked to: what to dye. It reports whether the NPC was one.
func (d *Dyer) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if d == nil || p == nil || npc == nil || npc.Dead || !IsArmorDyer(npc.Name) {
		return false
	}
	d.offer(sess, p, strings.TrimSpace(npc.Name), npc)
	return true
}

// Offer is a dyer's questions under speaker's name: an Armor Dyer's, and the
// genie's "Dye my armor" (genie.go; the user, 2026-11-08: "add an armor dyer
// near dabrish too and on genie").
func (d *Dyer) Offer(sess *session.Session, p *entity.Player, speaker string) {
	d.offer(sess, p, speaker, nil)
}

// offer is Offer asked by npc (nil: the genie's).
func (d *Dyer) offer(sess *session.Session, p *entity.Player, speaker string, npc *entity.NPC) {
	if d == nil || p == nil {
		return
	}
	forgetDialogs(p)
	closeDyePicker(sess, p)
	if d.dyes == nil {
		d.say(sess, speaker, "My vats are dry today.")
		return
	}
	greeting := "I dye armor, robes, shields and weapons any color you like, free of charge. Only those who see the live game's colors see a dye."
	if entity.CapesOn() {
		greeting = "I dye armor, robes, capes, shields and weapons any color you like, free of charge. Only those who see the live game's colors see a dye."
	}
	d.say(sess, speaker, greeting)
	room := dialogRoom(sess)
	d.ask(sess, p, dyerAsk{speaker: speaker, stage: askWhat, room: room, npc: npc}, "What shall I dye?", choiceLabels(dyerMenuIn(room)))
}

func choiceLabels(cs []dyerChoice) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.label
	}
	return out
}

// chosen goes on from a choice of what to dye: its own question, every dye
// off, or the colours' families. parent is its index in dyerMenuIn(room), the
// first question as it was asked.
func (d *Dyer) chosen(sess *session.Session, p *entity.Player, speaker string, npc *entity.NPC, c dyerChoice, parent, room int) {
	switch {
	case c.sub != nil:
		d.ask(sess, p, dyerAsk{speaker: speaker, stage: askWhich, parent: parent, room: room, npc: npc}, "Which one?", choiceLabels(c.sub))
	case c.slots == nil:
		d.dye(sess, p, speaker, allDyeSlots(), 0, "Every dye is off.")
	case pickerDirect(sess):
		// a client with the picker: no palette, the picker at once (the
		// owner, 2026-11-18: "lets remove the color presets from the dyer
		// simplify with the rgb picker")
		d.openPicker(sess, p, dyerAsk{speaker: speaker, stage: askFamily, slots: c.slots, what: strings.ToLower(c.label), npc: npc, picker: true})
	default:
		what := strings.ToLower(c.label)
		families := familyLabels()
		picker := pickerOffered(sess, len(families))
		if picker {
			families = append(families, DyePickLabel)
		}
		d.ask(sess, p, dyerAsk{speaker: speaker, stage: askFamily, slots: c.slots, what: what, npc: npc, picker: picker},
			fmt.Sprintf("Which colors for %s?", what), families)
	}
}

// Answer is HandleDialogueOption's question for a dyer's box. It reports
// whether the box was a dyer's (an old one does nothing).
func (d *Dyer) Answer(sess *session.Session, p *entity.Player, box uint32, option byte) bool {
	if !IsDyerBox(box) {
		return false
	}
	if d == nil || p == nil || option == 255 {
		return true // a close: the PS2 sends one before the real pick when two boxes were open
	}
	d.mu.Lock()
	a, ok := d.asked[p.CharServerID]
	if ok && a.id == box {
		delete(d.asked, p.CharServerID)
	}
	d.mu.Unlock()
	if !ok || a.id != box {
		return true
	}
	n := int(option)
	switch a.stage {
	case askWhat:
		if menu := dyerMenuIn(a.room); n < len(menu) {
			d.chosen(sess, p, a.speaker, a.npc, menu[n], n, a.room)
		}
	case askWhich:
		if menu := dyerMenuIn(a.room); a.parent < len(menu) {
			if sub := menu[a.parent].sub; n < len(sub) {
				d.chosen(sess, p, a.speaker, a.npc, sub[n], a.parent, a.room)
			}
		}
	case askFamily:
		if a.picker && n == len(database.DyeFamilies) {
			d.openPicker(sess, p, a)
			return true
		}
		if n < len(database.DyeFamilies) {
			shades := dyerShades(n)
			answers := make([]string, len(shades))
			for i, c := range shades {
				answers[i] = c.Label
			}
			d.ask(sess, p, dyerAsk{speaker: a.speaker, stage: askShade, slots: a.slots, what: a.what, family: n},
				fmt.Sprintf("Which color for %s?", a.what), answers)
		}
	case askShade:
		if shades := dyerShades(a.family); n < len(shades) {
			c := shades[n]
			done := fmt.Sprintf("There: %s.", strings.ToLower(c.Label))
			if c.RGBA == 0 {
				done = "The dye is off."
			}
			d.dye(sess, p, a.speaker, a.slots, c.RGBA, done)
		}
	}
	return true
}

func allDyeSlots() []int {
	return dyeSlotsHere()
}

// dyeSlotsHere is the dye slots this server has: every one, but the cape's on
// a server without capes (world.capes), where a cape is never worn, so a dyer's
// "Take every dye off" and #dye's "all" and its listing are what they were.
func dyeSlotsHere() []int {
	out := make([]int, 0, database.DyeSlots)
	for i := 0; i < database.DyeSlots; i++ {
		if i != database.DyeCape || entity.CapesOn() {
			out = append(out, i)
		}
	}
	return out
}

// dyeSlotNamesHere is dyeSlotsHere's words, #dye's help.
func dyeSlotNamesHere() string {
	var names []string
	for _, s := range dyeSlotsHere() {
		names = append(names, database.DyeSlotNames[s])
	}
	return strings.Join(names, " ")
}

// dye writes the slots' colour and puts the dyes on the character.
func (d *Dyer) dye(sess *session.Session, p *entity.Player, speaker string, slots []int, rgba uint32, done string) {
	cur, err := d.dyes.CharDyes(p.CharServerID)
	if err != nil {
		log.Printf("Dyer: %s's dyes: %v", p.Name, err)
		d.say(sess, speaker, "Something went wrong with my vats. Try again.")
		return
	}
	who := database.GMEdit{AccountID: sess.AccountID(), Via: "game"}
	if addr := sess.RemoteAddr(); addr != nil {
		who.IP = addr.IP.String()
	}
	for _, s := range slots {
		if _, _, err := d.dyes.SetCharDye(p.CharServerID, s, rgba, who, "the Armor Dyer"); err != nil {
			log.Printf("Dyer: %s's %s: %v", p.Name, database.DyeSlotNames[s], err)
			d.say(sess, speaker, "Something went wrong with my vats. Try again.")
			return
		}
		cur[s] = rgba
	}
	if d.gw != nil {
		d.gw.SetDyes(p.CharServerID, entity.Dyes(cur))
	}
	d.say(sess, speaker, done)
	names := make([]string, len(slots))
	for i, s := range slots {
		names[i] = database.DyeSlotNames[s]
	}
	log.Printf("Dyer: %s's %s: %s", p.Name, strings.Join(names, ", "), database.FormatDye(rgba))
}

// familyLabels is the dyers' families' words, a box's answers.
func familyLabels() []string {
	out := make([]string, len(database.DyeFamilies))
	for i, f := range database.DyeFamilies {
		out[i] = f.Label
	}
	return out
}

// openPicker opens the client's colour picker (dyepicker.go) for the slots
// asked: the colour of the first of them now, previewed on the character; a
// colour picked dyes them all as a palette's colour does.
func (d *Dyer) openPicker(sess *session.Session, p *entity.Player, a dyerAsk) {
	cur, err := d.dyes.CharDyes(p.CharServerID)
	if err != nil {
		log.Printf("Dyer: %s's dyes: %v", p.Name, err)
		d.say(sess, a.speaker, "Something went wrong with my vats. Try again.")
		return
	}
	slots := make([]uint8, len(a.slots))
	for i, s := range a.slots {
		slots[i] = uint8(s)
	}
	var now uint32
	if len(a.slots) > 0 {
		now = cur[a.slots[0]]
	}
	o := dyePickerOpen{target: DyePickChar, current: now, title: "Dye " + a.what, slots: slots}
	openDyePicker(sess, p, a.npc, a.speaker, a.what, o, func(sess *session.Session, p *entity.Player, rgba uint32) {
		d.dye(sess, p, a.speaker, a.slots, rgba, fmt.Sprintf("There: %s.", colourWords(rgba)))
	})
}
