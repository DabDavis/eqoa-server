package database

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// npcoverseer: the NPCs who, talked to, say which bosses are up (handlers/overseer.go;
// petition 1699, Skywalker: "Overseer Yurgulch can be used to check whether Bertoxxulous,
// Uzrathixius, or Poxathixius spawns are up and available to kill", the wiki's Plane of
// Disease: Bertoxxulous Quest). A row is one boss one overseer tells of, in ord's order:
//
//   - overseer, boss: NPC names, any case;
//   - world: the boss's world, -1 the overseer's own;
//   - trigger: the name of the gmtriggers row that brings the boss (a throne's, an
//     altar's), ” for a boss of its own npcs row: while a run of that trigger's lock
//     lives and the boss is not up, its place rests, until the run ends;
//   - greeting: said first, from the overseer's first row by ord that has one;
//   - up, down, ready, resting, unknown: the line for each state, ” for the plain
//     default; %b the boss, %t when ("soon", "in about 5 hours"), %p the player.
//     down: dead, back by its respawn at %t; ready: a trigger's boss not up, the
//     trigger free to fire; resting: its lock held, free at %t; unknown: none up and
//     no time known (a GM holds it, no row of it stands, its trigger is off).
const npcOverseerSchema = `
	CREATE TABLE IF NOT EXISTS npcoverseer (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		overseer TEXT NOT NULL,
		boss     TEXT NOT NULL,
		world    INTEGER NOT NULL DEFAULT -1,
		trigger  TEXT NOT NULL DEFAULT '',
		ord      INTEGER NOT NULL DEFAULT 0,
		greeting TEXT NOT NULL DEFAULT '',
		up       TEXT NOT NULL DEFAULT '',
		down     TEXT NOT NULL DEFAULT '',
		ready    TEXT NOT NULL DEFAULT '',
		resting  TEXT NOT NULL DEFAULT '',
		unknown  TEXT NOT NULL DEFAULT '',
		note     TEXT NOT NULL DEFAULT '',
		source   TEXT NOT NULL DEFAULT 'gm'
	);`

// ensureNPCOverseer makes table npcoverseer (initSchema).
func (db *SQLiteDB) ensureNPCOverseer() error {
	if _, err := db.Exec(npcOverseerSchema); err != nil {
		return fmt.Errorf("npcoverseer: %w", err)
	}
	return nil
}

// NPCOverseerRow is one row of npcoverseer.
type NPCOverseerRow struct {
	ID                                          int
	Overseer, Boss                              string
	World                                       int
	Trigger                                     string
	Ord                                         int
	Greeting, Up, Down, Ready, Resting, Unknown string
}

// npcOverseerFor is how long a read of npcoverseer is kept: a tool's row is said
// within it, a GM's edit at once (InvalidateNPCOverseer).
const npcOverseerFor = 30 * time.Second

var npcOverseerCache struct {
	sync.Mutex
	rows map[string][]NPCOverseerRow
	at   time.Time
	db   *SQLiteDB
}

// InvalidateNPCOverseer makes the next NPCOverseers read the table again.
func InvalidateNPCOverseer() {
	npcOverseerCache.Lock()
	npcOverseerCache.at = time.Time{}
	npcOverseerCache.Unlock()
}

// NPCOverseers is npcoverseer by NPCClassKey of the overseer's name, each one's rows
// by ord then id (a row of a blank overseer or boss left out); the map is shared,
// never written. Never call it holding the World's lock.
func (db *SQLiteDB) NPCOverseers() (map[string][]NPCOverseerRow, error) {
	npcOverseerCache.Lock()
	defer npcOverseerCache.Unlock()
	if npcOverseerCache.db == db && time.Since(npcOverseerCache.at) < npcOverseerFor {
		return npcOverseerCache.rows, nil
	}
	rows, err := db.Query(`SELECT id, overseer, boss, world, trigger, ord, greeting, up, down, ready, resting, unknown
		FROM npcoverseer`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]NPCOverseerRow)
	for rows.Next() {
		var r NPCOverseerRow
		if err := rows.Scan(&r.ID, &r.Overseer, &r.Boss, &r.World, &r.Trigger, &r.Ord,
			&r.Greeting, &r.Up, &r.Down, &r.Ready, &r.Resting, &r.Unknown); err != nil {
			return nil, err
		}
		k := NPCClassKey(r.Overseer)
		if k == "" || strings.TrimSpace(r.Boss) == "" {
			continue
		}
		out[k] = append(out[k], r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, rs := range out {
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].Ord != rs[j].Ord {
				return rs[i].Ord < rs[j].Ord
			}
			return rs[i].ID < rs[j].ID
		})
	}
	npcOverseerCache.rows, npcOverseerCache.at, npcOverseerCache.db = out, time.Now(), db
	return out, nil
}
