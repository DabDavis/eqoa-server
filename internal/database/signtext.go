package database

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// signtext: what a sign post or an announcement board says when talked to
// (handlers/signs.go): the user, 2026-10-30, "we also need to add signpost
// dialog, in cities usually it is the title of the building, on the roads
// outside town, they usually give directions to nearest towns in 4
// directions or so". No record of what EQOA's signs said is known (the
// server sent it, and no capture holds one), so tools/seed_signs.py makes
// each sign's words from where it stands (source "ours"), a GM's to change
// in the panel.
//
// A row is one sign, by its npcs row (npcid); the row of npcid 0 is the
// news every board says after its own words (empty: none). kind is what the
// words are: building (the building it stands at), place (the place its
// name gives), city (its city's places each way), road (the towns each way),
// board, news or gm; only board and news change what is said. text is the
// words, as many lines as they have; CLIENT shows a box of 217 characters
// (a longer text goes in several, handlers/quests.go's splitSay) and starts
// one of its colour codes at a '|'.
const signTextSchema = `
CREATE TABLE IF NOT EXISTS signtext (
	npcid  INTEGER PRIMARY KEY,
	kind   TEXT NOT NULL DEFAULT 'gm',
	text   TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT 'gm',
	note   TEXT NOT NULL DEFAULT ''
);`

// The kinds of a signtext row.
const (
	SignKindBuilding = "building"
	SignKindPlace    = "place"
	SignKindCity     = "city"
	SignKindRoad     = "road"
	SignKindBoard    = "board"
	SignKindNews     = "news"
	SignKindGM       = "gm"
)

// SignNewsID is the npcid of the news row.
const SignNewsID = 0

// SignTextMax is the most characters a sign says: a few boxes.
const SignTextMax = 1000

// SignKinds are the kinds a row may have.
var SignKinds = []string{SignKindBuilding, SignKindPlace, SignKindCity, SignKindRoad, SignKindBoard, SignKindNews, SignKindGM}

// ValidSignKind reports a kind a row may have.
func ValidSignKind(k string) bool {
	for _, s := range SignKinds {
		if k == s {
			return true
		}
	}
	return false
}

// SignTextProblem is what is wrong with a row of these values, or "": the
// news row and only it is npcid 0 of kind news, a sign is a positive npcid,
// and the words fit SignTextMax.
func SignTextProblem(npcid int, kind, text string) string {
	switch {
	case npcid < 0:
		return "npcid is the sign's npcs row (0 for the news row)"
	case !ValidSignKind(kind):
		return "kind is one of " + strings.Join(SignKinds, ", ")
	case (npcid == SignNewsID) != (kind == SignKindNews):
		return "the news row is npcid 0 and kind news, and no other row is either"
	case len([]rune(text)) > SignTextMax:
		return fmt.Sprintf("text is at most %d characters", SignTextMax)
	}
	return ""
}

func (db *SQLiteDB) ensureSignText() error {
	if _, err := db.Exec(signTextSchema); err != nil {
		return fmt.Errorf("signtext: %w", err)
	}
	return nil
}

// SignText is one row of signtext.
type SignText struct {
	NPCID              int
	Kind, Text, Source string
}

// signTextFor is how long a read of signtext is kept: a row a tool writes is
// said within it, a GM's edit at once (InvalidateSignText).
const signTextFor = 30 * time.Second

var signTextCache struct {
	sync.Mutex
	rows map[int]SignText
	at   time.Time
	db   *SQLiteDB
}

// InvalidateSignText makes the next SignTexts read the table again.
func InvalidateSignText() {
	signTextCache.Lock()
	signTextCache.at = time.Time{}
	signTextCache.Unlock()
}

// SignTexts is every row of signtext by npcid, read at most every
// signTextFor; the map is shared, never written. Never call it holding the
// World's lock or another query's rows: it may query (the database has one
// connection).
func (db *SQLiteDB) SignTexts() (map[int]SignText, error) {
	signTextCache.Lock()
	defer signTextCache.Unlock()
	if signTextCache.db == db && time.Since(signTextCache.at) < signTextFor {
		return signTextCache.rows, nil
	}
	rows, err := db.Query(`SELECT npcid, kind, text, source FROM signtext`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]SignText)
	for rows.Next() {
		var s SignText
		if err := rows.Scan(&s.NPCID, &s.Kind, &s.Text, &s.Source); err != nil {
			return nil, err
		}
		out[s.NPCID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	signTextCache.rows, signTextCache.at, signTextCache.db = out, time.Now(), db
	return out, nil
}
