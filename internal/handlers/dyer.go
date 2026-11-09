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
// Without capes the questions are what they always were.

// dyerBoxMark marks a dyer's question's id.
const dyerBoxMark = 0x56000000

// IsDyerBox reports a box a dyer asked.
func IsDyerBox(id uint32) bool { return id&0xFF000000 == dyerBoxMark }

// IsArmorDyer is whether an NPC of the name is a dyer.
func IsArmorDyer(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "armor dyer" || n == "armour dyer"
}

// dyerChoice is an answer to what to dye: the slots it dyes, or a further
// question (sub), or neither: every dye off.
type dyerChoice struct {
	label string
	slots []int
	sub   []dyerChoice
}

func one(label string, slot int) dyerChoice { return dyerChoice{label: label, slots: []int{slot}} }

var dyerChoices = []dyerChoice{
	{label: "All my armor", slots: []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs,
		database.DyeBoots, database.DyeHelm, database.DyeRobe}},
	{label: "One piece of armor", sub: []dyerChoice{one("Chest", database.DyeChest), one("Bracers", database.DyeBracer),
		one("Gloves", database.DyeGloves), one("Legs", database.DyeLegs), one("Boots", database.DyeBoots),
		one("Helm", database.DyeHelm)}},
	{label: "Robe, shield or weapons", sub: []dyerChoice{one("Robe", database.DyeRobe), one("Shield", database.DyeShield),
		one("Primary weapon", database.DyePrimary), one("Secondary weapon", database.DyeSecondary)}},
	{label: "Take every dye off"},
}

// dyerChoicesCapes are dyerChoices on a server with capes: the cape in "All my
// armor", the second of the robe's question, and "My cape", the third answer,
// straight to the colours (the owner's choice, 2026-11-09: until then the cape
// alone was only under the robe's question; then "up front"). CLIENT's answer
// box shows three answers a page, so "My cape" ends the first page and the
// robe's question and "Take every dye off" are on the second, after More.
var dyerChoicesCapes = []dyerChoice{
	{label: "All my armor", slots: []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs,
		database.DyeBoots, database.DyeHelm, database.DyeRobe, database.DyeCape}},
	dyerChoices[1],
	one("My cape", database.DyeCape),
	{label: "Robe, cape, shield or weapons", sub: []dyerChoice{one("Robe", database.DyeRobe), one("Cape", database.DyeCape),
		one("Shield", database.DyeShield), one("Primary weapon", database.DyePrimary), one("Secondary weapon", database.DyeSecondary)}},
	dyerChoices[3],
}

// dyerMenu is what a dyer asks on this server: dyerChoicesCapes where capes
// are worn (world.capes), else dyerChoices.
func dyerMenu() []dyerChoice {
	if entity.CapesOn() {
		return dyerChoicesCapes
	}
	return dyerChoices
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
	speaker string
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
	d.Offer(sess, p, strings.TrimSpace(npc.Name))
	return true
}

// Offer is a dyer's questions under speaker's name: an Armor Dyer's, and the
// genie's "Dye my armor" (genie.go; the user, 2026-11-08: "add an armor dyer
// near dabrish too and on genie").
func (d *Dyer) Offer(sess *session.Session, p *entity.Player, speaker string) {
	if d == nil || p == nil {
		return
	}
	forgetDialogs(p)
	if d.dyes == nil {
		d.say(sess, speaker, "My vats are dry today.")
		return
	}
	greeting := "I dye armor any color you like, free of charge. Only those who see the live game's colors see a dye."
	if entity.CapesOn() {
		greeting = "I dye armor and capes any color you like, free of charge. Only those who see the live game's colors see a dye."
	}
	d.say(sess, speaker, greeting)
	d.ask(sess, p, dyerAsk{speaker: speaker, stage: askWhat}, "What shall I dye?", choiceLabels(dyerMenu()))
}

func choiceLabels(cs []dyerChoice) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.label
	}
	return out
}

// chosen goes on from a choice of what to dye: its own question, every dye
// off, or the colours' families. parent is its index in dyerChoices.
func (d *Dyer) chosen(sess *session.Session, p *entity.Player, speaker string, c dyerChoice, parent int) {
	switch {
	case c.sub != nil:
		d.ask(sess, p, dyerAsk{speaker: speaker, stage: askWhich, parent: parent}, "Which one?", choiceLabels(c.sub))
	case c.slots == nil:
		d.dye(sess, p, speaker, allDyeSlots(), 0, "Every dye is off.")
	default:
		what := strings.ToLower(c.label)
		families := make([]string, len(database.DyeFamilies))
		for i, f := range database.DyeFamilies {
			families[i] = f.Label
		}
		d.ask(sess, p, dyerAsk{speaker: speaker, stage: askFamily, slots: c.slots, what: what},
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
		if menu := dyerMenu(); n < len(menu) {
			d.chosen(sess, p, a.speaker, menu[n], n)
		}
	case askWhich:
		if sub := dyerMenu()[a.parent].sub; n < len(sub) {
			d.chosen(sess, p, a.speaker, sub[n], a.parent)
		}
	case askFamily:
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
