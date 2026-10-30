package handlers

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
)

// A character's paths, as a rehabilitator undoes them (rehab.go). The user,
// 2026-10-30: a path respec undoes "the level 20 pick (the class quest's
// choice of two scrolls)" and "the 30/40/49 path quests". In the quests (the
// import's keys, tools/quests/data/) those picks are:
//
//   - level 20: a city's level 20 class quest (1-20/<city>/<class>/L20),
//     whose last step asks the guide's choice of two paths, a scroll each
//     (tools/fill_class_quest_rewards.py). One that kept a single path asks
//     nothing, so there is no pick of it to undo;
//   - levels 30, 40 and 49: a 30/40/49 group's class reward
//     (30,40,49/<group>/L<30|40|49>-<class>): a talk step or two asking for
//     an ability, a weapon, or both.
//
// Undoing a level's path undoes its quest and, at 30 and 40, the later
// levels' of the same group ("the 30/40/49 branch quest and the ones after
// it in that branch"). Their rows are forgotten (QuestReset), so their givers
// offer them again and the player picks again, the same or the other. What
// they gave goes back: the spells out of the book (0x00B8), the items taken
// where the character holds them. A step's kept pick (charquests.picks) says
// what it gave; a step done before picks were kept counts all its options,
// so what the book and the pack hold of them goes. A spell another quest the
// character keeps gives too (the 60 epics offer the 49 paths' other
// abilities), or that a merchant sells as a scroll, stays.

// The import keys of the quests that ask a path's pick.
var (
	reL20Key  = regexp.MustCompile(`^1-20/[^/]+/([a-z]+)/L20$`)
	rePathKey = regexp.MustCompile(`^30,40,49/(group-[^/]+)/L(30|40|49)-([a-z]+)$`)
)

// pathClasses are the classes as the import's keys spell them.
var pathClasses = map[string]bool{
	"warrior": true, "ranger": true, "paladin": true, "shadowknight": true, "monk": true, "bard": true,
	"rogue": true, "druid": true, "shaman": true, "cleric": true, "magician": true, "necromancer": true,
	"enchanter": true, "wizard": true, "alchemist": true,
}

// importKey is a quest's import key, read off its source ("EQOA-Data
// 1-20/freeport/warrior/L20 (tools/import_quests.py)"), or "".
func importKey(source string) string {
	rest, ok := strings.CutPrefix(strings.TrimSpace(source), "EQOA-Data ")
	if !ok || !strings.HasSuffix(rest, "(tools/import_quests.py)") {
		return ""
	}
	key, _, _ := strings.Cut(rest, " ")
	return key
}

// pathOf is the path a quest asks the pick of: its level and its 30/40/49
// group ("" at level 20); false for any other quest, and for one of those
// keys that asks no choice.
func pathOf(qu *database.Quest) (level int, group string, ok bool) {
	if qu == nil {
		return 0, "", false
	}
	key := importKey(qu.Source)
	if m := reL20Key.FindStringSubmatch(key); m != nil && pathClasses[m[1]] {
		level = 20
	} else if m := rePathKey.FindStringSubmatch(key); m != nil && pathClasses[m[3]] {
		level, _ = strconv.Atoi(m[2])
		group = m[1]
	} else {
		return 0, "", false
	}
	for i := range qu.Steps {
		if len(qu.Steps[i].Choices.Options) > 0 {
			return level, group, true
		}
	}
	return 0, "", false
}

// pathQuest is a quest asking a path's pick that a character has a row of.
type pathQuest struct {
	level int
	group string
	quest *database.Quest // the catalog's, never written once read
	row   database.CharQuest
}

// pathQuests is every quest asking a path's pick that the character has a
// row of, done or under way, by level, then id.
func (q *QuestEngine) pathQuests(serverID uint32) []pathQuest {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.charLocked(serverID)
	var out []pathQuest
	for id, cq := range c.quests {
		qu := q.cat.byID[id]
		level, group, ok := pathOf(qu)
		if !ok {
			continue
		}
		out = append(out, pathQuest{level: level, group: group, quest: qu, row: *cq})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].level != out[j].level {
			return out[i].level < out[j].level
		}
		return out[i].quest.ID < out[j].quest.ID
	})
	return out
}

// passed is whether a quest's row is past one of its steps: every step of a
// finished quest, the steps before the one to do of one under way.
func passed(row database.CharQuest, step int) bool {
	return row.Done > 0 || (row.Active() && step < row.Step)
}

// optionPick is an option a quest's row may have been given: kept is
// whether its pick was kept (else it is one of a step's options, any of
// which may have been the one).
type optionPick struct {
	database.StepOption
	kept bool
}

// pickedOptions is what a quest's row was given at its choices: at each step
// passed, the option it kept the pick of, else every option (a pick from
// before picks were kept). An option that stays did nothing.
func pickedOptions(qu *database.Quest, row database.CharQuest) []optionPick {
	picks := database.ParsePicks(row.Picks)
	var out []optionPick
	for i := range qu.Steps {
		st := &qu.Steps[i]
		opts := st.Choices.Options
		if len(opts) == 0 || !passed(row, st.Step) {
			continue
		}
		if k, ok := picks[st.Step]; ok && k < len(opts) {
			if !opts[k].Stay {
				out = append(out, optionPick{opts[k], true})
			}
			continue
		}
		for _, o := range opts {
			if !o.Stay {
				out = append(out, optionPick{o, false})
			}
		}
	}
	return out
}

// pickedAnswers is the answers a quest's row kept the picks of, by step;
// none when it kept none.
func pickedAnswers(qu *database.Quest, row database.CharQuest) []string {
	picks := database.ParsePicks(row.Picks)
	var out []string
	for i := range qu.Steps {
		st := &qu.Steps[i]
		if k, ok := picks[st.Step]; ok && k < len(st.Choices.Options) && passed(row, st.Step) {
			out = append(out, st.Choices.Options[k].Answer)
		}
	}
	return out
}

// spellsKept is the spells the character's quests give but those in except:
// for each quest it has a row of, at each step passed, the step's own spells
// for its class and its choice's (the kept pick's, else every option's).
func (q *QuestEngine) spellsKept(serverID uint32, class int, except map[int]bool) map[int]bool {
	out := map[int]bool{}
	if q == nil {
		return out
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.charLocked(serverID)
	for id, cq := range c.quests {
		qu := q.cat.byID[id]
		if qu == nil || except[id] {
			continue
		}
		for i := range qu.Steps {
			if passed(*cq, qu.Steps[i].Step) {
				for _, sp := range qu.Steps[i].RewardFor(class).Spells {
					out[sp] = true
				}
			}
		}
		for _, o := range pickedOptions(qu, *cq) {
			for _, sp := range o.Reward.Spells {
				out[sp] = true
			}
		}
	}
	return out
}

// forgetQuests forgets a character's rows of the quests (QuestReset): each
// may be taken again, and a quest under way leaves its journal.
func (q *QuestEngine) forgetQuests(serverID uint32, ids []int) error {
	for _, id := range ids {
		if _, err := q.SetCharacterQuest(serverID, id, QuestReset, 0); err != nil {
			return err
		}
	}
	return nil
}
