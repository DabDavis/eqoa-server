package handlers

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// A season's currency (the owner, 2026-11-16: "the weapon enchanter will cost
// christmas currency for our christmas event. the tree is the beginning of that
// event"; table seasonrewards, database/seasonrewards.go): while its season is
// on (gmseasons, the Christmas Season trigger) a player earns its token four
// ways, and none while it is off (the tokens kept stay kept):
//
//   - a kill's drop: drop_pct of the kills of a creature the player does not see
//     green, named_pct of a named one's (a name SOE wrote capitalised: a
//     generic creature's is lower case), each credited player rolling its own,
//     on the corpse for anyone who may loot it (questloot.go's shared drops:
//     the kill group, then anyone once it unlocks), every roll's tokens one
//     stack (2026-11-16: "lootable by anyone with kill credit, not a
//     specific player. This is crushing multiboxers"; each character's view
//     held its own until then);
//   - a gift once a day (UTC) for each character, gift_count of them, on talking
//     to its gift NPC (the Holiday Elf by the tree): the talk goes on to the
//     elf's quests (the holiday quests, daily: quests.repeatable 2);
//   - playtime: hourly tokens for each full hour in the world, the minutes kept
//     in seasonplay over logouts and restarts.
//
// The season's quests pay their own rewards (queststeps.rewarditems) and stand
// with their giver, who is the season's (npcseason).

// seasonRewardsTick is how often the playtime is counted and the seasons read.
const seasonRewardsTick = time.Minute

var seasonRewards atomic.Pointer[SeasonRewards]

// Rewards is the running season rewards, or nil.
func Rewards() *SeasonRewards { return seasonRewards.Load() }

// SeasonRewards gives the seasons' currencies.
type SeasonRewards struct {
	db    *database.SQLiteDB
	gw    *entity.World
	sm    *session.Manager
	inv   *database.InventoryStore
	items *database.ItemPatternStore
	// drop puts an item on a corpse for whoever may loot it (questloot.go's
	// AddSharedDrop)
	drop func(id entity.EntityID, died time.Time, rec database.CharInventoryEntry)

	mu     sync.Mutex
	active []database.SeasonReward // the rows of the seasons that are on
	kills  chan entity.KillCredit
	stop   chan struct{}
	roll   func() float64
	now    func() time.Time
	// played is each character's seconds toward its next hour, by season (as
	// seasonplay holds them, written each tick)
	played map[string]map[uint32]int
}

// NewSeasonRewards makes the rewards over db; Start sets them running.
func NewSeasonRewards(db *database.SQLiteDB, gw *entity.World, sm *session.Manager, inv *database.InventoryStore) *SeasonRewards {
	r := &SeasonRewards{db: db, gw: gw, sm: sm, inv: inv, kills: make(chan entity.KillCredit, 512), stop: make(chan struct{}),
		roll: rand.Float64, now: time.Now, played: map[string]map[uint32]int{}}
	if db != nil {
		r.items = database.NewItemPatternStore(db)
	}
	r.Refresh()
	return r
}

// Refresh reads which seasons are on and their rewards: each tick, and at once
// when a season turns (gmtriggers.go's seasonLocked).
func (r *SeasonRewards) Refresh() {
	if r == nil || r.db == nil {
		return
	}
	rows, err := r.db.ActiveSeasonRewards()
	if err != nil {
		log.Printf("SeasonRewards: %v", err)
		return
	}
	r.mu.Lock()
	r.active = rows
	r.mu.Unlock()
}

func (r *SeasonRewards) activeRows() []database.SeasonReward {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]database.SeasonReward(nil), r.active...)
}

// Start hears the kills, counts the playtime once a minute, and makes this the
// running rewards.
func (r *SeasonRewards) Start() {
	if r.gw != nil {
		r.gw.OnKill(func(k entity.KillCredit) {
			select {
			case r.kills <- k:
			default: // never block the World
			}
		})
	}
	go func() {
		t := time.NewTicker(seasonRewardsTick)
		defer t.Stop()
		for {
			select {
			case <-r.stop:
				return
			case k := <-r.kills:
				r.Killed(k)
			case <-t.C:
				r.Refresh()
				r.Tick(seasonRewardsTick)
			}
		}
	}()
	seasonRewards.Store(r)
}

// Stop ends the worker.
func (r *SeasonRewards) Stop() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	seasonRewards.CompareAndSwap(r, nil)
}

// Named is whether a creature's name is an individual's (capitalised as SOE
// wrote it: "Lord Nagafen", where a generic creature's is "a decaying
// skeleton", "large rat").
func Named(name string) bool {
	name = strings.TrimSpace(name)
	for _, c := range name {
		return unicode.IsUpper(c)
	}
	return false
}

// Killed rolls each credited player's drop of every season that is on: none of
// a creature the player sees green.
func (r *SeasonRewards) Killed(k entity.KillCredit) {
	rows := r.activeRows()
	if len(rows) == 0 || r.drop == nil {
		return
	}
	for _, kp := range k.Players {
		var p *entity.Player
		if kp.Session != nil && r.gw != nil {
			p = r.gw.SessionPlayer(kp.Session)
		}
		if p == nil || entity.ConColor(p.Level, k.Level) == entity.ConGreen {
			continue
		}
		for _, row := range rows {
			pct := row.DropPct
			if Named(k.NPCName) {
				pct = row.NamedPct
			}
			if pct <= 0 || r.roll()*100 >= pct {
				continue
			}
			pat := r.pattern(row.Item)
			if pat == nil {
				continue
			}
			r.drop(k.NPCID, k.Died, database.CharInventoryEntry{StackLeft: 1, EquipLoc: -1, Pattern: *pat})
			log.Printf("SeasonRewards: %s: %q dropped %q on %s's roll, for its kill group", row.Season, k.NPCName, pat.ItemName, kp.Name)
		}
	}
}

func (r *SeasonRewards) pattern(id int) *database.ItemPatternRecord {
	if r.items == nil {
		return nil
	}
	p, err := r.items.GetItemPattern(id)
	if err != nil || p == nil {
		log.Printf("SeasonRewards: item %d: %v", id, err)
		return nil
	}
	return p
}

// give hands n of item to the player and tells it why.
func (r *SeasonRewards) give(sess *session.Session, item, n int, why string) int {
	pat := r.pattern(item)
	if pat == nil || n < 1 {
		return 0
	}
	got, err := GiveItemNow(sess, r.gw, r.inv, *pat, n)
	if err != nil {
		log.Printf("SeasonRewards: giving %q: %v", pat.ItemName, err)
	}
	if got > 0 {
		SendClientMessage(sess, fmt.Sprintf("%s You have received %s.", why, countOf(got, pat.ItemName)))
	}
	if got < n {
		SendClientMessage(sess, fmt.Sprintf("Your inventory is full: %s could not be given.", countOf(n-got, pat.ItemName)))
	}
	return got
}

// Talk gives a character its gift of the day when it talks to a season's gift
// NPC. It never ends the talk: the NPC's quests and words go on.
func (r *SeasonRewards) Talk(sess *session.Session, p *entity.Player, npc *entity.NPC) {
	if r == nil || p == nil || npc == nil || r.db == nil {
		return
	}
	for _, row := range r.activeRows() {
		if row.GiftNPC == "" || database.NPCClassKey(row.GiftNPC) != database.NPCClassKey(npc.Name) || row.GiftCount < 1 {
			continue
		}
		day := r.now().UTC().Format("2006-01-02")
		first, err := r.db.TakeSeasonGift(row.Season, p.CharServerID, day)
		if err != nil {
			log.Printf("SeasonRewards: %s's gift for %s: %v", row.Season, p.Name, err)
			continue
		}
		if !first {
			SendClientMessage(sess, fmt.Sprintf("%s has given you today's gift already. Come back tomorrow!", npc.Name))
			continue
		}
		n := r.give(sess, row.Item, row.GiftCount, fmt.Sprintf("%s hands you a gift.", npc.Name))
		log.Printf("SeasonRewards: %s: %s's gift of the day to %s: %d", row.Season, npc.Name, p.Name, n)
	}
}

// Tick counts d of playtime for every character in the world, and gives each
// the hourly tokens of every season that is on for each full hour it has.
func (r *SeasonRewards) Tick(d time.Duration) {
	rows := r.activeRows()
	if len(rows) == 0 || r.sm == nil || r.gw == nil {
		return
	}
	type in struct {
		sess *session.Session
		p    *entity.Player
	}
	var who []in
	r.sm.ForEach(func(s *session.Session) bool {
		if p := r.gw.SessionPlayer(s); p != nil && r.gw.IsLivePlayer(p) {
			who = append(who, in{s, p})
		}
		return true
	})
	for _, row := range rows {
		if row.Hourly < 1 {
			continue
		}
		k := database.SeasonKey(row.Season)
		r.mu.Lock()
		got := r.played[k]
		if got == nil {
			got, _ = r.db.SeasonPlay(row.Season)
			if got == nil {
				got = map[uint32]int{}
			}
			r.played[k] = got
		}
		r.mu.Unlock()
		for _, w := range who {
			id := w.p.CharServerID
			s := got[id] + int(d/time.Second)
			for s >= 3600 {
				s -= 3600
				// its own goroutine's: under the session's item lists' hold (itemsync.go)
				withItems(w.sess, func() { r.give(w.sess, row.Item, row.Hourly, "An hour of the season in Norrath!") })
				log.Printf("SeasonRewards: %s: an hour played by %s: %d", row.Season, w.p.Name, row.Hourly)
			}
			got[id] = s
			if err := r.db.SetSeasonPlay(row.Season, id, s); err != nil {
				log.Printf("SeasonRewards: %s's playtime for %s: %v", row.Season, w.p.Name, err)
			}
		}
	}
}
