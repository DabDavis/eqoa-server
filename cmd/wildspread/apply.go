package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/eqoa/server/internal/database"
)

const logSchema = `
CREATE TABLE wildspread_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
CREATE TABLE wildspread_moves (
	npc_row INTEGER PRIMARY KEY, -- the npcs row moved
	world   INTEGER NOT NULL,
	name    TEXT NOT NULL,
	old_x REAL NOT NULL, old_y REAL NOT NULL, old_z REAL NOT NULL,
	new_x REAL NOT NULL, new_y REAL NOT NULL, new_z REAL NOT NULL,
	clump   INTEGER NOT NULL
);
CREATE TABLE wildspread_adds (
	npc_row      INTEGER PRIMARY KEY, -- the npcs row added
	world        INTEGER NOT NULL,
	name         TEXT NOT NULL,
	level        INTEGER NOT NULL,
	template_row INTEGER NOT NULL
);`

func hasLog(db *database.SQLiteDB) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'wildspread_meta'`).Scan(&n)
	return n > 0, err
}

// applyPlan writes the plan in one transaction, with its log.
func applyPlan(db *database.SQLiteDB, p *plan, seed int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(logSchema); err != nil {
		return fmt.Errorf("the log tables: %w", err)
	}
	var seqBefore sql.NullInt64
	if err := tx.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'npcs'`).Scan(&seqBefore); err != nil &&
		err != sql.ErrNoRows {
		return err
	}
	moved, added := 0, 0
	for _, wp := range p.worlds {
		for _, m := range wp.moves {
			var ox, oy, oz float64
			if err := tx.QueryRow(`SELECT x_coord, y_coord, z_coord FROM npcs WHERE id = ?`, m.row.id).Scan(&ox, &oy, &oz); err != nil {
				return fmt.Errorf("npcs %d: %w", m.row.id, err)
			}
			if math.Abs(ox-m.row.x) > 0.01 || math.Abs(oy-m.row.y) > 0.01 || math.Abs(oz-m.row.z) > 0.01 {
				return fmt.Errorf("npcs %d (%s) moved while the plan was made: plan again", m.row.id, m.row.name)
			}
			if _, err := tx.Exec(`UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ?`,
				m.x, m.y, m.z, m.row.id); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO wildspread_moves VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				m.row.id, wp.w, m.row.name, ox, oy, oz, m.x, m.y, m.z, m.clump); err != nil {
				return err
			}
			moved++
		}
		for _, f := range wp.fills {
			id, err := insertCopy(tx, f, wp.w)
			if err != nil {
				return fmt.Errorf("a %s from npcs %d: %w", f.kind.spelling, f.template.id, err)
			}
			if _, err := tx.Exec(`INSERT INTO wildspread_adds VALUES (?, ?, ?, ?, ?)`,
				id, wp.w, f.kind.spelling, f.level, f.template.id); err != nil {
				return err
			}
			added++
		}
	}
	params, _ := json.Marshal(map[string]float64{"step": step, "maxRise": maxRise, "onGround": onGround,
		"townClear": townClear, "npcClear": npcClear, "campKeep": campKeep, "clumpLink": clumpLink,
		"territory": territory, "gapFill": gapFill, "fillSpacing": fillSpacing, "kindReach": kindReach})
	meta := map[string]string{
		"applied":    time.Now().UTC().Format(time.RFC3339),
		"seed":       fmt.Sprint(seed),
		"seq_before": fmt.Sprint(seqBefore.Int64),
		"had_seq":    fmt.Sprint(seqBefore.Valid),
		"moved":      fmt.Sprint(moved),
		"added":      fmt.Sprint(added),
		"params":     string(params),
	}
	for k, v := range meta {
		if _, err := tx.Exec(`INSERT INTO wildspread_meta VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	say("applied: %d moved, %d added (wildspread_moves, wildspread_adds)", moved, added)
	return nil
}

// insertCopy adds a new row copying the template's every column but its id,
// its place, its level, npc_id (0: no loot rows of its own) and its death.
func insertCopy(tx *sql.Tx, f fill, world int) (int64, error) {
	rows, err := tx.Query(`SELECT * FROM npcs WHERE id = ?`, f.template.id)
	if err != nil {
		return 0, err
	}
	cols, err := rows.Columns()
	if err != nil {
		rows.Close()
		return 0, err
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if !rows.Next() {
		rows.Close()
		return 0, fmt.Errorf("no such row")
	}
	if err := rows.Scan(ptrs...); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	set := map[string]any{
		"npc_name": f.kind.spelling, "x_coord": f.x, "y_coord": f.y, "z_coord": f.z, "facing": f.facing,
		"world": world, "npc_level": f.level, "npc_id": 0, "dt_kill": nil, "zone": 0,
	}
	var names, marks []string
	var args []any
	for i, c := range cols {
		if c == "id" {
			continue
		}
		names = append(names, c)
		marks = append(marks, "?")
		if v, ok := set[c]; ok {
			args = append(args, v)
		} else {
			args = append(args, vals[i])
		}
	}
	res, err := tx.Exec(`INSERT INTO npcs (`+strings.Join(names, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// undoPlan takes an applied plan out (the file's doc in main.go).
func undoPlan(db *database.SQLiteDB) error {
	ok, err := hasLog(db)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no plan is applied (no wildspread_meta)")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	meta := map[string]string{}
	rows, err := tx.Query(`SELECT k, v FROM wildspread_meta`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		meta[k] = v
	}
	rows.Close()
	type mv struct {
		id                     int64
		name                   string
		ox, oy, oz, nx, ny, nz float64
	}
	var mvs []mv
	rows, err = tx.Query(`SELECT npc_row, name, old_x, old_y, old_z, new_x, new_y, new_z FROM wildspread_moves`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var m mv
		if err := rows.Scan(&m.id, &m.name, &m.ox, &m.oy, &m.oz, &m.nx, &m.ny, &m.nz); err != nil {
			rows.Close()
			return err
		}
		mvs = append(mvs, m)
	}
	rows.Close()
	back, left := 0, 0
	for _, m := range mvs {
		res, err := tx.Exec(`UPDATE npcs SET x_coord = ?, y_coord = ?, z_coord = ? WHERE id = ?
			AND abs(x_coord - ?) < 0.001 AND abs(y_coord - ?) < 0.001 AND abs(z_coord - ?) < 0.001`,
			m.ox, m.oy, m.oz, m.id, m.nx, m.ny, m.nz)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			back++
		} else {
			left++
			say("npcs %d (%s) is not where this tool put it (moved or deleted since): left", m.id, m.name)
		}
	}
	res, err := tx.Exec(`DELETE FROM npcs WHERE id IN (SELECT npc_row FROM wildspread_adds)`)
	if err != nil {
		return err
	}
	deleted, _ := res.RowsAffected()
	var maxID sql.NullInt64
	if err := tx.QueryRow(`SELECT MAX(id) FROM npcs`).Scan(&maxID); err != nil {
		return err
	}
	var seqBefore int64
	fmt.Sscan(meta["seq_before"], &seqBefore)
	if meta["had_seq"] == "true" && maxID.Int64 <= seqBefore {
		if _, err := tx.Exec(`UPDATE sqlite_sequence SET seq = ? WHERE name = 'npcs'`, seqBefore); err != nil {
			return err
		}
	}
	for _, t := range []string{"wildspread_moves", "wildspread_adds", "wildspread_meta"} {
		if _, err := tx.Exec(`DROP TABLE ` + t); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	say("undone: %d moved back, %d left where a GM had put them, %d added rows deleted", back, left, deleted)
	return nil
}
