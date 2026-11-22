package handlers

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// A guarded quest-definition edit (board entry 1250; the GM panel's
// quest_definition_patch, internal/gmweb/ops_questpatch.go): a reviewed manifest of
// rows changed in one transaction, the quests it touches held out of play from
// before the first check until the catalog is published once, and never while
// anyone has them.
//
// Lock order: defMu (every publication of the catalog: Reload and a guarded edit,
// so a load begun earlier can never publish over a newer one), then QuestEngine.mu
// (taken and let go, never held across the database's work), then the database.
// Reload's own load runs under defMu and takes q.mu only to swap the catalog in, so
// nothing here calls a function that takes defMu while holding it (publishLocked is
// Reload without the lock).

// ErrQuestInPlay refuses a guarded edit of a quest someone has (under way or
// finished: a row in charquests, or in a character's memory).
var ErrQuestInPlay = errors.New("a character has the quest")

// ErrQuestUnpublished is a guarded edit committed whose catalog could not be
// published: its quests stay held until a reload succeeds (ReloadQuests).
var ErrQuestUnpublished = errors.New("committed, but the quests could not be read again")

// questHeldLine is what a player is told of a quest held for an edit (ours).
const questHeldLine = "That is being changed just now. Try again shortly."

// heldLocked is whether the quest is held out of play. Must be called with q.mu held.
func (q *QuestEngine) heldLocked(id int) bool { return q.held[id] }

// PublishGen counts the catalog's publications (each Reload, each guarded edit).
func (q *QuestEngine) PublishGen() uint64 { return q.pubGen.Load() }

// InPlay names the characters with any of the quests in memory (a row read or
// written this run, saved or not), sorted, as "character <id> quest <id>".
func (q *QuestEngine) InPlay(ids []int) []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.inPlayLocked(ids)
}

func (q *QuestEngine) inPlayLocked(ids []int) []string {
	var out []string
	for sid, c := range q.chars {
		for _, id := range ids {
			if c.quests[id] != nil {
				out = append(out, fmt.Sprintf("character %d quest %d", sid, id))
			}
		}
	}
	sort.Strings(out)
	return out
}

// GuardedEdit holds the quests ids out of play (no offer, accept, answer to an old
// box, step or hand-in of them), refuses when anyone has one in memory, runs commit
// (the caller's transaction: its own checks of the rows and of the database's
// characters, then the writes; an error from it changes nothing here), then reads
// the quests and publishes them once, and lets the quests go. It answers the
// publication's generation. A publication that fails after the commit keeps the
// quests held (ErrQuestUnpublished) until a later reload succeeds.
func (q *QuestEngine) GuardedEdit(ids []int, commit func() error) (uint64, error) {
	if q == nil {
		return 0, errors.New("the quests are not running")
	}
	q.defMu.Lock()
	defer q.defMu.Unlock()
	q.mu.Lock()
	if who := q.inPlayLocked(ids); len(who) > 0 {
		q.mu.Unlock()
		return 0, fmt.Errorf("%w: %s", ErrQuestInPlay, strings.Join(who, ", "))
	}
	if q.held == nil {
		q.held = map[int]bool{}
	}
	mine := map[int]bool{}
	for _, id := range ids {
		if !q.held[id] {
			mine[id] = true
		}
		q.held[id] = true
	}
	q.mu.Unlock()
	letGo := func() {
		q.mu.Lock()
		for id := range mine {
			delete(q.held, id)
		}
		q.mu.Unlock()
	}
	if err := commit(); err != nil {
		letGo()
		return 0, err
	}
	if err := q.publishLocked(); err != nil {
		q.mu.Lock()
		if q.unpublished == nil {
			q.unpublished = map[int]bool{}
		}
		for _, id := range ids {
			q.unpublished[id] = true
		}
		q.mu.Unlock()
		return 0, fmt.Errorf("%w (%v): quests %v stay out of play until a quest reload succeeds", ErrQuestUnpublished, err, ids)
	}
	letGo()
	return q.pubGen.Load(), nil
}
