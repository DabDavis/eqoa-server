package database

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// questnotyet: what a quest giver says to a player who meets everything its
// next quest asks but the level (handlers/quests_notyet.go): the user,
// 2026-10-30, "for the early quest givers if they are the next quest in line,
// but the player is too low level like dust is level 6 and cannot take level
// 7 q yet let the quest npc say come back when you are stronger, but word it
// in the city flavor language". No record of what the live game's givers said
// then exists here: the lines are ours, each city's in its people's voice (its
// quests' words and its town dialog's, npcdialog), a GM's to change in the
// panel.
//
// A row is one line and the city whose givers say it:
//   - city: the giver's city, spelled any way the game's data spells it
//     (QuestCityKey): the starting city of the quest's import key
//     ("1-20/neriak-nektulos/..." is Neriak's), Weres for the were quests,
//     else the town the giver stands in (its faction's, as npcdialog's town,
//     else the coach map's); "" the lines of a giver whose city has none of
//     its own;
//   - line: the words, said as the giver's with its name before them in a
//     dialogue box (0x0046: 217 characters show, a longer line goes on in
//     another box); %p is the player's name.
//
// The table is made with its seed lines (source seed): a database that has it
// keeps what is in it, rows a GM took out included.
const questNotYetSchema = `
CREATE TABLE IF NOT EXISTS questnotyet (
	id     INTEGER PRIMARY KEY AUTOINCREMENT,
	city   TEXT NOT NULL DEFAULT '',
	line   TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT 'gm'
);`

// questNotYetSeed is the lines the table is made with: two for each starting
// city of tools/quests/data and for the were quests, one for each other town
// a later guide's giver stands in, and one for a giver of no known city. Every
// one says "come back when you are stronger" in its city's way (Oggok's first
// is the user's lane brief's own example) and fits any speaker of the city:
// no address names the player's sex, as town dialog's one pool of lines is
// heard from every speaker.
var questNotYetSeed = []struct{ City, Line string }{
	{"Neriak", "You are not yet worth the trouble it would take to use you. Come back when you are stronger."},
	{"Neriak", "The Houses have no use for the weak, and neither do I. Return when you are stronger, or do not return at all."},
	{"Freeport", "What comes next would chew you up and sell the bones down at the docks. Come back when you are stronger."},
	{"Freeport", "Freeport pays for results, not for funerals. Come back when you are strong enough to earn your coin."},
	{"Qeynos", "Your heart is willing, but the road ahead is harder than you know. Come back when you are stronger."},
	{"Qeynos", "Qeynos needs you alive, not brave and buried. Train a while longer, then come back when you are stronger."},
	{"Halas", "Ha! Ye have the spirit, but not yet the arm. Go and hunt a while, and come back when ye are stronger!"},
	{"Halas", "The frost out there would crack ye like a dry twig. Fill your belly, swing your blade, and come back when ye are stronger."},
	{"Rivervale", "Oh, you are not quite ready for this one, dear. Have a good meal and a good rest, and come back when you are stronger."},
	{"Rivervale", "The goblins out there would gobble you up like a second breakfast! Come back when you are a bit stronger."},
	{"Klick'Anon", "My calculations say you would not survive the next part. Recalibrate a while, and come back when you are stronger."},
	{"Klick'Anon", "You are a fine prototype, but not yet field-tested. Come back when you are stronger, and we will run the trial again."},
	{"Moradhim", "Soft iron breaks on the anvil. Back to the forge with you, and come back when you are stronger."},
	{"Moradhim", "By Brell's beard, you would not last a day down there. Come back when you are stronger, and we will talk over an ale."},
	{"Highbourne", "One does not open the second volume before mastering the first. Come back when you are stronger."},
	{"Highbourne", "Your enthusiasm is noted. Your readiness is not. Study, practice, and come back when you are stronger."},
	{"Fayspire", "Patience. Even the spires were not raised in a day. Come back when you are stronger."},
	{"Fayspire", "Fayspires has buried too many who went out too soon. Come back when you are stronger."},
	{"Tethelin", "A sapling bends in every wind. Grow a while longer, and come back when you are stronger."},
	{"Tethelin", "The forest is patient, and so must you be. Come back when you are stronger."},
	{"Surefall Glade", "Out there everything is hunting, and today you would be the prey. Come back when you are stronger."},
	{"Surefall Glade", "Let the woods toughen you a while longer. The glade will wait. Come back when you are stronger."},
	{"Grobb", "You too weak. Froglok eat you. Come back when you strong."},
	{"Grobb", "Swamp eat little ones like you. Go hunt. Get big. Then come back."},
	{"Oggok", "You too puny. Come back when you big and strong."},
	{"Oggok", "You small. You squish easy. Go smash things. Come back when you big."},
	{"Weres", "What prowls these lands by moonlight would tear you apart. Come back when you are stronger."},
	{"Weres", "Too few seasons are on you yet. Come back when the blood in you runs stronger."},
	{"Highpass", "The Serpent's Spine breaks the unready like brittle ice. Come back when you are stronger."},
	{"Castle Lightwolf", "Out here the nights have teeth. Come back when you are strong enough to bite back."},
	{"Oasis", "The sands swallow the unready whole. Come back when you are stronger."},
	{"Forkwatch", "Three roads meet here, and trouble walks all of them. Come back when you are stronger."},
	{"Zentar's Keep", "The hexbones would pick you clean before the snow covered you. Come back when you are stronger."},
	{"Muniel's Tea Garden", "Sit, have a cup of bitterleaf, and give it time. Come back when you are stronger."},
	{"Dark Solace", "Everyone here is running from something. You would not get far enough. Come back when you are stronger."},
	{"Murnf", "Out this far, help is a long way off. Come back when you are stronger."},
	{"Hazinak", "The docks of Hazinak have swallowed better than you. Come back when you are stronger."},
	{"Bobble-by-Water", "The crocodiles would have you before you reached the reeds. Come back when you are a bit stronger."},
	{"Blackwater", "The dark water keeps what it takes. Come back when you are stronger."},
	{"Wyndhaven", "The wind out here would carry you off. Come back when you are stronger."},
	{"Darvar Manor", "Darvar Manor has no use for the unproven. Come back when you are stronger."},
	{"Graumord", "Fear would eat you alive on this isle. Come back when you are stronger."},
	{"", "You are not ready for what I would ask of you. Come back when you are stronger."},
}

// ensureQuestNotYet makes table questnotyet (initSchema), with its seed lines
// when the table is new.
func (db *SQLiteDB) ensureQuestNotYet() error {
	var had int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'questnotyet'`).Scan(&had); err != nil {
		return fmt.Errorf("questnotyet: %w", err)
	}
	if _, err := db.Exec(questNotYetSchema); err != nil {
		return fmt.Errorf("questnotyet: %w", err)
	}
	if had > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("questnotyet: %w", err)
	}
	defer tx.Rollback()
	for _, s := range questNotYetSeed {
		if _, err := tx.Exec(`INSERT INTO questnotyet (city, line, source) VALUES (?, ?, 'seed')`, s.City, s.Line); err != nil {
			return fmt.Errorf("questnotyet: the seed lines: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("questnotyet: %w", err)
	}
	return nil
}

// QuestNotYetLine is one row of questnotyet.
type QuestNotYetLine struct {
	ID   int
	City string
	Line string
}

// questNotYetFor is how long a read of questnotyet is kept: a row a tool
// writes is said within it, a GM's edit at once (InvalidateQuestNotYet).
const questNotYetFor = 30 * time.Second

var questNotYetCache struct {
	sync.Mutex
	lines []QuestNotYetLine
	at    time.Time
	db    *SQLiteDB
}

// InvalidateQuestNotYet makes the next QuestNotYet read the table again.
func InvalidateQuestNotYet() {
	questNotYetCache.Lock()
	questNotYetCache.at = time.Time{}
	questNotYetCache.Unlock()
}

// QuestNotYet is every line of questnotyet, read at most every
// questNotYetFor. Never call it holding the World's lock or another query's
// rows: it may query (the database has one connection).
func (db *SQLiteDB) QuestNotYet() ([]QuestNotYetLine, error) {
	questNotYetCache.Lock()
	defer questNotYetCache.Unlock()
	if questNotYetCache.db == db && time.Since(questNotYetCache.at) < questNotYetFor {
		return questNotYetCache.lines, nil
	}
	rows, err := db.Query(`SELECT id, city, line FROM questnotyet ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QuestNotYetLine
	for rows.Next() {
		var l QuestNotYetLine
		if err := rows.Scan(&l.ID, &l.City, &l.Line); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	questNotYetCache.lines, questNotYetCache.at, questNotYetCache.db = out, time.Now(), db
	return out, nil
}

// questCityAliases are the cities the game's data spells more than one way,
// by QuestCityKey's letters: the quest files' slugs ("neriak-nektulos"), the
// factions' towns (entity.DialogTown: "Klick`Anon", "Fayspires", "Highpass")
// and the coach map's (entity.TownAt: "Klik'Anon", "Highpass Hold", "Hazinak
// (Evil)").
var questCityAliases = map[string]string{
	"klikanon":       "klickanon",
	"neriaknektulos": "neriak",
	"nektulos":       "neriak",
	"fayspires":      "fayspire",
	"surefall":       "surefallglade",
	"highpasshold":   "highpass",
	"hazinakgood":    "hazinak",
	"hazinakevil":    "hazinak",
	"gerntarmines":   "gerntar",
	"were":           "weres",
}

// QuestCityKey is a city's name as questnotyet's rows are matched: its letters
// and digits in lower case, one spelling for a city spelled several ways
// (questCityAliases); "" for none.
func QuestCityKey(city string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(city) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	k := b.String()
	if a, ok := questCityAliases[k]; ok {
		return a
	}
	return k
}
