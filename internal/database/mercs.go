package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// charmerc: each character's hired mercenaries (handlers/merc.go,
// entity/merc.go), a row a merc, up to MercSlots a character (the owner,
// 2026-11-12: "if a player wants to have 3 mercenaries in their group it is
// fine"), each under its slot (1 to MercSlots: /merc 2, its follow place),
// kept until it is dismissed so the merc comes back at the next login and
// through any restart: its name, class, race and sex, its face and hair as
// the liaison chose them, its stance, and when the rest after its death ends
// (UTC, RFC 3339; ” none). hired is when (UTC, RFC 3339). path is the class
// path its book takes (handlers/mercpath.go: the index of its class's path,
// -1 its class's default; the owner, 2026-11-11: "lets add path choice to each
// merc"). pet is the kind of pet its hirer picked (handlers/mercpetkind.go:
// "air", "bones"; ” its best: petition 1564). offensive is a healer's
// /merc offensive (1: it casts its hits and damage over time as well as its
// heals; 0, defensive, the default, heals and buffs alone: the owner,
// 2026-11-12, "add defensive/offensive for healers"). spells_off is the
// spells its hirer turned off in its menu (handlers/mercmenu.go), their ids
// comma-separated. aoe is a Cocktail alchemist's /merc aoe (1: it throws
// its area cocktails; 0, /merc single, the default, its single-target
// potions: handlers/mercinv.go). master is the Master Class its hirer gave
// it at 55 or more (handlers/mercmaster.go: cmcatalog's mastery name, "" none;
// the owner, 2026-11-15). spell_slots is the ten spells its hirer put on
// its ten slots in vanilla's spell window (handlers/mercwin.go: the owner,
// 2026-11-15, "merc spell menu should be like players also with spell book
// and 10 spells"), ten ids comma-separated, 0 an empty slot; empty none set, its
// whole book cast as before. hide_helm is its hirer's /merc helm off (1: its
// helm worn and kept, its look shown bare-headed to every client; 0, the
// default, shown: petition 2965). hide_look is its hirer's /merc look off (1:
// every armour piece's look hidden, the helm's too, their auras with them,
// all still worn; 0, the default, shown: petition 2965's follow-up). were is the were form
// its hirer gave it (handlers/mercwere.go: the form's spell name, one its hirer's own book
// holds; "" none: petition 3069). surname is its hirer's /merc surname, shown
// after its name where a character's surname is (handlers/mercsurname.go: ""
// none; petition 3430, the owner 2026-11-28). A merc on standby (out of a
// full group for a player: entity/mercbench.go) keeps its row as it is.
const charMercSchema = `
	CREATE TABLE IF NOT EXISTS charmerc (
		serverid    INTEGER NOT NULL,
		slot        INTEGER NOT NULL DEFAULT 1,
		name        TEXT NOT NULL,
		class       INTEGER NOT NULL,
		race        INTEGER NOT NULL,
		sex         INTEGER NOT NULL DEFAULT 0,
		face        INTEGER NOT NULL DEFAULT 0,
		hair_color  INTEGER NOT NULL DEFAULT 0,
		hair_length INTEGER NOT NULL DEFAULT 0,
		hair_style  INTEGER NOT NULL DEFAULT 0,
		stance      INTEGER NOT NULL DEFAULT 2,
		dead_until  TEXT NOT NULL DEFAULT '',
		hired       TEXT NOT NULL DEFAULT '',
		path        INTEGER NOT NULL DEFAULT -1,
		pet         TEXT NOT NULL DEFAULT '',
		offensive   INTEGER NOT NULL DEFAULT 0,
		spells_off  TEXT NOT NULL DEFAULT '',
		aoe         INTEGER NOT NULL DEFAULT 0,
		master      TEXT NOT NULL DEFAULT '',
		spell_slots TEXT NOT NULL DEFAULT '',
		hide_helm   INTEGER NOT NULL DEFAULT 0,
		hide_look   INTEGER NOT NULL DEFAULT 0,
		were        TEXT NOT NULL DEFAULT '',
		surname     TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (serverid, slot)
	);`

// MercSlots is how many mercs a character may have hired at once.
const MercSlots = 3

// familiars: what a familiar is and does, by the item whose Use calls it
// (handlers/familiar.go, entity/familiar.go; a GM's petition 1445,
// Healbot the first: tools/seed_familiars.py), GM-editable: its name, body
// (CHAR.ESF, as npcs.modelid) and size, its hit points and what it regenerates
// each six-second tick, the item's cast and its visual (0: a buff's stand-in),
// and its heal of its owner: so much (0: none), below heal_below percent of
// its owner's hit points, at most every heal_every_ms, its visual (0: a heal's).
const familiarsSchema = `
	CREATE TABLE IF NOT EXISTS familiars (
		patternid     INTEGER PRIMARY KEY,
		name          TEXT NOT NULL,
		model         INTEGER NOT NULL DEFAULT 0,
		size          REAL NOT NULL DEFAULT 1,
		hp            INTEGER NOT NULL DEFAULT 100000,
		regen         INTEGER NOT NULL DEFAULT 0,
		cast_ms       INTEGER NOT NULL DEFAULT 2000,
		cast_fx       INTEGER NOT NULL DEFAULT 0,
		heal          INTEGER NOT NULL DEFAULT 0,
		heal_below    INTEGER NOT NULL DEFAULT 75,
		heal_every_ms INTEGER NOT NULL DEFAULT 6000,
		heal_fx       INTEGER NOT NULL DEFAULT 0,
		note          TEXT NOT NULL DEFAULT '',
		source        TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureMercs makes tables charmerc and familiars (initSchema).
func (db *SQLiteDB) ensureMercs() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'charmerc'`).Scan(&n); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if n > 0 {
		// path, pet, offensive and spells_off, to a table made before them
		for _, col := range []struct{ name, def string }{{"path", "INTEGER NOT NULL DEFAULT -1"}, {"pet", "TEXT NOT NULL DEFAULT ''"},
			{"offensive", "INTEGER NOT NULL DEFAULT 0"}, {"spells_off", "TEXT NOT NULL DEFAULT ''"}, {"aoe", "INTEGER NOT NULL DEFAULT 0"},
			{"master", "TEXT NOT NULL DEFAULT ''"}, {"spell_slots", "TEXT NOT NULL DEFAULT ''"},
			{"hide_helm", "INTEGER NOT NULL DEFAULT 0"}, {"hide_look", "INTEGER NOT NULL DEFAULT 0"},
			{"were", "TEXT NOT NULL DEFAULT ''"}, {"surname", "TEXT NOT NULL DEFAULT ''"}} {
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('charmerc') WHERE name = ?`, col.name).Scan(&n); err != nil {
				return fmt.Errorf("charmerc: %w", err)
			}
			if n == 0 {
				if _, err := db.Exec(`ALTER TABLE charmerc ADD COLUMN ` + col.name + ` ` + col.def); err != nil {
					return fmt.Errorf("charmerc: its %s column: %w", col.name, err)
				}
			}
		}
		if err := db.mercSlotsTable(); err != nil {
			return err
		}
	}
	if _, err := db.Exec(charMercSchema); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if _, err := db.Exec(familiarsSchema); err != nil {
		return fmt.Errorf("familiars: %w", err)
	}
	return db.ensureMercItems() // mercitems.go: what mercs hold, the alchemists' stock
}

// mercSlotsTable makes a table of one merc a character (its key the
// character's id alone, before 2026-11-12) the table of slots: each row its
// slot 1, in one transaction.
func (db *SQLiteDB) mercSlotsTable() error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('charmerc') WHERE name = 'slot'`).Scan(&n); err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	if n > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("charmerc: %w", err)
	}
	defer tx.Rollback()
	const cols = `serverid, name, class, race, sex, face, hair_color, hair_length, hair_style, stance, dead_until, hired, path, pet`
	for _, q := range []string{
		`ALTER TABLE charmerc RENAME TO charmerc_one`,
		charMercSchema,
		`INSERT INTO charmerc (slot, ` + cols + `) SELECT 1, ` + cols + ` FROM charmerc_one`,
		`DROP TABLE charmerc_one`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("charmerc: its slots: %w", err)
		}
	}
	return tx.Commit()
}

// MercRow is one of a character's hires.
type MercRow struct {
	ServerID                               uint32
	Slot                                   int // 1 to MercSlots (0 writes as 1)
	Name                                   string
	Class, Race, Sex                       int
	Face, HairColor, HairLength, HairStyle int
	Stance                                 int
	Path                                   int       // its class path, -1 its class's default
	Pet                                    string    // the pet kind its hirer picked, "" its best
	Offensive                              bool      // a healer's /merc offensive (false: defensive)
	SpellsOff                              []int     // the spells turned off in its menu, by id
	AoE                                    bool      // a Cocktail alchemist's area cocktails (false: single-target potions)
	Master                                 string    // its Master Class (cmcatalog's mastery), "" none
	SpellSlots                             []int     // its ten spell slots by spell id (0 empty), nil none set
	HideHelm                               bool      // /merc helm off: its helm's look hidden (petition 2965)
	HideLook                               bool      // /merc look off: every armour piece's look hidden, the helm's too
	Were                                   string    // /merc were: the were form's spell name, "" none (petition 3069)
	Surname                                string    // /merc surname: after its name on vanilla's plates, "" none (petition 3430)
	DeadUntil                              time.Time // zero: alive
	Hired                                  time.Time
}

const mercCols = `serverid, slot, name, class, race, sex, face, hair_color, hair_length, hair_style, stance, dead_until, hired, path, pet, offensive, spells_off, aoe, master, spell_slots, hide_helm, hide_look, were, surname`

// scanMerc reads a row of mercCols.
func scanMerc(sc interface{ Scan(...any) error }) (MercRow, error) {
	var r MercRow
	var dead, hired, off, slots string
	if err := sc.Scan(&r.ServerID, &r.Slot, &r.Name, &r.Class, &r.Race, &r.Sex, &r.Face, &r.HairColor, &r.HairLength, &r.HairStyle,
		&r.Stance, &dead, &hired, &r.Path, &r.Pet, &r.Offensive, &off, &r.AoE, &r.Master, &slots, &r.HideHelm, &r.HideLook, &r.Were, &r.Surname); err != nil {
		return MercRow{}, err
	}
	r.SpellsOff = parseSpellIDs(off)
	r.SpellSlots = ParseMercSpellSlots(slots)
	r.DeadUntil, _ = time.Parse(time.RFC3339, dead)
	r.Hired, _ = time.Parse(time.RFC3339, hired)
	return r, nil
}

// MercNamesElsewhere is the names (in lower case) of every other
// character's hires, each with its earliest hire time (zero where unknown),
// and of every character: a new hire's name is none of them, and a hire
// whose name an earlier hire of another character's has is named again at
// its owner's login (handlers/merc.go: petition 2409, two "Arthas" in the
// world, and vanilla's merc list, OpcodeMercsData, names its mercs by name).
func (db *SQLiteDB) MercNamesElsewhere(serverID uint32) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	rows, err := db.Query(`SELECT lower(name), hired FROM charmerc WHERE serverid != ?`, serverID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var name, hired string
		if err := rows.Scan(&name, &hired); err != nil {
			rows.Close()
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, hired)
		if was, ok := out[name]; !ok || (!t.IsZero() && (was.IsZero() || t.Before(was))) {
			out[name] = t
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	chars, err := db.Query(`SELECT lower(charName) FROM characters WHERE serverid != ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer chars.Close()
	for chars.Next() {
		var name string
		if err := chars.Scan(&name); err != nil {
			return nil, err
		}
		if _, ok := out[name]; !ok {
			out[name] = time.Time{}
		}
	}
	return out, chars.Err()
}

// Merc is the character's hire of the slot, false for none.
func (db *SQLiteDB) Merc(serverID uint32, slot int) (MercRow, bool, error) {
	r, err := scanMerc(db.QueryRow(`SELECT `+mercCols+` FROM charmerc WHERE serverid = ? AND slot = ?`, serverID, max(slot, 1)))
	if errors.Is(err, sql.ErrNoRows) {
		return MercRow{}, false, nil
	}
	if err != nil {
		return MercRow{}, false, err
	}
	return r, true, nil
}

// Mercs is the character's hires, by slot.
func (db *SQLiteDB) Mercs(serverID uint32) ([]MercRow, error) {
	rows, err := db.Query(`SELECT `+mercCols+` FROM charmerc WHERE serverid = ? ORDER BY slot`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MercRow
	for rows.Next() {
		r, err := scanMerc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetMerc writes the character's hire of the row's slot in place of any it
// had there.
func (db *SQLiteDB) SetMerc(r MercRow) error {
	dead, hired := "", ""
	if !r.DeadUntil.IsZero() {
		dead = r.DeadUntil.UTC().Format(time.RFC3339)
	}
	if !r.Hired.IsZero() {
		hired = r.Hired.UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(`INSERT OR REPLACE INTO charmerc (`+mercCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ServerID,
		max(r.Slot, 1), r.Name, r.Class, r.Race, r.Sex, r.Face, r.HairColor, r.HairLength, r.HairStyle, r.Stance, dead, hired, r.Path, r.Pet,
		r.Offensive, joinSpellIDs(r.SpellsOff), r.AoE, r.Master, joinMercSpellSlots(r.SpellSlots), r.HideHelm, r.HideLook, r.Were, r.Surname)
	return err
}

// SetMercPath keeps the character's merc's class path (-1 its class's
// default).
func (db *SQLiteDB) SetMercPath(serverID uint32, slot, path int) error {
	_, err := db.Exec(`UPDATE charmerc SET path = ? WHERE serverid = ? AND slot = ?`, path, serverID, max(slot, 1))
	return err
}

// SetMercPet keeps the pet kind the character's merc summons ("" its best).
func (db *SQLiteDB) SetMercPet(serverID uint32, slot int, pet string) error {
	_, err := db.Exec(`UPDATE charmerc SET pet = ? WHERE serverid = ? AND slot = ?`, pet, serverID, max(slot, 1))
	return err
}

// SetMercMaster keeps the character's merc's Master Class ("" none).
func (db *SQLiteDB) SetMercMaster(serverID uint32, slot int, master string) error {
	_, err := db.Exec(`UPDATE charmerc SET master = ? WHERE serverid = ? AND slot = ?`, master, serverID, max(slot, 1))
	return err
}

// SetMercSpellsOff keeps the spells the character's merc's menu turned off.
func (db *SQLiteDB) SetMercSpellsOff(serverID uint32, slot int, off []int) error {
	_, err := db.Exec(`UPDATE charmerc SET spells_off = ? WHERE serverid = ? AND slot = ?`, joinSpellIDs(off), serverID, max(slot, 1))
	return err
}

// MercSpellSlotCount is the slots of a merc's spell window: a player's ten
// (the game's two bars of five).
const MercSpellSlotCount = 10

// SetMercSpellSlots keeps the ten spells on the character's merc's slots (nil:
// none set, its whole book cast).
func (db *SQLiteDB) SetMercSpellSlots(serverID uint32, slot int, slots []int) error {
	_, err := db.Exec(`UPDATE charmerc SET spell_slots = ? WHERE serverid = ? AND slot = ?`, joinMercSpellSlots(slots), serverID, max(slot, 1))
	return err
}

// ParseMercSpellSlots reads charmerc.spell_slots: empty, none set (nil), else
// MercSpellSlotCount ids (a word not a number, or a slot past what is kept,
// empty: 0).
func ParseMercSpellSlots(s string) []int {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	out := make([]int, MercSpellSlotCount)
	for i, f := range strings.Split(s, ",") {
		if i >= MercSpellSlotCount {
			break
		}
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil && n > 0 {
			out[i] = n
		}
	}
	return out
}

// joinMercSpellSlots writes charmerc.spell_slots (empty for nil).
func joinMercSpellSlots(slots []int) string {
	if slots == nil {
		return ""
	}
	parts := make([]string, MercSpellSlotCount)
	for i := range parts {
		n := 0
		if i < len(slots) {
			n = max(slots[i], 0)
		}
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

// parseSpellIDs reads charmerc.spells_off: ids comma-separated (a word not
// a number passed over).
func parseSpellIDs(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// joinSpellIDs writes charmerc.spells_off.
func joinSpellIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}

// SetMercOffensive keeps the character's healer merc's offensive (true) or
// defensive (false) mode.
func (db *SQLiteDB) SetMercOffensive(serverID uint32, slot int, on bool) error {
	_, err := db.Exec(`UPDATE charmerc SET offensive = ? WHERE serverid = ? AND slot = ?`, on, serverID, max(slot, 1))
	return err
}

// SetMercAoE keeps the character's Cocktail alchemist merc's throws: its
// area cocktails (true) or its single-target potions (false).
func (db *SQLiteDB) SetMercAoE(serverID uint32, slot int, on bool) error {
	_, err := db.Exec(`UPDATE charmerc SET aoe = ? WHERE serverid = ? AND slot = ?`, on, serverID, max(slot, 1))
	return err
}

// SetMercHideHelm keeps the character's merc's /merc helm: hide true hides
// its helm's look (petition 2965), false shows it.
func (db *SQLiteDB) SetMercHideHelm(serverID uint32, slot int, hide bool) error {
	_, err := db.Exec(`UPDATE charmerc SET hide_helm = ? WHERE serverid = ? AND slot = ?`, hide, serverID, max(slot, 1))
	return err
}

// SetMercHideLook keeps the character's merc's /merc look: hide true hides
// every armour piece's look (petition 2965's follow-up), false shows them.
func (db *SQLiteDB) SetMercHideLook(serverID uint32, slot int, hide bool) error {
	_, err := db.Exec(`UPDATE charmerc SET hide_look = ? WHERE serverid = ? AND slot = ?`, hide, serverID, max(slot, 1))
	return err
}

// SetMercWere keeps the character's merc's were form (its spell's name, ""
// none: petition 3069).
func (db *SQLiteDB) SetMercWere(serverID uint32, slot int, were string) error {
	_, err := db.Exec(`UPDATE charmerc SET were = ? WHERE serverid = ? AND slot = ?`, were, serverID, max(slot, 1))
	return err
}

// SetMercStance keeps the character's merc's stance.
func (db *SQLiteDB) SetMercStance(serverID uint32, slot, stance int) error {
	_, err := db.Exec(`UPDATE charmerc SET stance = ? WHERE serverid = ? AND slot = ?`, stance, serverID, max(slot, 1))
	return err
}

// SetMercRest keeps when the character's merc's rest after its death ends
// (zero: alive again).
func (db *SQLiteDB) SetMercRest(serverID uint32, slot int, until time.Time) error {
	v := ""
	if !until.IsZero() {
		v = until.UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(`UPDATE charmerc SET dead_until = ? WHERE serverid = ? AND slot = ?`, v, serverID, max(slot, 1))
	return err
}

// DeleteMerc ends the character's hire of the slot, what it held put in the
// keeping (mercitems.go: back to its owner's pack as it has room) in the same
// transaction: no hire ends with an item lost.
func (db *SQLiteDB) DeleteMerc(serverID uint32, slot int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE charmercitem SET slot = ?, equiploc = -1 WHERE serverid = ? AND slot = ?`, MercKeeping, serverID, max(slot, 1)); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM charmerc WHERE serverid = ? AND slot = ?`, serverID, max(slot, 1)); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteMercs ends every hire of the character's (its deletion), what they
// held and its alchemists' stock with it, as its own pack goes.
func (db *SQLiteDB) DeleteMercs(serverID uint32) error {
	for _, q := range []string{`DELETE FROM charmerc WHERE serverid = ?`, `DELETE FROM charmercitem WHERE serverid = ?`,
		`DELETE FROM charmercstock WHERE serverid = ?`} {
		if _, err := db.Exec(q, serverID); err != nil {
			return err
		}
	}
	return nil
}

// FamiliarRow is a familiar as table familiars has it.
type FamiliarRow struct {
	PatternID   int
	Name        string
	Model       uint32
	Size        float64
	HP, Regen   int
	CastMS      int
	CastFX      uint32
	Heal        int
	HealBelow   int
	HealEveryMS int
	HealFX      uint32
}

// Familiar is the familiar the item calls, false for an item that calls none.
func (db *SQLiteDB) Familiar(patternID int) (FamiliarRow, bool, error) {
	var r FamiliarRow
	var model, castFX, healFX int64
	err := db.QueryRow(`SELECT patternid, name, model, size, hp, regen, cast_ms, cast_fx, heal, heal_below, heal_every_ms, heal_fx
		FROM familiars WHERE patternid = ?`, patternID).Scan(&r.PatternID, &r.Name, &model, &r.Size, &r.HP, &r.Regen, &r.CastMS,
		&castFX, &r.Heal, &r.HealBelow, &r.HealEveryMS, &healFX)
	if errors.Is(err, sql.ErrNoRows) {
		return FamiliarRow{}, false, nil
	}
	if err != nil {
		return FamiliarRow{}, false, err
	}
	r.Model, r.CastFX, r.HealFX = uint32(model), uint32(castFX), uint32(healFX)
	return r, true, nil
}

// FamiliarItems is the items that call a familiar, by their record ids.
func (db *SQLiteDB) FamiliarItems() (map[int]bool, error) {
	rows, err := db.Query(`SELECT patternid FROM familiars`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
