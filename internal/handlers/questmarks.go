package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eqoa/pkg/pkg/buffer"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// Quest marks for a client's map (protocol.OpcodeQuestMarkers, the user: "add
// quest markers for givers to the map too", "only if eligible for the
// class/level"). Once the player is in the world the server offers them (a
// u32, 1), as it offers others' power; a client that draws them answers 1 to
// have them and 0 to stop, and from then on is sent the whole set
// (OpcodeQuestMarkersData) whenever it changes:
//
//   - a giver with a quest the player may take now (mayTakeLocked: enabled,
//     its levels, its classes and races, the quests it requires finished,
//     neither under way nor finished): kind 1, "!", each such quest with its
//     levels, the start of the giver's offer and its rewards;
//   - the NPC of a step the player has to do there (talk, show, give): kind
//     2, "?", with the step's journal line and what it needs;
//   - a kill or loot step's creatures: kind 3, "?" too, at the nearest of
//     their spawns (of every name the step counts), with the kills or drops
//     done of those needed.
//
// A PS2 ignores the offer and is sent nothing. Each NPC's mark is at its spawn
// (its nearest few when the name stands in more than one place); the marks
// are worked out from the engine's own state every second for the clients
// that asked, the spawns read from the World at most every half minute, and a
// set goes out only when it differs from the last one sent.

// The kinds of mark.
const (
	markTake byte = 1 // a quest to take from this NPC
	markStep byte = 2 // a step to do at this NPC
	markHunt byte = 3 // a kill or loot step's creatures
)

// The bounds of a set, so one stays a few datagrams: marks, quests a mark,
// spawns of one name, a text's bytes and the whole body's.
const (
	maxMarks       = 40
	maxMarkQuests  = 4
	maxMarkSpots   = 2
	maxMarkText    = 140
	maxMarksBytes  = 6000
	markSpotsEvery = 30 * time.Second
)

// questMark is one mark.
type questMark struct {
	kind   byte
	world  uint8
	x, z   float32
	npc    string
	quests []markQuest
	// dist is its distance from the player in the player's world (-1 in
	// another), for the order
	dist float32
}

// markQuest is one quest's lines on a mark: its name and levels, the line
// (the offer's start, or the step's journal line) and the detail (its
// rewards, or what the step needs).
type markQuest struct {
	name     string
	min, max int
	line     string
	detail   string
}

// questText is a quest's words for the marks, made when the quests are read
// (Reload): the start of its offer, its rewards in all (and for each class
// its steps reward besides, its own and the class's), and each step's needs
// (items and tunar), with the items' and spells' names.
type questText struct {
	offer   string
	rewards string
	byClass map[int]string
	needs   map[int]string
}

// rewardsFor is the quest's rewards for a player of a class.
func (t *questText) rewardsFor(class int) string {
	if t == nil {
		return ""
	}
	if s, ok := t.byClass[class]; ok {
		return s
	}
	return t.rewards
}

// buildQuestMarkersOffer is OpcodeQuestMarkers from the server: a u32, 1.
func buildQuestMarkersOffer() []byte {
	return build(protocol.OpcodeQuestMarkers, 6, func(w *buffer.Writer) {
		w.WriteUint32(1)
	})
}

// SendQuestMarkersOffer offers the session's client the quest marks.
func SendQuestMarkersOffer(sess *session.Session) {
	queueReliable(sess, protocol.OpcodeQuestMarkers, buildQuestMarkersOffer())
}

// QuestMarkersHandler takes a client's OpcodeQuestMarkers: whether it wants
// the marks.
type QuestMarkersHandler struct {
	world *entity.World
}

// NewQuestMarkersHandler creates the quest marks handler.
func NewQuestMarkersHandler(w *entity.World) *QuestMarkersHandler {
	return &QuestMarkersHandler{world: w}
}

// RegisterHandlers registers the quest marks handler.
func (h *QuestMarkersHandler) RegisterHandlers(r *Registry) {
	r.Register(protocol.OpcodeQuestMarkers, h.HandleQuestMarkers)
}

// HandleQuestMarkers makes the client's answer its character's: a u32,
// nonzero to be sent the marks, 0 to stop. Nothing is answered.
func (h *QuestMarkersHandler) HandleQuestMarkers(sess *session.Session, msg *protocol.Message) error {
	data := msg.Data()
	if len(data) < 4 || h.world == nil {
		log.Printf("QuestMarkers: short (%d bytes) from %s", len(data), sess.RemoteAddr())
		return nil
	}
	on := binary.LittleEndian.Uint32(data[:4]) != 0
	p := h.world.SessionPlayer(sess)
	q := Quests()
	if p == nil || q == nil {
		log.Printf("QuestMarkers: %s has no player in the world, or no quests run", sess.RemoteAddr())
		return nil
	}
	q.SetMarkers(sess, p, on)
	log.Printf("QuestMarkers: %s %s", p.Name, map[bool]string{true: "draws them", false: "stopped"}[on])
	return nil
}

// SetMarkers makes the character's client have the marks (on) or stop (off);
// on, its set goes out at once whatever was sent before.
func (q *QuestEngine) SetMarkers(sess *session.Session, p *entity.Player, on bool) {
	if q == nil || p == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	c := q.namedLocked(p.CharServerID, p.Name)
	c.marksOn, c.marksSent = on, ""
	if on {
		q.sendMarksLocked(sess, p, c)
	}
}

// syncMarks sends each client that asked its set when it changed. Every
// second, off the engine's own goroutine.
func (q *QuestEngine) syncMarks(now time.Time) {
	if q.sessionOf == nil || q.playerOf == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, c := range q.chars {
		if !c.marksOn {
			continue
		}
		sess := q.sessionOf(id)
		if sess == nil {
			continue
		}
		q.sendMarksLocked(sess, q.playerOf(sess), c)
	}
}

// sendMarksLocked sends the character's set when it differs from the last
// one sent. Must be called with q.mu held.
func (q *QuestEngine) sendMarksLocked(sess *session.Session, p *entity.Player, c *questChar) {
	if sess == nil || p == nil {
		return
	}
	marks, ok := q.marksLocked(q.lookAt(sess), p, c, time.Now())
	if !ok {
		return
	}
	body := encodeQuestMarks(marks)
	if string(body) == c.marksSent {
		return
	}
	c.marksSent = string(body)
	queueReliable(sess, protocol.OpcodeQuestMarkersData, append(binary.LittleEndian.AppendUint16(nil, uint16(protocol.OpcodeQuestMarkersData)), body...))
}

// markSpotsLocked is where the quests' NPCs and creatures stand, by folded
// name, read from the World at most every markSpotsEvery (and after a Reload).
// Must be called with q.mu held.
func (q *QuestEngine) markSpotsLocked(now time.Time) map[string][]entity.NPCSpot {
	if q.gw == nil {
		return nil
	}
	if q.spots != nil && now.Sub(q.spotsAt) < markSpotsEvery {
		return q.spots
	}
	names := map[string]bool{}
	for _, qu := range q.cat.all {
		if !qu.Enabled {
			continue
		}
		names[foldName(qu.Giver)] = true
		for i := range qu.Steps {
			st := &qu.Steps[i]
			if database.AtNPC(st.Kind) || database.Hunts(st.Kind) {
				for _, t := range st.Targets() {
					names[foldName(t)] = true
				}
			}
		}
	}
	q.spots, q.spotsAt = q.gw.NPCSpotsNamed(names), now
	return q.spots
}

// marksLocked is the player's marks, the steps' first then the quests to take
// (its standings asked of look), each in the player's world first and nearest
// first; false when the player is not in the world (dead, loading). Must be
// called with q.mu held.
func (q *QuestEngine) marksLocked(look *factionLook, p *entity.Player, c *questChar, now time.Time) ([]questMark, bool) {
	var world uint8
	var x, z float32
	if q.gw != nil {
		w, px, _, pz, ok := q.gw.PlayerPlace(p)
		if !ok {
			return nil, false
		}
		world, x, z = w, px, pz
	} else {
		world, x, z = p.World, p.X, p.Z
	}
	spots := q.markSpotsLocked(now)
	dist := func(s entity.NPCSpot) float32 {
		if s.World != world {
			return -1
		}
		return float32(math.Hypot(float64(s.X-x), float64(s.Z-z)))
	}
	// the nearest maxMarkSpots of the spawns of a step's names (of its first,
	// only the one a step pins)
	near := func(names []string, pinned int) []entity.NPCSpot {
		var out []entity.NPCSpot
		for i, name := range names {
			for _, s := range spots[foldName(name)] {
				if i > 0 || pinned == 0 || int(s.ID) == pinned {
					out = append(out, s)
				}
			}
		}
		sort.SliceStable(out, func(i, j int) bool {
			di, dj := dist(out[i]), dist(out[j])
			if (di < 0) != (dj < 0) {
				return di >= 0
			}
			return di < dj
		})
		if len(out) > maxMarkSpots {
			out = out[:maxMarkSpots]
		}
		return out
	}
	byKey := map[string]*questMark{}
	var marks []*questMark
	add := func(kind byte, names []string, pinned int, mq markQuest) {
		for _, s := range near(names, pinned) {
			k := fmt.Sprintf("%d %d %.0f %.0f", kind, s.World, s.X, s.Z)
			m := byKey[k]
			if m == nil {
				m = &questMark{kind: kind, world: s.World, x: s.X, z: s.Z, npc: strings.TrimSpace(s.Name), dist: dist(s)}
				byKey[k] = m
				marks = append(marks, m)
			}
			if len(m.quests) < maxMarkQuests {
				m.quests = append(m.quests, mq)
			}
		}
	}
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		st := qu.Step(cq.Step)
		if st == nil {
			continue
		}
		mq := markQuest{name: qu.Name, min: qu.MinLevel, max: qu.MaxLevel, line: st.Journal}
		switch {
		case database.AtNPC(st.Kind):
			mq.detail = q.texts[qu.ID].needs[st.Step]
			add(markStep, []string{st.Target}, st.NPCID, mq)
		case st.Kind == database.StepKill:
			mq.detail = fmt.Sprintf("%s: %d of %d killed", st.TargetWords(), cq.Count, max(st.Count, 1))
			add(markHunt, st.Targets(), st.NPCID, mq)
		case st.Kind == database.StepLoot:
			_, need, _ := st.LootItem()
			mq.detail = fmt.Sprintf("%s: %d of %d", q.texts[qu.ID].needs[st.Step], cq.Count, need)
			add(markHunt, st.Targets(), st.NPCID, mq)
		}
	}
	givers := make([]string, 0, len(q.cat.givers))
	for g := range q.cat.givers {
		givers = append(givers, g)
	}
	sort.Strings(givers)
	for _, g := range givers {
		// in the order the giver offers them (nextOfferLocked): the ones never
		// finished first, a finished repeatable one after
		var takes []*database.Quest
		for _, qu := range q.cat.givers[g] {
			if q.mayTakeLocked(look, p, c, qu) == "" {
				takes = append(takes, qu)
			}
		}
		sort.SliceStable(takes, func(i, j int) bool { return c.timesDone(takes[i].ID) < c.timesDone(takes[j].ID) })
		for _, qu := range takes {
			t := q.texts[qu.ID]
			add(markTake, []string{qu.Giver}, 0, markQuest{name: qu.Name, min: qu.MinLevel, max: qu.MaxLevel, line: t.offer, detail: t.rewardsFor(p.Class)})
		}
	}
	sort.SliceStable(marks, func(i, j int) bool {
		a, b := marks[i], marks[j]
		if (a.kind == markTake) != (b.kind == markTake) {
			return a.kind != markTake
		}
		if (a.dist < 0) != (b.dist < 0) {
			return a.dist >= 0
		}
		return a.dist < b.dist
	})
	if len(marks) > maxMarks {
		marks = marks[:maxMarks]
	}
	out := make([]questMark, len(marks))
	for i, m := range marks {
		out[i] = *m
	}
	return out, true
}

// clip is s within n bytes, cut at a rune and marked.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimSpace(s[:cut]) + "..."
}

// putText writes a u8 byte count and the text, clipped to maxMarkText.
func putText(b []byte, s string) []byte {
	s = clip(s, maxMarkText)
	return append(append(b, byte(len(s))), s...)
}

// encodeQuestMarks is OpcodeQuestMarkersData's body: a u8 count, then each
// mark's u8 kind, u8 world, f32 x and z, the NPC's name and a u8 count of
// quests, each its name, u8 lowest and highest levels, its line and its
// detail (a text: a u8 byte count, then UTF-8). Marks past maxMarksBytes are
// left out.
func encodeQuestMarks(marks []questMark) []byte {
	out := []byte{0}
	n := 0
	for _, m := range marks {
		var b []byte
		b = append(b, m.kind, m.world)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(m.x))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(m.z))
		b = putText(b, m.npc)
		b = append(b, byte(len(m.quests)))
		for _, mq := range m.quests {
			b = putText(b, mq.name)
			b = append(b, byte(max(0, min(255, mq.min))), byte(max(0, min(255, mq.max))))
			b = putText(b, mq.line)
			b = putText(b, mq.detail)
		}
		if len(out)+len(b) > maxMarksBytes || n == 255 {
			break
		}
		out = append(out, b...)
		n++
	}
	out[0] = byte(n)
	return out
}

// questTexts makes the quests' words for the marks: the start of each offer
// (its first line, the giver's name left off), its rewards in all, and each
// step's needs, naming items and spells from the database (when the quests
// are read: never per tick); and the names of every item a step wants, by
// patternid, for the trackers.
func (q *QuestEngine) questTexts(all []*database.Quest) (map[int]*questText, map[int]string) {
	itemName := map[int]string{}
	nameOf := func(id int) string {
		if n, ok := itemName[id]; ok {
			return n
		}
		n := fmt.Sprintf("item %d", id)
		if q.items != nil {
			if pat, err := q.items.GetItemPattern(id); err == nil && pat != nil && strings.TrimSpace(pat.ItemName) != "" {
				n = strings.TrimSpace(pat.ItemName)
			}
		}
		itemName[id] = n
		return n
	}
	spellOf := func(id int) string {
		if q.spells != nil {
			if sp, err := q.spells.SpellPattern(id); err == nil && sp != nil && strings.TrimSpace(sp.SpellName) != "" {
				return strings.TrimSpace(sp.SpellName)
			}
		}
		return fmt.Sprintf("spell %d", id)
	}
	items := func(list []database.ItemCount) []string {
		var out []string
		for _, it := range list {
			out = append(out, countOf(max(it.Count, 1), nameOf(it.PatternID)))
		}
		return out
	}
	out := map[int]*questText{}
	for _, qu := range all {
		t := &questText{needs: map[int]string{}}
		if lines := sayLines(qu.Offer); len(lines) > 0 {
			line := lines[0]
			if i := strings.Index(line, ": "); i > 0 && i < 40 {
				line = line[i+2:] // "Captain Norgam: ..." the giver's own words
			}
			t.offer = line
		}
		// the rewards of every step for a class (-1: none, the steps' own)
		// one choice's option's rewards in a few words ("Ash Staff, 50 XP")
		optionOf := func(rw database.ClassReward) string {
			got := items(rw.Items)
			for _, id := range rw.Spells {
				got = append(got, spellOf(id))
			}
			if rw.XP > 0 {
				got = append(got, fmt.Sprintf("%d XP", rw.XP))
			}
			if rw.Tunar > 0 {
				got = append(got, fmt.Sprintf("%d tunar", rw.Tunar))
			}
			return strings.Join(got, ", ")
		}
		rewardsOf := func(class int) string {
			xp, tunar := 0, 0
			var got []string
			for i := range qu.Steps {
				rw := qu.Steps[i].RewardFor(class)
				xp += rw.XP
				tunar += rw.Tunar
				got = append(got, items(rw.Items)...)
				for _, id := range rw.Spells {
					got = append(got, spellOf(id))
				}
				// a step's choice: one of its options' rewards
				var picks []string
				for _, o := range qu.Steps[i].Choices.Options {
					if w := optionOf(o.Reward); w != "" && !o.Stay {
						picks = append(picks, w)
					}
				}
				if len(picks) > 0 {
					got = append(got, "a choice of "+strings.Join(picks, " or "))
				}
			}
			var reward []string
			if xp > 0 {
				reward = append(reward, fmt.Sprintf("%d XP", xp))
			}
			if tunar > 0 {
				reward = append(reward, fmt.Sprintf("%d tunar", tunar))
			}
			return strings.Join(append(reward, got...), ", ")
		}
		t.rewards = rewardsOf(-1)
		for _, st := range qu.Steps {
			for class := range st.ByClass {
				if _, done := t.byClass[class]; !done {
					if t.byClass == nil {
						t.byClass = map[int]string{}
					}
					t.byClass[class] = rewardsOf(class)
				}
			}
		}
		for _, st := range qu.Steps {
			var need []string
			if st.Kind == database.StepLoot {
				if id, _, ok := st.LootItem(); ok {
					need = append(need, nameOf(id))
				}
			} else {
				need = append(need, items(st.Items)...)
			}
			if st.Tunar > 0 {
				need = append(need, fmt.Sprintf("%d tunar", st.Tunar))
			}
			if len(need) > 0 {
				t.needs[st.Step] = strings.Join(need, ", ")
			}
		}
		out[qu.ID] = t
	}
	return out, itemName
}
