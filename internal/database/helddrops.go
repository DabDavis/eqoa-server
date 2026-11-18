package database

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// What a creature holding a weapon or a shield drops of it (the owner,
// 2026-11-17: "if a mob is holding a weapon/shield they will drop it as loot";
// asked how often: "always drop what it holds"). An NPC's row names only the
// MODEL in each hand (npcs.primary_weapon, secondary_weapon, shield: a DictID,
// the channel's items 0-2), never an item, so the item is found by its look:
// an itempattern record whose model is the held model, worn in a hand's slot
// (HeldHandSlots) for a weapon, the shield's (HeldShieldSlot) for a shield,
// and of the creature's level: the nearest levelreq at or below it. Several
// records of that level: the lowest patternid, so every kill of a kind drops
// the same thing. None at or below its level, or none of the model at all:
// nothing extra drops.
//
// Any record of the look may drop (the owner, 2026-11-18, ticket 2366: "they
// can drop these also"): No Trade, Lore, No Rent, an heirloom, an Ascended
// copy, an epic, a quest's item. Only an item a player owns alone (itemowner)
// and a GM's test record (a name saying test) never do.

// HeldHandSlots is the itempattern equip slots of an item held in a hand:
// 12 either hand, 14 the primary, 15 a two-hander, 16 a ranged weapon, 18 a
// held item (a wand, a totem, a book).
var HeldHandSlots = []int{12, 14, 15, 16, 18}

// HeldShieldSlot is a shield's itempattern equip slot.
const HeldShieldSlot = 13

// HeldDrops is the catalog's droppable hand and shield records by model,
// each list in levelreq order (BuildHeldDrops).
type HeldDrops struct {
	hands   map[uint32][]CharInventoryEntry
	shields map[uint32][]CharInventoryEntry
	// Records is how many records it holds; Left how many hand or shield
	// records were left out, by why
	Records int
	Left    map[string]int
	// named is table npcheld's: the record a creature of a name drops of what it holds in a
	// hand (NPCHeldPrimary, NPCHeldSecondary, NPCHeldShield), by NPCClassKey
	named map[string][3]*CharInventoryEntry
}

// heldHand reports a hand's equip slot.
func heldHand(slot int) bool {
	for _, s := range HeldHandSlots {
		if s == slot {
			return true
		}
	}
	return false
}

// heldWordsBarred is a name's words that keep a record off a held drop.
var heldWordsBarred = []string{"test"} // a GM's test record; epics, artifacts and heirlooms drop (the owner, 2026-11-18)

// heldDropWhyNot is why a hand or shield record is no held drop ("" it is
// one), excluded the ids that are barred by table (an heirloom, an Ascended
// copy, an owner's, a quest's), by why.
func heldDropWhyNot(p *ItemPatternRecord, excluded map[int]string) string {
	if why, ok := excluded[p.PatternID]; ok {
		return why
	}
	name := strings.ToLower(strings.TrimSpace(p.ItemName))
	switch {
	case name == "":
		return "no name"
	case p.Model == 0:
		return "no model"
	}
	for _, w := range heldWordsBarred {
		if strings.Contains(name, w) {
			return w
		}
	}
	return ""
}

// BuildHeldDrops makes the model map of records (the catalog's, any slots):
// the hand and shield records a held item may drop as, excluded those barred
// by table (id -> why).
func BuildHeldDrops(records []CharInventoryEntry, excluded map[int]string) *HeldDrops {
	d := &HeldDrops{hands: map[uint32][]CharInventoryEntry{}, shields: map[uint32][]CharInventoryEntry{}, Left: map[string]int{}}
	for _, e := range records {
		p := &e.Pattern
		hand, shield := heldHand(p.EquipSlot), p.EquipSlot == HeldShieldSlot
		if !hand && !shield {
			continue
		}
		if why := heldDropWhyNot(p, excluded); why != "" {
			d.Left[why]++
			continue
		}
		e.ItemID, e.StackLeft, e.Charges, e.EquipLoc, e.Location, e.ListNum = 0, 1, 0, -1, 0, 0
		e.RemainHP = p.MaxHP
		m := d.hands
		if shield {
			m = d.shields
		}
		m[uint32(p.Model)] = append(m[uint32(p.Model)], e)
		d.Records++
	}
	for _, m := range []map[uint32][]CharInventoryEntry{d.hands, d.shields} {
		for _, l := range m {
			sort.SliceStable(l, func(i, j int) bool {
				if a, b := max(l[i].Pattern.LevelReq, 0), max(l[j].Pattern.LevelReq, 0); a != b {
					return a < b
				}
				return l[i].Pattern.PatternID < l[j].Pattern.PatternID
			})
		}
	}
	return d
}

// Has reports whether any record looks like the model held (a shield's or a
// hand's), whatever its level.
func (d *HeldDrops) Has(model uint32, shield bool) bool {
	if d == nil {
		return false
	}
	if shield {
		return len(d.shields[model]) > 0
	}
	return len(d.hands[model]) > 0
}

// Pick is the record a creature of level, named name (its kind: NPCClassKey),
// drops of the model it held (a shield's or a hand's): the nearest levelreq at
// or below level, one of several of that levelreq by the name. ok is false
// when no record looks like it (none: false too) or none is of its level.
func (d *HeldDrops) Pick(model uint32, shield bool, level int, name string) (CharInventoryEntry, bool) {
	if d == nil || model == 0 {
		return CharInventoryEntry{}, false
	}
	l := d.hands[model]
	if shield {
		l = d.shields[model]
	}
	top := -1
	for i := range l {
		if max(l[i].Pattern.LevelReq, 0) <= level {
			top = i
		}
	}
	if top < 0 {
		return CharInventoryEntry{}, false
	}
	req := max(l[top].Pattern.LevelReq, 0)
	first := top
	for first > 0 && max(l[first-1].Pattern.LevelReq, 0) == req {
		first--
	}
	n := top - first + 1
	_ = n
	return l[first], true // of several of that levelreq the lowest patternid (the list's order)
}

// heldBarredTables is each table whose patternid column bars a record, and why.
// The owner, 2026-11-18 (ticket 2366): "they can drop these also": an heirloom,
// an Ascended copy, a quest's item or object, Lore, No Trade, epic all drop as
// held; only an item a player owns alone (a GM's gift of one of a kind) stays off.
var heldBarredTables = []struct{ table, why string }{
	{"itemowner", "owned"},
}

// HeldDrops reads the catalog's hand and shield records into the model map
// (BuildHeldDrops), barred what the tables bar (a table a database lacks bars
// nothing).
func (s *InventoryStore) HeldDrops() (*HeldDrops, error) {
	excluded := map[int]string{}
	for _, t := range heldBarredTables {
		if err := s.barHeld(excluded, `SELECT patternid FROM `+t.table, t.why); err != nil {
			return nil, err
		}
	}
	slots := make([]string, 0, len(HeldHandSlots)+1)
	for _, sl := range append(append([]int{}, HeldHandSlots...), HeldShieldSlot) {
		slots = append(slots, strconv.Itoa(sl))
	}
	rows, err := s.db.Query(`SELECT 0, 1, p.maxhp, 0, -1, 0, 0, ` + itemPatternColumns + `
		FROM itempattern p WHERE p.equipslot IN (` + strings.Join(slots, ",") + `) ORDER BY p.patternid`)
	if err != nil {
		return nil, fmt.Errorf("itempattern: %w", err)
	}
	defer rows.Close()
	var recs []CharInventoryEntry
	for rows.Next() {
		e, err := scanItemRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		recs = append(recs, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d := BuildHeldDrops(recs, excluded)
	if err := s.loadNamedHeld(d); err != nil {
		return nil, err
	}
	return d, nil
}

// npcheld: the record a creature of a name drops of what it holds (the owner, 2026-11-18, of Vallon Zek
// and Sapphyra's Sword: "put sword in hand if he will drop it"). The held drops go by the held MODEL
// (Pick), which finds a record of that look, never a No Trade or Lore one; a row here names the very
// record instead, for one hand of one name (hand 0 the primary, 1 the secondary, 2 the shield), whatever
// its trade, rent or Lore: the creature drops it whenever it dies holding something in that hand
// (handlers/helddrop.go). The row's npcs hand should hold the record's look (a tool sets both). npc_name
// as npcclass keys a name. Read at the start, with the held drops (an edit counts at the next start).
const npcHeldSchema = `
	CREATE TABLE IF NOT EXISTS npcheld (
		npc_name  TEXT NOT NULL,
		hand      INTEGER NOT NULL,
		patternid INTEGER NOT NULL,
		note      TEXT NOT NULL DEFAULT '',
		source    TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (npc_name, hand)
	);`

// The hands of npcheld.hand.
const (
	NPCHeldPrimary   = 0
	NPCHeldSecondary = 1
	NPCHeldShield    = 2
)

// ensureNPCHeld makes table npcheld (initSchema).
func (db *SQLiteDB) ensureNPCHeld() error {
	if _, err := db.Exec(npcHeldSchema); err != nil {
		return fmt.Errorf("npcheld: %w", err)
	}
	return nil
}

// loadNamedHeld reads table npcheld into d; a row whose record is gone, or of a hand past the
// shield, is left out (logged by the caller's report: Left["npcheld"]).
func (s *InventoryStore) loadNamedHeld(d *HeldDrops) error {
	rows, err := s.db.Query(`SELECT npc_name, hand, patternid FROM npcheld ORDER BY npc_name, hand`)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return fmt.Errorf("npcheld: %w", err)
	}
	type held struct {
		name      string
		hand, pid int
	}
	var all []held
	for rows.Next() {
		var h held
		if err := rows.Scan(&h.name, &h.hand, &h.pid); err != nil {
			rows.Close()
			return fmt.Errorf("npcheld: %w", err)
		}
		all = append(all, h)
	}
	rows.Close()
	for _, h := range all {
		if h.hand < NPCHeldPrimary || h.hand > NPCHeldShield {
			d.Left["npcheld"]++
			continue
		}
		e, err := scanItemRow(s.db.QueryRow(`SELECT 0, 1, p.maxhp, 0, -1, 0, 0, `+itemPatternColumns+`
			FROM itempattern p WHERE p.patternid = ?`, h.pid).Scan)
		if err != nil {
			d.Left["npcheld"]++
			continue
		}
		e.ItemID, e.StackLeft, e.Charges, e.EquipLoc, e.Location, e.ListNum = 0, 1, 0, -1, 0, 0
		e.RemainHP = e.Pattern.MaxHP
		if d.named == nil {
			d.named = map[string][3]*CharInventoryEntry{}
		}
		k := NPCClassKey(h.name)
		hands := d.named[k]
		rec := e
		hands[h.hand] = &rec
		d.named[k] = hands
	}
	return nil
}

// Named is the record npcheld names for a creature of that name's hand (NPCHeldPrimary ...), if any.
func (d *HeldDrops) Named(name string, hand int) (CharInventoryEntry, bool) {
	if d == nil || hand < NPCHeldPrimary || hand > NPCHeldShield {
		return CharInventoryEntry{}, false
	}
	if e := d.named[NPCClassKey(name)][hand]; e != nil {
		return *e, true
	}
	return CharInventoryEntry{}, false
}

// barHeld bars the patternids q selects as why; a missing table bars nothing.
func (s *InventoryStore) barHeld(excluded map[int]string, q, why string) error {
	rows, err := s.db.Query(q)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil
		}
		return fmt.Errorf("%s: %w", why, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if _, ok := excluded[id]; !ok {
			excluded[id] = why
		}
	}
	return rows.Err()
}

// OwnLootProfiles is the names (lootcreature's, NPCClassKey) of a loot profile
// of their own: every profile but the default's (xp) and trash.
func (s *InventoryStore) OwnLootProfiles() (map[string]bool, error) {
	out := map[string]bool{}
	rows, err := s.db.Query(`SELECT npc_name FROM lootcreature WHERE profile NOT IN ('', ?, 'trash')`, DefaultLootProfile)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[NPCClassKey(n)] = true
	}
	return out, rows.Err()
}
