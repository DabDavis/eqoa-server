package handlers

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/session"
)

// archivistunlock.go — the masteries the Spell Archivist opens (petition
// 1391, Jibrish, a GM, 2026-11-09: "Spell archivist should unlock the follow
// CM's: Bulging muscles, intestinal fortitude, Superior intellect"). Each rank
// of those General lines (STR, STA and INT, ranks 1-4 at 20, 30, 40 and 50)
// needs its trial finished before it can be bought (cmcatalog.require_quests,
// entity/cmbuy.go's gate reading the quests a character finished; the
// stat-boost quests of EQOA-Data's Misc guide, tools/gate_stat_masteries.py).
// "Unlock my masteries" marks every such trial of the lines' ranks offered to
// the character's class and race finished, as a GM's "complete" does
// (QuestEngine.SetCharacterQuest: no reward, a trial under way leaves the
// journal), and says so: the ranks are then its to buy with mastery points in
// the mastery window, each from its level and after the one before, as ever.
// No rank is given and no point spent. A trial not in the game bars no one
// already and is left; one finished already is left as it is. The four other
// stat lines (Avoid Danger, Unrivaled Grace, Sages Wit, Razzle Dazzle) keep
// their trials.

// archivistUnlockLines are the lines he opens, by their ranks' names less the
// rank's number (cmcatalog's mastery column spells the first "Buldging").
var archivistUnlockLines = []string{"Bulging Muscles", "Intestinal Fortitude", "Superior Intellect"}

// reRankNumber is a rank's number at the end of its name ("Bulging Muscles 2").
var reRankNumber = regexp.MustCompile(`\s+\d+$`)

// What he says of them (ours; a box shows 217 characters, his name included).
const (
	archivistUnlockAnswer = "Unlock my masteries"
	archivistUnlocked     = "There: the trials of %s are behind you. Their ranks are yours to buy with mastery points in your mastery window, each from its level."
	archivistUnlockedAll  = "The trials of %s are behind you already: their ranks are yours to buy in your mastery window, each from its level."
	archivistUnlockNone   = "I keep no mastery's trials for you."
)

// unlockLine is the line of archivistUnlockLines a rank is of, or "".
func unlockLine(name string) string {
	base := strings.TrimSpace(reRankNumber.ReplaceAllString(strings.TrimSpace(name), ""))
	for _, l := range archivistUnlockLines {
		if strings.EqualFold(base, l) {
			return l
		}
	}
	return ""
}

// unlockQuests is what "Unlock my masteries" does for a character of the
// class and race: the lines it has ranks of (in archivistUnlockLines' order),
// and the trials to mark finished, in id order: of each of those ranks' quest
// groups that the character has not met (finished; a group with no quest in
// the game bars no one), its first quest in the game.
func unlockQuests(arch *spellArchive, inGame map[int]string, finished map[int]bool, class, race int) (lines []string, mark []int) {
	if arch == nil {
		return nil, nil
	}
	has, marked := map[string]bool{}, map[int]bool{}
	for _, r := range arch.ranks {
		line := unlockLine(r.Name)
		if line == "" || !r.OfferedTo(class, race) {
			continue
		}
		has[line] = true
		for _, g := range r.QuestGroups {
			met, first := false, 0
			for _, id := range g {
				if _, in := inGame[id]; !in {
					continue
				}
				if finished[id] {
					met = true
					break
				}
				if first == 0 {
					first = id
				}
			}
			if !met && first != 0 && !marked[first] {
				marked[first] = true
				mark = append(mark, first)
			}
		}
	}
	for _, l := range archivistUnlockLines {
		if has[l] {
			lines = append(lines, l)
		}
	}
	sort.Ints(mark)
	return lines, mark
}

// unlockMasteries is "Unlock my masteries" (the notes at the top).
func (a *Archivist) unlockMasteries(sess *session.Session, id uint32) {
	p, _, ok := a.here(sess, id)
	if !ok {
		return
	}
	qe := a.quests()
	a.book.Lock()
	arch, err := a.archived()
	a.book.Unlock()
	if err != nil || qe == nil {
		log.Printf("Archivist: %s's masteries: the archive %v, quests running %v", p.Name, err, qe != nil)
		a.say(sess, archivistTrouble)
		return
	}
	lines, mark := unlockQuests(arch, qe.InGame(), qe.FinishedBy(p.CharServerID), p.Class, p.Race)
	if len(lines) == 0 {
		a.say(sess, archivistUnlockNone)
		return
	}
	if len(mark) == 0 {
		a.say(sess, fmt.Sprintf(archivistUnlockedAll, joinAnd(lines)))
		return
	}
	var done []int
	for _, q := range mark {
		if _, err := qe.SetCharacterQuest(p.CharServerID, q, QuestComplete, 0); err != nil {
			log.Printf("Archivist: %s's trial %d not marked finished: %v", p.Name, q, err)
			continue
		}
		done = append(done, q)
	}
	log.Printf("Archivist: %s (class %d, level %d) opened %s: trials %v marked finished (%d of %d)",
		p.Name, p.Class, p.Level, strings.Join(lines, ", "), done, len(done), len(mark))
	if len(done) < len(mark) {
		a.say(sess, archivistTrouble)
		return
	}
	a.say(sess, fmt.Sprintf(archivistUnlocked, joinAnd(lines)))
}
