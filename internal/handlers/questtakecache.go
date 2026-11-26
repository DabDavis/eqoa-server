package handlers

import (
	"reflect"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// questtakecache.go — what a marks pass keeps from the one before. The owner
// 2026-11-26: "can we optimize the CPU cost"; the quest marks were 23% of
// each shard's CPU (pprof, 30 s of the live Heirloom shard with 68 players;
// the Live shard 24%): every second, for every player, marksLocked asked
// mayTakeLocked of each of the ~2,470 quests (15%: refusals built with
// fmt.Sprintf and thrown away, UnmetRequire over the finished set), and read
// the player's pack from the database for a hand-in's count (6-7%), though
// almost every pass sends nothing (the set is the last one sent).
//
// Two things are kept per character, each exactly as long as what it was made
// from stands, so every pass's set is the one a pass that kept nothing makes
// (TestQuestTakeCacheMatchesAFreshPass holds them byte for byte):
//
//   - takeCands: the quests the character could take as far as the tests of
//     mayTakeAtLocked that read only the catalog, its level, class and race,
//     and its quests' steps and finishings say: enabled with steps, its
//     levels, its classes and races, the quests it requires finished, its
//     excluding twins neither finished nor under way, not under way itself,
//     not finished unless repeatable, not standing finished
//     (standsFinishedLocked). Each is a refusal mayTakeAtLocked makes too, and
//     before any test that reads anything else (a hold, the season, the day,
//     a fever, the pack, a standing), so a quest refused here is refused
//     there whatever the rest holds, and with nothing read on the way that a
//     later test or ask would see differently. The rest of mayTakeLocked is
//     still asked, every pass, of the quests kept: those tests are the ones
//     that move with time or with what is not watched here. A quest whose
//     requirements, twins or own row are a were fever marker's (wereinfect.go:
//     finished by the infection in force, a row taken when its infection is
//     first read, in a pass) is always kept, for mayTakeLocked to ask. Made
//     again when the catalog is another (a Reload), the level, class, race (or
//     the race it passes as: racedefs) changes, any of the character's quest
//     rows is added, taken or moves a step or a finishing, and at least every
//     takeCandsMaxAge besides.
//   - packCache: the pack's rows as HandInRows read them, kept while the
//     character's pack version stands (database/invversion.go: SQLite's own
//     hooks on every write of charinventory in this process, PRAGMA
//     data_version for another process's), read InventorySettle after its
//     last change, and at most packMaxAge besides.

// takeCandsMaxAge is how long a takeCands stands at most with nothing it was
// made from changed: a net under the watched inputs, not a freshness any
// test of it needs.
const takeCandsMaxAge = 60 * time.Second

// packMaxAge is how long a kept pack read stands at most with its version
// unchanged: a net under the hooks.
const packMaxAge = 30 * time.Second

// The invalidations a test may switch off (QuestEngine.cacheIgnore), to show
// that the equivalence test catches each one's loss.
const (
	ignoreLevel uint8 = 1 << iota
	ignoreQuestRows
	ignoreCatalog
	ignorePackVersion
)

// takeCands is a character's quests still to be asked of mayTakeLocked, by
// giver (q.cat.givers' keys and order), and what they were made from.
type takeCands struct {
	cat                     uintptr // q.cat.byID's identity: the catalog
	level, class, race, wre int
	quests                  map[int]cqKey
	at                      time.Time
	byGiver                 map[string][]*database.Quest
}

// cqKey is what of a character's quest row the kept tests read.
type cqKey struct{ step, done int }

// packCache is a kept read of the character's pack, and heldItems' rows
// made of it (once made: never written after).
type packCache struct {
	rows   []database.CharInventoryEntry
	seq    uint64
	readAt time.Time
	held   []database.CharInventoryEntry
	heldOK bool
}

// catIdentity is the catalog's identity: a Reload makes a new byID.
func (q *QuestEngine) catIdentity() uintptr {
	return reflect.ValueOf(q.cat.byID).Pointer()
}

// takeCandsLocked is the character's quests to ask mayTakeLocked of, by
// giver, made again when what they were made from changed; nil when none
// are kept (q.noTakeCache): every quest is to be asked. Must be called with
// q.mu held.
func (q *QuestEngine) takeCandsLocked(p *entity.Player, c *questChar, now time.Time) map[string][]*database.Quest {
	if q.noTakeCache || p == nil || c == nil {
		return nil
	}
	ign := q.cacheIgnore
	if k := c.takes; k != nil && (k.cat == q.catIdentity() || ign&ignoreCatalog != 0) &&
		(k.level == p.Level || ign&ignoreLevel != 0) && k.class == p.Class && k.race == p.Race && k.wre == entity.WireRace(p.Race) &&
		now.Sub(k.at) < takeCandsMaxAge && now.Sub(k.at) >= 0 && (sameQuestRows(k.quests, c.quests) || ign&ignoreQuestRows != 0) {
		return k.byGiver
	}
	markers := q.wereQuestsLocked().markerLine
	k := &takeCands{cat: q.catIdentity(), level: p.Level, class: p.Class, race: p.Race, wre: entity.WireRace(p.Race), at: now,
		quests: make(map[int]cqKey, len(c.quests)), byGiver: map[string][]*database.Quest{}}
	for id, cq := range c.quests {
		k.quests[id] = cqKey{cq.Step, cq.Done}
	}
	for g, list := range q.cat.givers {
		for _, qu := range list {
			if !q.takeRefusedLocked(p, c, qu, markers) {
				k.byGiver[g] = append(k.byGiver[g], qu)
			}
		}
	}
	c.takes = k
	return k.byGiver
}

// sameQuestRows is whether the character's quest rows are the ones kept.
func sameQuestRows(kept map[int]cqKey, now map[int]*database.CharQuest) bool {
	if len(kept) != len(now) {
		return false
	}
	for id, cq := range now {
		if k, ok := kept[id]; !ok || k != (cqKey{cq.Step, cq.Done}) {
			return false
		}
	}
	return true
}

// takeRefusedLocked is whether mayTakeLocked refuses the quest by a test
// takeCands watches the inputs of (above), whatever else holds. Must be
// called with q.mu held.
func (q *QuestEngine) takeRefusedLocked(p *entity.Player, c *questChar, qu *database.Quest, markers map[int]string) bool {
	switch {
	case !qu.Enabled, qu.First() == nil:
		return true
	case qu.MinLevel > 0 && p.Level < qu.MinLevel, qu.MaxLevel > 0 && p.Level > qu.MaxLevel:
		return true
	case len(qu.Classes) > 0 && !containsInt(qu.Classes, p.Class):
		return true
	case len(qu.Races) > 0 && !questRaceTakes(qu.Races, p.Race):
		return true
	}
	// a fever marker's: mayTakeLocked's alone (a row a pass may take)
	if _, ok := markers[qu.ID]; ok {
		return false
	}
	for _, g := range qu.Required() {
		for _, id := range g {
			if _, ok := markers[id]; ok {
				return false
			}
		}
	}
	for _, id := range qu.Excludes {
		if _, ok := markers[id]; ok {
			return false
		}
	}
	finished := func(id int) bool {
		return c.hasFinished(id) || q.standsFinishedLocked(p, c, q.cat.byID[id])
	}
	if _, unmet := database.UnmetRequire(qu.Required(), finished); unmet {
		return true
	}
	for _, id := range qu.Excludes {
		if id == qu.ID {
			continue
		}
		if finished(id) {
			return true
		}
		if cq := c.quests[id]; cq != nil && cq.Active() {
			return true
		}
	}
	if cq := c.quests[qu.ID]; cq != nil && (cq.Active() || cq.Done > 0 && !qu.Repeatable) {
		return true
	}
	return !qu.Repeatable && q.standsFinishedLocked(p, c, qu)
}

// packReadLocked is the player's pack's rows as HandInRows reads them: the
// character's kept read while its version stands (packCache), else read and
// kept; and the kept read they are (nil: not kept). Must be called with q.mu
// held.
func (q *QuestEngine) packReadLocked(p *entity.Player) ([]database.CharInventoryEntry, *packCache, error) {
	id := p.CharServerID
	c := q.chars[id]
	if q.noPackCache || c == nil {
		rows, err := q.inv.HandInRows(id)
		return rows, nil, err
	}
	now := time.Now()
	q.inv.NoteOtherWriters(now) // a data tool's write: every version moves
	st, ok := database.InventoryVersion(id)
	if k := c.pack; ok && k != nil && (k.seq == st.Seq && st.Settled(k.readAt) || q.cacheIgnore&ignorePackVersion != 0) &&
		now.Sub(k.readAt) < packMaxAge && now.Sub(k.readAt) >= 0 {
		return k.rows, k, nil
	}
	rows, err := q.inv.HandInRows(id)
	if err != nil || !ok {
		c.pack = nil
		return rows, nil, err
	}
	c.pack = &packCache{rows: rows, seq: st.Seq, readAt: now}
	return rows, c.pack, nil
}
