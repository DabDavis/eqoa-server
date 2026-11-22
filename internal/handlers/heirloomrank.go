package handlers

import (
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
)

// heirloomrank.go — the Heirloom Rank (work board 1927 section 8b, a GM's spec; bought at an
// Heirloom Keeper with Heirloom Marks, marks.go): each rank raises THIS character's heirloom cap
// one level past heirloomTopLevel (49), to 55 at rank 6, and adds the AC of table heirloomrankac
// (class, rank) to each heirloom armour piece of role head, hands, chest, robe, bracers, waist,
// legs, feet or shield; never an earring, a necklace, a ring, a bracelet or a weapon. The AC goes on
// the record's view with the rest of its figures, so the tooltip shows it.
//
// Past 49 an heirloom's best of its role never takes a raid's record: an Ascended one
// (itemascended), one on a loot table whose name starts with "boss ", "raid", "dungeon ",
// "heroic" or "epic ", or holds "ascended", nor any of patternids 965000-999999. An epic heirloom
// keeps the 49 path epic's figures (heirloom.go). The stats' share (10 of each, 40 HP, 40 power)
// still tops out at 49. A merc's heirlooms take its owner's rank (ViewForOwner).
//
// All of it only with world.heirloom_marks (SetHeirloomMarks): off, every rank reads 0.

// heirloomMarksOn is world.heirloom_marks.
var heirloomMarksOn atomic.Bool

// SetHeirloomMarks turns Heirloom Marks, their Keeper, the Hot zones, the Heirloom Rank and
// Relive on or off (world.heirloom_marks; off unless set).
func SetHeirloomMarks(on bool) { heirloomMarksOn.Store(on) }

// HeirloomMarksOn is world.heirloom_marks.
func HeirloomMarksOn() bool { return heirloomMarksOn.Load() }

// HeirloomRankMost is the highest rank.
const HeirloomRankMost = 6

// heirloomRankTTL is how old a character's rank and the AC table may be (the spec: "read at each
// award or at most 30 s old"); a rank bought is set at once (SetRank).
const heirloomRankTTL = 30 * time.Second

type heirloomRankOf struct {
	rank int
	at   time.Time
}

// heirloomRankArmour are the roles a rank's AC goes on.
var heirloomRankArmour = map[string]bool{
	"head": true, "hands": true, "chest": true, "robe": true, "bracers": true, "waist": true, "legs": true, "feet": true, "shield": true,
}

// heirloomRaidFirst and heirloomRaidLast are the patternids no heirloom takes past 49.
const (
	heirloomRaidFirst = 965000
	heirloomRaidLast  = 999999
)

func heirloomRaidID(id int) bool { return id >= heirloomRaidFirst && id <= heirloomRaidLast }

// heirloomRaidTable is whether a loot table's records are a raid's.
func heirloomRaidTable(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, pre := range []string{"boss ", "raid", "dungeon ", "heroic", "epic "} {
		if strings.HasPrefix(n, pre) {
			return true
		}
	}
	return strings.Contains(n, "ascended")
}

// rankOf is a character's Heirloom Rank: 0 with the marks off, else its charmarks row's, at most
// heirloomRankTTL old.
func (h *Heirlooms) rankOf(serverID uint32) int {
	if h == nil || serverID == 0 || !HeirloomMarksOn() {
		return 0
	}
	h.mu.Lock()
	r, ok := h.ranks[serverID]
	h.mu.Unlock()
	if ok && h.now().Sub(r.at) <= heirloomRankTTL {
		return r.rank
	}
	m, err := h.db.CharMarksOf(serverID)
	if err != nil {
		log.Printf("Heirlooms: character %d's rank: %v", serverID, err)
	}
	r = heirloomRankOf{rank: min(max(m.HeirloomRank, 0), HeirloomRankMost), at: h.now()}
	h.mu.Lock()
	if h.ranks == nil {
		h.ranks = map[uint32]heirloomRankOf{}
	}
	h.ranks[serverID] = r
	h.mu.Unlock()
	return r.rank
}

// SetRank is a rank just bought: the view takes it at once.
func (h *Heirlooms) SetRank(serverID uint32, rank int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.ranks == nil {
		h.ranks = map[uint32]heirloomRankOf{}
	}
	h.ranks[serverID] = heirloomRankOf{rank: rank, at: h.now()}
	h.mu.Unlock()
}

// rankArmour adds the rank's AC to an armour piece's view.
func (h *Heirlooms) rankArmour(def database.HeirloomDef, rank int, p *database.ItemPatternRecord) {
	if rank <= 0 || !heirloomRankArmour[def.Role] {
		return
	}
	ac := h.rankACFor(def.Class, rank)
	if ac == 0 {
		return
	}
	cur := 0
	if p.AC != nil {
		cur = *p.AC
	}
	v := cur + ac
	p.AC = &v
}

// rankACFor is the table's AC for class at rank, the table read again once heirloomRankTTL old.
func (h *Heirlooms) rankACFor(class, rank int) int {
	h.mu.Lock()
	stale := h.rankAC == nil || h.now().Sub(h.rankACAt) > heirloomRankTTL
	h.mu.Unlock()
	if stale {
		t, err := h.db.HeirloomRankAC()
		if err != nil {
			log.Printf("Heirlooms: table heirloomrankac: %v", err)
			t = map[[2]int]int{}
		}
		h.mu.Lock()
		h.rankAC, h.rankACAt = t, h.now()
		h.mu.Unlock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rankAC[[2]int{class, rank}]
}

// raidRecords is every record no heirloom takes past 49 (but the id range, heirloomRaidID), read
// again past heirloomTTL.
func (h *Heirlooms) raidRecords() map[int]bool {
	h.mu.Lock()
	set, at := h.raidRecs, h.raidAt
	h.mu.Unlock()
	if set != nil && h.now().Sub(at) <= heirloomTTL {
		return set
	}
	set = map[int]bool{}
	if rs, err := h.db.Query(`SELECT patternid FROM itemascended`); err == nil {
		for rs.Next() {
			var id int
			if rs.Scan(&id) == nil {
				set[id] = true
			}
		}
		rs.Close()
	}
	if rs, err := h.db.Query(`SELECT tablename, patternid FROM loottables`); err == nil {
		for rs.Next() {
			var name string
			var id int
			if rs.Scan(&name, &id) == nil && heirloomRaidTable(name) {
				set[id] = true
			}
		}
		rs.Close()
	}
	h.mu.Lock()
	h.raidRecs, h.raidAt = set, h.now()
	h.mu.Unlock()
	return set
}
