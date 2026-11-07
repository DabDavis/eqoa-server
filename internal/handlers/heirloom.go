package handlers

import (
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// heirloom.go — test gear whose figures follow its wearer, like WoW's
// heirlooms (the user, 2026-11-06: "test armor vendors for starting cities
// armor that scales per level, similar to wow heirloom armor. it will be Best
// in Slot for each class. also give it a healing effect per hit like the
// breastplate of the anointed. the weapons that look like the lvl 60 epics
// that scale also per class. also give them full set of jewelry, belts", then
// "the test armor npc will be next to the starting q npc, the cost is 0
// tunar"). database/heirlooms.go keeps which records are heirlooms (table
// heirlooms: a class and a role); tools/seed_heirlooms.py writes the records,
// those rows and the vendors (an Heirloom Outfitter beside every class's
// first quest giver, selling that class's set for nothing).
//
// An heirloom record keeps its own name, look, wear slot and class; its
// figures (stats, resists, AC, hit points and power, PoT and HoT, a weapon's
// damage and range) are those of the best item of its role (heirloomRoles)
// its class may wear at its wearer's level, of the wearer's race (CLIENT's
// CanUse: database.WearablePatterns), by #template's measures (template.go:
// templateScore for armour and jewellery, a weapon's DMG first: byDamage):
// a warrior of 12's heirloom helm is the best helm a warrior of 12 could
// wear (the user's choice: "BiS of its level"). No conjured item (NO RENT)
// no broken record (a PoT or resist past 500, the simulator's rule, or a
// weapon past 1,000 damage: the epics' most is 928), no prop or test record
// and none of no known level (heirloomUnfit) is taken. The figures are
// put on the record as a character's items are read (database.PatternView,
// set at the start), so the worn gear's stats (wornGear), the login's
// inventory, the merchant's list for its buyer and every record sent carry
// the wearer's; a level up refreshes the worn gear at once (World.OnLevelUp),
// the item windows at the next login (the user's choice). The chest piece
// healed as the Breastplate of the Anointed until the user's "lets turn off
// the healing effect on hit from heirloom" (2026-11-06). The records are NO
// TRADE and worth nothing (the user's choice).
//
// An heirloom weapon may BE an epic (heirlooms.epic; the user, 2026-11-06:
// "the heirloom weapons for every class lets make their lvl 60 epics. also add
// extra choices of weapons to choose from that are epic also. including some
// level 50 epics also", then "also the bow heirloom needs to be epic also";
// tools/heirloom_epics.py): from the epic's own level (60, or 49 for the path
// quests' epics) the wearer's view is that epic's figures (heirloomEpicFigures:
// its stats, damage and reach) and its proc (ItemPatternRecord.ProcName: a
// weapon's proc is read by its name, entity/weaponproc.go); below it the best
// of its role, as every heirloom. The epic record is read as the best are,
// kept heirloomTTL.

// heirloomRole is a role's wear slots and how its best is chosen.
type heirloomRole struct {
	wear   []int
	weapon bool // by the weapon's damage first (byDamage), else by templateScore (byScore)
}

// heirloomRoles are the roles a record may have.
var heirloomRoles = map[string]heirloomRole{
	"head":     {wear: []int{1}},
	"hands":    {wear: []int{19}},
	"ear":      {wear: []int{3}},
	"neck":     {wear: []int{4}},
	"chest":    {wear: []int{5}},
	"bracers":  {wear: []int{7}},
	"bracelet": {wear: []int{6}},
	"ring":     {wear: []int{8}},
	"waist":    {wear: []int{9}},
	"legs":     {wear: []int{10}},
	"feet":     {wear: []int{11}},
	"robe":     {wear: []int{2}},
	"shield":   {wear: []int{13}},
	"main":     {wear: []int{12, 14}, weapon: true},
	"twohand":  {wear: []int{15}, weapon: true},
	"ranged":   {wear: []int{16, 17}, weapon: true},
}

// heirloomTTL is how long a choice and the table's rows are kept before they
// are read again (a GM's item edits count within it).
const heirloomTTL = 10 * time.Minute

// heirloomTestWord is a test record's name.
var heirloomTestWord = regexp.MustCompile(`\btest\b`)

// heirloomBrokenOver and heirloomDamageOver are the broken records' marks
// (tools/fix_broken_records.py's simBroken: a PoT or a resist past 500; no
// real weapon deals past 1,000).
const (
	heirloomBrokenOver = 500
	heirloomDamageOver = 1000
)

// Heirlooms puts each character's heirloom figures on its records.
type Heirlooms struct {
	db  *database.SQLiteDB
	inv *database.InventoryStore
	now func() time.Time

	mu     sync.Mutex
	defs   map[int]database.HeirloomDef
	defsAt time.Time
	best   map[heirloomKey]heirloomPick
	epics  map[int]heirloomPick // an epic heirloom's epic record, by its id
	// known are the characters in the world, by serverid: class, race and
	// level as the World last said them (login, level up); others are read
	// off their rows
	known map[uint32]heirloomWho
}

type heirloomKey struct {
	class, race, level int
	role               string
}

type heirloomPick struct {
	p  database.ItemPatternRecord
	ok bool
	at time.Time
}

type heirloomWho struct{ class, race, level int }

// NewHeirlooms makes the view over db's items.
func NewHeirlooms(db *database.SQLiteDB) *Heirlooms {
	return &Heirlooms{db: db, inv: database.NewInventoryStore(db), now: time.Now,
		best: map[heirloomKey]heirloomPick{}, epics: map[int]heirloomPick{}, known: map[uint32]heirloomWho{}}
}

// Start sets the view on the database and hears the World's level ups:
// refresh is called off the lock with the player whose level rose.
func (h *Heirlooms) Start(gw *entity.World, refresh func(p *entity.Player)) {
	h.db.SetPatternView(h.View)
	if gw != nil {
		gw.OnLevelUp(func(sess *session.Session, p *entity.Player) {
			h.Note(p.CharServerID, int(p.Class), int(p.Race), p.Level)
			if refresh != nil {
				go refresh(p)
			}
		})
	}
}

// Note is a character's class, race and level as the World has them.
func (h *Heirlooms) Note(serverID uint32, class, race, level int) {
	if h == nil || serverID == 0 {
		return
	}
	h.mu.Lock()
	h.known[serverID] = heirloomWho{class, race, level}
	h.mu.Unlock()
}

// Forget is a character gone from the world (its row is read again).
func (h *Heirlooms) Forget(serverID uint32) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.known, serverID)
	h.mu.Unlock()
}

// View puts the heirloom's figures for the character on p; any other record
// is left as it is (database.PatternView).
func (h *Heirlooms) View(serverID uint32, p *database.ItemPatternRecord) {
	def, ok := h.def(p.PatternID)
	if !ok {
		return
	}
	race, level, ok := h.who(serverID)
	if !ok {
		return
	}
	epicNow := false
	if def.Epic != 0 {
		if epic, ok := h.epicOf(def.Epic); ok && level >= max(epic.LevelReq, 1) {
			heirloomEpicFigures(p, &epic)
			epicNow = true
		}
	}
	if best, ok := h.bestOf(def.Class, race, level, def.Role); ok {
		if !epicNow {
			heirloomFigures(p, &best)
		} else if best.WeaponDamage > p.WeaponDamage {
			// an epic deals no less than the best of its kind at the level (the
			// user, 2026-11-06: Tmac's Fistaglin's Bow, 759, out-shot the
			// heirloom bow's Quicksilver, 754: "lets at least match it")
			p.WeaponDamage = best.WeaponDamage
			p.Unk5 = max(p.Unk5, best.Unk5)
		}
	}
	// every heirloom, weapons and shields too (the user: "let heirloom weapon
	// get stats also and shields"), the same figures for every class; an epic
	// from its level keeps the epic's own (the user's choice, 2026-11-06)
	if !epicNow {
		heirloomStats(p, level)
	}
}

// The heirlooms' stats (the user, 2026-11-06: "lets tone down the heirloom to
// +10 all stats 40 hp", then his choices: power 40 too, scaled up to it at 60,
// an epic keeping its own from its level): each piece, armour, jewellery, a
// shield or a weapon, carries heirloomStatTop of every stat (STR, STA, AGI,
// DEX, WIS, INT, CHA) and heirloomPoolTop hit points and power at
// heirloomTopLevel, its share below (rounded up: 1 of each at level 1), for
// every class, whatever its level's best carries. Until then a piece took its
// best's stats, hit points and power, raised to its class's (HP and power 1.25
// a level, STR, STA and two of the class's stats one per 5 levels: 75, 75 and
// 12 at 60).
const (
	heirloomTopLevel = 60
	heirloomStatTop  = 10
	heirloomPoolTop  = 40
)

// heirloomShare is top's share at the wearer's level: top at heirloomTopLevel
// and over, rounded up below it.
func heirloomShare(top, level int) int {
	lv := min(max(level, 1), heirloomTopLevel)
	return (top*lv + heirloomTopLevel - 1) / heirloomTopLevel
}

// heirloomStats sets an heirloom's stats, hit points and power for its
// wearer's level, in place of its best's: heirloomStatTop's share of each
// stat and heirloomPoolTop's of hit points and power (10, 40 and 40 at 60; 5,
// 20 and 20 at 30; 1, 1 and 1 at 1).
func heirloomStats(p *database.ItemPatternRecord, level int) {
	stat, pool := heirloomShare(heirloomStatTop, level), heirloomShare(heirloomPoolTop, level)
	n := func(v int) *int { return &v }
	p.Str, p.Sta, p.Agi, p.Dex = n(stat), n(stat), n(stat), n(stat)
	p.Wis, p.Intelligence, p.Cha = n(stat), n(stat), n(stat)
	p.HPMAX, p.POWMAX = n(pool), n(pool)
}

// epicOf is an epic heirloom's epic record, kept heirloomTTL.
func (h *Heirlooms) epicOf(id int) (database.ItemPatternRecord, bool) {
	h.mu.Lock()
	e, cached := h.epics[id]
	h.mu.Unlock()
	if cached && h.now().Sub(e.at) <= heirloomTTL {
		return e.p, e.ok
	}
	e = heirloomPick{at: h.now()}
	if r, err := database.NewItemPatternStore(h.db).GetItemPattern(id); err != nil {
		log.Printf("Heirlooms: the epic %d: %v", id, err)
	} else if r != nil {
		e.p, e.ok = *r, true
	}
	h.mu.Lock()
	h.epics[id] = e
	h.mu.Unlock()
	return e.p, e.ok
}

// def is a record's row, the table read again past heirloomTTL.
func (h *Heirlooms) def(patternID int) (database.HeirloomDef, bool) {
	h.mu.Lock()
	stale := h.defs == nil || h.now().Sub(h.defsAt) > heirloomTTL
	h.mu.Unlock()
	if stale {
		defs, err := h.db.Heirlooms()
		if err != nil {
			log.Printf("Heirlooms: the table: %v", err)
			defs = map[int]database.HeirloomDef{}
		}
		h.mu.Lock()
		h.defs, h.defsAt = defs, h.now()
		h.mu.Unlock()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.defs[patternID]
	return d, ok
}

// who is the character's race and level: the World's word, else its row.
func (h *Heirlooms) who(serverID uint32) (race, level int, ok bool) {
	h.mu.Lock()
	k, known := h.known[serverID]
	h.mu.Unlock()
	if known {
		return k.race, k.level, true
	}
	if err := h.db.QueryRow(`SELECT race, level FROM characters WHERE serverid = ?`, serverID).Scan(&race, &level); err != nil {
		return 0, 0, false
	}
	return race, level, true
}

// bestOf is the best real item of the role a character of class and race may
// wear at level, kept heirloomTTL.
func (h *Heirlooms) bestOf(class, race, level int, role string) (database.ItemPatternRecord, bool) {
	r, ok := heirloomRoles[role]
	if !ok {
		return database.ItemPatternRecord{}, false
	}
	key := heirloomKey{class, race, max(level, 1), role}
	h.mu.Lock()
	b, cached := h.best[key]
	h.mu.Unlock()
	if cached && h.now().Sub(b.at) <= heirloomTTL {
		return b.p, b.ok
	}
	// an item's mask is asked about the race the character passes as (race.go):
	// no item's mask has a race of racedefs' bit
	items, err := h.inv.WearablePatterns(class, entity.WireRace(race), key.level, r.wear)
	if err != nil {
		log.Printf("Heirlooms: the items of %s for class %d at %d: %v", role, class, key.level, err)
		return database.ItemPatternRecord{}, false
	}
	h.mu.Lock()
	defs := h.defs
	h.mu.Unlock()
	items = slices.DeleteFunc(items, func(it database.ItemPatternRecord) bool {
		_, heirloom := defs[it.PatternID]
		return heirloom || heirloomUnfit(&it)
	})
	order := byScore
	if r.weapon {
		order = byDamage
	}
	slices.SortStableFunc(items, order)
	b = heirloomPick{at: h.now()}
	if len(items) > 0 {
		b.p, b.ok = items[0], true
	}
	h.mu.Lock()
	h.best[key] = b
	h.mu.Unlock()
	return b.p, b.ok
}

// heirloomUnfit is an item no heirloom takes its figures from: a conjured
// one (NO RENT), a broken record, a prop (the "prop shield" of AC 534 no
// player was meant to wear), a test record (the "Froglok Test" blade and mace
// of level 1 and damage 561) or one whose level is not known (levelreq 0: 74
// real items, the Kiktar Nin Kukri's 497 damage among them, that a level 1
// would otherwise take).
func heirloomUnfit(p *database.ItemPatternRecord) bool {
	name := strings.ToLower(p.ItemName)
	if p.Rent == 0 || p.LevelReq < 1 || p.WeaponDamage > heirloomDamageOver || strings.HasPrefix(name, "prop ") ||
		heirloomTestWord.MatchString(name) {
		return true
	}
	for _, v := range []*int{p.PoT, p.PR, p.DR, p.FR, p.CR, p.LR, p.AR} {
		if v != nil && *v > heirloomBrokenOver {
			return true
		}
	}
	return false
}

// heirloomFigures puts best's figures on p: stats, resists, AC, hit points
// and power, PoT and HoT, a weapon's damage and range.
func heirloomFigures(p, best *database.ItemPatternRecord) {
	cp := func(v *int) *int {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	p.Str, p.Sta, p.Agi, p.Dex = cp(best.Str), cp(best.Sta), cp(best.Agi), cp(best.Dex)
	p.Wis, p.Intelligence, p.Cha = cp(best.Wis), cp(best.Intelligence), cp(best.Cha)
	p.HPMAX, p.POWMAX, p.PoT, p.HoT, p.AC = cp(best.HPMAX), cp(best.POWMAX), cp(best.PoT), cp(best.HoT), cp(best.AC)
	p.PR, p.DR, p.FR, p.CR, p.LR, p.AR = cp(best.PR), cp(best.DR), cp(best.FR), cp(best.CR), cp(best.LR), cp(best.AR)
	if best.WeaponDamage > 0 {
		p.WeaponDamage = best.WeaponDamage
		// a ranged weapon's reach: the best's, or the record's own when longer
		// (the ranger's heirloom bows reach 55, the user's)
		p.Unk5 = max(p.Unk5, best.Unk5)
	}
}

// heirloomEpicFigures puts the epic's figures on p, as heirloomFigures puts a
// best's, and its proc: p fights as the epic does.
func heirloomEpicFigures(p, epic *database.ItemPatternRecord) {
	heirloomFigures(p, epic)
	p.ProcName, p.ProcDesc = epic.ItemName, epic.ItemDesc
}
