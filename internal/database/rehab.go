package database

import (
	"fmt"
	"strings"
	"time"
)

// The rehabilitators (handlers/rehab.go; the user, 2026-10-30): Rehabilitator
// Anja in Muniel's Tea Garden and Rehabilitator Frejo near Freeport undo what
// a character chose. Each character has free resets ("1 free CM [reset] and
// stat points [reset] per char. 2 free path respects per char"), then pays:
// tunar for its masteries and stat points ("1mil for cms, 2mil for both"), a
// Rose of Rehabilitation for a path ("Roses for paths"), which the
// rehabilitators sell ("for 10 million tunar") and a class's level 60 epic
// gives ("1 free path respec w/ 60 epic. Or rose as reward").
//
// rehabsettings is the GMs' (the GM panel's "Rehabilitators' prices"): the
// free resets every character has and the prices, a row a setting, read at
// each talk to a rehabilitator. A setting with no row is its default here; a
// new table is given every default. charresets is each character's count of a
// kind of reset: the free ones it used, the ones it paid for, and any more a
// GM gave it.

// RoseOfRehabilitation is the Rose of Rehabilitation's item (itempattern),
// made by tools/seed_rehabilitation.py (ROSE_ID there): it pays for a path
// chosen again once the free path respecs are used.
const RoseOfRehabilitation = 709300

// RoseOfRehabilitationName is its name, as the tool writes it.
const RoseOfRehabilitationName = "Rose of Rehabilitation"

// The kinds of reset a character counts (charresets.kind).
const (
	ResetMastery  = "mastery"  // every Combat Mastery unlearned, its points back
	ResetTraining = "training" // every training point back, the stats as born
	ResetPath     = "path"     // a path chosen again: a quest's pick changed for another of its options
)

// ResetKinds is every kind, in the menu's order.
var ResetKinds = []string{ResetMastery, ResetTraining, ResetPath}

// RehabSettings is rehabsettings read: the free resets a character has and
// what a rehabilitator asks, in tunar.
type RehabSettings struct {
	FreeMastery, FreeTraining, FreePath int
	// PriceMastery and PriceTraining are a reset of one alone; PriceBoth
	// both together when neither has a free one left; PriceRose a Rose of
	// Rehabilitation
	PriceMastery, PriceTraining, PriceBoth, PriceRose int
}

// Free is the free resets of a kind every character has.
func (s RehabSettings) Free(kind string) int {
	switch kind {
	case ResetMastery:
		return s.FreeMastery
	case ResetTraining:
		return s.FreeTraining
	case ResetPath:
		return s.FreePath
	}
	return 0
}

// RehabSetting is one setting: its name, its default and what it is.
type RehabSetting struct {
	Name    string
	Default int
	Note    string
	set     func(*RehabSettings, int)
}

// RehabSettingRows are the settings, in the panel's order, with their
// defaults: the user's prices of 2026-10-30, and 1,000,000 for a stat point
// reset alone, which the user did not price (the masteries' price, to be
// confirmed).
var RehabSettingRows = []RehabSetting{
	{"free_mastery", 1, "free Combat Mastery resets a character has",
		func(s *RehabSettings, v int) { s.FreeMastery = v }},
	{"free_training", 1, "free stat (training) point resets a character has",
		func(s *RehabSettings, v int) { s.FreeTraining = v }},
	{"free_path", 2, "free path respecs a character has; past them a path costs a Rose of Rehabilitation",
		func(s *RehabSettings, v int) { s.FreePath = v }},
	{"price_mastery", 1_000_000, "tunar for a Combat Mastery reset once the free ones are used",
		func(s *RehabSettings, v int) { s.PriceMastery = v }},
	{"price_training", 1_000_000, "tunar for a stat point reset once the free ones are used (not priced by the user: the masteries' price, to confirm)",
		func(s *RehabSettings, v int) { s.PriceTraining = v }},
	{"price_both", 2_000_000, "tunar for masteries and stat points together, when neither has a free reset left",
		func(s *RehabSettings, v int) { s.PriceBoth = v }},
	{"price_rose", 10_000_000, "tunar for a Rose of Rehabilitation at a rehabilitator",
		func(s *RehabSettings, v int) { s.PriceRose = v }},
}

// DefaultRehabSettings is every setting at its default.
func DefaultRehabSettings() RehabSettings {
	var s RehabSettings
	for _, r := range RehabSettingRows {
		r.set(&s, r.Default)
	}
	return s
}

// RehabSettingNames is every setting's name.
func RehabSettingNames() []string {
	out := make([]string, len(RehabSettingRows))
	for i, r := range RehabSettingRows {
		out[i] = r.Name
	}
	return out
}

// MaxRehabValue is the most a setting holds: a price is taken from a purse,
// which CLIENT reads as a signed 32-bit number.
const MaxRehabValue = MaxBankTunar

const rehabSchema = `
CREATE TABLE IF NOT EXISTS rehabsettings (
	setting TEXT PRIMARY KEY,
	value   INTEGER NOT NULL,
	note    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS charresets (
	serverid INTEGER NOT NULL,
	kind     TEXT NOT NULL,
	used     INTEGER NOT NULL DEFAULT 0,
	paid     INTEGER NOT NULL DEFAULT 0,
	bonus    INTEGER NOT NULL DEFAULT 0,
	updated  TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (serverid, kind)
);`

// ensureRehab makes the two tables, a new rehabsettings with every default
// (initSchema). A setting a GM deleted comes back at its default at the next
// start, as the game reads it meanwhile.
func (db *SQLiteDB) ensureRehab() error {
	if _, err := db.Exec(rehabSchema); err != nil {
		return fmt.Errorf("rehabilitators: %w", err)
	}
	for _, r := range RehabSettingRows {
		if _, err := db.Exec(`INSERT INTO rehabsettings (setting, value, note) VALUES (?, ?, ?)
			ON CONFLICT (setting) DO NOTHING`, r.Name, r.Default, r.Note); err != nil {
			return fmt.Errorf("rehabsettings: %w", err)
		}
	}
	return nil
}

// RehabSettings is rehabsettings as it is now: each setting's row, else its
// default; a value below 0 counts as 0, past MaxRehabValue as that.
func (db *SQLiteDB) RehabSettings() (RehabSettings, error) {
	s := DefaultRehabSettings()
	rows, err := db.Query(`SELECT setting, value FROM rehabsettings`)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var v int64
		if err := rows.Scan(&name, &v); err != nil {
			return s, err
		}
		for _, r := range RehabSettingRows {
			if strings.EqualFold(strings.TrimSpace(name), r.Name) {
				r.set(&s, int(min(max(v, 0), MaxRehabValue)))
			}
		}
	}
	return s, rows.Err()
}

// CharReset is a character's count of one kind of reset.
type CharReset struct {
	Kind string
	// Used is the free ones it used, Paid those it paid for (tunar, or a
	// rose for a path), Bonus free ones a GM gave it besides the setting's
	Used, Paid, Bonus int
	Updated           string
}

// Left is the free resets of its kind a character has left, when every
// character has free of them.
func (r CharReset) Left(free int) int {
	return max(0, free+r.Bonus-r.Used)
}

// CharResets is a character's counts by kind; a kind with no row is all 0.
func (db *SQLiteDB) CharResets(serverID uint32) (map[string]CharReset, error) {
	out := map[string]CharReset{}
	for _, k := range ResetKinds {
		out[k] = CharReset{Kind: k}
	}
	rows, err := db.Query(`SELECT kind, used, paid, bonus, updated FROM charresets WHERE serverid = ?`, serverID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r CharReset
		if err := rows.Scan(&r.Kind, &r.Used, &r.Paid, &r.Bonus, &r.Updated); err != nil {
			return out, err
		}
		out[r.Kind] = r
	}
	return out, rows.Err()
}

// CountReset counts a reset a character had: a free one used, else one paid
// for.
func (db *SQLiteDB) CountReset(serverID uint32, kind string, free bool, at time.Time) error {
	col := "paid"
	if free {
		col = "used"
	}
	_, err := db.Exec(`INSERT INTO charresets (serverid, kind, `+col+`, updated) VALUES (?, ?, 1, ?)
		ON CONFLICT (serverid, kind) DO UPDATE SET `+col+` = `+col+` + 1, updated = excluded.updated`,
		serverID, kind, at.UTC().Format(time.RFC3339))
	return err
}

// deleteCharResets forgets a deleted character's counts: a server id made
// again must not inherit them.
func (db *SQLiteDB) deleteCharResets(serverID uint32) error {
	_, err := db.Exec(`DELETE FROM charresets WHERE serverid = ?`, serverID)
	return err
}

// MerchantScrollNames is the names of the spell scrolls the merchants sell:
// npcitem's items with the scroll art (icon, stored signed or not). A spell
// a path taught that is sold so stays in the book when the path is changed
// (handlers/rehab.go): it may have been bought.
func (db *SQLiteDB) MerchantScrollNames(icon int) ([]string, error) {
	signed := int64(int32(icon))
	rows, err := db.Query(`SELECT DISTINCT p.itemname FROM npcitem n JOIN itempattern p ON p.patternid = n.patternid
		WHERE p.itemicon IN (?, ?)`, signed, int64(uint32(signed)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
