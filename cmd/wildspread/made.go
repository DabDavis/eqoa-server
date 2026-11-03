package main

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
)

// Kinds this server has no row of, made for a world's gaps from the body of
// a kind it has: Prima's bestiary's Rathe Mountains beasts of levels 21 to
// 60 (the user, 2026-10-31: "yes add rathe west"). Rathe's west, its land of
// levels 45 to 63, had no wildlife the bestiary lists at those levels, so
// the fill left it empty. Each made kind is drawn only where the bestiary
// would draw it (its section, its levels: fillGaps' fallback), its rows
// copies of its body's (the model, the size, the flags), and it takes its
// body's rows in the tables a name keys (madeTables: its class, its class
// template, its loot region and its loot pool), so it fights, drops and
// wanders as its body's kind does. A name that is no animal's by its words
// (entity.WildlifeWord: a hydra, a timberwolf) gets an npcmove row that
// wanders it, as tools/seed_npc_move.py writes one for an animal by its
// body. All of it is written by -apply and logged in wildspread_kinds;
// -undo takes it out.

// madeKind is a kind to make: its name as the bestiary writes it (in lower
// case: a creature no one in particular), the kind whose body it takes, and
// the world it is made for.
type madeKind struct {
	name, body string
	world      int
}

// madeKinds are Prima's Rathe Mountains beasts this server has no row of
// (2026-10-31), each with the nearest body it has: the basilisks a
// basilisk's, the dire timberwolf a timber wolf's, the rhinoceroses a
// rhinoceros's (Odus's), the hydras the Cave Hydra's (Lavastorm's: a
// hydra's, the one this server has). Its aviaks, hobgoblins and undead ogres
// are camps, not wildlife, and are not made.
var madeKinds = []madeKind{
	{"mountain basilisk", "basilisk", 1},
	{"lightning basilisk", "basilisk", 1},
	{"electric basilisk", "basilisk", 1},
	{"firestorm basilisk", "basilisk", 1},
	{"dire timberwolf", "timber wolf", 1},
	{"wooly rhinoceros", "rhinoceros", 1},
	{"ironhorn rhinoceros", "rhinoceros", 1},
	{"hydra", "cave hydra", 1},
	{"dune hydra", "cave hydra", 1},
	{"mountain hydra", "cave hydra", 1},
}

// madeLootRegion is the loot region (lootcreature) a made kind of each world
// takes, its body's being another world's.
var madeLootRegion = map[int]string{1: "rathe mountains", 2: "odus"}

// made is what a made kind carries: its body's key and the world it is for.
type made struct {
	body  string
	world int
}

// makeKinds adds to the plan's kinds those of madeKinds for worlds whose
// name has no row, the bestiary lists for the world and whose body has rows;
// it says what it made and what it did not.
func (p *plan) makeKinds(worlds map[int]bool) {
	by := map[string][]*npcRow{}
	for _, r := range p.in.rows {
		by[r.key] = append(by[r.key], r)
	}
	for _, m := range madeKinds {
		if !worlds[m.world] {
			continue
		}
		key := database.NPCClassKey(m.name)
		switch {
		case len(by[key]) > 0:
			say("made kinds: %q has rows of its own: not made", m.name)
			continue
		case len(p.prima[key]) == 0:
			say("made kinds: Prima's bestiary does not list %q: not made", m.name)
			continue
		}
		var body []*npcRow
		for _, r := range by[database.NPCClassKey(m.body)] {
			if r.model != 0 && !p.in.players[r.model] && r.typ&0x80 == 0 {
				body = append(body, r)
			}
		}
		if len(body) == 0 {
			say("made kinds: %q's body, %q, has no row: not made", m.name, m.body)
			continue
		}
		p.kinds[key] = &kind{key: key, spelling: m.name, rows: body, spots: map[int]int{},
			made: &made{body: database.NPCClassKey(m.body), world: m.world}}
		say("made kinds: %q from %q's body (%d rows)", m.name, m.body, len(body))
	}
}

// madeTables are the tables a name keys that a made kind takes its body's
// rows of: the table, its name column, and its key column when its rows are
// numbered (lootpool's id), else "" (the name is its key).
var madeTables = []struct{ table, name, id string }{
	{"npcclass", "name", ""},
	{"npctemplate", "name", ""},
	{"lootcreature", "npc_name", ""},
	{"lootpool", "npc_name", "id"},
}

func kindsSchema() string {
	return `
CREATE TABLE ` + logs.kinds + ` (
	seq   INTEGER PRIMARY KEY AUTOINCREMENT,
	name  TEXT NOT NULL,    -- the made kind
	tbl   TEXT NOT NULL,    -- the table a row was added to
	rowid INTEGER NOT NULL  -- the row's rowid there
);`
}

// writeMadeKind adds a made kind's rows to the tables a name keys (its
// body's, renamed; its world's loot region; an npcmove row wandering one
// that is no animal by its words), each logged in wildspread_kinds. A table
// that already holds a row of the name keeps it.
func writeMadeKind(tx *sql.Tx, k *kind) (int, error) {
	n := 0
	logRow := func(tbl string, res sql.Result) error {
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO `+logs.kinds+` (name, tbl, rowid) VALUES (?, ?, ?)`, k.spelling, tbl, id)
		n++
		return err
	}
	for _, t := range madeTables {
		var have int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM `+t.table+` WHERE lower(trim(`+t.name+`)) = ?`, k.key).Scan(&have); err != nil {
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			return n, err
		}
		if have > 0 {
			continue
		}
		rows, err := tx.Query(`SELECT * FROM `+t.table+` WHERE lower(trim(`+t.name+`)) = ?`, k.made.body)
		if err != nil {
			return n, err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return n, err
		}
		var copies [][]any
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return n, err
			}
			copies = append(copies, vals)
		}
		rows.Close()
		for _, vals := range copies {
			var names, marks []string
			var args []any
			for i, c := range cols {
				switch c {
				case t.id:
					continue
				case t.name:
					args = append(args, k.spelling)
				case "source":
					args = append(args, "wildspread")
				case "region":
					if r, ok := madeLootRegion[k.made.world]; ok && t.table == "lootcreature" {
						args = append(args, r)
					} else {
						args = append(args, vals[i])
					}
				default:
					args = append(args, vals[i])
				}
				names = append(names, c)
				marks = append(marks, "?")
			}
			res, err := tx.Exec(`INSERT INTO `+t.table+` (`+strings.Join(names, ", ")+`) VALUES (`+strings.Join(marks, ", ")+`)`, args...)
			if err != nil {
				return n, fmt.Errorf("%s: %w", t.table, err)
			}
			if err := logRow(t.table, res); err != nil {
				return n, err
			}
		}
	}
	if entity.WildlifeWord(k.spelling) == "" {
		var have int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM npcmove WHERE lower(trim(name)) = ?`, k.key).Scan(&have); err != nil {
			return n, err
		}
		if have == 0 {
			res, err := tx.Exec(`INSERT INTO npcmove (name, roam, radius, water, note, source) VALUES (?, 1, 0, '', ?, 'wildspread')`,
				k.spelling, fmt.Sprintf("wanders: an animal by its body (as %s)", k.made.body))
			if err != nil {
				return n, fmt.Errorf("npcmove: %w", err)
			}
			if err := logRow("npcmove", res); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// undoMadeKinds deletes every row writeMadeKind logged.
func undoMadeKinds(tx *sql.Tx) (int, error) {
	rows, err := tx.Query(`SELECT tbl, rowid FROM ` + logs.kinds)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	type ref struct {
		tbl string
		id  int64
	}
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.tbl, &r.id); err != nil {
			rows.Close()
			return 0, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	n := 0
	for _, r := range refs {
		res, err := tx.Exec(`DELETE FROM `+r.tbl+` WHERE rowid = ?`, r.id)
		if err != nil {
			return n, err
		}
		if c, _ := res.RowsAffected(); c == 1 {
			n++
		}
	}
	if _, err := tx.Exec(`DROP TABLE ` + logs.kinds); err != nil {
		return n, err
	}
	return n, nil
}
