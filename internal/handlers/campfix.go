package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Camp fix reports (board entry 513, the GM Jibrish's ask of 2026-11-09: "a
// command players can use to report a camp's creatures standing in the wrong
// spot or facing the wrong way, or one too many or too few"; petition 1423:
// "Verify everyone on the server can use the /campfix set of commands. No gm
// or tester flag needed."). A player stands where a creature belongs, turns its
// character the way the creature should face, and marks the spot; the marks
// wait in memory, the session's, until it sends them. Sending files a row of
// campfix_reports (database/campfix.go) and a work board entry; the report
// waits on the GM panel's Camp reports page until a GM approves some or all of
// its marks or declines it (campfixreview.go, gmweb/ops_campfix.go; CLAUDE.md,
// "Camp fix reports: how they are reviewed"), and no spawn changes before
// that. Nothing here changes one.
//
//   - /campfix opens a report; typed again, sends it. /campfix list, undo,
//     note <text>, cancel.
//   - /mark [name]: an existing spawn of that name (no name: the target's)
//     should stand here, facing the way the player faces: the target when it
//     is one of the name within campfixSpan, else one left open until the
//     sending picks it. A turn, the facing alone, when that spawn lies within
//     campfixApart of the player.
//   - /markadd <name>: one more of that name should stand here.
//   - /markremove: the target's spawn (within campfixRemoveTarget), else the
//     spawn the player stands on (the nearest within campfixRemoveStand),
//     should go.
//
// Every command is everyone's, in every spelling: /name (vanilla forwards an
// unknown /command), #name (a PS2 keeps an unknown /command to itself but sends
// a # line as chat) and a PS2's "/t name". /campfix is routed beside /stuck,
// ahead of the GM's # commands and the tells; /mark, /markadd and /markremove
// are the report's only while its sender has one open (chat.go), so a GM's own
// /mark (gmcamps.go) is shadowed while its report is open and back once it is
// sent or cancelled, "/t mark bob" is a tell when none is open, and any other
// mark with none open is told to open one.
//
// A report holds at most CampfixMostMarks marks, all in one world within
// campfixSpan of the first, none within campfixApart of another, none made
// dead, flying or loading a world. It lasts until sent or cancelled; a logout
// discards it, and so do campfixIdle with no command (the next one says so);
// a death keeps it. A restart that keeps the players (keep.go) does not keep
// the reports: they are this process's memory.
//
// A mark keeps the player's facing (Player.Facing, the heading of its latest
// 0x40, radians over [-pi, pi)) and its npcs row byte (facingByte): a client's
// heading byte b is the row byte (b+128) mod 256, exact for all 256, and the
// loader and the channel's byte 15 (entity.FacingByte, rounded) give it back
// unchanged, so a creature placed from a mark faces exactly the way the player
// faced.

// The limits (ours).
const (
	// CampfixMostMarks is the most marks a report holds, moves, turns, adds
	// and removes together.
	CampfixMostMarks = 15
	// campfixSpan: every mark within this of the first; a name's spawns "near"
	// a mark; an open move's spawn at the sending.
	campfixSpan float32 = 300
	// campfixApart: no mark within this of another; a /mark whose spawn lies
	// within this of the player is a turn.
	campfixApart float32 = 2
	// /markremove: the target's spawn within campfixRemoveTarget of the player,
	// else the nearest spawn within campfixRemoveStand.
	campfixRemoveTarget float32 = 100
	campfixRemoveStand  float32 = 15
	campfixNoteMost             = 200
	campfixIdle                 = 30 * time.Minute
	// the sending's: one report an account every campfixEvery, campfixMine of
	// its own waiting, campfixAll waiting in all
	campfixEvery = 10 * time.Minute
	campfixMine  = 3
	campfixAll   = 100
	// the creatures recorded: every row's within campfixNearReach of a mark,
	// nearest first, at most campfixNearMost
	campfixNearReach float32 = 100
	campfixNearMost          = 60
	// the flags' figures
	campfixFew       = 3
	campfixLevelOff  = 8
	campfixFarMove   = 150
	campfixTownReach = 150
	campfixAbove     = 3
	// the board entry's place: the nearest #tele camp within campfixCampName,
	// else the nearest town within campfixTownName
	campfixCampName float32 = 300
	campfixTownName float32 = 500
)

// The kinds of mark.
const (
	campfixMove   = "move"
	campfixTurn   = "turn"
	campfixAdd    = "add"
	campfixRemove = "remove"
)

// The lines (the plan's, but those it left to the build).
const (
	campfixLineOff       = "Camp reports are off on this server."
	campfixLineOpenFirst = "Open a camp report with /campfix first."
	campfixLineNone      = "You have no camp report open: /campfix opens one."
	campfixLineIdle      = "Your camp report was discarded: 30 minutes passed with no command. /campfix opens a new one."
	campfixLineFull      = "15 marks is the most: /campfix sends them."
	campfixLineDead      = "You cannot mark while you are dead."
	campfixLineFlying    = "You cannot mark while you fly: land where the creature should stand."
	campfixLineLoading   = "Wait for the world to load before you mark."
	campfixLineOtherWld  = "A camp report covers one camp: your first mark is in another world."
	campfixLineFar       = "A camp report covers one camp: you are %d from your first mark."
	campfixLineTooNear   = "Mark %d is within 2 of here: a mark stands 2 from every other."
	campfixLineMarkWho   = "Target the creature, or name it: /mark <name>."
	campfixLineAddWho    = "Name the creature to add: /markadd <name>."
	campfixLineRemoveWho = "Stand on the creature's spawn, or target it."
	campfixLineHasMark   = "%s (spawn %d) already has a mark: mark %d."
	campfixLineNoneNear  = "No %s near here to move: /markadd adds one."
	campfixLineEvery     = "Every %s near here already has a mark: /markadd for another."
	campfixLineEveryRm   = "Every %s near here already has a mark: /campfix undo drops the last."
	campfixLineNoName    = "No creature named %s here: target it to see its name."
	campfixLineNameMany  = "%q could be %s: type more of the name."
	campfixLineTaken     = "Taken as %s."
	campfixLineNothing   = "Nothing to send: /campfix cancel closes it."
	campfixLineSoon      = "One camp report every 10 minutes: %d %s to go."
	campfixLineMine      = "You have 3 camp reports waiting for a GM."
	campfixLineAll       = "The GMs have 100 camp reports to read: try again later."
	campfixLineUsage     = "Usage: /campfix (opens a report; again, sends it), /campfix list, /campfix undo, /campfix note <text>, /campfix cancel"
	campfixLineNoteUsage = "Usage: /campfix note <text> - a note for the GMs, 200 characters at most."
	campfixLineNoMarks   = "Your camp report has no marks."
	campfixLineNoneSuff  = " (none near here: a GM will check)"
)

// campfixOpenLines are what opening a report prints, exactly.
var campfixOpenLines = []string{
	"Camp report open: up to 15 marks. Stand where a creature belongs, turn your character the way it should face:",
	"/mark <name> - one already here should stand on this spot (target it first to pick which one)",
	"/markadd <name> - one more should stand on this spot",
	"/markremove - the one whose spawn you stand on (or your target) should go",
	"/campfix list, /campfix undo, /campfix note <text>, /campfix cancel. /campfix again sends it.",
	"On a PS2 type /t first: /t campfix, /t mark bandit, /t markadd bandit, /t markremove.",
}

var (
	campfixOffFlag atomic.Bool  // world.campfix false
	campfixShardOf atomic.Value // this server's name on the server list
)

// SetCampfix turns the camp reports on or off (world.campfix, on unless false).
// Off, every command answers campfixLineOff; a GM's own /mark is as ever.
func SetCampfix(on bool) { campfixOffFlag.Store(!on) }

// CampfixOn reports whether the camp reports are on.
func CampfixOn() bool { return !campfixOffFlag.Load() }

// SetCampfixShard names this server as the server list does (its shard's
// entry), for the board entries' details: each shard posts on its own board.
func SetCampfixShard(name string) { campfixShardOf.Store(name) }

func campfixShard() string {
	if s, ok := campfixShardOf.Load().(string); ok && s != "" {
		return s
	}
	return DefaultShardName
}

// campfixMark is one of a report's marks: what it is, what was typed, the
// creature's name, where the player stood and faced, when; and the spawn it
// names (a turn's, a picked move's, a removal's; ID 0 for an add and for a
// move left open until the sending), as it was then.
type campfixMark struct {
	kind            string
	typed           string
	name            string
	world           uint8
	x, y, z, facing float32
	row             entity.CampfixNPC
	noneNear        bool // an add whose name had no spawn within campfixSpan
	at              time.Time
}

// campfixReport is a session's open report: the character that opened it (a
// logout makes another), its marks, its note, its last command.
type campfixReport struct {
	player *entity.Player
	marks  []campfixMark
	note   string
	last   time.Time
}

// campfixReports are the open reports, by session, and where a sent one goes.
type campfixReports struct {
	mu   sync.Mutex
	open map[*session.Session]*campfixReport
	db   *database.SQLiteDB
	now  func() time.Time // tests
}

func (cf *campfixReports) clock() time.Time {
	if cf.now != nil {
		return cf.now()
	}
	return time.Now()
}

// holds reports whether sess has a report open for player, a lapsed one
// included (the next command says it lapsed). One left by another character of
// the session is forgotten.
func (cf *campfixReports) holds(sess *session.Session, player *entity.Player) bool {
	cf.mu.Lock()
	defer cf.mu.Unlock()
	r := cf.open[sess]
	if r != nil && r.player != player {
		delete(cf.open, sess)
		r = nil
	}
	return r != nil
}

// take is sess's open report for player, its last command now; lapsed when it
// lapsed (campfixIdle with no command), discarded then. Must be called with mu
// held.
func (cf *campfixReports) take(sess *session.Session, player *entity.Player, now time.Time) (r *campfixReport, lapsed bool) {
	r = cf.open[sess]
	switch {
	case r == nil:
		return nil, false
	case r.player != player:
		delete(cf.open, sess)
		return nil, false
	case now.Sub(r.last) > campfixIdle:
		delete(cf.open, sess)
		log.Printf("Campfix: %s's report lapsed after %s with no command (%d marks)", player.Name, now.Sub(r.last).Round(time.Second), len(r.marks))
		return nil, true
	}
	r.last = now
	return r, false
}

// campfixLeft forgets the reports a player leaving the world held (a logout
// discards its report).
func (h *ChatHandler) campfixLeft(p *entity.Player) {
	cf := &h.campfix
	cf.mu.Lock()
	defer cf.mu.Unlock()
	for s, r := range cf.open {
		if r.player == p {
			delete(cf.open, s)
		}
	}
}

// campfixWords are the commands, in lower case.
var campfixWords = map[string]bool{"campfix": true, "mark": true, "markadd": true, "markremove": true}

// campfixLine reads a camp report's command off a chat line, in any spelling:
// /campfix, #campfix or a PS2's "/t campfix" ("/tell" too), and /mark,
// /markadd and /markremove the same ways. cmd is the command in lower case,
// rest what follows it, tell whether it came as a /t.
func campfixLine(line string) (cmd, rest string, tell, ok bool) {
	word, after, _ := strings.Cut(line, " ")
	w := strings.ToLower(word)
	if w == "/t" || w == "/tell" {
		word, after, _ = strings.Cut(strings.TrimLeft(after, " "), " ")
		w, tell = strings.ToLower(word), true
	} else if len(w) < 2 || w[0] != '/' && w[0] != '#' {
		return "", "", false, false
	} else {
		w = w[1:]
	}
	if !campfixWords[w] {
		return "", "", false, false
	}
	return w, strings.TrimSpace(after), tell, true
}

// campfixRoute takes a camp report's command (campfixLine) and reports whether
// it did: false for a mark that is not the report's (no report open: a GM's
// own /mark, which gmcamps.go takes; "/t mark bob", a tell).
func (h *ChatHandler) campfixRoute(sess *session.Session, player *entity.Player, cmd, rest string, tell bool) bool {
	if cmd == "campfix" {
		h.campfixCommand(sess, player, rest)
		return true
	}
	if !h.campfix.holds(sess, player) {
		switch {
		case tell && rest != "":
			return false // "/t mark bob": a tell (a tell with no text is no tell)
		case !tell && cmd == "mark" && h.isGM(sess):
			return false // the GM's own /mark and #mark (gmcamps.go)
		case !CampfixOn():
			SendErrorMessage(sess, campfixLineOff)
		default:
			SendErrorMessage(sess, campfixLineOpenFirst)
		}
		return true
	}
	h.campfixMarkCommand(sess, player, cmd, rest)
	return true
}

// campfixSay is a reply to the sender alone.
func campfixSay(sess *session.Session, text string) {
	sess.Queue().AddWithSequence(buildClientMessage(text))
}

// campfixCommand is /campfix [list | undo | note <text> | cancel].
func (h *ChatHandler) campfixCommand(sess *session.Session, player *entity.Player, rest string) {
	if !CampfixOn() {
		SendErrorMessage(sess, campfixLineOff)
		return
	}
	cf := &h.campfix
	cf.mu.Lock()
	defer cf.mu.Unlock()
	now := cf.clock()
	r, lapsed := cf.take(sess, player, now)
	if lapsed {
		SendErrorMessage(sess, campfixLineIdle)
		return
	}
	sub, arg, _ := strings.Cut(strings.TrimSpace(rest), " ")
	sub = strings.ToLower(sub)
	if r == nil && sub != "" && sub != "list" && sub != "undo" && sub != "note" && sub != "cancel" {
		SendErrorMessage(sess, campfixLineUsage)
		return
	}
	if r == nil && sub != "" {
		SendErrorMessage(sess, campfixLineNone)
		return
	}
	switch sub {
	case "":
		if r == nil {
			h.campfixOpen(sess, player, now)
			return
		}
		h.campfixSend(sess, player, r, now)
	case "list":
		h.campfixList(sess, r)
	case "undo":
		if len(r.marks) == 0 {
			SendErrorMessage(sess, campfixLineNoMarks)
			return
		}
		m := r.marks[len(r.marks)-1]
		r.marks = r.marks[:len(r.marks)-1]
		campfixSay(sess, fmt.Sprintf("Mark %d dropped: %s. %d left.", len(r.marks)+1, campfixWhat(m), len(r.marks)))
	case "note":
		note, cut := campfixCleanNote(arg)
		if note == "" {
			SendErrorMessage(sess, campfixLineNoteUsage)
			return
		}
		r.note = note
		line := "Note kept: " + note
		if cut {
			line += fmt.Sprintf(" (cut to %d characters)", campfixNoteMost)
		}
		campfixSay(sess, line)
	case "cancel":
		delete(cf.open, sess)
		campfixSay(sess, "Camp report discarded: nothing was sent.")
		log.Printf("Campfix: %s cancelled a report of %d marks", player.Name, len(r.marks))
	default:
		SendErrorMessage(sess, campfixLineUsage)
	}
}

// campfixOpen opens a report for sess's player. Must be called with mu held.
func (h *ChatHandler) campfixOpen(sess *session.Session, player *entity.Player, now time.Time) {
	cf := &h.campfix
	if cf.open == nil {
		cf.open = make(map[*session.Session]*campfixReport)
	}
	for s, r := range cf.open { // reports whose character is no longer its session's
		if h.gameWorld.SessionPlayer(s) != r.player {
			delete(cf.open, s)
		}
	}
	cf.open[sess] = &campfixReport{player: player, last: now}
	for _, l := range campfixOpenLines {
		campfixSay(sess, l)
	}
	log.Printf("Campfix: %s opened a report at (%.1f, %.1f, %.1f) world %d", player.Name, player.X, player.Y, player.Z, player.World)
}

// campfixCleanNote is a note as a report keeps it: no control character, at
// most campfixNoteMost characters, trimmed, profanity masked; cut says it was
// longer.
func campfixCleanNote(s string) (note string, cut bool) {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(s, "") {
		if unicode.IsControl(r) {
			continue
		}
		if n == campfixNoteMost {
			cut = strings.TrimSpace(s) != strings.TrimSpace(b.String())
			break
		}
		b.WriteRune(r)
		n++
	}
	return maskProfanity(strings.TrimSpace(b.String())), cut
}

// campfixKey is a name as two are compared: in lower case, trimmed.
func campfixKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// campfixStripArticle is text without a leading "a ", "an " or "the " (the rows
// say "bandit", not "a bandit").
func campfixStripArticle(t string) string {
	l := strings.ToLower(t)
	for _, a := range []string{"a ", "an ", "the "} {
		if strings.HasPrefix(l, a) {
			return strings.TrimSpace(t[len(a):])
		}
	}
	return t
}

// campfixShown is what a player typed, as a line says it back.
func campfixShown(t string) string {
	if s := consoleLine(t, 64); s != "" {
		return s
	}
	return "that"
}

func campfixDist(ax, ay, az, bx, by, bz float32) float32 {
	dx, dy, dz := float64(ax-bx), float64(ay-by), float64(az-bz)
	return float32(math.Sqrt(dx*dx + dy*dy + dz*dz))
}

func campfixXZ(ax, az, bx, bz float32) float32 {
	return float32(math.Hypot(float64(ax-bx), float64(az-bz)))
}

// campfixRound is a distance as a line says it.
func campfixRound(d float32) int { return int(math.Round(float64(d))) }

// claims are the spawns the report's marks name, each with its mark's index.
func (r *campfixReport) claims() map[entity.EntityID]int {
	out := map[entity.EntityID]int{}
	for i, m := range r.marks {
		if m.row.ID != 0 {
			out[m.row.ID] = i
		}
	}
	return out
}

// openMoves is how many of the report's moves of the name (campfixKey) are
// left open until the sending.
func (r *campfixReport) openMoves(key string) int {
	n := 0
	for _, m := range r.marks {
		if m.kind == campfixMove && m.row.ID == 0 && campfixKey(m.name) == key {
			n++
		}
	}
	return n
}

// placeRefused is why the player may not mark where it stands for this report
// (another world than the first mark's, too far from it, too near another
// mark), "" when it may.
func (r *campfixReport) placeRefused(me entity.CampfixPlayer) string {
	if len(r.marks) == 0 {
		return ""
	}
	first := r.marks[0]
	if first.world != me.World {
		return campfixLineOtherWld
	}
	if d := campfixDist(first.x, first.y, first.z, me.X, me.Y, me.Z); d > campfixSpan {
		return fmt.Sprintf(campfixLineFar, int(math.Ceil(float64(d))))
	}
	for i, m := range r.marks {
		if campfixDist(m.x, m.y, m.z, me.X, me.Y, me.Z) <= campfixApart {
			return fmt.Sprintf(campfixLineTooNear, i+1)
		}
	}
	return ""
}

// campfixMarkCommand is /mark, /markadd or /markremove (cmd) on an open report.
func (h *ChatHandler) campfixMarkCommand(sess *session.Session, player *entity.Player, cmd, rest string) {
	if !CampfixOn() {
		SendErrorMessage(sess, campfixLineOff)
		return
	}
	cf := &h.campfix
	cf.mu.Lock()
	defer cf.mu.Unlock()
	now := cf.clock()
	r, lapsed := cf.take(sess, player, now)
	switch {
	case lapsed:
		SendErrorMessage(sess, campfixLineIdle)
		return
	case r == nil:
		SendErrorMessage(sess, campfixLineOpenFirst)
		return
	case len(r.marks) >= CampfixMostMarks:
		SendErrorMessage(sess, campfixLineFull)
		return
	}
	me, ok := h.gameWorld.CampfixPlayerOf(sess, now)
	if !ok {
		return
	}
	why := ""
	switch {
	case me.Dead:
		why = campfixLineDead
	case me.Flying:
		why = campfixLineFlying
	case sess.StateChannelsHeld():
		why = campfixLineLoading
	default:
		why = r.placeRefused(me)
	}
	if why != "" {
		SendErrorMessage(sess, why)
		return
	}
	var m campfixMark
	var said []string
	switch cmd {
	case "mark":
		m, said, why = h.campfixMoveMark(r, me, rest)
	case "markadd":
		m, said, why = h.campfixAddMark(me, rest)
	default:
		m, said, why = h.campfixRemoveMark(r, me)
	}
	for _, l := range said {
		campfixSay(sess, l)
	}
	if why != "" {
		SendErrorMessage(sess, why)
		return
	}
	m.world, m.x, m.y, m.z, m.facing, m.at = me.World, me.X, me.Y, me.Z, me.Facing, now
	r.marks = append(r.marks, m)
	campfixSay(sess, h.campfixMarkLine(r, len(r.marks)-1))
	log.Printf("Campfix: %s marked %d: %s at (%.1f, %.1f, %.1f) world %d, facing %.3f (byte %d)",
		player.Name, len(r.marks), campfixWhat(m), m.x, m.y, m.z, m.world, m.facing, facingByte(float64(m.facing)))
}

// campfixName is the creature typed where the player stands (campfixMatch
// over the world's NPCs).
func (h *ChatHandler) campfixName(me entity.CampfixPlayer, typed string) (string, []entity.CampfixNPC, []string, string) {
	return campfixMatch(typed, func(names []string) ([]entity.CampfixNPC, []entity.CampfixName) {
		return h.gameWorld.CampfixScan(me.World, me.X, me.Y, me.Z, campfixSpan, names)
	})
}

// campfixScanFunc is what matching a name reads of a place: the NPCs of rows
// whose spawn lies within campfixSpan of it, nearest first, and what the world
// holds of each of names (World.CampfixScan; for a report filed off the
// database, a petition's, campfixRows.scan over its rows).
type campfixScanFunc func(names []string) ([]entity.CampfixNPC, []entity.CampfixName)

// campfixMatch is the creature typed, as a report reads a name: matched
// without case against the names of the NPCs of rows in the world, the whole
// text, else the text without a leading article, else the one name among the
// spawns within campfixSpan holding it. It returns the name as its nearest
// spawn spells it, those spawns (nearest first), the lines saying how it was
// taken, and the refusal ("" when found).
func campfixMatch(typed string, scan campfixScanFunc) (string, []entity.CampfixNPC, []string, string) {
	stripped := campfixStripArticle(typed)
	near, of := scan([]string{typed, stripped})
	for _, o := range of {
		if o.Count > 0 {
			return o.Nearest.Name, near, nil, ""
		}
	}
	q := campfixKey(stripped)
	spelled := map[string]string{} // by key, as its nearest spawn spells it
	var keys []string
	for _, n := range near {
		k := campfixKey(n.Name)
		if q == "" || !strings.Contains(k, q) {
			continue
		}
		if _, seen := spelled[k]; !seen {
			spelled[k] = n.Name
			keys = append(keys, k)
		}
	}
	switch len(keys) {
	case 0:
		return "", near, nil, fmt.Sprintf(campfixLineNoName, campfixShown(typed))
	case 1:
		return spelled[keys[0]], near, []string{fmt.Sprintf(campfixLineTaken, spelled[keys[0]])}, ""
	}
	sort.Strings(keys)
	var names []string
	for i, k := range keys {
		if i == 5 {
			names = append(names, fmt.Sprintf("%d more", len(keys)-5))
			break
		}
		names = append(names, spelled[k])
	}
	return "", near, nil, fmt.Sprintf(campfixLineNameMany, campfixShown(typed), orList(names))
}

// campfixMoveMark is /mark [name]: which spawn of the name moves (the target
// when it is one within campfixSpan, else one left open until the sending; a
// turn when that spawn lies within campfixApart of the player), or why none
// may.
func (h *ChatHandler) campfixMoveMark(r *campfixReport, me entity.CampfixPlayer, typed string) (campfixMark, []string, string) {
	m := campfixMark{kind: campfixMove, typed: typed}
	var near []entity.CampfixNPC
	var said []string
	if typed == "" {
		if !me.HasTarget {
			return m, nil, campfixLineMarkWho
		}
		m.name = me.Target.Name
		near, _ = h.gameWorld.CampfixScan(me.World, me.X, me.Y, me.Z, campfixSpan, nil)
	} else {
		var why string
		if m.name, near, said, why = h.campfixName(me, typed); why != "" {
			return m, said, why
		}
	}
	key := campfixKey(m.name)
	var spawns []entity.CampfixNPC
	for _, n := range near {
		if campfixKey(n.Name) == key {
			spawns = append(spawns, n)
		}
	}
	if len(spawns) == 0 {
		return m, said, fmt.Sprintf(campfixLineNoneNear, m.name)
	}
	claims := r.claims()
	open := r.openMoves(key)
	if t := me.Target; me.HasTarget && campfixKey(t.Name) == key && t.Dist <= campfixSpan {
		if i, ok := claims[t.ID]; ok {
			return m, said, fmt.Sprintf(campfixLineHasMark, t.Name, t.ID, i+1)
		}
		free := 0
		for _, s := range spawns {
			if _, c := claims[s.ID]; !c && s.ID != t.ID {
				free++
			}
		}
		if free < open { // each open move needs a spawn of its own
			return m, said, fmt.Sprintf(campfixLineEvery, m.name)
		}
		m.row, m.name = t, t.Name
	} else {
		var free []entity.CampfixNPC
		for _, s := range spawns {
			if _, c := claims[s.ID]; !c {
				free = append(free, s)
			}
		}
		if len(free) <= open {
			return m, said, fmt.Sprintf(campfixLineEvery, m.name)
		}
		if free[0].Dist <= campfixApart { // standing on it: a turn of it
			m.row, m.name = free[0], free[0].Name
		}
	}
	if m.row.ID != 0 && m.row.Dist <= campfixApart {
		m.kind = campfixTurn
	}
	return m, said, ""
}

// campfixAddMark is /markadd <name>: any name in the player's world, flagged
// when it has no spawn within campfixSpan.
func (h *ChatHandler) campfixAddMark(me entity.CampfixPlayer, typed string) (campfixMark, []string, string) {
	m := campfixMark{kind: campfixAdd, typed: typed}
	if typed == "" {
		return m, nil, campfixLineAddWho
	}
	name, near, said, why := h.campfixName(me, typed)
	if why != "" {
		return m, said, why
	}
	m.name, m.noneNear = name, true
	for _, n := range near {
		if campfixKey(n.Name) == campfixKey(name) {
			m.noneNear = false
			break
		}
	}
	return m, said, ""
}

// campfixRemoveMark is /markremove: the target's spawn when within
// campfixRemoveTarget of the player, else the spawn nearest it within
// campfixRemoveStand; refused for none, one another mark names, and one the
// open moves of its name need.
func (h *ChatHandler) campfixRemoveMark(r *campfixReport, me entity.CampfixPlayer) (campfixMark, []string, string) {
	m := campfixMark{kind: campfixRemove}
	row, found := me.Target, me.HasTarget && me.Target.Dist <= campfixRemoveTarget
	if !found {
		if near, _ := h.gameWorld.CampfixScan(me.World, me.X, me.Y, me.Z, campfixRemoveStand, nil); len(near) > 0 {
			row, found = near[0], true
		}
	}
	if !found {
		return m, nil, campfixLineRemoveWho
	}
	claims := r.claims()
	if i, ok := claims[row.ID]; ok {
		return m, nil, fmt.Sprintf(campfixLineHasMark, row.Name, row.ID, i+1)
	}
	key := campfixKey(row.Name)
	if open := r.openMoves(key); open > 0 {
		near, _ := h.gameWorld.CampfixScan(me.World, me.X, me.Y, me.Z, campfixSpan, nil)
		free := 0
		for _, n := range near {
			if _, c := claims[n.ID]; !c && n.ID != row.ID && campfixKey(n.Name) == key {
				free++
			}
		}
		if free < open {
			return m, nil, fmt.Sprintf(campfixLineEveryRm, row.Name)
		}
	}
	m.name, m.row = row.Name, row
	return m, nil, ""
}

// campfixWhatRecord is a filed mark in a few words, as campfixWhat says it.
func campfixWhatRecord(m CampfixMarkRecord) string {
	switch {
	case m.Kind == campfixAdd:
		return "add " + m.Name
	case m.Row != 0:
		return fmt.Sprintf("%s %s (spawn %d)", m.Kind, m.Name, m.Row)
	}
	return m.Kind + " " + m.Name
}

// campfixWhat is a mark in a few words.
func campfixWhat(m campfixMark) string {
	switch {
	case m.kind == campfixAdd:
		return "add " + m.name
	case m.row.ID != 0:
		return fmt.Sprintf("%s %s (spawn %d)", m.kind, m.name, m.row.ID)
	}
	return m.kind + " " + m.name
}

// campfixMarkLine is mark i as its command answers it.
func (h *ChatHandler) campfixMarkLine(r *campfixReport, i int) string {
	m := r.marks[i]
	n := i + 1
	switch m.kind {
	case campfixTurn:
		return fmt.Sprintf("Mark %d of %d: turn %s (spawn %d) to face your way.", n, CampfixMostMarks, m.name, m.row.ID)
	case campfixMove:
		row := m.row
		if row.ID == 0 {
			row = h.campfixResolve(r.marks)[i] // the spawn the sending would pick now
		}
		if row.ID == 0 {
			return fmt.Sprintf("Mark %d of %d: move %s here, facing your way.", n, CampfixMostMarks, m.name)
		}
		return fmt.Sprintf("Mark %d of %d: move %s (spawn %d, %d away) here, facing your way.", n, CampfixMostMarks, m.name,
			row.ID, campfixRound(campfixDist(row.X, row.Y, row.Z, m.x, m.y, m.z)))
	case campfixAdd:
		l := fmt.Sprintf("Mark %d of %d: add %s here, facing your way.", n, CampfixMostMarks, m.name)
		if m.noneNear {
			l += campfixLineNoneSuff
		}
		return l
	}
	return fmt.Sprintf("Mark %d of %d: remove %s (spawn %d, %d from you).", n, CampfixMostMarks, m.name, m.row.ID, campfixRound(m.row.Dist))
}

// campfixList is /campfix list: a line a mark, each with its facing as its row
// byte (and a move's and a turn's the byte its spawn had), then the note.
func (h *ChatHandler) campfixList(sess *session.Session, r *campfixReport) {
	if len(r.marks) == 0 {
		campfixSay(sess, "Your camp report has no marks yet: /mark, /markadd and /markremove make them.")
	} else {
		campfixSay(sess, fmt.Sprintf("Your camp report: %d of %d marks.", len(r.marks), CampfixMostMarks))
	}
	rows := h.campfixResolve(r.marks)
	for i, m := range r.marks {
		at := fmt.Sprintf("(%.1f, %.1f, %.1f)", m.x, m.y, m.z)
		fb := facingByte(float64(m.facing))
		var l string
		switch m.kind {
		case campfixTurn:
			l = fmt.Sprintf("turn %s (spawn %d) at %s to face %d; it faced %d", m.name, m.row.ID, at, fb, facingByte(float64(m.row.Facing)))
		case campfixMove:
			switch row := rows[i]; {
			case m.row.ID != 0:
				l = fmt.Sprintf("move %s (spawn %d) to %s, facing %d; it faced %d", m.name, row.ID, at, fb, facingByte(float64(row.Facing)))
			case row.ID != 0:
				l = fmt.Sprintf("move %s (spawn %d, picked when sent) to %s, facing %d; it faced %d", m.name, row.ID, at, fb, facingByte(float64(row.Facing)))
			default:
				l = fmt.Sprintf("move %s (no spawn of it left free near) to %s, facing %d", m.name, at, fb)
			}
		case campfixAdd:
			l = fmt.Sprintf("add %s at %s, facing %d", m.name, at, fb)
			if m.noneNear {
				l += campfixLineNoneSuff
			}
		default:
			l = fmt.Sprintf("remove %s (spawn %d) at %s, facing %d", m.name, m.row.ID, at, fb)
		}
		campfixSay(sess, fmt.Sprintf("  %d: %s", i+1, l))
	}
	if r.note != "" {
		campfixSay(sess, "Note: "+r.note)
	}
	campfixSay(sess, "/campfix sends it, /campfix undo drops the last mark, /campfix cancel discards it.")
}

// campfixResolve is the spawn each of marks names: a turn's, a picked move's
// and a removal's own; for each move left open, of its name's spawns within
// campfixSpan of its mark that no other mark names, the pairs nearest first
// (greedy by the pair's distance: enough at 15 marks), its Dist that pair's;
// ID 0 for an add, and for an open move left with none.
func (h *ChatHandler) campfixResolve(marks []campfixMark) []entity.CampfixNPC {
	out := make([]entity.CampfixNPC, len(marks))
	claimed := map[entity.EntityID]bool{}
	var open []int
	var pts [][3]float32
	for i, m := range marks {
		switch {
		case m.row.ID != 0:
			out[i] = m.row
			claimed[m.row.ID] = true
		case m.kind == campfixMove:
			open = append(open, i)
			pts = append(pts, [3]float32{m.x, m.y, m.z})
		}
	}
	if len(open) == 0 {
		return out
	}
	cands := h.gameWorld.CampfixNear(marks[open[0]].world, pts, campfixSpan, 0)
	type pair struct {
		m, c int
		d    float32
	}
	var pairs []pair
	for _, i := range open {
		key := campfixKey(marks[i].name)
		for j, c := range cands {
			if claimed[c.ID] || campfixKey(c.Name) != key {
				continue
			}
			if d := campfixDist(c.X, c.Y, c.Z, marks[i].x, marks[i].y, marks[i].z); d <= campfixSpan {
				pairs = append(pairs, pair{i, j, d})
			}
		}
	}
	sort.Slice(pairs, func(a, b int) bool {
		pa, pb := pairs[a], pairs[b]
		switch {
		case pa.d != pb.d:
			return pa.d < pb.d
		case pa.m != pb.m:
			return pa.m < pb.m
		}
		return cands[pa.c].ID < cands[pb.c].ID
	})
	usedM, usedC := map[int]bool{}, map[int]bool{}
	for _, p := range pairs {
		if usedM[p.m] || usedC[p.c] {
			continue
		}
		usedM[p.m], usedC[p.c] = true, true
		c := cands[p.c]
		c.Dist, c.Movement = p.d, ""
		out[p.m] = c
	}
	return out
}

// The report's JSON (campfix_reports' marks, near and flags), as the sending
// and a petition's filing (campfixpetition.go) write them and the GM panel's
// review (campfixreview.go) reads them back.

// CampfixMarkRecord is one of a report's marks: what it asks, where the
// player stood and faced, and the spawn it names as it was then.
type CampfixMarkRecord struct {
	Kind       string  `json:"kind"`
	Typed      string  `json:"typed"`
	Name       string  `json:"name"`
	World      int     `json:"world"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Z          float64 `json:"z"`
	Facing     float64 `json:"facing"`      // radians, the player's (Player.Facing)
	FacingByte int     `json:"facing_byte"` // the npcs row's facing for it (facingByte)
	Row        int     `json:"row"`         // the spawn it moves, turns or removes; 0 for an add
	Template   int     `json:"template,omitempty"`
	FromX      float64 `json:"from_x"` // its spawn (an add's: its template's)
	FromY      float64 `json:"from_y"`
	FromZ      float64 `json:"from_z"`
	FromFacing int     `json:"from_facing"` // its spawn's facing byte (an add's: its template's)
	Level      int     `json:"level"`
	Distance   float64 `json:"distance"` // from its spawn (an add's template) to the mark
	At         string  `json:"at"`
}

// CampfixNearRecord is an NPC of a row whose spawn was within
// campfixNearReach of a mark. Alive and Movement are the world's: a report
// filed off the database alone (a petition's) has neither.
type CampfixNearRecord struct {
	Row        int      `json:"row"`
	Name       string   `json:"name"`
	Level      int      `json:"level"`
	X          float64  `json:"x"`
	Y          float64  `json:"y"`
	Z          float64  `json:"z"`
	FacingByte int      `json:"facing_byte"`
	Alive      *bool    `json:"alive,omitempty"`
	Movement   string   `json:"movement"`
	Camp       int64    `json:"camp,omitempty"`
	Kinds      []string `json:"kinds,omitempty"`
	SameName   bool     `json:"same_name"`
	Distance   float64  `json:"distance"` // to its nearest mark
}

// CampfixFlagRecord is what the filing flagged of a mark (1-based) for the
// review: its kind and its words.
type CampfixFlagRecord struct {
	Mark int    `json:"mark"`
	Flag string `json:"flag"`
	Text string `json:"text"`
}

func campfix2(v float32) float64 { return math.Round(float64(v)*100) / 100 }

// campfixFiled is a report as the sending files it: its row, the creatures
// recorded, the marks' rows and templates, the flags.
type campfixFiled struct {
	row   database.CampfixReport
	marks []CampfixMarkRecord
	near  []CampfixNearRecord
	flags []CampfixFlagRecord
	camps map[int]bool // npcs rows that are camps' spawns (campspawns)
}

// campfixSend is /campfix with an open report: refused by the limits (the
// report stays open), else filed and the report closed.
func (h *ChatHandler) campfixSend(sess *session.Session, player *entity.Player, r *campfixReport, now time.Time) {
	cf := &h.campfix
	if len(r.marks) == 0 {
		SendErrorMessage(sess, campfixLineNothing)
		return
	}
	if cf.db == nil {
		SendErrorMessage(sess, "Camp reports are not kept on this server.")
		return
	}
	acct := sess.AccountID()
	lim, err := cf.db.CampfixLimitsOf(acct)
	if err != nil {
		SendErrorMessage(sess, "Your camp report could not be sent: try again in a moment.")
		log.Printf("Campfix: %s's report: the limits not read: %v", player.Name, err)
		return
	}
	switch {
	case lim.LastSentKnown && now.Sub(lim.LastSent) < campfixEvery:
		n := int(math.Ceil((campfixEvery - now.Sub(lim.LastSent)).Minutes()))
		SendErrorMessage(sess, fmt.Sprintf(campfixLineSoon, n, plural(n, "minute", "minutes")))
		return
	case lim.Waiting >= campfixMine:
		SendErrorMessage(sess, campfixLineMine)
		return
	case lim.WaitingAll >= campfixAll:
		SendErrorMessage(sess, campfixLineAll)
		return
	}
	f := h.campfixFile(sess, player, r, now)
	who := campfixFiler{Name: player.Name, Level: player.Level, Class: player.Class, CharID: player.CharServerID, AccountID: acct}
	post := func(id int64) database.WorkPost {
		return campfixPost(h.gameWorld, id, who, f.marks, r.note, f.near, f.flags, f.camps, now)
	}
	rep, entry, err := cf.db.FileCampfixReport(f.row, now, post, database.GMEdit{AccountID: acct, Via: "campfix"})
	if err != nil {
		SendErrorMessage(sess, "Your camp report could not be sent: it is still open, and /campfix tries again.")
		log.Printf("Campfix: %s's report not filed: %v", player.Name, err)
		return
	}
	delete(cf.open, sess)
	campfixSay(sess, fmt.Sprintf("Camp report %d sent: %s, %d %s nearby recorded. A GM checks every report before anything changes. Thank you!",
		rep.ID, campfixKindCounts(r.marks, true), len(f.near), plural(len(f.near), "creature", "creatures")))
	log.Printf("Campfix: %s (account %d) sent report %d, board entry %d: %d marks (%s), %d nearby, %d flags",
		player.Name, acct, rep.ID, entry.ID, len(r.marks), campfixKindCounts(r.marks, false), len(f.near), len(f.flags))
}

// campfixKindCounts is how many marks of each kind, "2 moves, 1 add" (long:
// the reply's) or "2 move, 1 add" (the title's), kinds with none left out.
func campfixKindCounts(marks []campfixMark, long bool) string {
	kinds := make([]string, len(marks))
	for i, m := range marks {
		kinds[i] = m.kind
	}
	return campfixKinds(kinds, long)
}

// campfixKinds is campfixKindCounts of the marks' kinds.
func campfixKinds(kinds []string, long bool) string {
	var parts []string
	for _, k := range []string{campfixMove, campfixTurn, campfixAdd, campfixRemove} {
		n := 0
		for _, mk := range kinds {
			if mk == k {
				n++
			}
		}
		switch {
		case n == 0:
		case long:
			parts = append(parts, fmt.Sprintf("%d %s", n, plural(n, k, k+"s")))
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	return strings.Join(parts, ", ")
}

// campfixFile is what the sending files of r: the open moves resolved, the
// creatures near the marks recorded, the flags computed from the world (the
// code reviews nothing: it flags), and the row with them as JSON.
func (h *ChatHandler) campfixFile(sess *session.Session, player *entity.Player, r *campfixReport, now time.Time) campfixFiled {
	gw := h.gameWorld
	world := r.marks[0].world
	rows := h.campfixResolve(r.marks)
	f := campfixFiled{camps: map[int]bool{}}
	campRows := map[int]database.CampSpawnRow{}
	if spawns, err := h.campfix.db.CampSpawns(); err == nil {
		for _, c := range spawns {
			campRows[c.NPCID] = c
			f.camps[c.NPCID] = true
		}
	} else {
		log.Printf("Campfix: campspawns not read: %v", err)
	}
	spots, err := h.campfix.db.StartSpots()
	if err != nil {
		log.Printf("Campfix: the start spots not read: %v", err)
	}

	// the marks, each with the spawn it names (an add's template: its name's
	// row nearest the mark) and its name's count in the world
	pts := make([][3]float32, len(r.marks))
	subjects := make([]entity.CampfixNPC, len(r.marks))
	counts := make([]int, len(r.marks))
	marked := map[string]bool{}
	for i, m := range r.marks {
		pts[i] = [3]float32{m.x, m.y, m.z}
		marked[campfixKey(m.name)] = true
		_, of := gw.CampfixScan(world, m.x, m.y, m.z, 0, []string{m.name})
		counts[i] = of[0].Count
		subjects[i] = rows[i]
		if m.kind == campfixAdd && of[0].Count > 0 {
			subjects[i] = of[0].Nearest
		}
		s := subjects[i]
		j := CampfixMarkRecord{Kind: m.kind, Typed: m.typed, Name: m.name, World: int(m.world),
			X: campfix2(m.x), Y: campfix2(m.y), Z: campfix2(m.z), Facing: math.Round(float64(m.facing)*10000) / 10000,
			FacingByte: facingByte(float64(m.facing)), At: m.at.UTC().Format(time.RFC3339)}
		if s.ID != 0 {
			j.FromX, j.FromY, j.FromZ, j.FromFacing = campfix2(s.X), campfix2(s.Y), campfix2(s.Z), facingByte(float64(s.Facing))
			j.Level = s.Level
			j.Distance = campfix2(campfixDist(s.X, s.Y, s.Z, m.x, m.y, m.z))
			if m.kind == campfixAdd {
				j.Template = int(s.ID)
			} else {
				j.Row = int(s.ID)
			}
		}
		f.marks = append(f.marks, j)
	}

	// the creatures near
	for _, n := range gw.CampfixNear(world, pts, campfixNearReach, campfixNearMost) {
		j := CampfixNearRecord{Row: int(n.ID), Name: n.Name, Level: n.Level, X: campfix2(n.X), Y: campfix2(n.Y), Z: campfix2(n.Z),
			FacingByte: facingByte(float64(n.Facing)), Alive: campfixBool(!n.Dead), Movement: n.Movement, SameName: marked[campfixKey(n.Name)],
			Distance: campfix2(n.Dist)}
		if c, ok := campRows[int(n.ID)]; ok {
			j.Camp, j.Kinds = c.Camp, c.Kinds
		}
		f.near = append(f.near, j)
	}

	// the flags
	near300 := map[string]bool{}
	for _, n := range gw.CampfixNear(world, pts, campfixSpan, 0) {
		near300[campfixKey(n.Name)] = true
	}
	median, haveMedian := campfixMedian(f.near)
	combos := campfixNewcomers(spots)
	flag := func(i int, kind, text string) {
		f.flags = append(f.flags, CampfixFlagRecord{Mark: i + 1, Flag: kind, Text: text})
	}
	for i, m := range r.marks {
		s := subjects[i]
		places := m.kind == campfixMove || m.kind == campfixAdd
		if counts[i] <= campfixFew {
			flag(i, "few", fmt.Sprintf("%s has %d %s in %s: few in the world: named or rare?", m.name, counts[i],
				plural(counts[i], "spawn", "spawns"), worldName(world)))
		}
		if !near300[campfixKey(m.name)] {
			flag(i, "none_near", fmt.Sprintf("%s had no spawn within %.0f of the marks", m.name, campfixSpan))
		}
		if m.kind == campfixMove && s.ID == 0 {
			flag(i, "no_spawn", fmt.Sprintf("no spawn of %s was left free near it to move: the report's other marks name them", m.name))
		}
		if s.ID != 0 && haveMedian && math.Abs(float64(s.Level)-median) > campfixLevelOff {
			flag(i, "level", fmt.Sprintf("%s is level %d, %s from the median level %s of the creatures nearby", m.name, s.Level,
				campfixNum(math.Abs(float64(s.Level)-median)), campfixNum(median)))
		}
		if m.kind == campfixMove && s.ID != 0 {
			if d := campfixDist(s.X, s.Y, s.Z, m.x, m.y, m.z); d > campfixFarMove {
				flag(i, "far_move", fmt.Sprintf("a move of %d", campfixRound(d)))
			}
		}
		if places && s.ID != 0 && gw.CampfixNewcomerThreat(s.ID, combos) {
			if where := h.campfixNewcomerPlace(world, m.x, m.z, spots); where != "" {
				flag(i, "aggro", fmt.Sprintf("%s attacks a level 1 of some race on sight, and the mark is within %d of %s",
					m.name, campfixTownReach, where))
			}
		}
		if places {
			g := gw.CampfixGroundAt(world, m.x, m.y, m.z)
			if g.FloorKnown && m.y-g.Floor > campfixAbove {
				flag(i, "above_ground", fmt.Sprintf("the mark is %.1f above the ground's floor under it", m.y-g.Floor))
			}
			if g.WetKnown && g.Wet && s.ID != 0 && gw.CampfixKeepsToLand(s.ID) {
				flag(i, "in_water", fmt.Sprintf("%s keeps to the land, and the mark is in the water", m.name))
			}
		}
	}

	first := r.marks[0]
	f.row = database.CampfixReport{AccountID: sess.AccountID(), CharName: player.Name, Level: player.Level, Class: player.Class,
		World: int(world), X: campfix2(first.x), Z: campfix2(first.z), Note: r.note,
		Marks: campfixJSON(f.marks), Near: campfixJSON(f.near), Flags: campfixJSON(f.flags)}
	return f
}

func campfixJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

// campfixMedian is the median level of the creatures recorded near the marks.
func campfixMedian(near []CampfixNearRecord) (float64, bool) {
	if len(near) == 0 {
		return 0, false
	}
	lv := make([]int, len(near))
	for i, n := range near {
		lv[i] = n.Level
	}
	sort.Ints(lv)
	if len(lv)%2 == 1 {
		return float64(lv[len(lv)/2]), true
	}
	return float64(lv[len(lv)/2-1]+lv[len(lv)/2]) / 2, true
}

// campfixNum is a figure as a flag says it: a whole one bare, else one place.
func campfixNum(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// campfixNewcomers are the new characters a creature is asked about: every
// race, class and home city defaultcharacter starts one of (spots), or, with
// no spots, the game's ten races of every class, a human of either city.
func campfixNewcomers(spots []database.StartSpot) [][3]int {
	seen := map[[3]int]bool{}
	var out [][3]int
	for _, s := range spots {
		k := [3]int{s.Race, s.Class, s.HumType}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) > 0 {
		return out
	}
	for race := 0; race < 10; race++ {
		for class := 0; class < 15; class++ {
			if race == 0 {
				out = append(out, [3]int{race, class, 1}, [3]int{race, class, 2})
			} else {
				out = append(out, [3]int{race, class, 0})
			}
		}
	}
	return out
}

// campfixNewcomerPlace is what a place is within campfixTownReach of where new
// players are: a town (towns.go) or a defaultcharacter start spot, "" neither.
func (h *ChatHandler) campfixNewcomerPlace(world uint8, x, z float32, spots []database.StartSpot) string {
	if h.gameWorld.NearTown(world, x, z, campfixTownReach) {
		return "a town"
	}
	for _, s := range spots {
		if s.World == int(world) && campfixXZ(s.X, s.Z, x, z) <= campfixTownReach {
			return fmt.Sprintf("a new %s %s's start spot", strings.ToLower(entity.RaceName(s.Race)), strings.ToLower(entity.ClassName(s.Class)))
		}
	}
	return ""
}

// campfixPlace is where a report's first mark is, as its board entry names
// it: the nearest #tele camp within campfixCampName (CampMarkers: where its
// anchor stands in gw, else as written; gw may be nil), else the nearest town
// within campfixTownName (TownMarks), else its x and z; after the world's
// name.
func campfixPlace(gw *entity.World, world uint8, x, z float32) string {
	label, best := "", float32(-1)
	for _, c := range CampMarkers(gw, int(world)) {
		if d := campfixXZ(c.X, c.Z, x, z); d <= campfixCampName && (best < 0 || d < best || d == best && c.Label < label) {
			label, best = c.Label, d
		}
	}
	if label == "" {
		for _, t := range TownMarks(int(world)) {
			if d := campfixXZ(t.X, t.Z, x, z); d <= campfixTownName && (best < 0 || d < best || d == best && t.Name < label) {
				label, best = t.Name, d
			}
		}
	}
	if label != "" {
		return worldName(world) + " near " + label
	}
	return fmt.Sprintf("%s at %.0f, %.0f", worldName(world), x, z)
}

// campfixFiler is whom a report is from, as its board entry says it: the
// character (its name, level and class then, its id) and its account; and for
// one filed from a petition (campfixpetition.go), the petition, when it was
// filed, and who made it a report.
type campfixFiler struct {
	Name          string
	Level         int
	Class         int
	CharID        uint32
	AccountID     int
	Petition      int64
	PetitionFiled time.Time
	FiledBy       string
}

// campfixPost is a report's board entry as filed: owner the GMs, planned,
// area data, touching npcs (and campspawns when a camp's spawn is a mark's row
// or template, camps), with campfixTitle and campfixDetails.
func campfixPost(gw *entity.World, id int64, who campfixFiler, marks []CampfixMarkRecord, note string,
	near []CampfixNearRecord, flags []CampfixFlagRecord, camps map[int]bool, now time.Time) database.WorkPost {
	title := campfixTitle(gw, id, who, marks)
	details := campfixDetails(gw, id, who, marks, note, near, flags, now)
	touches := "npcs"
	for _, m := range marks {
		if camps[m.Row] || camps[m.Template] {
			touches += ", campspawns"
			break
		}
	}
	owner, status, area := CampfixBoardOwner, database.WorkPlanned, "data"
	return database.WorkPost{Title: &title, Area: &area, Touches: &touches, Status: &status, Owner: &owner, Details: &details}
}

// CampfixBoardOwner is the owner of a report's board entry: the GMs, who
// approve or decline it on the panel (no session applies one).
const CampfixBoardOwner = "GMs (Camp reports page)"

// campfixTitle is the board entry's title, at most database.WorkMaxTitle
// characters: "Camp fix report 512: Kaelith (L24 ranger), Tunaria near bandits:
// 2 move, 1 add, 1 remove", the place cut short when it does not fit.
func campfixTitle(gw *entity.World, id int64, who campfixFiler, marks []CampfixMarkRecord) string {
	first := marks[0]
	head := fmt.Sprintf("Camp fix report %d: %s (L%d %s), ", id, who.Name, who.Level, strings.ToLower(entity.ClassName(who.Class)))
	kinds := make([]string, len(marks))
	for i, m := range marks {
		kinds[i] = m.Kind
	}
	tail := ": " + campfixKinds(kinds, false)
	place := campfixPlace(gw, uint8(first.World), float32(first.X), float32(first.Z))
	if room := database.WorkMaxTitle - utf8.RuneCountInString(head) - utf8.RuneCountInString(tail); utf8.RuneCountInString(place) > room {
		place = campfixCut(place, room-3) + "..."
	}
	return campfixCut(head+place+tail, database.WorkMaxTitle)
}

// campfixCut is s cut to at most n characters.
func campfixCut(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// campfixCell is text as a markdown table's cell holds it.
func campfixCell(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "|", `\|`).Replace(s)
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func campfixAt(x, y, z float64) string { return fmt.Sprintf("%.1f, %.1f, %.1f", x, y, z) }

// CampfixReviewLine is the first line of a report's board entry: where it is
// decided.
const CampfixReviewLine = "A GM approves or declines it on the GM panel's Camp reports page (the server's CLAUDE.md, " +
	"Camp fix reports); nothing is applied before that, and no session applies one itself."

// campfixDetails is the board entry's details, under database.WorkMaxDetails
// characters: the line on the review, the petition it came from, who, where,
// when, the build and the shard, the note as its writer's text, the marks,
// the flags and the creatures nearby, the last cut short (and said so) when
// they do not all fit.
func campfixDetails(gw *entity.World, id int64, who campfixFiler, marks []CampfixMarkRecord, note string,
	near []CampfixNearRecord, flags []CampfixFlagRecord, now time.Time) string {
	first := marks[0]
	var b strings.Builder
	b.WriteString(CampfixReviewLine + "\n\n")
	if who.Petition != 0 {
		fmt.Fprintf(&b, "- **From:** petition %d, filed %s; made a camp report by %s. Its marks are where the character stood "+
			"when it filed the petition.\n", who.Petition, who.PetitionFiled.UTC().Format("2006-01-02 15:04:05 UTC"), campfixCell(who.FiledBy))
	}
	fmt.Fprintf(&b, "- **Who:** %s, a level %d %s (character %d, account %d)\n", campfixCell(who.Name), who.Level,
		entity.ClassName(who.Class), who.CharID, who.AccountID)
	fmt.Fprintf(&b, "- **Where:** %s (world %d); the first mark at (%.1f, %.1f, %.1f)\n",
		campfixCell(campfixPlace(gw, uint8(first.World), float32(first.X), float32(first.Z))), first.World, first.X, first.Y, first.Z)
	fmt.Fprintf(&b, "- **When:** %s (report %d)\n", now.UTC().Format("2006-01-02 15:04:05 UTC"), id)
	fmt.Fprintf(&b, "- **Build:** %s; **shard:** %s\n\n", serverBuild(), campfixCell(campfixShard()))
	switch {
	case note == "":
		b.WriteString("**Note:** none.\n\n")
	case who.Petition != 0:
		fmt.Fprintf(&b, "**Note**, by whoever made it a report:\n\n> %s\n\n", note)
	default:
		fmt.Fprintf(&b, "**Note**, the player's own text:\n\n> %s\n\n", note)
	}

	b.WriteString("### Marks\n\n")
	b.WriteString("| # | Kind | Typed | Creature | Place | Facing | Row | From | From facing | Distance |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for i, m := range marks {
		creature, row, from, fromFacing, dist := m.Name, "-", "-", "-", "-"
		if m.Row != 0 || m.Template != 0 {
			creature = fmt.Sprintf("%s (L%d)", m.Name, m.Level)
			row = fmt.Sprint(m.Row)
			if m.Template != 0 {
				row = fmt.Sprintf("template %d", m.Template)
			}
			from, fromFacing, dist = campfixAt(m.FromX, m.FromY, m.FromZ), fmt.Sprint(m.FromFacing), fmt.Sprintf("%.1f", m.Distance)
		} else if m.Kind == campfixMove {
			row = "none free"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %.4f rad, byte %d | %s | %s | %s | %s |\n", i+1, m.Kind, campfixCell(m.Typed),
			campfixCell(creature), campfixAt(m.X, m.Y, m.Z), m.Facing, m.FacingByte, row, from, fromFacing, dist)
	}

	b.WriteString("\n### Flags\n\n")
	if len(flags) == 0 {
		b.WriteString("None.\n")
	}
	for _, fl := range flags {
		what := "?"
		if fl.Mark >= 1 && fl.Mark <= len(marks) {
			what = campfixWhatRecord(marks[fl.Mark-1])
		}
		fmt.Fprintf(&b, "- Mark %d (%s): %s\n", fl.Mark, campfixCell(what), campfixCell(fl.Text))
	}

	b.WriteString("\n### Nearby\n\n")
	if len(near) == 0 {
		fmt.Fprintf(&b, "No NPC of a row spawns within %.0f of a mark.\n", campfixNearReach)
		return b.String()
	}
	fmt.Fprintf(&b, "%d NPCs of rows whose spawn is within %.0f of a mark, nearest first (Movement as the panel says it, to its first colon):\n\n",
		len(near), campfixNearReach)
	b.WriteString("| Row | Name | Level | Spawn | Facing | Alive | Movement | Camp | Same name | To a mark |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	head := b.String()
	var lines []string
	for _, n := range near {
		camp := "-"
		if n.Camp != 0 {
			camp = fmt.Sprintf("%d: %s", n.Camp, strings.Join(n.Kinds, ", "))
		}
		move, _, _ := strings.Cut(n.Movement, ":")
		lines = append(lines, fmt.Sprintf("| %d | %s | %d | %s | %d | %s | %s | %s | %s | %.1f |\n", n.Row, campfixCell(n.Name), n.Level,
			campfixAt(n.X, n.Y, n.Z), n.FacingByte, campfixKnown(n.Alive), campfixCell(move), campfixCell(camp), campfixYes(n.SameName), n.Distance))
	}
	most := database.WorkMaxDetails - 1 // under the board's bound
	size := utf8.RuneCountInString(head)
	for _, l := range lines {
		size += utf8.RuneCountInString(l)
	}
	if size <= most {
		return head + strings.Join(lines, "")
	}
	for k := len(lines) - 1; k >= 0; k-- {
		cut := fmt.Sprintf("\n(The table is cut: %d more of the %d left out to keep this under %d characters; the report's near column holds them all.)\n",
			len(lines)-k, len(lines), database.WorkMaxDetails)
		body := head + strings.Join(lines[:k], "") + cut
		if utf8.RuneCountInString(body) <= most {
			return body
		}
	}
	return campfixCut(head, most)
}

func campfixYes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func campfixBool(b bool) *bool { return &b }

// campfixKnown is an optional yes or no as a table's cell says it: "-" when
// not known.
func campfixKnown(b *bool) string {
	if b == nil {
		return "-"
	}
	return campfixYes(*b)
}
