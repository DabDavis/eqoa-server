package handlers

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/fallen"
	"github.com/eqoa/server/internal/session"
)

// memorial.go — the Memorial of the Fallen in Freeport (the owner, 2026-11-18:
// "add a memorial to deceased hardcore players in freeport with timestamps and
// names", "also class cm count etc").
//
// Any NPC named MemorialName is the memorial (tools/memorial_fallen_20261118.py
// stands one, the disc's stone obelisk of Freeport's sign posts made larger,
// between the well and the east sign post on each shard). A talk to it is the
// shard's fallen Hardcore characters, the newest fall first, memorialPerPage a
// talk in dialogue boxes (0x0046, packed: sayPacked, 217 characters a box,
// within CLIENT's ring of eleven), each a line: "Meshuggah, level 52 Shadow
// Knight, 140 CMs, fell 2026-11-18 08:45 UTC in Freeport, slain by a Zek
// sentry." The next talk shows the next page, and after the last the first
// again. Nothing past ASCII or a '|' (a PS2's TextBox: claude-notes
// ps2-textbox-bar-crash-20261107.md) is said: names, killers and places are
// cleaned (memorialClean).

// MemorialName is the memorial's name: an NPC of it is the memorial.
const MemorialName = "Memorial of the Fallen"

// memorialPerPage is how many falls a talk shows.
const memorialPerPage = 6

// The memorial's words (ours).
const (
	memorialHead    = "Memorial of the Fallen. Here stand the Hardcore heroes%s whose first death was their last, the newest first."
	memorialEmpty   = "No Hardcore hero has fallen yet. May it stay so."
	memorialClosed  = "The names on the memorial cannot be read just now. Try again in a while."
	memorialPageOf  = "Page %d of %d. Speak to the memorial again for more."
	memorialOffNote = "There are no Hardcore characters on this server."
)

// memorialDB is the shard's database (registry.go); nil: the memorial says it
// cannot be read.
var memorialDB atomic.Pointer[database.SQLiteDB]

// memorialPages is the page each character reads next, by its serverid.
var memorialPages sync.Map

// IsMemorial is whether an NPC of the name is the memorial.
func IsMemorial(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), MemorialName)
}

// memorialClean is s with nothing a PS2 cannot show: ASCII printable, no '|',
// no '%'.
func memorialClean(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 && r < 0x7F && r != '|' && r != '%' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// MemorialLine is a fall's line.
func MemorialLine(f database.FallRecord) string {
	at := time.Unix(f.At, 0).UTC().Format("2006-01-02 15:04")
	cms := countOfThing(f.CMs, "CM", "CMs")
	return memorialClean(fmt.Sprintf("%s, level %d %s, %s, fell %s UTC in %s, %s.", f.Name, f.Level, entity.ClassName(f.Class),
		cms, at, f.Place, fallVerb(f.Cause, f.Killer)))
}

// memorialLines is page (from 0) of falls: its heading, its lines, its page
// line; and the page that follows it.
func memorialLines(falls []database.FallRecord, page int, shard string) (lines []string, next int) {
	where := ""
	if shard != "" {
		where = " of " + memorialClean(shard)
	}
	lines = []string{fmt.Sprintf(memorialHead, where)}
	if len(falls) == 0 {
		return append(lines, memorialEmpty), 0
	}
	pages := (len(falls) + memorialPerPage - 1) / memorialPerPage
	if page < 0 || page >= pages {
		page = 0
	}
	for _, f := range falls[page*memorialPerPage : min(len(falls), (page+1)*memorialPerPage)] {
		lines = append(lines, MemorialLine(f))
	}
	if pages > 1 {
		lines = append(lines, fmt.Sprintf(memorialPageOf, page+1, pages))
	}
	return lines, (page + 1) % pages
}

// memorialTalk is a talk to npc: the memorial's names when npc is the memorial.
func memorialTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || p == nil || !IsMemorial(npc.Name) {
		return false
	}
	forgetDialogs(p)
	db := memorialDB.Load()
	if db == nil {
		sayPacked(sess, "", []string{memorialClosed})
		return true
	}
	falls, err := db.FallenList(0)
	if err != nil {
		log.Printf("Memorial: the fallen: %v", err)
		sayPacked(sess, "", []string{memorialClosed})
		return true
	}
	page := 0
	if v, ok := memorialPages.Load(p.CharServerID); ok {
		page = v.(int)
	}
	lines, next := memorialLines(falls, page, hardcoreCfg.Load().shard)
	if len(falls) == 0 && !HardcoreOn() {
		lines = []string{memorialOffNote}
	}
	memorialPages.Store(p.CharServerID, next)
	sayPacked(sess, "", lines)
	log.Printf("Memorial: %s read page %d (%d fallen)", p.Name, page+1, len(falls))
	return true
}

// ---------------------------------------------------------------------------
// The Hardcore Leaderboard beside it (the owner, 2026-11-18: "next to deceased
// memorial for hardcore players also have Living Hardcore Leaderboard"): an NPC
// named LeaderboardName (the same tool stands it a few paces off the
// memorial, Highpass's papered bulletin board, the boards' look) lists the
// shard's living Hardcore characters ranked (database.LivingHardcore: level,
// then CMs, then experience, then the longest alive), the top
// leaderboardTop, leaderboardPerPage a talk.

// LeaderboardName is the living leaderboard's name.
const LeaderboardName = "Hardcore Leaderboard"

const (
	leaderboardTop     = 20
	leaderboardPerPage = 10
)

const (
	leaderboardHead  = "Hardcore Leaderboard: the living Hardcore heroes%s, ranked by level, then Combat Mastery, then experience."
	leaderboardEmpty = "No Hardcore hero walks the world yet. Will you be the first?"
	leaderboardPage  = "Page %d of %d. Speak to the board again for more."
)

// leaderboardPages is the page each character reads next.
var leaderboardPages sync.Map

// IsLeaderboard is whether an NPC of the name is the living leaderboard.
func IsLeaderboard(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), LeaderboardName)
}

// LeaderboardLine is rank's line: "1. Meshuggah, level 52 Shadow Knight, 140
// CMs, alive 12d 4h".
func LeaderboardLine(rank int, h database.LivingHero, now time.Time) string {
	line := fmt.Sprintf("%d. %s, level %d %s, %s", rank, h.Name, h.Level, entity.ClassName(h.Class), countOfThing(h.CMs, "CM", "CMs"))
	if h.Born > 0 && now.Unix() > h.Born {
		line += ", alive " + fallen.Duration(now.Unix()-h.Born)
	}
	return memorialClean(line)
}

// leaderboardLines is page (from 0) of the ranked: its heading, its lines, its
// page line; and the page that follows it.
func leaderboardLines(heroes []database.LivingHero, page int, shard string, now time.Time) (lines []string, next int) {
	where := ""
	if shard != "" {
		where = " of " + memorialClean(shard)
	}
	lines = []string{fmt.Sprintf(leaderboardHead, where)}
	if len(heroes) == 0 {
		return append(lines, leaderboardEmpty), 0
	}
	pages := (len(heroes) + leaderboardPerPage - 1) / leaderboardPerPage
	if page < 0 || page >= pages {
		page = 0
	}
	for i := page * leaderboardPerPage; i < min(len(heroes), (page+1)*leaderboardPerPage); i++ {
		lines = append(lines, LeaderboardLine(i+1, heroes[i], now))
	}
	if pages > 1 {
		lines = append(lines, fmt.Sprintf(leaderboardPage, page+1, pages))
	}
	return lines, (page + 1) % pages
}

// leaderboardTalk is a talk to npc: the living ranked when npc is the board.
func leaderboardTalk(sess *session.Session, p *entity.Player, npc *entity.NPC) bool {
	if npc == nil || npc.Dead || p == nil || !IsLeaderboard(npc.Name) {
		return false
	}
	forgetDialogs(p)
	db := memorialDB.Load()
	if db == nil {
		sayPacked(sess, "", []string{memorialClosed})
		return true
	}
	heroes, err := db.LivingHardcore(leaderboardTop)
	if err != nil {
		log.Printf("Leaderboard: the living: %v", err)
		sayPacked(sess, "", []string{memorialClosed})
		return true
	}
	page := 0
	if v, ok := leaderboardPages.Load(p.CharServerID); ok {
		page = v.(int)
	}
	lines, next := leaderboardLines(heroes, page, hardcoreCfg.Load().shard, time.Now())
	if len(heroes) == 0 && !HardcoreOn() {
		lines = []string{memorialOffNote}
	}
	leaderboardPages.Store(p.CharServerID, next)
	sayPacked(sess, "", lines)
	return true
}
