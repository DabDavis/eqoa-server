package handlers

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// mercmenu.go — a merc's own menu (the owner, 2026-11-12: "let players press
// Square to open a dialog with their mercs"). A talk to one of the player's
// own mercs (0x0004: vanilla sends it for its own merc's □; a PS2's □ on any
// pet opens CLIENT's details box and sends nothing, InteractWithTarget, so
// /merc [name or 1-3] menu opens it there) asks, in the board's boxes
// (mercBoxMark):
//
//   - Advice: a few lines of its class's own, in its voice;
//   - Spells: the spells it casts (its book as entity.MercSpells lists it,
//     the highest first), five a box with "More", each "on" or "off"; a choice
//     turns it the other way, kept in its row (charmerc.spells_off) and its
//     kit (entity.MercKit.SpellsOff: its book leaves it out);
//   - Stance: passive, assist or aggressive (/merc stance), the one it holds
//     marked;
//   - a healer's Defensive or Offensive, the other of what it is (/merc
//     defensive, offensive);
//   - a pet class's Pet: its kinds and Best (/merc pet: mercpetkind.go);
//   - a Cocktail alchemist's Throws: AoE or single (/merc aoe, single:
//     mercinv.go);
//   - Items: what it holds (Inventory, each record its Equip or Unequip and
//     Take back), Give an item, an alchemist's Potion stock, and what waits
//     in the keeping (mercinv.go);
//   - a merc of 55 or more's Master: its Master Class's kinds, then a kind's
//     names (/merc master: mercmaster.go);
//   - Dismiss, asked again before it goes (/merc dismiss).
//
// A box holds six answers (maxDialogOptions): a menu of more shows its first
// five and More, the rest in a second box.
//
// Another player's merc has no menu: its talk is anything else's.

// mercMenuPage is how many spells a box lists (the dungeon board's five; a
// client that shows more answers, its room less one: dialogroom.go).
const mercMenuPage = 5

const (
	mercMenuAsk     = "%s, your level %d %s %s: 'What do you need of me?'"
	mercSpellsAsk   = "%s's spells (%d of %d): a choice turns it on or off."
	mercStanceAsk   = "%s is %s. How far does it take a fight?"
	mercDismissAsk  = "Dismiss %s? The Mercenary Recruitment board hires you another."
	mercSpellOnLine = "%s casts %s again."
	mercSpellOff    = "%s leaves %s alone."
	mercNoSpells    = "%s casts no spells."
	mercPetAsk      = "%s summons %s. Which pet?"
	mercSlotsFull   = "%s's ten spell slots are full: take one off to cast %s."
)

// mercAdvice is each class's merc's advice, in its voice (ours).
var mercAdvice = map[int]string{
	0:  "Let me take the first blow. Strike what I strike and I hold its anger; pull one at a time and none of you falls.",
	1:  "My bow opens the fight and my blades finish it. Keep the foe on our tank and I strike from where it cannot see.",
	2:  "I hold the foe and mend us when it goes badly. Pull to me, and turn undead my way: I am made for them.",
	3:  "I hold the foe with fear and blood. Let me take its anger, and leave its life to drain into mine.",
	4:  "I strike fast with bare hands. Let our tank take the foe first, then I am at its back.",
	5:  "My songs carry us all: keep near me, and they keep you. I sing first and strike when my power is spare.",
	6:  "Let our tank hold it, then I strike from behind. If it turns on me, I am gone before it lands a blow.",
	7:  "Stay within my reach and I keep you standing. Out of a fight, let me rest my power: a healer out of power heals no one.",
	8:  "Keep the foe on our tank and I keep the tank alive. My spirits ward you before the fight; let me rest between them.",
	9:  "Keep the foe on our tank and I keep the tank standing. Do not pull while I rest: a cleric out of power saves no one.",
	10: "Give our tank a moment to hold the foe, then I burn it down. If I go quiet, I am keeping power for my best spell.",
	11: "My plagues work slowly: let them run. Pull the next only when this one falls, and keep me out of its reach.",
	12: "I keep your power flowing and your guard runed. Pull one at a time: a crowd is the end of an enchanter.",
	13: "Let our tank hold it first, then I strike hard. If I stop, I am saving power for the spell that ends it.",
	14: "My draughts and charms do the work. Let our tank hold it, and I keep you strong while we fight.",
}

// mercAdviceHealer is said by a healer of its mode.
var mercAdviceHealer = map[bool]string{
	false: "I am defensive now: I heal and ward, nothing more. Tell me to be offensive and I strike when all of you are whole.",
	true:  "I am offensive now: I strike when all of you are whole, and heal first whenever one of you is hurt.",
}

// menuMerc is what /merc says of the merc of the slot out; ok false, none out.
func (m *Mercs) menuMerc(sess *session.Session, slot int) (entity.MercInfo, bool) {
	for _, info := range m.gw.MercInfosOf(sess) {
		if info.Slot == slot {
			return info, true
		}
	}
	return entity.MercInfo{}, false
}

// menu asks the merc's menu (the file's doc).
func (m *Mercs) menu(sess *session.Session, p *entity.Player, slot int) {
	info, ok := m.menuMerc(sess, slot)
	if !ok {
		SendErrorMessage(sess, mercNoneOut(slot, nil))
		return
	}
	keys, answers := m.menuChoices(p, slot, info)
	m.menuBox(sess, p, slot, info, keys, answers, 0)
}

// menuChoices is the menu's answers (the file's doc) and their meanings.
func (m *Mercs) menuChoices(p *entity.Player, slot int, info entity.MercInfo) (keys, answers []string) {
	add := func(k, a string) { keys, answers = append(keys, k), append(answers, a) }
	add("advice", "Advice")
	add("spells", "Spells")
	add("stance", "Stance: "+entity.MercStanceNames[info.Stance])
	if mode := entity.MercModeName(info.Class, info.Offensive); mode != "" {
		add("mode", map[bool]string{false: "Be offensive", true: "Be defensive"}[info.Offensive])
	}
	if len(mercPetKinds[info.Class]) > 0 {
		add("pet", "Pet: "+m.menuPetSaid(p, slot, info.Class))
	}
	if m.db != nil {
		if row, ok, _ := m.db.Merc(p.CharServerID, slot); ok && mercCocktail(row) {
			add("throws", "Throws: "+map[bool]string{true: "AoE", false: "single"}[row.AoE])
		}
	}
	if info.Level >= entity.MercMasterLevel && m.db != nil {
		if row, ok, _ := m.db.Merc(p.CharServerID, slot); ok && len(m.gw.MercMasterClasses(row.Class, row.Race)) > 0 {
			held := "none"
			if mc, ok := m.mercMasterOf(row); ok {
				held = mc.Name
			}
			add("master", "Master: "+held) // mercmaster.go
		}
	}
	if m.db != nil && len(m.wereForms(p.CharServerID)) > 0 {
		held := "none"
		if row, ok, _ := m.db.Merc(p.CharServerID, slot); ok && row.Were != "" {
			held = row.Were
		}
		add("were", "Were form: "+held) // mercwere.go
	}
	add("items", "Items")
	add("dismiss", "Dismiss")
	return keys, answers
}

// menuBox asks the menu's box of the page: all of it when it fits a box,
// else its first five and More, then the rest.
func (m *Mercs) menuBox(sess *session.Session, p *entity.Player, slot int, info entity.MercInfo, keys, answers []string, page int) {
	stage := mercStageMenu
	if len(keys) > maxDialogOptions {
		split := maxDialogOptions - 1
		if page == 0 {
			keys, answers = append(keys[:split:split], "more"), append(answers[:split:split], "More")
		} else {
			stage = mercStageMore
			keys, answers = append(keys[split:len(keys):len(keys)], "back"), append(answers[split:len(answers):len(answers)], "Back")
		}
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: stage, slot: slot, keys: keys},
		fmt.Sprintf(mercMenuAsk, info.Name, info.Level, entity.RaceName(info.Race), entity.ClassName(info.Class)), answers)
	m.mu.Unlock()
}

// menuAnswer is a choice in a merc's menu's boxes (Answer's).
func (m *Mercs) menuAnswer(sess *session.Session, p *entity.Player, a mercAsk, n int) {
	switch a.stage {
	case mercStageItems, mercStageInv, mercStageItem, mercStageGive, mercStageStock:
		m.itemsAnswer(sess, p, a, n) // mercinv.go: its row's, out or not
		return
	}
	info, ok := m.menuMerc(sess, a.slot)
	if !ok {
		SendErrorMessage(sess, mercNoneOut(a.slot, nil))
		return
	}
	if a.stage == mercStageWere {
		m.wereAnswer(sess, p, a, n) // mercwere.go
		return
	}
	if a.stage == mercStageMaster || a.stage == mercStageMasterList {
		m.masterAnswer(sess, p, a, info, n) // mercmaster.go
		return
	}
	order := func(words string) { m.Command(sess, p, fmt.Sprintf("%d %s", a.slot, words)) }
	switch a.stage {
	case mercStageMenu, mercStageMore:
		healer := entity.MercModeName(info.Class, info.Offensive) != ""
		if n >= len(a.keys) {
			return
		}
		switch a.keys[n] {
		case "more", "back":
			keys, answers := m.menuChoices(p, a.slot, info)
			m.menuBox(sess, p, a.slot, info, keys, answers, map[string]int{"more": 1, "back": 0}[a.keys[n]])
		case "throws":
			if row, ok, _ := m.db.Merc(p.CharServerID, a.slot); ok {
				order(map[bool]string{true: "single", false: "aoe"}[row.AoE])
			}
		case "items":
			m.menuItems(sess, p, a.slot)
		case "master":
			m.menuMaster(sess, p, a.slot, info) // mercmaster.go
		case "were":
			m.menuWere(sess, p, a.slot, info) // mercwere.go
		case "advice":
			line := mercAdvice[info.Class]
			if healer {
				line += " " + mercAdviceHealer[info.Offensive]
			}
			for _, part := range splitSay(info.Name+": '"+line+"'", questBoxChars) {
				SendDialogueBox(sess, 0, part)
			}
		case "spells":
			if m.openSpellsWindow(sess, p, a.slot) {
				return // vanilla's spell window (mercwin.go)
			}
			m.menuSpells(sess, p, a.slot, 0)
		case "stance":
			a.stage = mercStageStance
			var answers []string
			for _, s := range []entity.MercStance{entity.MercPassive, entity.MercAssist, entity.MercAggressive} {
				label := strings.ToUpper(entity.MercStanceNames[s][:1]) + entity.MercStanceNames[s][1:]
				if s == info.Stance {
					label += " (now)"
				}
				answers = append(answers, label)
			}
			m.mu.Lock()
			m.askLocked(sess, p, a, fmt.Sprintf(mercStanceAsk, info.Name, entity.MercStanceNames[info.Stance]), answers)
			m.mu.Unlock()
		case "mode":
			order(map[bool]string{false: "offensive", true: "defensive"}[info.Offensive])
		case "pet":
			a.stage = mercStagePet
			var answers []string
			for _, k := range menuPetKinds(info.Class) {
				answers = append(answers, k.name)
			}
			answers = append(answers, "Best")
			m.mu.Lock()
			m.askLocked(sess, p, a, fmt.Sprintf(mercPetAsk, info.Name, m.menuPetSaid(p, a.slot, info.Class)), answers)
			m.mu.Unlock()
		case "dismiss":
			a.stage = mercStageDismiss
			m.mu.Lock()
			m.askLocked(sess, p, a, fmt.Sprintf(mercDismissAsk, info.Name), []string{"Yes, dismiss " + info.Name, "No"})
			m.mu.Unlock()
		}
	case mercStageStance:
		if n < 3 {
			order("stance " + []string{"passive", "assist", "aggressive"}[n])
		}
	case mercStageDismiss:
		if n == 0 {
			order("dismiss")
		}
	case mercStagePet:
		kinds := menuPetKinds(info.Class)
		switch {
		case n < len(kinds):
			order("pet " + strings.ToLower(kinds[n].name))
		case n == len(kinds):
			order("pet " + mercPetBest)
		}
	case mercStageSpells:
		m.menuSpellChoice(sess, p, a, info, n)
	}
}

// menuPetKinds is the class's kinds its menu offers: each but one that
// another's words hold (the magician's Efreet and Ghilan, the Fire and Water
// lines' tops since 2026-11-12), so the box keeps its six answers with Best.
func menuPetKinds(class int) []mercPetKind {
	var out []mercPetKind
	for _, k := range mercPetKinds[class] {
		alias := false
		for _, o := range mercPetKinds[class] {
			alias = alias || o.name != k.name && slices.Contains(o.words, strings.ToLower(k.name))
		}
		if !alias {
			out = append(out, k)
		}
	}
	return out
}

// menuPetSaid is the pet kind the merc of the slot summons, in words
// (mercPetSaid of its row's pick).
func (m *Mercs) menuPetSaid(p *entity.Player, slot, class int) string {
	pick := ""
	if m.db != nil {
		if row, ok, _ := m.db.Merc(p.CharServerID, slot); ok {
			pick = row.Pet
		}
	}
	return mercPetSaid(class, pick)
}

// menuSpells asks a page of the merc's spells, each on or off.
func (m *Mercs) menuSpells(sess *session.Session, p *entity.Player, slot, page int) {
	info, _ := m.menuMerc(sess, slot)
	spells, ok := m.gw.MercSpells(sess, slot)
	if !ok {
		SendErrorMessage(sess, mercNoneOut(slot, nil))
		return
	}
	if len(spells) == 0 {
		mercSay(sess, fmt.Sprintf(mercNoSpells, info.Name))
		return
	}
	per := dialogRoom(sess) - 1 // mercMenuPage on a PS2
	pages := (len(spells) + per - 1) / per
	page = max(0, min(page, pages-1))
	var answers []string
	for _, s := range spells[page*per : min((page+1)*per, len(spells))] {
		answers = append(answers, s.Name+map[bool]string{false: ": on", true: ": off"}[s.Off])
	}
	if pages > 1 {
		answers = append(answers, "More")
	}
	m.mu.Lock()
	m.askLocked(sess, p, mercAsk{stage: mercStageSpells, slot: slot, page: page, per: per},
		fmt.Sprintf(mercSpellsAsk, info.Name, page+1, pages), answers)
	m.mu.Unlock()
}

// menuSpellChoice is a choice in a spells box: "More" the next page (the
// first after the last), a spell turned the other way (kept, then the same
// page again).
func (m *Mercs) menuSpellChoice(sess *session.Session, p *entity.Player, a mercAsk, info entity.MercInfo, n int) {
	spells, ok := m.gw.MercSpells(sess, a.slot)
	if !ok {
		return
	}
	per := a.perBox()
	from := a.page * per
	shown := spells[min(from, len(spells)):min(from+per, len(spells))]
	if n == len(shown) {
		next := a.page + 1
		if next*per >= len(spells) {
			next = 0
		}
		m.menuSpells(sess, p, a.slot, next)
		return
	}
	if n > len(shown) {
		return
	}
	s := shown[n]
	var off []int
	var row database.MercRow
	haveRow := false
	if m.db != nil {
		row, haveRow, _ = m.db.Merc(p.CharServerID, a.slot)
	}
	if haveRow {
		// its row's (one off its book now, a level since, stays off); a spell
		// off only by its ten slots is not put in spells_off (mercwin.go)
		for _, id := range row.SpellsOff {
			if id != s.ID {
				off = append(off, id)
			}
		}
	} else {
		for _, x := range spells {
			if x.Off && x.ID != s.ID {
				off = append(off, x.ID)
			}
		}
	}
	if !s.Off {
		off = append(off, s.ID)
	}
	slices.Sort(off)
	// the ten slots, where its hirer set them in vanilla's spell window
	// (mercwin.go): on puts it on the first empty slot, off takes it off
	if haveRow && row.SpellSlots != nil {
		slots := slices.Clone(row.SpellSlots)
		at := slices.Index(slots, s.ID)
		switch {
		case s.Off && at < 0:
			free := slices.Index(slots, 0)
			if free < 0 {
				mercSay(sess, fmt.Sprintf(mercSlotsFull, info.Name, s.Name))
				m.menuSpells(sess, p, a.slot, a.page)
				return
			}
			slots[free] = s.ID
		case !s.Off && at >= 0:
			slots[at] = 0
		}
		if err := m.db.SetMercSpellSlots(p.CharServerID, a.slot, slots); err != nil {
			log.Printf("Merc: %s's %q's spell slots not kept: %v", p.Name, info.Name, err)
			mercSay(sess, mercKitFailed)
			return
		}
		m.gw.SetMercLoadout(sess, a.slot, slots)
	}
	if m.db != nil {
		if err := m.db.SetMercSpellsOff(p.CharServerID, a.slot, off); err != nil {
			log.Printf("Merc: %s's %q's spells not kept: %v", p.Name, info.Name, err)
			mercSay(sess, mercKitFailed)
			return
		}
	}
	m.gw.SetMercSpellsOff(sess, a.slot, off)
	m.pushSpells(sess, p, a.slot)
	log.Printf("Merc: %s turns %q (spell %d) of %q %s", p.Name, s.Name, s.ID, info.Name, map[bool]string{true: "on", false: "off"}[s.Off])
	if s.Off {
		SendClientMessage(sess, fmt.Sprintf(mercSpellOnLine, info.Name, s.Name))
	} else {
		SendClientMessage(sess, fmt.Sprintf(mercSpellOff, info.Name, s.Name))
	}
	m.menuSpells(sess, p, a.slot, a.page)
}
