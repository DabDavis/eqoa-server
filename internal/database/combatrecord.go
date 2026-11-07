package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// charrecord: each character's combat record, lifetime, a row each (the user,
// 2026-11-07: "lets add a combat record for pvp and duels to the menu for players,
// has lifetime kills, deaths, etc."; entity/combatrecord.go says what counts). A
// character's row is made at its first count and only ever counted up, one
// transaction an event, so it survives logouts, keep restarts and swaps as any row
// does; nothing before this table's first deploy is in it.
//
// PvP: kills, deaths, assists (the players who hurt the one killed in the 30 s
// before its death, other than the killer), the kill streak (kills since its last
// PvP death) and the best one, the highest-level player it killed, its last kill
// and its last death (who, at what level, when: RFC 3339 in UTC). Duels: won, lost
// (any end that names it the loser: defeated, surrendered, fled, gone, dead of
// something else), of those surrendered, drawn, and the tunar its wagers won (the
// loser's stake) and lost (its own). Each shard keeps its own, in its own database.
const charRecordSchema = `
	CREATE TABLE IF NOT EXISTS charrecord (
		serverid         INTEGER PRIMARY KEY,
		pvp_kills        INTEGER NOT NULL DEFAULT 0,
		pvp_deaths       INTEGER NOT NULL DEFAULT 0,
		pvp_assists      INTEGER NOT NULL DEFAULT 0,
		streak           INTEGER NOT NULL DEFAULT 0,
		best_streak      INTEGER NOT NULL DEFAULT 0,
		best_kill        TEXT NOT NULL DEFAULT '',
		best_kill_level  INTEGER NOT NULL DEFAULT 0,
		last_kill        TEXT NOT NULL DEFAULT '',
		last_kill_level  INTEGER NOT NULL DEFAULT 0,
		last_kill_at     TEXT NOT NULL DEFAULT '',
		last_death       TEXT NOT NULL DEFAULT '',
		last_death_level INTEGER NOT NULL DEFAULT 0,
		last_death_at    TEXT NOT NULL DEFAULT '',
		duel_wins        INTEGER NOT NULL DEFAULT 0,
		duel_losses      INTEGER NOT NULL DEFAULT 0,
		duel_surrenders  INTEGER NOT NULL DEFAULT 0,
		duel_draws       INTEGER NOT NULL DEFAULT 0,
		wager_won        INTEGER NOT NULL DEFAULT 0,
		wager_lost       INTEGER NOT NULL DEFAULT 0,
		updated          TEXT NOT NULL DEFAULT ''
	);`

// ensureCharRecord makes table charrecord (initSchema).
func (db *SQLiteDB) ensureCharRecord() error {
	if _, err := db.Exec(charRecordSchema); err != nil {
		return fmt.Errorf("charrecord: %w", err)
	}
	return nil
}

// CombatRecord is a character's row of charrecord; a character with none has the
// zero record.
type CombatRecord struct {
	ServerID                     uint32
	PvPKills, PvPDeaths, Assists int
	Streak, BestStreak           int
	BestKill                     string
	BestKillLevel                int
	LastKill                     string
	LastKillLevel                int
	LastKillAt                   time.Time
	LastDeath                    string // who killed it
	LastDeathLevel               int
	LastDeathAt                  time.Time
	DuelWins, DuelLosses         int
	DuelSurrenders, DuelDraws    int
	WagerWon, WagerLost          int
	Updated                      time.Time
}

// Empty reports a record with nothing counted.
func (r CombatRecord) Empty() bool {
	return r.PvPKills == 0 && r.PvPDeaths == 0 && r.Assists == 0 && r.DuelWins == 0 && r.DuelLosses == 0 && r.DuelDraws == 0
}

// HasPvP reports a record with any PvP in it.
func (r CombatRecord) HasPvP() bool {
	return r.PvPKills != 0 || r.PvPDeaths != 0 || r.Assists != 0
}

// HasDuels reports a record with any duel in it.
func (r CombatRecord) HasDuels() bool {
	return r.DuelWins != 0 || r.DuelLosses != 0 || r.DuelDraws != 0
}

// RecordFighter is a character as an event names it: its row's serverid, its
// name and level then.
type RecordFighter struct {
	ServerID uint32
	Name     string
	Level    int
}

// PvPKill is one PvP death: who killed whom, and the players who helped.
type PvPKill struct {
	Killer, Victim RecordFighter
	Assists        []RecordFighter
	At             time.Time
}

// DuelEnd is one duel's end: its two (A the challenger), the loser (0: a draw),
// whether it surrendered, and the tunar the winner took of the loser's stake.
type DuelEnd struct {
	A, B        RecordFighter
	Loser       uint32
	Surrendered bool
	Wager       int
	At          time.Time
}

const recordStamp = time.RFC3339

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(recordStamp)
}

// recordTx runs fn in one transaction, every row it names made first.
func (db *SQLiteDB) recordTx(ids []uint32, now time.Time, fn func(tx *sql.Tx) error) error {
	if err := db.ensureCharRecord(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if id == 0 {
			return errors.New("charrecord: a fighter of no character")
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO charrecord (serverid) VALUES (?)`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE charrecord SET updated = ? WHERE serverid = ?`, stamp(now), id); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordPvPKill counts one PvP death, in one transaction: the killer's kill (its
// streak one more, its best kept, its best kill by level, its last kill), the
// victim's death (its streak ended, its last death) and each helper's assist. A
// helper named twice, or as the killer or the victim, counts once or not at all.
func (db *SQLiteDB) RecordPvPKill(k PvPKill) error {
	if k.Killer.ServerID == k.Victim.ServerID {
		return fmt.Errorf("charrecord: %s killing itself is no PvP kill", k.Killer.Name)
	}
	ids := []uint32{k.Killer.ServerID, k.Victim.ServerID}
	var helpers []RecordFighter
	seen := map[uint32]bool{k.Killer.ServerID: true, k.Victim.ServerID: true}
	for _, a := range k.Assists {
		if a.ServerID == 0 || seen[a.ServerID] {
			continue
		}
		seen[a.ServerID] = true
		helpers = append(helpers, a)
		ids = append(ids, a.ServerID)
	}
	at := stamp(k.At)
	return db.recordTx(ids, k.At, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE charrecord SET pvp_kills = pvp_kills + 1, streak = streak + 1,
			best_streak = MAX(best_streak, streak + 1), last_kill = ?, last_kill_level = ?, last_kill_at = ?
			WHERE serverid = ?`, k.Victim.Name, k.Victim.Level, at, k.Killer.ServerID); err != nil {
			return err
		}
		// the highest-level kill: the first of a level stays
		if _, err := tx.Exec(`UPDATE charrecord SET best_kill = ?, best_kill_level = ?
			WHERE serverid = ? AND (best_kill = '' OR best_kill_level < ?)`,
			k.Victim.Name, k.Victim.Level, k.Killer.ServerID, k.Victim.Level); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE charrecord SET pvp_deaths = pvp_deaths + 1, streak = 0,
			last_death = ?, last_death_level = ?, last_death_at = ? WHERE serverid = ?`,
			k.Killer.Name, k.Killer.Level, at, k.Victim.ServerID); err != nil {
			return err
		}
		for _, a := range helpers {
			if _, err := tx.Exec(`UPDATE charrecord SET pvp_assists = pvp_assists + 1 WHERE serverid = ?`, a.ServerID); err != nil {
				return err
			}
		}
		return nil
	})
}

// RecordDuelEnd counts one duel's end, in one transaction: a draw to both, else
// the winner's win and the wager it took, the loser's loss (and surrender) and the
// wager it lost.
func (db *SQLiteDB) RecordDuelEnd(d DuelEnd) error {
	if d.A.ServerID == d.B.ServerID {
		return fmt.Errorf("charrecord: a duel of %s with itself", d.A.Name)
	}
	if d.Loser != 0 && d.Loser != d.A.ServerID && d.Loser != d.B.ServerID {
		return fmt.Errorf("charrecord: duel loser %d is neither %s nor %s", d.Loser, d.A.Name, d.B.Name)
	}
	if d.Wager < 0 {
		return fmt.Errorf("charrecord: a wager of %d", d.Wager)
	}
	return db.recordTx([]uint32{d.A.ServerID, d.B.ServerID}, d.At, func(tx *sql.Tx) error {
		if d.Loser == 0 {
			_, err := tx.Exec(`UPDATE charrecord SET duel_draws = duel_draws + 1 WHERE serverid IN (?, ?)`, d.A.ServerID, d.B.ServerID)
			return err
		}
		winner := d.A.ServerID
		if d.Loser == d.A.ServerID {
			winner = d.B.ServerID
		}
		if _, err := tx.Exec(`UPDATE charrecord SET duel_wins = duel_wins + 1, wager_won = wager_won + ? WHERE serverid = ?`,
			d.Wager, winner); err != nil {
			return err
		}
		surrendered := 0
		if d.Surrendered {
			surrendered = 1
		}
		_, err := tx.Exec(`UPDATE charrecord SET duel_losses = duel_losses + 1, duel_surrenders = duel_surrenders + ?,
			wager_lost = wager_lost + ? WHERE serverid = ?`, surrendered, d.Wager, d.Loser)
		return err
	})
}

const charRecordColumns = `serverid, pvp_kills, pvp_deaths, pvp_assists, streak, best_streak, best_kill, best_kill_level,
	last_kill, last_kill_level, last_kill_at, last_death, last_death_level, last_death_at,
	duel_wins, duel_losses, duel_surrenders, duel_draws, wager_won, wager_lost, updated`

func scanCombatRecord(row interface{ Scan(...any) error }) (CombatRecord, error) {
	var r CombatRecord
	var killAt, deathAt, updated string
	err := row.Scan(&r.ServerID, &r.PvPKills, &r.PvPDeaths, &r.Assists, &r.Streak, &r.BestStreak, &r.BestKill, &r.BestKillLevel,
		&r.LastKill, &r.LastKillLevel, &killAt, &r.LastDeath, &r.LastDeathLevel, &deathAt,
		&r.DuelWins, &r.DuelLosses, &r.DuelSurrenders, &r.DuelDraws, &r.WagerWon, &r.WagerLost, &updated)
	if err != nil {
		return r, err
	}
	r.LastKillAt, _ = time.Parse(recordStamp, killAt)
	r.LastDeathAt, _ = time.Parse(recordStamp, deathAt)
	r.Updated, _ = time.Parse(recordStamp, updated)
	return r, nil
}

// CombatRecordOf is the character's record: the zero record (its ServerID set)
// for one that has none.
func (db *SQLiteDB) CombatRecordOf(serverID uint32) (CombatRecord, error) {
	r, err := scanCombatRecord(db.QueryRow(`SELECT `+charRecordColumns+` FROM charrecord WHERE serverid = ?`, serverID))
	if errors.Is(err, sql.ErrNoRows) {
		return CombatRecord{ServerID: serverID}, nil
	}
	return r, err
}

// CombatRecordByName is the record of the character of that name (any case) and
// its name as it is spelt; found false when no character has it.
func (db *SQLiteDB) CombatRecordByName(name string) (r CombatRecord, spelt string, found bool, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return r, "", false, nil
	}
	var id uint32
	err = db.QueryRow(`SELECT serverid, charName FROM characters WHERE charName = ? COLLATE NOCASE ORDER BY serverid LIMIT 1`, name).Scan(&id, &spelt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, "", false, nil
	}
	if err != nil {
		return r, "", false, err
	}
	r, err = db.CombatRecordOf(id)
	return r, spelt, err == nil, err
}

// deleteCharRecord forgets a deleted character's record.
func (db *SQLiteDB) deleteCharRecord(serverID uint32) error {
	_, err := db.Exec(`DELETE FROM charrecord WHERE serverid = ?`, serverID)
	return err
}
