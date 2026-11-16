package database

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

// instanceaura: an aura on ONE copy of an item, not on its record (the owner,
// 2026-11-16: "for weapons with aura they can be marked for an aura though
// they are the same weapon as other players, it will have a tag 'Aura:
// Virtuous' for example. so it doesnt just stick to all his weps";
// claude-notes/instance-auras-20261116.md).
//
// itemid is the copy: a charinventory row's itemid (> 0), or a mercenary's
// record, charmercitem's id NEGATED (< 0; handlers/mercinv.go hands a merc's
// worn records to wornLook as ItemID -id). The copy keeps its row wherever it
// goes whole: worn and taken off, into the bank and out (the row moves), the
// account's bank to another character (the row's serverid moves). Where a move
// makes a NEW row (a trade, a bank move that files a part, an auction's
// delivery, a merc given the item and given it back), the aura is carried onto
// it in the same step (CarryInstanceAura). A copy destroyed or sold leaves its
// row behind, read by nothing (itemids are AUTOINCREMENT: never used again).
//
// tag is the word shown with the copy, "Aura: <tag>" at the head of its
// description, on every client (the item record's text is sent per copy:
// handlers' writeItem), at most InstanceTagMost letters; ” none.
//
// color/kind/source/nodes is the copy's first aura, color2 ... nodes2 its
// second, as charaura's hand rows' (a held item's kinds: HandAuraKind, nodes
// where along it 0-AuraAlongMost; kind 0 with nothing else none). Read for a
// held item (the main hand's or the off hand's: entity.pieceAuras) and sent in
// the hand's places on the channel (AuraMainHand/AuraOffHand and their
// seconds), so a client that draws hand auras needs nothing new. A row with a
// tag and no aura is a tag alone: the item's record aura (itemaura) still
// shows.
//
// Precedence on a hand: a character's own hand row (charaura 9/10) first,
// then the copy's (instanceaura), then the record's (itemaura). Whichever
// shows takes the hand's place whole, its first and its second.
const instanceAuraSchema = `
	CREATE TABLE IF NOT EXISTS instanceaura (
		itemid  INTEGER PRIMARY KEY,
		tag     TEXT NOT NULL DEFAULT '',
		color   INTEGER NOT NULL DEFAULT 0,
		kind    INTEGER NOT NULL DEFAULT 0,
		source  INTEGER NOT NULL DEFAULT 0,
		nodes   INTEGER NOT NULL DEFAULT 0,
		color2  INTEGER NOT NULL DEFAULT 0,
		kind2   INTEGER NOT NULL DEFAULT 0,
		source2 INTEGER NOT NULL DEFAULT 0,
		nodes2  INTEGER NOT NULL DEFAULT 0,
		note    TEXT NOT NULL DEFAULT '',
		source_tool TEXT NOT NULL DEFAULT ''
	);`

// InstanceTagMost is the most characters a copy's tag has: "Aura: " and it
// open the description (CLIENT keeps 255 characters of it).
const InstanceTagMost = 24

// ensureInstanceAura makes table instanceaura (initSchema).
func (db *SQLiteDB) ensureInstanceAura() error {
	if _, err := db.Exec(instanceAuraSchema); err != nil {
		return fmt.Errorf("instanceaura: %w", err)
	}
	return nil
}

// InstanceAura is a row of instanceaura, read.
type InstanceAura struct {
	ItemID int // charinventory.itemid, or -charmercitem.id
	Tag    string
	First  ItemAura
	Second ItemAura
	Note   string
	Source string // who wrote it: gm, or a tool's name (its --undo takes out its own)
}

// Shows reports whether the copy draws an aura of its own on a hand (its
// first or its second): it then takes the hand's place whole.
func (a InstanceAura) Shows() bool { return a.First.SecondShows() || a.Second.SecondShows() }

// Empty reports whether the row says nothing: no tag and no aura (deleted).
func (a InstanceAura) Empty() bool { return a.Tag == "" && !a.Shows() }

// ValidInstanceTag holds a tag to what every client shows safely: letters,
// digits, spaces, an apostrophe, a hyphen or a comma (two auras' words:
// "Virtuous, Twinkling"), at most InstanceTagMost (none of
// the '|' a PS2's TextBox overruns on: claude-notes/ps2-textbox-bar-crash-20261107.md).
func ValidInstanceTag(tag string) bool {
	if utf8.RuneCountInString(tag) > InstanceTagMost || strings.TrimSpace(tag) != tag {
		return false
	}
	for _, r := range tag {
		if !(r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '\'' || r == '-' || r == ',')) {
			return false
		}
	}
	return true
}

// validInstanceHand holds one of a copy's auras: none, or a held item's kind
// with its along.
func validInstanceHand(a ItemAura) bool {
	if a.Kind == AuraDefault || a.Kind == AuraNone {
		return true // none (a colour alone draws nothing on a hand)
	}
	return HandAuraKind(int64(a.Kind)) && int(a.Nodes) <= AuraAlongMost
}

// CheckInstanceAura refuses a row the client cannot draw or show.
func CheckInstanceAura(a InstanceAura) error {
	switch {
	case a.ItemID == 0:
		return fmt.Errorf("instanceaura: no item copy (itemid 0)")
	case !ValidInstanceTag(a.Tag):
		return fmt.Errorf("instanceaura: a tag is at most %d letters, digits, spaces, ' or -", InstanceTagMost)
	case !validInstanceHand(a.First) || !validInstanceHand(a.Second):
		return fmt.Errorf("instanceaura: a held item's aura is 3 particles, 7 an epic's look or 9 an energy skin, along 0-%d (or none)", AuraAlongMost)
	}
	return nil
}

// InstanceAuras is every row of instanceaura that says something (a tag, or
// an aura it draws), by itemid; a row out of the client's ranges left out.
func (db *SQLiteDB) InstanceAuras() (map[int]InstanceAura, error) {
	rows, err := db.Query(`SELECT itemid, tag, color, kind, source, nodes, color2, kind2, source2, nodes2, note, source_tool FROM instanceaura`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]InstanceAura)
	for rows.Next() {
		a, ok, err := scanInstanceAura(rows.Scan)
		if err != nil {
			return nil, err
		}
		if ok && CheckInstanceAura(a) == nil && !a.Empty() {
			out[a.ItemID] = a
		}
	}
	return out, rows.Err()
}

// scanInstanceAura reads a row (ok false: its 32-bit values out of range).
func scanInstanceAura(scan func(...any) error) (InstanceAura, bool, error) {
	var a InstanceAura
	var c1, k1, s1, n1, c2, k2, s2, n2 int64
	if err := scan(&a.ItemID, &a.Tag, &c1, &k1, &s1, &n1, &c2, &k2, &s2, &n2, &a.Note, &a.Source); err != nil {
		return a, false, err
	}
	ok := true
	read := func(c, k, s, n int64) ItemAura {
		cc, cok := tint32(c)
		ss, sok := tint32(s)
		if !cok || !sok || k < 0 || k > 255 || n < 0 || n > 255 {
			ok = false
		}
		return ItemAura{Color: cc, Kind: uint8(k), Source: ss, Nodes: uint8(n)}
	}
	a.First, a.Second = read(c1, k1, s1, n1), read(c2, k2, s2, n2)
	return a, ok, nil
}

// InstanceAuraOf is one copy's row (false: none).
func (db *SQLiteDB) InstanceAuraOf(itemID int) (InstanceAura, bool, error) {
	if err := db.ensureInstanceAura(); err != nil {
		return InstanceAura{}, false, err
	}
	a, _, err := scanInstanceAura(db.QueryRow(`SELECT itemid, tag, color, kind, source, nodes, color2, kind2, source2, nodes2, note, source_tool
		FROM instanceaura WHERE itemid = ?`, itemID).Scan)
	if err == sql.ErrNoRows {
		return InstanceAura{}, false, nil
	}
	return a, err == nil, err
}

// SetInstanceAuraAudited writes a copy's row whole (made when there is none;
// one that says nothing, Empty, deletes it), with an audit line: a GM's
// /aura weapon, the panel's set_instance_aura. An aura that draws nothing is
// kept as none (kind 0, nothing else).
func (db *SQLiteDB) SetInstanceAuraAudited(a InstanceAura, who GMEdit, detail string) error {
	if err := CheckInstanceAura(a); err != nil {
		return err
	}
	if !a.First.SecondShows() {
		a.First = ItemAura{}
	}
	if !a.Second.SecondShows() {
		a.Second = ItemAura{}
	}
	if _, err := db.Exec(GMAuditSchema); err != nil {
		return err
	}
	if err := db.ensureInstanceAura(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	fillWho(tx, &who)
	key := map[string]any{"itemid": a.ItemID}
	const sel = `SELECT * FROM instanceaura WHERE itemid = ?`
	before, err := fullRow(tx, sel, a.ItemID)
	if err != nil {
		return err
	}
	action := "update"
	switch {
	case a.Empty() && before == nil:
		return nil
	case a.Empty():
		action = "delete"
		_, err = tx.Exec(`DELETE FROM instanceaura WHERE itemid = ?`, a.ItemID)
	default:
		if before == nil {
			action = "insert"
		}
		if a.Source == "" {
			a.Source = "gm"
		}
		_, err = tx.Exec(`INSERT INTO instanceaura (itemid, tag, color, kind, source, nodes, color2, kind2, source2, nodes2, note, source_tool)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(itemid) DO UPDATE SET tag = excluded.tag, color = excluded.color, kind = excluded.kind, source = excluded.source,
				nodes = excluded.nodes, color2 = excluded.color2, kind2 = excluded.kind2, source2 = excluded.source2,
				nodes2 = excluded.nodes2, note = excluded.note, source_tool = excluded.source_tool`,
			a.ItemID, a.Tag, int64(a.First.Color), a.First.Kind, int64(a.First.Source), a.First.Nodes,
			int64(a.Second.Color), a.Second.Kind, int64(a.Second.Source), a.Second.Nodes, a.Note, a.Source)
	}
	if err != nil {
		return err
	}
	after, err := fullRow(tx, sel, a.ItemID)
	if err != nil {
		return err
	}
	if _, err := auditRowChange(tx, who, action, "instanceaura", key, before, after, detail); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	putInstanceCache(a) // the game reads it at once (no reload needed)
	return nil
}

// putInstanceCache puts a copy's row (an Empty one: none) in the cache.
func putInstanceCache(a InstanceAura) {
	instanceAuraMu.Lock()
	defer instanceAuraMu.Unlock()
	m := map[int]InstanceAura{}
	if old := instanceAuraCache.Load(); old != nil {
		for k, v := range *old {
			m[k] = v
		}
	}
	if a.Empty() {
		delete(m, a.ItemID)
	} else {
		m[a.ItemID] = a
	}
	instanceAuraCache.Store(&m)
}

// The copies' rows as the game reads them (instanceAuraCache), by itemid:
// loaded whole (SetInstanceAuraCache, from InstanceAuras: the world's
// LoadItemAuras at the start and after every GM edit) and moved with a copy
// (a carry), so a copy that changed rows shows its aura and tag at once.
var (
	instanceAuraMu    sync.Mutex
	instanceAuraCache atomic.Pointer[map[int]InstanceAura]
)

// SetInstanceAuraCache sets the copies' rows the game reads (InstanceAuras').
func SetInstanceAuraCache(by map[int]InstanceAura) {
	m := make(map[int]InstanceAura, len(by))
	for k, v := range by {
		m[k] = v
	}
	instanceAuraMu.Lock()
	instanceAuraCache.Store(&m)
	instanceAuraMu.Unlock()
}

// InstanceAuraHeld is a copy's row as the game reads it (false: none).
func InstanceAuraHeld(itemID int) (InstanceAura, bool) {
	m := instanceAuraCache.Load()
	if m == nil || itemID == 0 {
		return InstanceAura{}, false
	}
	a, ok := (*m)[itemID]
	return a, ok
}

// InstanceAuraCount is how many copies the game reads a row for.
func InstanceAuraCount() int {
	if m := instanceAuraCache.Load(); m != nil {
		return len(*m)
	}
	return 0
}

// InstanceTag is a copy's tag ("Virtuous", shown "Aura: Virtuous"), "" none.
func InstanceTag(itemID int) string {
	a, _ := InstanceAuraHeld(itemID)
	return a.Tag
}

// moveInstanceCache moves a copy's row in the cache, after its carry is kept.
func moveInstanceCache(from, to int) {
	instanceAuraMu.Lock()
	defer instanceAuraMu.Unlock()
	old := instanceAuraCache.Load()
	if old == nil {
		return
	}
	a, ok := (*old)[from]
	if !ok {
		return
	}
	m := make(map[int]InstanceAura, len(*old))
	for k, v := range *old {
		m[k] = v
	}
	delete(m, from)
	a.ItemID = to
	m[to] = a
	instanceAuraCache.Store(&m)
}

// carryInstanceAuraTx moves a copy's row from the row it was (from) to the row
// it now is (to), inside the move's own transaction: a merc given the item, an
// auction's delivery. No row: nothing (false). A row already on to (never,
// itemids being new) is replaced. The caller moves the cache after its commit
// (moveInstanceCache) when it reports true.
func carryInstanceAuraTx(q sqlRunner, from, to int) (bool, error) {
	if from == 0 || to == 0 || from == to {
		return false, nil
	}
	if _, err := q.Exec(instanceAuraSchema); err != nil {
		return false, err
	}
	var n int
	if err := q.QueryRow(`SELECT COUNT(*) FROM instanceaura WHERE itemid = ?`, from).Scan(&n); err != nil || n == 0 {
		return false, err
	}
	if _, err := q.Exec(`DELETE FROM instanceaura WHERE itemid = ?`, to); err != nil {
		return false, err
	}
	_, err := q.Exec(`UPDATE instanceaura SET itemid = ? WHERE itemid = ?`, to, from)
	return err == nil, err
}

// CarryInstanceAura is a carry alone (its own statement, the cache with it):
// a whole record given as a new row (a trade, the bank's part path).
func (s *InventoryStore) CarryInstanceAura(from, to int) error {
	moved, err := carryInstanceAuraTx(s.db, from, to)
	if moved {
		moveInstanceCache(from, to)
	}
	return err
}

// tagRows gives each copy read its tag (CharInventoryEntry.AuraTag).
func tagRows(rows []CharInventoryEntry) {
	if instanceAuraCache.Load() == nil {
		return
	}
	for i := range rows {
		rows[i].AuraTag = InstanceTag(rows[i].ItemID)
	}
}

// InstanceTagged is a description with a copy's tag at its head ("Aura:
// Virtuous", a line of its own), cut to max letters; no tag: as it was.
func InstanceTagged(desc, tag string, max int) string {
	if tag == "" {
		return desc
	}
	s := "Aura: " + tag
	if strings.TrimSpace(desc) != "" {
		s += "\n" + desc
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

// HandMain and HandOff name a hand for HeldCopy: the equip slots each holds
// in the order wornLook reads them (handlers/character.go: the main hand a
// two-hander 16, a primary 14, a held item 19; the off hand a second weapon
// 15, a held item 20, the ranged slot 18, then a shield 17).
const (
	HandMain = 0
	HandOff  = 1
)

var handEquipLocs = [2][]int{{16, 14, 19}, {15, 20, 18, 17}}

// HeldCopy is the copy a character (merc 0) or its mercenary of a hire slot
// holds in a hand, as the channel shows it: its instanceaura key (the
// charinventory itemid; a merc's record's -charmercitem.id), its record and
// name. 0: nothing of its own there (empty, or a merc's lent heirloom).
func (db *SQLiteDB) HeldCopy(serverID uint32, merc, hand int) (itemID, patternID int, name string, err error) {
	if hand != HandMain && hand != HandOff {
		return 0, 0, "", fmt.Errorf("no hand %d", hand)
	}
	for _, loc := range handEquipLocs[hand] {
		var q string
		args := []any{serverID, loc}
		if merc == 0 {
			q = `SELECT c.itemid, p.patternid, p.itemname FROM charinventory c JOIN itempattern p ON p.patternid = c.patternid
				WHERE c.serverid = ? AND c.equiploc = ? AND c.location = 0 ORDER BY c.itemid LIMIT 1`
		} else {
			q = `SELECT -m.id, p.patternid, p.itemname FROM charmercitem m JOIN itempattern p ON p.patternid = m.patternid
				WHERE m.serverid = ? AND m.equiploc = ? AND m.slot = ? ORDER BY m.id LIMIT 1`
			args = append(args, merc)
		}
		err = db.QueryRow(q, args...).Scan(&itemID, &patternID, &name)
		if err == nil {
			return itemID, patternID, name, nil
		}
		if err != sql.ErrNoRows {
			return 0, 0, "", err
		}
	}
	return 0, 0, "", nil
}
