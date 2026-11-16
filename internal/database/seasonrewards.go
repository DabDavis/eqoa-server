package database

import (
	"fmt"
	"strings"
)

// seasonrewards, seasongifts, seasonplay: a season's currency (the owner,
// 2026-11-16: "the weapon enchanter will cost christmas currency for our
// christmas event"; handlers/seasonrewards.go). A row of seasonrewards is a
// season's token (item, an itempattern id) and how it is earned while the
// season is on: drop_pct of the kills of a creature the killer does not see
// green, named_pct of a named one's; gift_count from gift_npc once a day (UTC)
// a character (seasongifts keeps the days); hourly for each full hour in the
// world (seasonplay keeps the seconds toward the next).
const seasonRewardsSchema = `
	CREATE TABLE IF NOT EXISTS seasonrewards (
		season     TEXT PRIMARY KEY COLLATE NOCASE,
		item       INTEGER NOT NULL DEFAULT 0,
		drop_pct   REAL NOT NULL DEFAULT 0,
		named_pct  REAL NOT NULL DEFAULT 0,
		gift_npc   TEXT NOT NULL DEFAULT '',
		gift_count INTEGER NOT NULL DEFAULT 0,
		hourly     INTEGER NOT NULL DEFAULT 0,
		note       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL DEFAULT 'gm'
	);
	CREATE TABLE IF NOT EXISTS seasongifts (
		season   TEXT NOT NULL,
		serverid INTEGER NOT NULL,
		day      TEXT NOT NULL,
		PRIMARY KEY (season, serverid)
	);
	CREATE TABLE IF NOT EXISTS seasonplay (
		season   TEXT NOT NULL,
		serverid INTEGER NOT NULL,
		seconds  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (season, serverid)
	);`

func (db *SQLiteDB) ensureSeasonRewards() error {
	if _, err := db.Exec(seasonRewardsSchema); err != nil {
		return fmt.Errorf("seasonrewards: %w", err)
	}
	return nil
}

// SeasonReward is a row of seasonrewards.
type SeasonReward struct {
	Season    string
	Item      int
	DropPct   float64
	NamedPct  float64
	GiftNPC   string
	GiftCount int
	Hourly    int
}

// ActiveSeasonRewards is the rewards of the seasons that are on (gmseasons).
func (db *SQLiteDB) ActiveSeasonRewards() ([]SeasonReward, error) {
	rows, err := db.Query(`SELECT r.season, r.item, r.drop_pct, r.named_pct, r.gift_npc, r.gift_count, r.hourly
		FROM seasonrewards r JOIN gmseasons s ON s.name = r.season COLLATE NOCASE WHERE s.active != 0 AND r.item > 0 ORDER BY r.season`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeasonReward
	for rows.Next() {
		var r SeasonReward
		if err := rows.Scan(&r.Season, &r.Item, &r.DropPct, &r.NamedPct, &r.GiftNPC, &r.GiftCount, &r.Hourly); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TakeSeasonGift records a character's gift of the season on day, and reports
// whether it is the day's first (false: given already that day).
func (db *SQLiteDB) TakeSeasonGift(season string, serverID uint32, day string) (bool, error) {
	res, err := db.Exec(`INSERT INTO seasongifts (season, serverid, day) VALUES (?, ?, ?)
		ON CONFLICT(season, serverid) DO UPDATE SET day = excluded.day WHERE seasongifts.day != excluded.day`,
		SeasonKey(season), serverID, day)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SeasonPlay is each character's seconds toward its next hour of the season.
func (db *SQLiteDB) SeasonPlay(season string) (map[uint32]int, error) {
	rows, err := db.Query(`SELECT serverid, seconds FROM seasonplay WHERE season = ?`, SeasonKey(season))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uint32]int{}
	for rows.Next() {
		var id uint32
		var s int
		if err := rows.Scan(&id, &s); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

// SetSeasonPlay keeps a character's seconds toward its next hour.
func (db *SQLiteDB) SetSeasonPlay(season string, serverID uint32, seconds int) error {
	_, err := db.Exec(`INSERT INTO seasonplay (season, serverid, seconds) VALUES (?, ?, ?)
		ON CONFLICT(season, serverid) DO UPDATE SET seconds = excluded.seconds`, SeasonKey(strings.TrimSpace(season)), serverID, seconds)
	return err
}
