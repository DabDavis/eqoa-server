package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The loot a creature's corpse rolls by CATEGORY (the user and jib,
// 2026-10-30: rates per tier, per kill; "500 more items change nothing about
// how much drops"). Four tables, all read at every corpse:
//
//   - lootpreset: named sets of rates; the one with active 1 is used ("jib",
//     the live game's GM's, and "user" are tools/seed_loot_categories.py's).
//   - lootrate: a preset's rates by PROFILE and category. A profile is a kind
//     of creature ("xp" the default; "named", "boss", "trash" ...). Each
//     category makes its own rolls, independent of the others: one at rate
//     (a chance a kill, 0-1), a stackable item minstack..maxstack at a time;
//     or, with counts set ("25,31,25,..." the chances of 0, 1, 2, ...
//     items), that many items, each drawn on its own.
//   - lootpool: the items a category's roll draws from, by weight, for the
//     levels minlevel..maxlevel: a creature's own rows (npc_name its name, as
//     npcclass keys it) when it has any for that category, else the rows of
//     its region and of every region ("*"). A pool's size never changes how
//     often its category drops, only which item comes.
//   - lootcreature: a creature's region and profile, by name: region "-"
//     rolls no category; no row, or an empty profile, is profile "xp" and
//     every region's rows alone.
//
// These rolls come beside the loot tables (npcloot: a boss's own items, a
// GM's tables), not in place of them; one set a corpse, whoever opens it.
const lootCategorySchema = `
	CREATE TABLE IF NOT EXISTS lootpreset (
		name   TEXT PRIMARY KEY,
		active INTEGER NOT NULL DEFAULT 0,
		note   TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS lootrate (
		preset   TEXT NOT NULL,
		profile  TEXT NOT NULL,
		category TEXT NOT NULL,
		rate     REAL NOT NULL DEFAULT 0,
		counts   TEXT NOT NULL DEFAULT '',
		minstack INTEGER NOT NULL DEFAULT 1,
		maxstack INTEGER NOT NULL DEFAULT 1,
		position INTEGER NOT NULL DEFAULT 0,
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm',
		PRIMARY KEY (preset, profile, category)
	);
	CREATE TABLE IF NOT EXISTS lootpool (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		category  TEXT NOT NULL,
		region    TEXT NOT NULL DEFAULT '*',
		npc_name  TEXT NOT NULL DEFAULT '',
		patternid INTEGER NOT NULL,
		weight    INTEGER NOT NULL DEFAULT 1,
		minlevel  INTEGER NOT NULL DEFAULT 1,
		maxlevel  INTEGER NOT NULL DEFAULT 100,
		source    TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE INDEX IF NOT EXISTS idx_lootpool_region ON lootpool(region, category);
	CREATE INDEX IF NOT EXISTS idx_lootpool_npc ON lootpool(npc_name);
	CREATE TABLE IF NOT EXISTS lootcreature (
		npc_name TEXT PRIMARY KEY,
		region   TEXT NOT NULL DEFAULT '',
		profile  TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// NoLootRegion is the lootcreature region of a creature that rolls no category.
const NoLootRegion = "-"

// AnyLootRegion is the lootpool region every creature's pool takes.
const AnyLootRegion = "*"

// DefaultLootProfile is the profile of a creature with none of its own, and
// the one whose rates a profile the active preset lacks takes.
const DefaultLootProfile = "xp"

// CategoryRoll is one category a creature rolls: its chance a kill, or the
// chances of each count of items (Counts[n] for n items; any scale), the
// stack a stackable item comes in, and the creature's pool for it at its
// level (none: it gives nothing).
type CategoryRoll struct {
	Category           string
	Rate               float64
	Counts             []float64
	MinStack, MaxStack int
	Choices            []LootChoice
}

// ParseLootCounts reads a counts column: the chances of 0, 1, 2, ... items,
// comma separated; empty, none. A negative or unreadable chance is an error.
func ParseLootCounts(s string) ([]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var out []float64
	for _, f := range strings.Split(s, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("counts %q: %q is no chance", s, f)
		}
		out = append(out, v)
	}
	return out, nil
}

// CategoryRolls is every category a creature of that name rolls at that
// level under the active preset, in the rates' order, each with its pool.
func (s *InventoryStore) CategoryRolls(name string, level int) ([]CategoryRoll, error) {
	key := NPCClassKey(name)
	region, profile := "", ""
	switch err := s.db.QueryRow(`SELECT region, profile FROM lootcreature WHERE npc_name = ?`, key).Scan(&region, &profile); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("lootcreature: %w", err)
	}
	if region == NoLootRegion {
		return nil, nil
	}
	if profile == "" {
		profile = DefaultLootProfile
	}
	var preset string
	switch err := s.db.QueryRow(`SELECT name FROM lootpreset WHERE active <> 0 ORDER BY name LIMIT 1`).Scan(&preset); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("lootpreset: %w", err)
	}
	out, err := s.lootRates(preset, profile)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 && profile != DefaultLootProfile {
		if out, err = s.lootRates(preset, DefaultLootProfile); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	at := map[string]int{}
	for i, r := range out {
		at[r.Category] = i
	}
	pool, err := s.db.Query(`
		SELECT l.category, l.npc_name <> '', 0, 1, p.maxhp, 0, -1, 0, 0,
		`+itemPatternColumns+`, l.weight
		FROM lootpool l
		JOIN itempattern p ON p.patternid = l.patternid
		WHERE l.weight > 0 AND l.minlevel <= ? AND l.maxlevel >= ?
		  AND (l.npc_name = ? OR (l.npc_name = '' AND (l.region = ? OR l.region = ?)))
		ORDER BY l.id`, level, level, key, region, AnyLootRegion)
	if err != nil {
		return nil, fmt.Errorf("lootpool: %w", err)
	}
	defer pool.Close()
	own := map[string][]LootChoice{}
	shared := map[string][]LootChoice{}
	for pool.Next() {
		var cat string
		var mine bool
		var c LootChoice
		e, err := scanItemRow(func(dest ...any) error {
			return pool.Scan(append(append([]any{&cat, &mine}, dest...), &c.Weight)...)
		})
		if err != nil {
			return nil, err
		}
		c.Item = e
		if mine {
			own[cat] = append(own[cat], c)
		} else {
			shared[cat] = append(shared[cat], c)
		}
	}
	if err := pool.Err(); err != nil {
		return nil, err
	}
	for cat, i := range at {
		if len(own[cat]) > 0 {
			out[i].Choices = own[cat]
		} else {
			out[i].Choices = shared[cat]
		}
	}
	return out, nil
}

// lootRates is a preset's rows for a profile, in order.
func (s *InventoryStore) lootRates(preset, profile string) ([]CategoryRoll, error) {
	rows, err := s.db.Query(`SELECT category, rate, counts, minstack, maxstack FROM lootrate
		WHERE preset = ? AND profile = ? ORDER BY position, category`, preset, profile)
	if err != nil {
		return nil, fmt.Errorf("lootrate: %w", err)
	}
	defer rows.Close()
	var out []CategoryRoll
	for rows.Next() {
		var r CategoryRoll
		var counts string
		if err := rows.Scan(&r.Category, &r.Rate, &counts, &r.MinStack, &r.MaxStack); err != nil {
			return nil, err
		}
		if r.Counts, err = ParseLootCounts(counts); err != nil {
			return nil, fmt.Errorf("lootrate %s/%s/%s: %w", preset, profile, r.Category, err)
		}
		if r.Rate <= 0 && len(r.Counts) == 0 {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
