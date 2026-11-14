package handlers

import (
	"fmt"
	"log"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// wereinfect.go — a were's bite and its cure (entity/wereinfect.go rolls the
// bite; the user, 2026-11-06: "players must be melee attacked by the were
// animal to be infected. the oasis NPCs around the campfire offer cures for a
// price"). A character infected carries its line's fever: the line's marker
// quest (tools/quests/data/weres.json, "Weres/<line>/infection", a draft that
// is never offered) finished. The line's third quest requires it beside its
// society's 1000 (the user's choice: "Yes, infection and 1000"), so the giver's
// "The fever is in you" comes once the bite has taken. One fever at a time; a
// werehunter's brand (Weres/werehunter/brand), a fever already or a form (any
// line's third quest finished) keeps a bite from taking. The campfire at the
// Oasis (the guides: Advisor Gerahl checks for any infection, Nitarha, here
// Nilarha, the wolf's, Wanda the bear's, Thimpra the rat's, Sanne the lion's;
// Brunjo, the sixth, the gator's) says what the character carries and cures a
// fever for CurePrice (the user's choice: 25,000); a were who has turned (its
// third quest finished) is not cured (the user's choice: "No, infection only").
// Declining the third quest is its giver's antidote (its offer: "Or take my
// antidote and stay as you are"): the fever passes (quests.go, choiceDecline).

// CurePrice is a fever's cure at the campfire, in tunar.
const CurePrice = 25000

// advisorBoxMark marks an advisor's question's id (0x0034's u32), beside the
// quests' 0x51, the triggers' 0x52, the duels' 0x53 and the rehabilitators'
// 0x54.
const advisorBoxMark = 0x55000000

// IsAdvisorBox reports a box an advisor asked.
func IsAdvisorBox(id uint32) bool { return id&0xFF000000 == advisorBoxMark }

// advisors are the campfire's six, by name in any case, each with the line
// whose fever it looks for ("" any).
var advisors = map[string]string{
	"advisor gerahl":  "",
	"advisor nilarha": "wolfwere",
	"advisor wanda":   "bearwere",
	"advisor thimpra": "ratwere",
	"advisor sanne":   "lionwere",
	"advisor brunjo":  "gatorwere",
}

// wereLines are the five lines, in order.
var wereLines = []string{"wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"}

// beast is a line's beast in words: "wolf".
func beast(line string) string {
	return strings.TrimSuffix(line, "were")
}

// wereQuests are the quests a bite and a cure look at, by line: the marker of
// its fever and its third quest; and the werehunters' brand.
type wereQuests struct {
	fever, turned map[string]int
	brand         int
}

// wereQuestsLocked reads them off the catalog by their import keys. Must be
// called with q.mu held.
func (q *QuestEngine) wereQuestsLocked() wereQuests {
	w := wereQuests{fever: map[string]int{}, turned: map[string]int{}}
	for id, qu := range q.cat.byID {
		key := database.QuestImportKey(qu.Source)
		if !strings.HasPrefix(key, "Weres/") {
			continue
		}
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		switch {
		case parts[1] == "werehunter" && parts[2] == "brand":
			w.brand = id
		case parts[2] == "infection":
			w.fever[parts[1]] = id
		case parts[2] == "p3":
			w.turned[parts[1]] = id
		}
	}
	return w
}

// feverOf is the line whose fever c carries, "" none; turnedOf the line whose
// third quest c finished, "" none.
func (w wereQuests) feverOf(c *questChar) string {
	for _, line := range wereLines {
		if id := w.fever[line]; id != 0 && c.hasFinished(id) {
			return line
		}
	}
	return ""
}

func (w wereQuests) turnedOf(c *questChar) string {
	for _, line := range wereLines {
		if id := w.turned[line]; id != 0 && c.hasFinished(id) {
			return line
		}
	}
	return ""
}

// heardInfection queues a bite that infects (entity's OnInfection, called
// with the World's lock held: it must not block).
func (q *QuestEngine) heardInfection(h entity.InfectionHit) {
	select {
	case q.infections <- h:
	default:
		log.Printf("Weres: %s's bite on %s was not taken (the queue is full)", h.NPCName, h.Name)
	}
}

// infect gives the character the line's fever when nothing keeps it from
// taking, and tells it.
func (q *QuestEngine) infect(h entity.InfectionHit) {
	q.mu.Lock()
	w := q.wereQuestsLocked()
	fever := w.fever[h.Line]
	c := q.charLocked(h.ServerID)
	why := ""
	switch {
	case fever == 0:
		why = "no marker quest for the line"
	case w.brand != 0 && c.hasFinished(w.brand):
		why = "a werehunter's brand"
	case w.feverOf(c) != "":
		why = "the " + beast(w.feverOf(c)) + "'s fever already"
	case w.turnedOf(c) != "":
		why = "a " + beast(w.turnedOf(c)) + " already"
	}
	q.mu.Unlock()
	if why != "" {
		log.Printf("Weres: %s's bite on %s did not take: %s", h.NPCName, h.Name, why)
		return
	}
	if _, err := q.SetCharacterQuest(h.ServerID, fever, QuestComplete, 0); err != nil {
		log.Printf("Weres: %s's fever not kept: %v", h.Name, err)
		return
	}
	if h.Session != nil {
		SendClientMessage(h.Session, fmt.Sprintf("A fever burns where the %s's claws found you.", strings.ToLower(h.NPCName)))
	}
	log.Printf("Weres: %s caught the %s's fever from %s", h.Name, beast(h.Line), h.NPCName)
}

// advisorBox is a cure an advisor offered a character.
type advisorBox struct {
	id      uint32
	speaker string
	line    string
}

// advisorLine is the line an advisor looks for ("" any) and whether the NPC
// is one.
func advisorLine(name string) (string, bool) {
	line, ok := advisors[strings.ToLower(strings.TrimSpace(name))]
	return line, ok
}

// AdvisorTalk is an advisor of the campfire talked to: what the character
// carries, and a cure offered for a fever. It reports whether the NPC was one.
func (q *QuestEngine) AdvisorTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	looks, ok := advisorLine(npc.Name)
	if !ok {
		return false
	}
	forgetDialogs(p)
	speaker := strings.TrimSpace(npc.Name)
	q.mu.Lock()
	w := q.wereQuestsLocked()
	c := q.charLocked(p.CharServerID)
	fever, turned := w.feverOf(c), w.turnedOf(c)
	q.mu.Unlock()
	say := func(l string) {
		for _, part := range splitSay(speaker+": "+l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
	switch {
	case turned != "" && (looks == "" || looks == turned):
		say(fmt.Sprintf("The %s has already claimed your blood. No cure reaches what has turned.", beast(turned)))
	case fever != "" && (looks == "" || looks == fever):
		say(fmt.Sprintf("You carry the %s's fever. I can draw it out of you for %s.", beast(fever), tunarText(CurePrice)))
		q.mu.Lock()
		q.boxSeq++
		b := advisorBox{id: advisorBoxMark | (q.boxSeq & 0xFFFFFF), speaker: speaker, line: fever}
		if q.cures == nil {
			q.cures = map[uint32]advisorBox{}
		}
		q.cures[p.CharServerID] = b
		q.mu.Unlock()
		SendQuestPopupMulti(sess, b.id, clipRunes(fmt.Sprintf("Be cured of the %s's fever for %s?", beast(fever), tunarText(CurePrice)), questAskChars),
			[]string{"Cure me.", "Not now."})
	case looks == "":
		say("You carry no were's fever.")
	default:
		say(fmt.Sprintf("You carry no %s's fever.", beast(looks)))
	}
	log.Printf("Weres: %s asked %s (fever %q, turned %q)", p.Name, speaker, fever, turned)
	return true
}

// AdvisorAnswer is HandleDialogueOption's question for an advisor's box: the
// cure paid for and given, or not. It reports whether the box was an
// advisor's (an old one does nothing); spend takes the tunar (the purse, kept
// and shown), false when the player has too little.
func (q *QuestEngine) AdvisorAnswer(sess *session.Session, p *entity.Player, box uint32, option byte,
	spend func(*session.Session, *entity.Player, int) bool) bool {
	if !IsAdvisorBox(box) {
		return false
	}
	if q == nil || p == nil || option == 255 {
		return true // a close: the PS2 sends one before the real pick when two boxes were open
	}
	q.mu.Lock()
	b, ok := q.cures[p.CharServerID]
	if ok && b.id == box {
		delete(q.cures, p.CharServerID)
	}
	w := q.wereQuestsLocked()
	c := q.charLocked(p.CharServerID)
	fever, turned := w.feverOf(c), w.turnedOf(c)
	q.mu.Unlock()
	if !ok || b.id != box {
		return true
	}
	say := func(l string) {
		for _, part := range splitSay(b.speaker+": "+l, questBoxChars) {
			SendDialogueBox(sess, 0, part)
		}
	}
	if option != 0 {
		say("As you wish. Come back before it turns.")
		return true
	}
	if fever != b.line || turned != "" {
		say("Your blood has changed since we spoke. Ask me again.")
		return true
	}
	if spend == nil || !spend(sess, p, CurePrice) {
		say(fmt.Sprintf("The cure costs %s.", tunarText(CurePrice)))
		return true
	}
	if _, err := q.SetCharacterQuest(p.CharServerID, w.fever[fever], QuestReset, 0); err != nil {
		log.Printf("Weres: %s's cure not kept: %v (the %d tunar back)", p.Name, err, CurePrice)
		spend(sess, p, -CurePrice)
		say("Something went wrong. Your tunar is returned.")
		return true
	}
	say(fmt.Sprintf("The %s's fever leaves your blood.", beast(fever)))
	economy.Spent(economy.Cure, p.CharServerID, p.Name, CurePrice)
	log.Printf("Weres: %s was cured of the %s's fever by %s for %d tunar", p.Name, beast(fever), b.speaker, CurePrice)
	return true
}

// antidoteLocked is a third quest declined: its giver's antidote takes the
// line's fever away (its offer's "Or take my antidote and stay as you are",
// its decline's "Then the fever passes"; the guides: "Do NOT take the
// antidote, or you lose your infection"). Must be called with q.mu held.
func (q *QuestEngine) antidoteLocked(c *questChar, qu *database.Quest) {
	key := database.QuestImportKey(qu.Source)
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "Weres" || parts[2] != "p3" {
		return
	}
	fever := q.wereQuestsLocked().fever[parts[1]]
	if fever == 0 || !c.hasFinished(fever) {
		return
	}
	if err := q.store.DeleteCharQuest(c.serverID, fever); err != nil {
		log.Printf("Weres: the antidote's fever not taken away: %v", err)
		return
	}
	delete(c.quests, fever)
	log.Printf("Weres: character %d took the %s's antidote: the fever passes", c.serverID, beast(parts[1]))
}
