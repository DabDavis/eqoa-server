package handlers

import (
	"log"
	"math/rand"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A quest giver whose next quest for the player waits on the player's level
// alone says so, in its city's words (the user, 2026-10-30: "for the early
// quest givers if they are the next quest in line, but the player is too low
// level like dust is level 6 and cannot take level 7 q yet let the quest npc
// say come back when you are stronger, but word it in the city flavor
// language"). Until then such a giver answered as if it had no quests: Talk
// found no step and no quest the player might take, so it said what any of
// its town's people say (npcdialog), or nothing. queststeps.notyet is not this
// case: it is a show or give step's answer while its items lack.
//
// The quest is one the giver gives whose every requirement the player meets
// but its lowest level (mayTakeAtLocked at that level: its quests first,
// class, race, standings; not under way, not finished unless repeatable), the
// nearest in level when there are several; and only when the giver has
// nothing else for the player: no step of its to do, no quest of its to take
// now, and no business of its own (a bind, a route, its words: HandleInteract
// asks NotYet after them, so a waysender giving a Portal quest still sends).
//
// Its words are questnotyet's for the giver's city (database/questnotyet.go):
// the starting city of the quest's import key ("1-20/neriak-nektulos/..." is
// Neriak's), Weres for a were quest, else the town the giver stands in (its
// faction's, entity.DialogTown, as town dialog's; else the coach map's,
// entity.TownAt), one at random and never the one said to the player last;
// a city with none says one of the lines of no city, and with no line at all
// questNotStrong. A line is the giver's, its name before it, in dialogue boxes
// (tellLocked: 217 characters a box), as a quest's other words are.

// questNotStrong is a giver's "not yet" when questnotyet has no line for it
// (ours, as the table's line of no city).
const questNotStrong = "You are not ready for what I would ask of you. Come back when you are stronger."

// NotYet is HandleInteract's question once nothing else has answered a talk
// (no step, no quest to take, no bind, route or words of the NPC's own): a
// giver whose next quest for the player waits on its level alone says its
// city's "not yet" (the file's comment). It reports whether it spoke.
func (q *QuestEngine) NotYet(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if q == nil || p == nil || npc == nil || npc.Dead {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	qu := q.notYetLocked(q.lookAt(sess), p, c, npc)
	if qu == nil {
		return false
	}
	c.box = nil
	q.sayNotYetLocked(sess, p, c, npc, qu)
	return true
}

// notYetLocked is the quest npc gives that waits on the player's level alone,
// the nearest in level, or nil: none, or npc has something else for the
// player (a step of its to do, a quest of its to take now).
func (q *QuestEngine) notYetLocked(look *factionLook, p *entity.Player, c *questChar, npc *entity.NPC) *database.Quest {
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		if st := qu.Step(cq.Step); st != nil && database.AtNPC(st.Kind) && stepNPC(st, npc.Name, npc.ServerID) {
			return nil // a step to do here, which Talk (or the window) does or says waits
		}
	}
	var best *database.Quest
	for _, qu := range q.cat.givers[foldName(npc.Name)] {
		if q.mayTakeLocked(look, p, c, qu) == "" {
			return nil // one to take now, which Talk offers
		}
		if qu.MinLevel <= p.Level || (best != nil && qu.MinLevel >= best.MinLevel) {
			continue
		}
		if q.mayTakeAtLocked(look, p, c, qu, qu.MinLevel) == "" {
			best = qu
		}
	}
	return best
}

// sayNotYetLocked says the giver's "not yet" for the quest: a line of its
// city's, its name before it.
func (q *QuestEngine) sayNotYetLocked(sess *session.Session, p *entity.Player, c *questChar, npc *entity.NPC, qu *database.Quest) {
	city := q.questCity(qu, npc)
	line, id := q.notYetLine(city, c.notYetLast)
	c.notYetLast = id
	speaker := strings.TrimSpace(qu.Giver)
	if speaker == "" {
		speaker = npc.Name
	}
	q.tellLocked(c, sess, speaker, line)
	where := city
	if where == "" {
		where = "no city"
	}
	log.Printf("Quests: %s, level %d, is short of %q (level %d): %s (%s) says to come back stronger", p.Name, p.Level, qu.Name, qu.MinLevel, speaker, where)
}

// questCity is the city whose words npc says for the quest (the file's
// comment), "" none known.
func (q *QuestEngine) questCity(qu *database.Quest, npc *entity.NPC) string {
	if city := importCity(qu.Source); city != "" {
		return city
	}
	if q.gw != nil {
		if town := entity.DialogTown(q.gw.NPCFaction(npc)); town != "" {
			return town
		}
	}
	if town, ok := entity.TownAt(npc.World, npc.SpawnX, npc.SpawnZ); ok {
		return town
	}
	return ""
}

// importCity is the city a quest's import key names, read off its source: the
// content import's ("EQOA-Data 1-20/neriak-nektulos/shadowknight/L7
// (tools/import_quests.py)") or the GM panel's draft's ("EQOA-Data
// Quests/1-20/freeport/warrior.json level 3"). A starting city's guide gives
// its city ("neriak-nektulos"), a were guide Weres, any other guide or a GM's
// own quest "".
func importCity(source string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(source), "EQOA-Data ")
	if !ok {
		return ""
	}
	key, _, _ := strings.Cut(rest, " ")
	key = strings.TrimPrefix(key, "Quests/")
	parts := strings.Split(key, "/")
	switch {
	case len(parts) >= 3 && parts[0] == "1-20" && parts[1] != "":
		return parts[1]
	case len(parts) >= 2 && strings.EqualFold(parts[0], "Weres"):
		return "Weres"
	}
	return ""
}

// notYetLine is a line of questnotyet for the city, at random, never the one
// said last (its id) while another fits: the city's own, else those of no
// city, else questNotStrong; and its id (0 for questNotStrong).
func (q *QuestEngine) notYetLine(city string, last int) (string, int) {
	var all []database.QuestNotYetLine
	if q.notYetLines != nil {
		var err error
		if all, err = q.notYetLines(); err != nil {
			log.Printf("Quests: questnotyet: %v", err)
		}
	}
	key := database.QuestCityKey(city)
	var own, none []database.QuestNotYetLine
	for _, l := range all {
		if strings.TrimSpace(l.Line) == "" {
			continue
		}
		switch k := database.QuestCityKey(l.City); {
		case k == "":
			none = append(none, l)
		case key != "" && k == key:
			own = append(own, l)
		}
	}
	lines := own
	if len(lines) == 0 {
		lines = none
	}
	if len(lines) == 0 {
		return questNotStrong, 0
	}
	if len(lines) > 1 && last != 0 {
		kept := lines[:0:0]
		for _, l := range lines {
			if l.ID != last {
				kept = append(kept, l)
			}
		}
		if len(kept) > 0 {
			lines = kept
		}
	}
	l := lines[rand.Intn(len(lines))]
	return strings.TrimSpace(l.Line), l.ID
}
