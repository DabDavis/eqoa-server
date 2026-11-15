package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/economy"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// wereinfect.go — a were's infection, its cure and its end (entity/werecrit.go
// rolls it on a were's opening crit: work 1461; the user, 2026-11-06: "players
// must be melee attacked by the were animal to be infected. the oasis NPCs
// around the campfire offer cures for a price").
//
// Since work 1463 (the GM Jibrish's of 2026-11-14: "A successful animal-were
// infection must appear on the player as a debuff icon for seven days, and that
// debuff must be the infection flag") a character's infection is its
// charinfection row (database/wereinfection.go): its line, caught at the crit's
// time, ending seven days of real time later (an absolute end: a logout, a
// zoning, a death or a restart does not stop it). Everything that asks whether a
// character is infected asks it (activeLocked): a bite's taking, the third
// quests' prerequisite, the campfire's diagnosis and cure, the antidote, the
// form's quest. Its visible form is an effect on the player (entity's
// PutInfection: "were-wolf infection" and the like, Disease Cloud's icon), put on
// at the catching and at each login (InfectionLogin) to the row's end, the time
// left said in words; effectSystem takes it off at its end as the sweep ends the
// row (sweepInfections, each second; offline characters' rows too), the line
// "The were-wolf infection has run its course." to one online. The line's hidden
// fever marker (Weres/<line>/infection, quests 101422-101426, which each third
// quest requires beside its society's 1000) stays as the infection's derived
// form, written and taken in the same transaction as the row; the prerequisite
// asks the row in its place (a marker the row does not stand behind counts for
// nothing), and a marker found with no row is migrated once (the tool's rule:
// tools/were_infection_migrate_20261115.py; MigrateFeverMarker) or taken.
//
// One infection at a time; a werehunter's brand (Weres/werehunter/brand), an
// infection already or a form (any line's third quest finished) keeps one from
// taking; it can be caught before a line's quests or its society's 1000. Kept
// first, shown after: a grant whose row could not be written is tried again with
// the same roll's key (retryInfections), never rolled again; one written whose
// effect could not be shown is shown at the next sweep or login. The campfire
// at the Oasis (the guides: Advisor Gerahl checks for any infection, Nitarha,
// here Nilarha, the wolf's, Wanda the bear's, Thimpra the rat's, Sanne the
// lion's; Brunjo, the sixth, the gator's) says what the character carries and
// cures it for CurePrice (the user's choice: 25,000); a were who has turned (its
// third quest finished) is not cured (the user's choice: "No, infection only").
// Declining the third quest is its giver's antidote (its offer: "Or take my
// antidote and stay as you are"): the infection passes (quests.go,
// choiceDecline). Finishing the third quest turns the character: the infection
// ends with it, the form its quest's own.

// CurePrice is an infection's cure at the campfire, in tunar.
const CurePrice = 25000

// advisorBoxMark marks an advisor's question's id (0x0034's u32), beside the
// quests' 0x51, the triggers' 0x52, the duels' 0x53 and the rehabilitators'
// 0x54.
const advisorBoxMark = 0x55000000

// IsAdvisorBox reports a box an advisor asked.
func IsAdvisorBox(id uint32) bool { return id&0xFF000000 == advisorBoxMark }

// advisors are the campfire's six, by name in any case, each with the line
// whose infection it looks for ("" any).
var advisors = map[string]string{
	"advisor gerahl":  "",
	"advisor nilarha": "wolfwere",
	"advisor wanda":   "bearwere",
	"advisor thimpra": "ratwere",
	"advisor sanne":   "lionwere",
	"advisor brunjo":  "gatorwere",
}

// wereLines are the five lines, in order.
var wereLines = entity.WereLines

// beast is a line's beast in words: "wolf".
func beast(line string) string {
	return strings.TrimSuffix(line, "were")
}

// wereQuests are the quests an infection and a cure look at, by line: the
// marker of its fever and its third quest; and the werehunters' brand.
// markerLine is each marker's line.
type wereQuests struct {
	fever, turned map[string]int
	markerLine    map[int]string
	brand         int
}

// markers is every line's marker id.
func (w wereQuests) markers() []int {
	var out []int
	for _, line := range wereLines {
		if id := w.fever[line]; id != 0 {
			out = append(out, id)
		}
	}
	return out
}

// wereQuestsOf reads them off the catalog by their import keys.
func wereQuestsOf(cat questCatalog) wereQuests {
	w := wereQuests{fever: map[string]int{}, turned: map[string]int{}, markerLine: map[int]string{}}
	for id, qu := range cat.byID {
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
			w.markerLine[id] = parts[1]
		case parts[2] == "p3":
			w.turned[parts[1]] = id
		}
	}
	return w
}

// wereQuestsLocked is the catalog's (read at each Reload). Must be called with
// q.mu held.
func (q *QuestEngine) wereQuestsLocked() wereQuests {
	if q.wereq.fever == nil {
		q.wereq = wereQuestsOf(q.cat)
	}
	return q.wereq
}

// turnedOf is the line whose third quest c finished, "" none.
func (w wereQuests) turnedOf(c *questChar) string {
	for _, line := range wereLines {
		if id := w.turned[line]; id != 0 && c.hasFinished(id) {
			return line
		}
	}
	return ""
}

// activeLocked is c's infection in force at now, nil for none: its row, read
// once (and its markers put in step with it: reconcileLocked). Must be called
// with q.mu held.
func (q *QuestEngine) activeLocked(c *questChar, now time.Time) *database.Infection {
	if !c.infectionRead {
		inf, err := q.store.CharInfection(c.serverID)
		if err != nil {
			log.Printf("Weres: character %d's infection not read: %v", c.serverID, err)
			return nil // read again at the next ask
		}
		c.infection, c.infectionRead = inf, true
		q.reconcileLocked(c, now)
	}
	if c.infection.Active(now) {
		return c.infection
	}
	return nil
}

// feverLineLocked is the line of c's infection in force, "" none.
func (q *QuestEngine) feverLineLocked(c *questChar, now time.Time) string {
	if inf := q.activeLocked(c, now); inf != nil {
		return inf.Line
	}
	return ""
}

// reconcileLocked puts c's fever markers in step with its infection: the active
// line's there, no other; a marker with no row behind it migrated once
// (MigrateFeverMarker), else taken. Must be called with q.mu held, c's row read.
func (q *QuestEngine) reconcileLocked(c *questChar, now time.Time) {
	w := q.wereQuestsLocked()
	for _, line := range wereLines {
		id := w.fever[line]
		cq := c.quests[id]
		if id == 0 || cq == nil || cq.Done == 0 {
			continue
		}
		if c.infection == nil {
			migrated, err := q.store.InfectionMigrated(c.serverID, id)
			if err != nil {
				log.Printf("Weres: character %d's marker %d: %v", c.serverID, id, err)
				continue
			}
			if !migrated {
				outcome, inf, err := q.store.MigrateFeverMarker(*cq, line, w.turnedOf(c) != "", now, entity.InfectionDuration)
				if err != nil {
					log.Printf("Weres: character %d's %s marker not migrated: %v", c.serverID, beast(line), err)
					continue
				}
				log.Printf("Weres: character %d's %s fever marker migrated: %s", c.serverID, beast(line), outcome)
				if inf != nil {
					c.infection = inf
					if !inf.Active(now) {
						delete(c.quests, id)
					}
				}
				continue
			}
		}
		if c.infection.Active(now) && c.infection.Line == line {
			continue
		}
		if err := q.store.DeleteFeverMarkers(c.serverID, []int{id}); err != nil {
			log.Printf("Weres: character %d's stale %s marker not taken: %v", c.serverID, beast(line), err)
			continue
		}
		delete(c.quests, id)
		log.Printf("Weres: character %d's %s marker taken: no infection in force stands behind it", c.serverID, beast(line))
	}
	if inf := c.infection; inf.Active(now) {
		if id := w.fever[inf.Line]; id != 0 && (c.quests[id] == nil || c.quests[id].Done == 0) {
			if err := q.store.SetFeverMarker(c.serverID, id, inf.AppliedAt); err != nil {
				log.Printf("Weres: character %d's %s marker not put back: %v", c.serverID, beast(inf.Line), err)
				return
			}
			c.quests[id] = &database.CharQuest{ServerID: c.serverID, QuestID: id, Done: 1, Updated: inf.AppliedAt.UTC().Format(time.RFC3339)}
		}
	}
}

// endLocked ends c's infection (how, at), its markers with it, in one
// transaction, and takes its effect off a player online; false for an expiry
// that found no row of that end. Must be called with q.mu held.
func (q *QuestEngine) endLocked(c *questChar, how string, at time.Time) (bool, error) {
	w := q.wereQuestsLocked()
	ended, err := q.store.EndInfection(c.serverID, how, at, w.markers())
	if err != nil {
		return false, err
	}
	if !ended && how == database.InfectionExpired {
		c.infectionRead = false // no row of that end: another stands now, read again
		return false, nil
	}
	if c.infection != nil && c.infection.EndedAt.IsZero() {
		c.infection.EndedAt, c.infection.EndedHow = at, how
	}
	for _, id := range w.markers() {
		delete(c.quests, id)
	}
	if sess := q.sessionOfLocked(c.serverID); sess != nil && q.gw != nil {
		q.gw.TakeInfection(sess)
	}
	q.nudge()
	return true, nil
}

// sessionOfLocked is the character's session in the world, nil for none.
func (q *QuestEngine) sessionOfLocked(serverID uint32) *session.Session {
	if q.sessionOf == nil {
		return nil
	}
	return q.sessionOf(serverID)
}

// infectionLater is a grant to try again: its hit and how many tries it has had.
type infectionLater struct {
	h     entity.InfectionHit
	tries int
}

// infectionRetries is how many times a grant is tried in all, a second apart.
const infectionRetries = 600

// later holds the grants to try again (laterMu: heardInfection adds to it
// under the World's lock).
type laterInfections struct {
	mu   sync.Mutex
	list []infectionLater
}

func (l *laterInfections) add(x infectionLater) {
	l.mu.Lock()
	l.list = append(l.list, x)
	l.mu.Unlock()
}

func (l *laterInfections) take() []infectionLater {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.list
	l.list = nil
	return out
}

// heardInfection queues a crit that infects (entity's OnInfection, called with
// the World's lock held: it must not block); a full queue keeps it for the next
// second's try.
func (q *QuestEngine) heardInfection(h entity.InfectionHit) {
	select {
	case q.infections <- h:
	default:
		q.later.add(infectionLater{h: h})
		log.Printf("Weres: %s's infection of %s waits a second (the queue is full)", h.NPCName, h.Name)
	}
}

// retryInfections tries the grants kept for later again.
func (q *QuestEngine) retryInfections() {
	for _, x := range q.later.take() {
		if !q.grant(x.h) {
			if x.tries+1 >= infectionRetries {
				log.Printf("Weres: %s's infection of %s (key %s) given up after %d tries", x.h.NPCName, x.h.Name, x.h.Key, x.tries+1)
				continue
			}
			q.later.add(infectionLater{h: x.h, tries: x.tries + 1})
		}
	}
}

// infect gives the character the crit's infection when nothing keeps it from
// taking, and shows it; one whose keeping failed is tried again with the same
// key.
func (q *QuestEngine) infect(h entity.InfectionHit) {
	if !q.grant(h) {
		q.later.add(infectionLater{h: h, tries: 1})
	}
}

// grant is infect's one try: false when the row could not be written (to try
// again), true when it was, was already, or may not be.
func (q *QuestEngine) grant(h entity.InfectionHit) bool {
	now := time.Now()
	at := h.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	q.mu.Lock()
	w := q.wereQuestsLocked()
	c := q.charLocked(h.ServerID)
	inf := q.activeLocked(c, now)
	why := ""
	switch {
	case !c.infectionRead:
		q.mu.Unlock()
		return false // its row unread: try again
	case inf != nil && h.Key != "" && inf.Key == h.Key:
		q.mu.Unlock()
		q.show(h.ServerID, false) // this grant, kept before: shown again
		return true
	case w.brand != 0 && c.hasFinished(w.brand):
		why = "a werehunter's brand"
	case inf != nil:
		why = "the " + entity.InfectionName(inf.Line) + " already"
	case w.turnedOf(c) != "":
		why = "a " + beast(w.turnedOf(c)) + " already"
	case entity.InfectionEffectID(h.Line) == 0:
		why = "no line " + h.Line
	}
	if why != "" {
		q.mu.Unlock()
		log.Printf("Weres: %s's infection of %s did not take: %s", h.NPCName, h.Name, why)
		return true
	}
	row := database.Infection{ServerID: h.ServerID, Line: h.Line, AppliedAt: at, ExpiresAt: at.Add(entity.InfectionDuration),
		Source: h.NPCName, Key: h.Key}
	marker := w.fever[h.Line]
	if _, err := q.store.ApplyInfection(row, marker); err != nil {
		q.mu.Unlock()
		log.Printf("Weres: %s's infection from %s not kept (to try again, key %s): %v", h.Name, h.NPCName, h.Key, err)
		return false
	}
	c.infection = &row
	if marker != 0 {
		c.quests[marker] = &database.CharQuest{ServerID: h.ServerID, QuestID: marker, Done: 1, Updated: at.UTC().Format(time.RFC3339)}
	}
	q.mu.Unlock()
	q.nudge()
	log.Printf("Weres: %s caught the %s from %s, until %s", h.Name, entity.InfectionName(h.Line), h.NPCName, row.ExpiresAt.UTC().Format(time.RFC3339))
	sess := q.sessionOfLocked(h.ServerID)
	if q.sessionOf == nil {
		sess = h.Session
	}
	if sess != nil {
		SendClientMessage(sess, fmt.Sprintf("A fever burns where the %s's claws found you.", strings.ToLower(h.NPCName)))
	}
	q.show(h.ServerID, true)
	return true
}

// show puts the character's infection in force on its player online, its name
// and time left said (fresh: as it is caught); false with none shown.
func (q *QuestEngine) show(serverID uint32, fresh bool) bool {
	if q.gw == nil || q.sessionOf == nil {
		return false
	}
	sess := q.sessionOf(serverID)
	if sess == nil {
		return false
	}
	now := time.Now()
	q.mu.Lock()
	inf := q.activeLocked(q.charLocked(serverID), now)
	var row database.Infection
	if inf != nil {
		row = *inf
	}
	q.mu.Unlock()
	if inf == nil {
		q.gw.TakeInfection(sess)
		return false
	}
	if !q.gw.PutInfection(sess, row.Line, row.ExpiresAt, now) {
		log.Printf("Weres: character %d's %s not shown (no player): at its login", serverID, entity.InfectionName(row.Line))
		return false
	}
	what := "You carry"
	if fresh {
		what = "You have contracted"
	}
	SendClientMessage(sess, fmt.Sprintf("%s the %s: %s left.", what, entity.InfectionName(row.Line), durationWords(row.ExpiresAt.Sub(now))))
	return true
}

// InfectionLogin puts the character's infection in force on its player as it
// enters the world (character.go's enterFromRecord: a login, a keep restart's
// resumption), and takes off one that is not; its markers put in step.
func (q *QuestEngine) InfectionLogin(sess *session.Session, p *entity.Player) {
	if q == nil || q.gw == nil || sess == nil || p == nil {
		return
	}
	now := time.Now()
	q.mu.Lock()
	inf := q.activeLocked(q.charLocked(p.CharServerID), now)
	var row database.Infection
	if inf != nil {
		row = *inf
	}
	q.mu.Unlock()
	if inf == nil {
		q.gw.TakeInfection(sess)
		return
	}
	if q.gw.PutInfection(sess, row.Line, row.ExpiresAt, now) {
		SendClientMessage(sess, fmt.Sprintf("You carry the %s: %s left.", entity.InfectionName(row.Line), durationWords(row.ExpiresAt.Sub(now))))
	}
}

// sweepInfections ends every infection whose seven days are over, online or
// not, with its markers (the line said to one online), and shows any in force
// on a player online that its effect is missing from. Each second.
func (q *QuestEngine) sweepInfections(now time.Time) {
	due, err := q.store.DueInfections(now)
	if err != nil {
		log.Printf("Weres: the infections' ends not read: %v", err)
	}
	for _, d := range due {
		q.mu.Lock()
		c := q.charLocked(d.ServerID)
		if c.infectionRead && c.infection != nil && c.infection.Key != "" && !c.infection.ExpiresAt.Equal(d.ExpiresAt) {
			c.infectionRead = false // a newer row than the one read: read again
		}
		ended, err := q.endLocked(c, database.InfectionExpired, d.ExpiresAt)
		sess := q.sessionOfLocked(d.ServerID)
		q.mu.Unlock()
		if err != nil || !ended {
			if err != nil {
				log.Printf("Weres: character %d's %s not ended: %v", d.ServerID, entity.InfectionName(d.Line), err)
			}
			continue
		}
		log.Printf("Weres: character %d's %s has run its course (%s)", d.ServerID, entity.InfectionName(d.Line), d.ExpiresAt.UTC().Format(time.RFC3339))
		if sess != nil {
			SendClientMessage(sess, fmt.Sprintf("The %s has run its course.", entity.InfectionName(d.Line)))
		}
	}
	if q.gw == nil || q.sessionOf == nil {
		return
	}
	var shown []uint32
	q.mu.Lock()
	for id, c := range q.chars {
		if c.infectionRead && c.infection.Active(now) {
			shown = append(shown, id)
		}
	}
	q.mu.Unlock()
	for _, id := range shown {
		sess := q.sessionOf(id)
		if sess == nil {
			continue
		}
		if line, _ := q.gw.Infection(sess); line == "" {
			q.show(id, false) // its effect missing: shown again
		}
	}
}

// durationWords is a time left as a player reads it: "6 days, 23 hours",
// "5 hours, 12 minutes", "40 seconds".
func durationWords(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= time.Minute {
		d = d.Round(time.Minute)
	}
	unit := func(n int, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	days, hours := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour)
	mins, secs := int(d%time.Hour/time.Minute), int(d%time.Minute/time.Second)
	switch {
	case days > 0:
		return unit(days, "day") + ", " + unit(hours, "hour")
	case hours > 0:
		return unit(hours, "hour") + ", " + unit(mins, "minute")
	case mins > 0:
		return unit(mins, "minute")
	}
	return unit(secs, "second")
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
// carries, and a cure offered for an infection. It reports whether the NPC was
// one.
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
	fever, turned := q.feverLineLocked(c, time.Now()), w.turnedOf(c)
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
	log.Printf("Weres: %s asked %s (infection %q, turned %q)", p.Name, speaker, fever, turned)
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
	fever, turned := q.feverLineLocked(c, time.Now()), w.turnedOf(c)
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
	q.mu.Lock()
	_, err := q.endLocked(c, database.InfectionCured, time.Now())
	q.mu.Unlock()
	if err != nil {
		log.Printf("Weres: %s's cure not kept: %v (the %d tunar back)", p.Name, err, CurePrice)
		spend(sess, p, -CurePrice)
		say("Something went wrong. Your tunar is returned.")
		return true
	}
	say(fmt.Sprintf("The %s's fever leaves your blood.", beast(fever)))
	economy.Spent(economy.Cure, p.CharServerID, p.Name, CurePrice)
	log.Printf("Weres: %s was cured of the %s by %s for %d tunar", p.Name, entity.InfectionName(fever), b.speaker, CurePrice)
	return true
}

// lineQuest is a quest's were line and part ("wolfwere", "p3"), "" for none.
func lineQuest(qu *database.Quest) (line, part string) {
	if qu == nil {
		return "", ""
	}
	parts := strings.Split(database.QuestImportKey(qu.Source), "/")
	if len(parts) != 3 || parts[0] != "Weres" {
		return "", ""
	}
	return parts[1], parts[2]
}

// antidoteLocked is a third quest declined: its giver's antidote takes the
// line's infection away (its offer's "Or take my antidote and stay as you are",
// its decline's "Then the fever passes"; the guides: "Do NOT take the
// antidote, or you lose your infection"). Must be called with q.mu held.
func (q *QuestEngine) antidoteLocked(c *questChar, qu *database.Quest) {
	line, part := lineQuest(qu)
	if part != "p3" || q.feverLineLocked(c, time.Now()) != line {
		return
	}
	if _, err := q.endLocked(c, database.InfectionAntidote, time.Now()); err != nil {
		log.Printf("Weres: the antidote's infection not taken away: %v", err)
		return
	}
	log.Printf("Weres: character %d took the %s's antidote: the infection passes", c.serverID, beast(line))
}

// turnedLocked is a third quest finished: the character has turned, its
// infection ends with it (the form is the quest's own). Must be called with
// q.mu held.
func (q *QuestEngine) turnedLocked(c *questChar, qu *database.Quest) {
	line, part := lineQuest(qu)
	if part != "p3" || q.activeLocked(c, time.Now()) == nil {
		return
	}
	if _, err := q.endLocked(c, database.InfectionTurned, time.Now()); err != nil {
		log.Printf("Weres: character %d's infection not ended at its turning: %v", c.serverID, err)
		return
	}
	log.Printf("Weres: character %d has turned %s: its infection ends", c.serverID, beast(line))
}
