package database

import (
	"fmt"
	"sync"
	"time"
)

// npcdialog: what an NPC says when it is talked to and nothing else answers
// (no quest, shop, route, bind or sign post's words): the user, 2026-10-29,
// "add dialog to all town NPCs and Guards throughout Norrath, but not
// wildlife of course", "we can improvise our own in eqoa flavor if needed",
// "feel free to vary dialog to make the world interesting". No record of the
// live game's lines for them exists here, and Sandstorm "guessed on theres":
// these are ours (tools/seed_town_dialog.py writes them, source "ours"), a
// GM's to change in the panel. entity/dialog.go chooses who speaks and which
// line.
//
// A row is one line and who says it to whom; an empty selector is any:
//   - town: the speaker's town, as its faction names it ("Freeport",
//     "Klick`Anon"; entity.DialogTown);
//   - role: guard, trader, priest, scholar, worker, noble or citizen, by the
//     speaker's name (entity.NPCRole);
//   - name: one NPC, by name;
//   - mood: "" (neutral, friendly or warm), else a comma list of neutral,
//     friendly, warm, wary and hostile, the listener's standing with the
//     speaker's faction (entity.DialogMood);
//   - race: the listener's race, -1 any;
//   - sex: the speaker's, "male" or "female" by its body (charactermodel),
//     "" any: a line of a husband or a wife is said by the one it fits (the
//     user, 2026-10-29: "most of the city folk are men so would be nice if we
//     had Male/Female lines");
//   - weight: how often against the other lines that fit.
//
// In a line, %p is the listener's name, %r its race, %c its class, %t the
// town and %n the speaker's name.
const npcDialogSchema = `
CREATE TABLE IF NOT EXISTS npcdialog (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	town   TEXT NOT NULL DEFAULT '',
	role   TEXT NOT NULL DEFAULT '',
	name   TEXT NOT NULL DEFAULT '',
	mood   TEXT NOT NULL DEFAULT '',
	race   INTEGER NOT NULL DEFAULT -1,
	sex    TEXT NOT NULL DEFAULT '',
	line   TEXT NOT NULL,
	weight INTEGER NOT NULL DEFAULT 1,
	source TEXT NOT NULL DEFAULT 'gm'
);`

func (db *SQLiteDB) ensureNPCDialog() error {
	if _, err := db.Exec(npcDialogSchema); err != nil {
		return fmt.Errorf("npcdialog: %w", err)
	}
	has, err := hasColumn(db.DB, "npcdialog", "sex")
	if err != nil {
		return fmt.Errorf("npcdialog: %w", err)
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE npcdialog ADD COLUMN sex TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("npcdialog: %w", err)
		}
	}
	return nil
}

// NPCDialogLine is one row of npcdialog.
type NPCDialogLine struct {
	ID                     int
	Town, Role, Name, Mood string
	Race                   int
	Sex                    string // "male", "female" or "" (any)
	Line                   string
	Weight                 int
}

// npcDialogFor is how long a read of npcdialog is kept: a row a tool writes
// is said within it, a GM's edit at once (InvalidateNPCDialog).
const npcDialogFor = 30 * time.Second

var npcDialogCache struct {
	sync.Mutex
	lines []NPCDialogLine
	at    time.Time
	db    *SQLiteDB
}

// InvalidateNPCDialog makes the next NPCDialog read the table again.
func InvalidateNPCDialog() {
	npcDialogCache.Lock()
	npcDialogCache.at = time.Time{}
	npcDialogCache.Unlock()
}

// NPCDialog is every line of npcdialog, read at most every npcDialogFor.
// Never call it holding the World's lock or another query's rows: it may
// query (the database has one connection).
func (db *SQLiteDB) NPCDialog() ([]NPCDialogLine, error) {
	npcDialogCache.Lock()
	defer npcDialogCache.Unlock()
	if npcDialogCache.db == db && time.Since(npcDialogCache.at) < npcDialogFor {
		return npcDialogCache.lines, nil
	}
	rows, err := db.Query(`SELECT id, town, role, name, mood, race, sex, line, weight FROM npcdialog ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NPCDialogLine
	for rows.Next() {
		var l NPCDialogLine
		if err := rows.Scan(&l.ID, &l.Town, &l.Role, &l.Name, &l.Mood, &l.Race, &l.Sex, &l.Line, &l.Weight); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	npcDialogCache.lines, npcDialogCache.at, npcDialogCache.db = out, time.Now(), db
	return out, nil
}
