package handlers

import (
	"fmt"
	"hash/fnv"
	"log"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Quest marks over the NPCs' heads (the GM Jibrish's petition 1813,
// 2026-11-12: "Add the ability for us to add yellow ! and ? over NPCs in
// game, visible to the player, if they toggle it on or we turn it on by
// default. The exclamation point over their head should only happen if
// there's a quest they can log right then, and the question mark if there's a
// quest they can progress right then such as via a turn in or completion").
// Beside the map's marks (questmarks.go), which a client draws on its map,
// these are drawn over the NPC itself by a client that asks
// (protocol.OpcodeQuestHeadMarksOffer; vanilla's Extended > UI > HUD > Quest
// Markers). For one player:
//
//   - "!" over a giver of a quest the player may take now (mayTakeLocked:
//     enabled, its levels, classes and races, the quests it requires
//     finished, its standings, neither under way nor finished unless
//     repeatable, a hand-in's items held);
//   - "?" over the NPC of a step of a quest under way that the player can do
//     there now: a talk step, or a show or give step with its items and tunar
//     held (lackingLocked), its giver willing (giverWontDeal); a step pinned
//     to one spawn marks that spawn alone. And over a giver of a quest the
//     player may take whose first step hands that giver items the player
//     holds now (readyHandInLocked's: a turn-in), in place of its "!".
//
// "?" goes before "!" at one NPC. Which quests are marked is the server's:
// quests.markers "flagged" (the default) the quests of table questmarks, "all"
// every quest (a "!" only near the player's level, as the map's
// markLevelNear), "off" none, and no offer is made. The marks are worked out
// by name in the engine, never per tick: when the quests change (a quest
// taken, a step done, a kill counted, a GM's edit: nudge), when the player's
// level, class, race, tunar or pack changes, and every headEvery besides (a
// standing changed by a kill), then given the World (entity/questheadmarks.go),
// which sends each client the marks of the NPCs it has in view whenever that
// set changes: an NPC coming into view gets its mark there. A PS2 never asks
// and is sent nothing.

// headEvery is how long a set of marks stands without anything it hangs on
// changing before it is worked out again (a standing moves on a kill).
const headEvery = 30 * time.Second

// The modes of quests.markers.
const (
	headFlagged int32 = iota // the quests of table questmarks
	headAll                  // every quest
	headOff                  // none, no offer
)

// questHeadMode is quests.markers (SetQuestHeadMarks).
var questHeadMode atomic.Int32

// SetQuestHeadMarks is quests.markers: "flagged" or "" the quests of table
// questmarks, "all" every quest, "off" none. Another word is an error.
func SetQuestHeadMarks(mode string) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "flagged":
		questHeadMode.Store(headFlagged)
	case "all":
		questHeadMode.Store(headAll)
	case "off":
		questHeadMode.Store(headOff)
	default:
		return fmt.Errorf("%q is not flagged, all or off", mode)
	}
	return nil
}

// QuestHeadMarksMode is quests.markers as set, in a word.
func QuestHeadMarksMode() string {
	switch questHeadMode.Load() {
	case headAll:
		return "all"
	case headOff:
		return "off"
	default:
		return "flagged"
	}
}

// questHeadLayout is the layout of OpcodeQuestHeadMarks the offer names.
const questHeadLayout = 1

// buildQuestHeadMarksOffer is OpcodeQuestHeadMarksOffer: the layout.
func buildQuestHeadMarksOffer() []byte {
	return build(protocol.OpcodeQuestHeadMarksOffer, 3, func(w *buffer.Writer) {
		w.WriteByte(questHeadLayout)
	})
}

// SendQuestHeadMarksOffer offers the session's client the marks over the
// NPCs' heads, unless quests.markers is off. A PS2 ignores it.
func SendQuestHeadMarksOffer(sess *session.Session) {
	if questHeadMode.Load() == headOff {
		return
	}
	queueReliable(sess, protocol.OpcodeQuestHeadMarksOffer, buildQuestHeadMarksOffer())
}

// QuestHeadMarksHandler takes a client's OpcodeQuestHeadMarksAnswer.
type QuestHeadMarksHandler struct {
	world *entity.World
}

// NewQuestHeadMarksHandler creates it.
func NewQuestHeadMarksHandler(w *entity.World) *QuestHeadMarksHandler {
	return &QuestHeadMarksHandler{world: w}
}

// RegisterHandlers registers it.
func (h *QuestHeadMarksHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeQuestHeadMarksAnswer, h.HandleAnswer)
}

// HandleAnswer is OpcodeQuestHeadMarksAnswer: a u8, nonzero to be sent the
// marks, 0 to stop (a wider answer is read by its first byte). Nothing is
// answered; on a server whose quests.markers is off it is ignored.
func (h *QuestHeadMarksHandler) HandleAnswer(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 1 || h.world == nil {
		log.Printf("QuestHeadMarks: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	if questHeadMode.Load() == headOff {
		return nil
	}
	on := data[0] != 0
	p := h.world.SetQuestHeadMarksWanted(sess, on)
	if p == nil {
		log.Printf("QuestHeadMarks: %s has no player in the world", sess.RemoteAddr())
		return nil
	}
	Quests().SetHeadMarks(p, on)
	log.Printf("QuestHeadMarks: %s %s", p.Name, map[bool]string{true: "draws them", false: "stopped"}[on])
	return nil
}

// SetHeadMarks has the engine work the character's marks out again at once
// (on: its client just asked) or forget them (off).
func (q *QuestEngine) SetHeadMarks(p *entity.Player, on bool) {
	if q == nil || p == nil {
		return
	}
	q.mu.Lock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.headKey, c.headSent = "", ""
	q.mu.Unlock()
	if !on && q.gw != nil {
		q.gw.SetQuestHeadMarks(p.CharServerID, nil)
	}
	q.nudge()
}

// resumeHeadMarks is a keep restart's: the client asked before it, so it is
// sent the marks again as if it had just asked.
func (q *QuestEngine) resumeHeadMarks(p *entity.Player) {
	if q == nil || q.gw == nil || p == nil {
		return
	}
	q.gw.WantQuestHeadMarks(p, true)
	q.SetHeadMarks(p, true)
}

// headMarksOn is whether p's client asked for the marks (a keep restart keeps
// it).
func (q *QuestEngine) headMarksOn(p *entity.Player) bool {
	return q != nil && q.gw != nil && q.gw.QuestHeadMarksWanted(p)
}

// syncHeadMarks works out the marks of each character in the world whose
// client asked, when what they hang on changed (headKeyLocked) or headEvery
// went by, and gives the World a set that differs from the last. Off the
// engine's own goroutine, every second and at each nudge.
func (q *QuestEngine) syncHeadMarks(now time.Time) {
	if q.gw == nil || q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		sess := q.sessionOf(id)
		var p *entity.Player
		if sess != nil {
			p = q.playerOf(sess)
		}
		if p == nil || questHeadMode.Load() == headOff || !q.gw.QuestHeadMarksWanted(p) {
			if c.headSent != "" {
				q.gw.SetQuestHeadMarks(id, nil)
			}
			c.headKey, c.headSent = "", ""
			continue
		}
		key := q.headKeyLocked(sess, p)
		if key == c.headKey && now.Sub(c.headAt) < headEvery {
			continue
		}
		c.headKey, c.headAt = key, now
		end := q.beginPassLocked(p)
		m := q.headMarksLocked(q.lookAt(sess), p, c)
		end()
		if s := headMarksText(m); s != c.headSent {
			c.headSent = s
			q.gw.SetQuestHeadMarks(id, m)
			log.Printf("QuestHeadMarks: %s: %s", p.Name, clip(s, 300))
		}
	}
}

// headKeyLocked is what a character's marks hang on, past its quests' and
// standings': the quests' changes (gen), the mode, its level, class, race and
// tunar, and its pack as its client's list holds it.
func (q *QuestEngine) headKeyLocked(sess *session.Session, p *entity.Player) string {
	h := fnv.New64a()
	if l := sessionInvList(sess); l != nil {
		l.mu.Lock()
		for _, r := range l.recs {
			fmt.Fprintf(h, "%d*%d,", r.patternID, r.qty)
		}
		l.mu.Unlock()
	}
	return fmt.Sprintf("%d %d %d %d %d %d %x", q.gen.Load(), questHeadMode.Load(), p.Level, p.Class, p.Race, p.Tunar, h.Sum64())
}

// markedWithFollowUps is table questmarks' quests and every quest that follows
// one, however far down its chain (successors: the enabled quests with a giver
// that require each). The GM Jibrish's petition 1875 (2026-11-13): "After
// turning in a quest, if the quest NPC has a follow up it is not showing a new
// yellow ! for the newbie quests, make sure this works for all the new player
// 1-10 quests": the table flags each race's and class's first quest
// (tools/questmarks_20261112.py), and their follow-ups, flagged by nobody, were
// shown nothing. A follow-up shows its "!" as soon as the player may take it:
// at the turn-in that finishes the quest before it (saveLocked's nudge works
// the marks out again at once), or at the level it asks.
func markedWithFollowUps(marked map[int]bool, successors map[int][]*database.Quest) map[int]bool {
	out := make(map[int]bool, len(marked))
	var walk func(id int)
	walk = func(id int) {
		for _, s := range successors[id] {
			if !out[s.ID] {
				out[s.ID] = true
				walk(s.ID)
			}
		}
	}
	for id, on := range marked {
		if on {
			out[id] = true
			walk(id)
		}
	}
	return out
}

// headShows is whether a quest is marked under quests.markers.
func (q *QuestEngine) headShows(qu *database.Quest) bool {
	if qu == nil || !qu.Enabled {
		return false
	}
	switch questHeadMode.Load() {
	case headAll:
		return true
	case headFlagged:
		return q.marked[qu.ID]
	}
	return false
}

// headMarksLocked is the character's marks now (its standings asked of
// look). Must be called with q.mu held.
func (q *QuestEngine) headMarksLocked(look *factionLook, p *entity.Player, c *questChar) *entity.QuestHeadMarks {
	m := &entity.QuestHeadMarks{Names: map[string]byte{}, Spawns: map[entity.EntityID]byte{}}
	put := func(name string, mark byte) {
		if k := foldName(name); k != "" && m.Names[k] < mark {
			m.Names[k] = mark
		}
	}
	// "?": a step of a quest under way the player can do at its NPC now
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if !q.headShows(qu) {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil || !database.AtNPC(st.Kind) || strings.TrimSpace(st.Target) == "" {
			continue
		}
		if giverWontDeal(look, qu, st) != "" {
			continue
		}
		if (st.Kind == database.StepGive || st.Kind == database.StepShow) && q.lackingLocked(p, st) != "" {
			continue
		}
		if st.NPCID != 0 {
			m.Spawns[entity.EntityID(st.NPCID)] = entity.QuestMarkStep
		} else {
			put(st.Target, entity.QuestMarkStep)
		}
	}
	// "!": a quest to take; "?" in its place for a hand-in held now
	all := questHeadMode.Load() == headAll
	for giver, quests := range q.cat.givers {
		if m.Names[giver] == entity.QuestMarkStep {
			continue
		}
		for _, qu := range quests {
			if !q.headShows(qu) || all && !markLevelNear(p.Level, qu.MinLevel) {
				continue
			}
			if q.mayTakeLocked(look, p, c, qu) != "" {
				continue
			}
			mark := entity.QuestMarkTake
			if st := qu.First(); st != nil && st.Kind == database.StepGive && len(st.Items) > 0 && foldName(st.Target) == giver &&
				q.lackingLocked(p, st) == "" {
				mark = entity.QuestMarkStep
			}
			put(giver, mark)
			if mark == entity.QuestMarkStep {
				break
			}
		}
	}
	return m
}

// headMarksText is a set of marks as one line, the same for the same set:
// "!name, ?name, ?#spawn" in order.
func headMarksText(m *entity.QuestHeadMarks) string {
	if m == nil {
		return ""
	}
	sign := map[byte]string{entity.QuestMarkTake: "!", entity.QuestMarkStep: "?"}
	var out []string
	for k, v := range m.Names {
		out = append(out, sign[v]+k)
	}
	for k, v := range m.Spawns {
		out = append(out, fmt.Sprintf("%s#%d", sign[v], k))
	}
	sort.Strings(out)
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}
