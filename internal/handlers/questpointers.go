package handlers

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// Pointers to a quest's next giver (petition 1692, Hige: "Ozmo Da Basher is
// not updating quest log for epic part 2"). A quest Q ends at one NPC; the
// quest that follows it, S (S's requires names Q), is offered by another,
// somewhere else, and once Q leaves the journal nothing in it said so: Four
// Sashes ends at Ozmo da Basher in Oggok, whose last words name Tilvyis in
// Neriak, and The Breastplate and the Leggings waits there unseen. Some 450
// of the enabled quests follow a quest that way.
//
// So the journal shows, after the quests under way, a line for each such S
// the player may take now (mayTakeLocked: enabled, its level, class and race,
// every quest it requires finished, its standings, neither under way nor
// finished) and has never finished, when it follows a quest the player has
// finished whose last step was not at S's giver:
//
//	The Breastplate and the Leggings: find Tilvyis S`Lynx in the south of Neriak west of the coachman
//
// The place is what the finished quest's last words say of the giver
// (placeInSays: "Tilvyis, in the south of Neriak west of the coachman, want
// you now."), else the coach town the giver stands near (entity.TownAt),
// else its world. The line is the journal's ordinary 0x007C, so a PS2 and
// vanilla show it as they show any quest's; vanilla's map already marks the
// giver's "!" (questmarks.go).
//
// Nothing is stored: the lines are worked out from the character's finished
// quests at each MemoryDump (JournalFor) and kept current every second and at
// each change (syncPointers), so taking S at its giver turns its line into
// S's own. They come last and only into the room the quests under way leave
// (CLIENT's eight lines), at most pointerCap of them, the last saying how
// many more there are; a quest taken takes their room first (addLineLocked).
// A player who deletes one (0x007D) is not shown it again until the next
// login. quests.next_giver_pointer false turns them off.

// pointerCap is the most pointer lines a journal shows.
const pointerCap = 3

// nextGiverPointer is quests.next_giver_pointer (SetNextGiverPointer).
var nextGiverPointer atomic.Bool

func init() { nextGiverPointer.Store(true) }

// SetNextGiverPointer is quests.next_giver_pointer: whether the journal shows
// the next quest's giver once a quest is finished. Unset: on.
func SetNextGiverPointer(on bool) { nextGiverPointer.Store(on) }

// NextGiverPointer is whether the pointers are on.
func NextGiverPointer() bool { return nextGiverPointer.Load() }

// questPointer is one pointer: the quest S it points to and its line.
type questPointer struct {
	quest int
	line  string
}

// successorsOf makes the catalog's successors: by quest id, the enabled
// quests with a giver that require it (in any group), each once, by id.
func successorsOf(all []*database.Quest) map[int][]*database.Quest {
	out := map[int][]*database.Quest{}
	for _, s := range all {
		if !s.Enabled || strings.TrimSpace(s.Giver) == "" {
			continue
		}
		seen := map[int]bool{}
		for _, g := range s.Required() {
			for _, id := range g {
				if id == s.ID || seen[id] {
					continue
				}
				seen[id] = true
				out[id] = append(out[id], s)
			}
		}
	}
	for id := range out {
		sort.Slice(out[id], func(i, j int) bool { return out[id][i].ID < out[id][j].ID })
	}
	return out
}

// lastStep is a quest's last step, or nil.
func lastStep(qu *database.Quest) *database.QuestStep {
	if qu == nil || len(qu.Steps) == 0 {
		return nil
	}
	last := &qu.Steps[0]
	for i := range qu.Steps {
		if qu.Steps[i].Step > last.Step {
			last = &qu.Steps[i]
		}
	}
	return last
}

// pointersLocked is the character's pointers, in order (the quest followed
// finished last first), every one of them: the journal shows the first few
// (pointerLines). look asks the standings. Must be called with q.mu held.
func (q *QuestEngine) pointersLocked(look *factionLook, p *entity.Player, c *questChar) []questPointer {
	if !NextGiverPointer() || p == nil || c == nil || len(q.cat.successors) == 0 {
		return nil
	}
	// the finished quests, the latest first
	var done []*database.CharQuest
	for _, cq := range c.quests {
		if cq.Done > 0 && len(q.cat.successors[cq.QuestID]) > 0 {
			done = append(done, cq)
		}
	}
	sort.Slice(done, func(i, j int) bool {
		if done[i].Updated != done[j].Updated {
			return done[i].Updated > done[j].Updated
		}
		return done[i].QuestID < done[j].QuestID
	})
	var out []questPointer
	seen := map[int]bool{}
	for _, cq := range done {
		for _, s := range q.cat.successors[cq.QuestID] {
			if seen[s.ID] {
				continue
			}
			seen[s.ID] = true
			if c.dismissed[s.ID] || c.timesDone(s.ID) > 0 || q.endsAtGiverLocked(c, s) {
				continue
			}
			if q.mayTakeLocked(look, p, c, s) != "" {
				continue
			}
			out = append(out, questPointer{quest: s.ID, line: q.pointerLineLocked(p, c, s)})
		}
	}
	return out
}

// endsAtGiverLocked is whether a quest S requires that the character has
// finished ended at S's giver: the player was just there, and S's giver
// offers it when talked to again.
func (q *QuestEngine) endsAtGiverLocked(c *questChar, s *database.Quest) bool {
	giver := foldName(s.Giver)
	for _, g := range s.Required() {
		for _, id := range g {
			if !c.hasFinished(id) {
				continue
			}
			if st := lastStep(q.cat.byID[id]); st != nil && database.AtNPC(st.Kind) && foldName(st.Target) == giver {
				return true
			}
		}
	}
	return false
}

// pointerLineLocked is S's pointer line: its name, its giver and where.
func (q *QuestEngine) pointerLineLocked(p *entity.Player, c *questChar, s *database.Quest) string {
	giver := strings.TrimSpace(s.Giver)
	where := ""
	// what the finished quests' last words say of the giver, the latest
	// finished first
	var reqs []*database.CharQuest
	for _, g := range s.Required() {
		for _, id := range g {
			if cq := c.quests[id]; cq != nil && cq.Done > 0 {
				reqs = append(reqs, cq)
			}
		}
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].Updated > reqs[j].Updated })
	for _, cq := range reqs {
		if st := lastStep(q.cat.byID[cq.QuestID]); st != nil {
			if where = placeInSays(st.Says, giver); where != "" {
				break
			}
		}
	}
	if where == "" {
		where = q.giverPlaceLocked(p, giver)
	}
	name := strings.TrimSpace(s.Name)
	if name == "" {
		name = fmt.Sprintf("Quest %d", s.ID)
	}
	line := name + ": find " + giver
	if where != "" {
		line += " " + where
	}
	return line
}

// pointerWorlds is how a pointer names a world its giver stands in when no
// town is near (the worlds players know by a name).
var pointerWorlds = map[uint8]string{
	0:  "in Tunaria",
	1:  "in the Rathe Mountains",
	2:  "on Odus",
	3:  "in Lavastorm",
	4:  "in the Plane of Sky",
	10: "in the Homelands",
	11: "in the Canyon of Banners",
}

// giverPlaceLocked is where the giver stands, as a pointer says it: "near" the
// coach town its spawn is near (the one nearest the player when it stands in
// several places), else its world's name, else "".
func (q *QuestEngine) giverPlaceLocked(p *entity.Player, giver string) string {
	spots := q.markSpotsLocked(time.Now())[foldName(giver)]
	if len(spots) == 0 {
		return ""
	}
	best := spots[0]
	if q.gw != nil {
		if world, x, _, z, ok := q.gw.PlayerPlace(p); ok {
			bestD := math.MaxFloat64
			for _, sp := range spots {
				d := math.MaxFloat64 / 2
				if sp.World == world {
					d = math.Hypot(float64(sp.X-x), float64(sp.Z-z))
				}
				if d < bestD {
					best, bestD = sp, d
				}
			}
		}
	}
	if town, ok := entity.TownAt(best.World, best.X, best.Z); ok {
		return "near " + town
	}
	return pointerWorlds[best.World]
}

// The words placeInSays looks for: a place said after the giver's name ("in
// the south of Neriak west of the coachman"), and one the sentence sends the
// player to ("Go to the monastery of Shon-To").
var (
	saysPlaceRE = regexp.MustCompile(`(?i)\b(?:in|at|by|near|inside|beside|behind|outside|beneath|beyond|past|across|north of|south of|east of|west of)\s+[^,.;:!?\n/]+`)
	saysGoToRE  = regexp.MustCompile(`(?i)\bgo to\s+([^,.;:!?\n/]+)`)
	sentenceRE  = regexp.MustCompile(`[^.!?\n/]+`)
)

// nameStop is the words of an NPC's name that do not name it alone (its
// title): a sentence is the giver's by one of its other words.
var nameStop = map[string]bool{
	"master": true, "captain": true, "sergeant": true, "professor": true, "prof": true, "elder": true,
	"guard": true, "keeper": true, "the": true, "of": true, "lord": true, "lady": true, "sir": true,
	"duke": true, "foreman": true, "director": true, "warlord": true, "underlord": true, "recruiter": true,
	"coachman": true, "coachwoman": true, "commander": true, "general": true, "high": true, "priest": true,
	"priestess": true, "brother": true, "sister": true, "trainer": true, "guildmaster": true, "da": true,
}

// placeInSays is where an NPC's words (a quest's last step's says) put the
// giver called giver, "" when they do not name it or say no place: in the
// first sentence naming the giver, the place said after the name, then the
// place the sentence sends the player to.
func placeInSays(says, giver string) string {
	var names []string
	for _, w := range strings.Fields(giver) {
		w = strings.Trim(w, ".,'\"")
		if len(w) >= 3 && !nameStop[strings.ToLower(w)] {
			names = append(names, regexp.QuoteMeta(w))
		}
	}
	if len(names) == 0 || strings.TrimSpace(says) == "" {
		return ""
	}
	nameRE := regexp.MustCompile(`(?i)\b(?:` + strings.Join(names, "|") + `)\b`)
	for _, sent := range sentenceRE.FindAllString(says, -1) {
		loc := nameRE.FindStringIndex(sent)
		if loc == nil {
			continue
		}
		var parts []string
		if m := saysPlaceRE.FindString(sent[loc[1]:]); m != "" {
			parts = append(parts, strings.TrimSpace(m))
		}
		if m := saysGoToRE.FindStringSubmatch(sent); m != nil {
			if t := strings.TrimSpace(m[1]); t != "" && !nameRE.MatchString(t) {
				parts = append(parts, "at "+t)
			}
		}
		return clipRunes(strings.Join(parts, " "), 80)
	}
	return ""
}

// pointerLines is the pointers' journal lines in room lines at most (and
// pointerCap): the last of them saying how many more there are, and the
// quests they stand for, in order.
func pointerLines(ps []questPointer, room int) ([]string, []int) {
	n := min(len(ps), room, pointerCap)
	if n <= 0 {
		return nil, nil
	}
	lines := make([]string, n)
	ids := make([]int, n)
	for i := 0; i < n; i++ {
		lines[i] = clipRunes(ps[i].line, questLineChars)
		ids[i] = ps[i].quest
	}
	if more := len(ps) - n; more > 0 {
		tail := fmt.Sprintf(" (+%d more)", more)
		lines[n-1] = clipRunes(ps[n-1].line, questLineChars-len(tail)) + tail
	}
	return lines, ids
}

// wantLinesLocked is the character's journal: its quests' lines, then its
// pointers' into the room left. Must be called with q.mu held.
func (q *QuestEngine) wantLinesLocked(look *factionLook, p *entity.Player, c *questChar) ([]string, []int) {
	lines := q.linesLocked(c)
	pl, ids := pointerLines(q.pointersLocked(look, p, c), questJournalLines-len(lines))
	for _, l := range pl {
		lines = append(lines, c.say(l))
	}
	return lines, ids
}

// JournalFor is Journal for a player in the world (the MemoryDump's), its
// pointers to the next quests' givers included, their standings asked of
// sess.
func (q *QuestEngine) JournalFor(sess *session.Session, p *entity.Player) (uint32, []string) {
	if q == nil || p == nil {
		return 0, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.box = nil
	lines, ids := q.wantLinesLocked(q.lookAt(sess), p, c)
	c.shown = append([]string(nil), lines...)
	c.pointerIDs = ids
	return c.token, lines
}

// syncPointers keeps the pointers of the characters in the world current.
func (q *QuestEngine) syncPointers() {
	if q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		if len(c.pointerIDs) == 0 && !NextGiverPointer() {
			continue
		}
		sess := q.sessionOf(id)
		if sess == nil {
			continue
		}
		q.syncPointersLocked(sess, q.playerOf(sess), c)
	}
}

// syncPointersLocked makes the client's journal the character's, pointers
// and all: the lines from the first that differs taken (the last first) and
// the rest added. Must be called with q.mu held.
func (q *QuestEngine) syncPointersLocked(sess *session.Session, p *entity.Player, c *questChar) {
	if p == nil {
		return
	}
	want, ids := q.wantLinesLocked(q.lookAt(sess), p, c)
	if sameLines(want, c.shown) {
		c.pointerIDs = ids
		return
	}
	k := 0
	for k < len(want) && k < len(c.shown) && want[k] == c.shown[k] {
		k++
	}
	for len(c.shown) > k {
		q.dropLineLocked(sess, c, len(c.shown)-1)
	}
	for _, l := range want[k:] {
		q.appendLineLocked(sess, c, l)
	}
	c.pointerIDs = ids
}

// stripPointersLocked takes the pointers' lines off the journal's end, so a
// quest's line added next comes before them (syncPointers puts them back).
func (q *QuestEngine) stripPointersLocked(sess *session.Session, c *questChar) {
	for len(c.pointerIDs) > 0 && len(c.shown) > 0 {
		q.dropLineLocked(sess, c, len(c.shown)-1)
	}
	c.pointerIDs = nil
}

// dropPointerLocked is the player's delete of a pointer's line (0x007D at idx,
// past the quests under way): not shown again this session. It reports
// whether idx was a pointer's.
func (q *QuestEngine) dropPointerLocked(sess *session.Session, c *questChar, idx int) bool {
	first := len(c.shown) - len(c.pointerIDs)
	if len(c.pointerIDs) == 0 || idx < first || idx >= len(c.shown) {
		return false
	}
	if c.dismissed == nil {
		c.dismissed = map[int]bool{}
	}
	c.dismissed[c.pointerIDs[idx-first]] = true
	q.dropLineLocked(sess, c, idx)
	q.nudge()
	return true
}
