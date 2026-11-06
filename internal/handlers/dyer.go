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
// nearby the hierloom npc also"). Talked to, a dyer asks what to dye (all the
// armour, one slot, or every dye taken off) and then the colour, and dyes it
// at once, for nothing: table chardyes (database.SetCharDye, a row edit with
// its gm_audit line as #dye's, so the GM panel lists and reverts it) and the
// character where it stands (World.SetDyes). Dyes are drawn by a client of
// the live game's look (vanilla, entity/livelook.go); a PS2 never sees them,
// and the dyer says so. Any NPC named "Armor Dyer" is one
// (tools/seed_heirlooms.py stands one beside each Heirloom Outfitter). Its
// questions go under box mark 0x56000000 (beside the quests' 0x51, the
// triggers' 0x52, the duels' 0x53, the rehabilitators' 0x54, the campfire
// advisors' 0x55); CLIENT's answer box shows three answers a page, with More
// and Back.

// dyerBoxMark marks a dyer's question's id.
const dyerBoxMark = 0x56000000

// IsDyerBox reports a box a dyer asked.
func IsDyerBox(id uint32) bool { return id&0xFF000000 == dyerBoxMark }

// IsArmorDyer is whether an NPC of the name is a dyer.
func IsArmorDyer(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return n == "armor dyer" || n == "armour dyer"
}

// dyerSlot is an answer to what to dye: the slots it dyes (none: every dye
// off).
type dyerSlot struct {
	label string
	slots []int
}

var dyerSlots = []dyerSlot{
	{"All my armor", []int{database.DyeChest, database.DyeBracer, database.DyeGloves, database.DyeLegs, database.DyeBoots,
		database.DyeHelm, database.DyeRobe}},
	{"Chest", []int{database.DyeChest}},
	{"Bracers", []int{database.DyeBracer}},
	{"Gloves", []int{database.DyeGloves}},
	{"Legs", []int{database.DyeLegs}},
	{"Boots", []int{database.DyeBoots}},
	{"Helm", []int{database.DyeHelm}},
	{"Robe", []int{database.DyeRobe}},
	{"Shield", []int{database.DyeShield}},
	{"Primary weapon", []int{database.DyePrimary}},
	{"Secondary weapon", []int{database.DyeSecondary}},
	{"Take every dye off", nil},
}

// dyerColour is a colour a dyer offers (0: the dye taken off).
type dyerColour struct {
	label string
	rgba  uint32
}

func tint(i int) uint32 { return database.RGBA(database.DiscTints[i]) }

var dyerColours = []dyerColour{
	{"White", 0xFFFFFFFF}, {"Black", 0x101010FF}, {"Grey", 0x808080FF}, {"Silver", tint(10)},
	{"Red", 0xD10000FF}, {"Maroon", tint(13)}, {"Orange", 0xFF8000FF}, {"Gold", 0xFAAF32FF},
	{"Yellow", 0xF0F000FF}, {"Green", 0x008000FF}, {"Olive", tint(7)}, {"Teal", 0x00C8B4FF},
	{"Sky blue", tint(2)}, {"Ice blue", tint(9)}, {"Blue", 0x0000C0FF}, {"Navy", tint(4)},
	{"Purple", 0x400080FF}, {"Pink", 0xFF5BADFF}, {"Brown", 0x966432FF}, {"Tan", tint(11)},
	{"No dye", 0},
}

// Dyer keeps each character's question in hand.
type Dyer struct {
	gw    *entity.World
	dyes  dyeStore
	mu    sync.Mutex
	seq   uint32
	asked map[uint32]dyerAsk
}

// dyerAsk is a question asked: its box, what it asks (the slots once chosen)
// and who asked it.
type dyerAsk struct {
	id      uint32
	slots   []int
	colour  bool
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
	forgetDialogs(p)
	speaker := strings.TrimSpace(npc.Name)
	if d.dyes == nil {
		d.say(sess, speaker, "My vats are dry today.")
		return true
	}
	d.say(sess, speaker, "I dye armor any color you like, free of charge. Only those who see the live game's colors see a dye.")
	answers := make([]string, len(dyerSlots))
	for i, s := range dyerSlots {
		answers[i] = s.label
	}
	d.ask(sess, p, dyerAsk{speaker: speaker}, "What shall I dye?", answers)
	return true
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
	if !a.colour {
		if int(option) >= len(dyerSlots) {
			return true
		}
		s := dyerSlots[option]
		if s.slots == nil {
			d.dye(sess, p, a.speaker, allDyeSlots(), 0, "Every dye is off.")
			return true
		}
		answers := make([]string, len(dyerColours))
		for i, c := range dyerColours {
			answers[i] = c.label
		}
		d.ask(sess, p, dyerAsk{speaker: a.speaker, slots: s.slots, colour: true},
			fmt.Sprintf("Which color for %s?", strings.ToLower(s.label)), answers)
		return true
	}
	if int(option) >= len(dyerColours) {
		return true
	}
	c := dyerColours[option]
	done := fmt.Sprintf("There: %s.", strings.ToLower(c.label))
	if c.rgba == 0 {
		done = "The dye is off."
	}
	d.dye(sess, p, a.speaker, a.slots, c.rgba, done)
	return true
}

func allDyeSlots() []int {
	out := make([]int, database.DyeSlots)
	for i := range out {
		out[i] = i
	}
	return out
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
