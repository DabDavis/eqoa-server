package handlers

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync/atomic"
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
//     levels, the start of the giver's offer and its rewards. A quest's name
//     is marked once, at its nearest giver (one quest is given in each city:
//     Armorcrafting by twelve smiths), and not at all while a quest of that
//     name is under way or finished (one finished that may be done again
//     aside); and only while its lowest level is near the player's
//     (SetQuestMarksBelow: a level 60 may take some 150 quests, three in
//     five of them for below level 50);
//   - the NPC of a step the player has to do there (talk, show, give): kind
//     2, "?", with the step's journal line and what it needs; a giver it is
//     at shows no "!" besides;
//   - a reach step's place (petition 1404: Totem of the Plains' cave in
//     Snowfist had no mark): kind 2, "?", at the step's spot, its journal line
//     for its name (it has no NPC; the line says where), as vanilla's own
//     guide marks one;
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

// markBelowDefault is how far below the player's level a quest's lowest level
// may be for its "!" while world.quest_marks_below is unset.
const markBelowDefault = 10

// questMarksBelow is world.quest_marks_below (SetQuestMarksBelow).
var questMarksBelow atomic.Int64

// SetQuestMarksBelow is world.quest_marks_below: how many levels below the
// player's a quest's lowest level may be for its giver's "!" (a quest with
// none is a level 1 quest). Unset or 0, markBelowDefault; below 0, any: every
// quest the player may take is marked, however far below its level.
func SetQuestMarksBelow(n int) { questMarksBelow.Store(int64(n)) }

// QuestMarksBelow is the levels SetQuestMarksBelow set (markBelowDefault for
// unset), or -1 for any.
func QuestMarksBelow() int {
	switch n := int(questMarksBelow.Load()); {
	case n < 0:
		return -1
	case n == 0:
		return markBelowDefault
	default:
		return n
	}
}

// markLevelNear is whether a quest of lowest level min is near enough a
// player of level level for its "!" (SetQuestMarksBelow).
func markLevelNear(level, min int) bool {
	n := QuestMarksBelow()
	return n < 0 || max(min, 1) >= level-n
}

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
	// ability: the quest gives the player's class an ability (abilityFor),
	// flagged after the set for a client to mark it apart (petition 2285)
	ability bool
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
	// ability is whether it gives every class an ability (a spell, by a
	// step's rewards or one of a choice's options), abilityBy a class's
	// where its steps reward that class besides (abilityFor)
	ability   bool
	abilityBy map[int]bool
}

// abilityFor is whether the quest gives a player of a class an ability: a
// spell among its steps' rewards for the class or among a step's choice's
// options (petition 2285: "highlights the ability quest
// differently compared to other quest ... most people only do the ability
// quest anyway").
func (t *questText) abilityFor(class int) bool {
	if t == nil {
		return false
	}
	if a, ok := t.abilityBy[class]; ok {
		return a
	}
	return t.ability
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
	if !QoLOnFor(sess) { // world.vanilla_ui, an [OG] character (vanillaui.go, ogaccount.go)
		return
	}
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
	on := qolAnswerFor(sess, binary.LittleEndian.Uint32(data[:4]) != 0)
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
	defer q.beginSyncLocked()()
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
	end := q.beginPassLocked(p)
	marks, ok := q.marksLocked(q.lookAt(sess), p, c, time.Now())
	end()
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
			if h := q.dropStep(st); h != nil {
				for _, t := range h.Targets() {
					names[foldName(t)] = true
				}
			}
		}
	}
	// the travel NPCs and where their trips land, for a step in another world
	for n, rs := range wayRoutes {
		names[foldName(n)] = true
		for _, r := range rs {
			if r.Arrive != "" {
				names[foldName(r.Arrive)] = true
			}
		}
	}
	q.spots, q.spotsAt = q.gw.NPCSpotsNamed(names), now
	q.camps, q.hops = nil, nil
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
	byKey := map[markKey]*questMark{}
	var marks []*questMark
	// the spots a step's mark is at: a giver there shows its "?" alone
	stepAt := map[markSpot]bool{}
	addAt := func(kind byte, s entity.NPCSpot, mq markQuest) {
		spot := markSpotOf(s)
		if kind == markTake && stepAt[spot] {
			return
		}
		if kind == markStep {
			stepAt[spot] = true
		}
		k := markKey{kind, spot}
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
	// a giver's: the nearest maxMarkSpots of its spawns that stand in the
	// world (its spawn-only originals only where none does); each giver's
	// worked out once a pass (the same spots, the same place: the same answer)
	giverAt := map[string][]entity.NPCSpot{}
	addGiver := func(name string, mq markQuest) {
		if out, ok := giverAt[name]; ok {
			for _, s := range out {
				addAt(markTake, s, mq)
			}
			return
		}
		var out, orig []entity.NPCSpot
		for _, s := range spots[foldName(name)] {
			if s.SpawnOnly {
				orig = append(orig, s)
			} else {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			out = orig
		}
		nearestFirst(out, world, x, z)
		if len(out) > maxMarkSpots {
			out = out[:maxMarkSpots]
		}
		giverAt[name] = out
		for _, s := range out {
			addAt(markTake, s, mq)
		}
	}
	var allow func(wayHop) bool // the way marks' trips the player may take, made at the first
	for _, cq := range c.active() {
		qu := q.cat.byID[cq.QuestID]
		if qu == nil {
			continue
		}
		// its step, or each of its group's not yet done (questanyorder.go)
		for _, st := range openSteps(qu, cq) {
			mq := markQuest{name: qu.Name, min: qu.MinLevel, max: qu.MaxLevel, line: st.Journal, ability: q.texts[qu.ID].abilityFor(p.Class)}
			switch {
			case database.AtNPC(st.Kind):
				mq.detail = q.texts[qu.ID].needs[st.Step]
			case st.Kind == database.StepKill:
				mq.detail = fmt.Sprintf("%s: %d of %d killed", st.TargetWords(), cq.Count, max(st.Count, 1))
			case st.Kind == database.StepLoot:
				_, need, _ := st.LootItem()
				mq.detail = fmt.Sprintf("%s: %d of %d", q.texts[qu.ID].needs[st.Step], cq.Count, need)
			case st.Kind == database.StepReach:
				// the place's name is the journal line, so the quest's own line
				// would only say it again
				mq.line = ""
			}
			kind, at := q.stepSpotsLocked(spots, st, c.serverID, world, x, z)
			for _, s := range at {
				addAt(kind, s, mq)
			}
			// none in the player's world, or in another of its homes (the
			// Homelands: coaches between them): the way there
			// (questmarkspots.go, questcoachway.go)
			if here := wayNodeAt(world, x, z); len(at) > 0 && wayNodeOf(at[0]) != here {
				if allow == nil {
					allow = q.wayAllowLocked(c.serverID)
				}
				if h, ok := q.wayToLocked(spots, here, wayNodeOf(at[0]), x, z, allow); ok {
					wq := mq
					wq.line = st.Journal
					wq.detail = wayLine(h, "")
					addAt(markStep, h.from, wq)
				}
			}
			// a give step's item that creatures drop, while the player lacks it:
			// their camp too, as a hunt (questmarksdrops.go; petition 3221)
			if h := q.dropStep(st); h != nil && q.lackingLocked(p, st) != "" {
				hq := mq
				hq.detail = fmt.Sprintf("%s drops it", h.TargetWords())
				hk, hat := q.stepSpotsLocked(spots, h, c.serverID, world, x, z)
				for _, s := range hat {
					addAt(hk, s, hq)
				}
			}
		}
	}
	// the quests to take, a name once: the names under way or finished (one
	// that may be done again aside) marked by none, and each other name by the
	// nearest of its givers (one in the player's world before one in another,
	// before one standing nowhere)
	taken := map[string]bool{}
	for _, cq := range c.quests {
		if qu := q.cat.byID[cq.QuestID]; qu != nil && (cq.Active() || cq.Done > 0 && !qu.Repeatable) {
			taken[q.cat.foldedName(qu)] = true
		}
	}
	giverDist := func(giver string) float32 {
		best := float32(math.MaxFloat32)
		for _, s := range spots[giver] {
			if d := dist(s); d >= 0 && d < best {
				best = d
			} else if d < 0 && best == math.MaxFloat32 {
				best = math.MaxFloat32 / 2
			}
		}
		return best
	}
	givers := q.cat.giverOrder // sorted (Reload)
	if len(givers) != len(q.cat.givers) {
		givers = make([]string, 0, len(q.cat.givers))
		for g := range q.cat.givers {
			givers = append(givers, g)
		}
		sort.Strings(givers)
	}
	type takeAt struct {
		qu   *database.Quest
		dist float32
	}
	// the quests still to be asked, the rest refused by what rarely changes,
	// kept from the pass before (questtakecache.go; nil: every one)
	cands := q.takeCandsLocked(p, c, now)
	giverQuests := func(g string) []*database.Quest {
		if cands != nil {
			return cands[g]
		}
		return q.cat.givers[g]
	}
	byName := map[string]takeAt{}
	for _, g := range givers {
		d := float32(-1)
		for _, qu := range giverQuests(g) {
			k := q.cat.foldedName(qu)
			if taken[k] || !markLevelNear(p.Level, qu.MinLevel) {
				continue
			}
			if d < 0 {
				d = giverDist(g) // qu.Giver folded is g (Reload)
			}
			if b, seen := byName[k]; seen && b.dist <= d {
				continue
			}
			if q.mayTakeLocked(look, p, c, qu) == "" {
				byName[k] = takeAt{qu, d}
			}
		}
	}
	chosen := map[int]bool{}
	for _, b := range byName {
		chosen[b.qu.ID] = true
	}
	for _, g := range givers {
		// in the order the giver offers them (nextOfferLocked): the ones never
		// finished first, a finished repeatable one after
		var takes []*database.Quest
		for _, qu := range giverQuests(g) { // the chosen are among them
			if chosen[qu.ID] {
				takes = append(takes, qu)
			}
		}
		sort.SliceStable(takes, func(i, j int) bool { return c.timesDone(takes[i].ID) < c.timesDone(takes[j].ID) })
		for _, qu := range takes {
			t := q.texts[qu.ID]
			addGiver(qu.Giver, markQuest{name: qu.Name, min: qu.MinLevel, max: qu.MaxLevel, line: t.offer, detail: t.rewardsFor(p.Class), ability: t.abilityFor(p.Class)})
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

// markSpot is a mark's place as the marks tell two apart: its world and its
// x and z each rounded to a whole number, as fmt's "%.0f" rounds them (half to
// even; -0 apart from 0). Until 2026-11-26 the key was that text, made with
// fmt.Sprintf twice for every mark of every pass (the owner 2026-11-26, the
// marks' CPU: questtakecache.go); the same places are the same key.
type markSpot struct {
	world uint8
	x, z  uint64
}

// markKey is a mark's kind at a place.
type markKey struct {
	kind byte
	spot markSpot
}

// markSpotOf is a spot's markSpot.
func markSpotOf(s entity.NPCSpot) markSpot {
	return markSpot{s.World, markCoord(s.X), markCoord(s.Z)}
}

// markCoord is a coordinate rounded as "%.0f" rounds it, as bits: every NaN
// one ("NaN"), each infinity its own.
func markCoord(f float32) uint64 {
	v := math.RoundToEven(float64(f))
	if math.IsNaN(v) {
		return math.Float64bits(math.NaN())
	}
	return math.Float64bits(v)
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

// Quest flags, one u8 a quest after a set of marks or a tracker (petition
// 2285): a client that reads them marks an ability quest apart; one that
// does not stops reading at the set's end, as vanilla before them did.
const questFlagAbility byte = 1

// questFlags is a quest's flags byte.
func questFlags(ability bool) byte {
	if ability {
		return questFlagAbility
	}
	return 0
}

// encodeQuestMarks is OpcodeQuestMarkersData's body: a u8 count, then each
// mark's u8 kind, u8 world, f32 x and z, the NPC's name and a u8 count of
// quests, each its name, u8 lowest and highest levels, its line and its
// detail (a text: a u8 byte count, then UTF-8); then, after the whole set,
// each of its marks' quests' u8 flags in the same order (questFlagAbility).
// Marks past maxMarksBytes (the flags in it) are left out.
func encodeQuestMarks(marks []questMark) []byte {
	out := []byte{0}
	n, flags := 0, 0
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
		if len(out)+len(b)+flags+len(m.quests) > maxMarksBytes || n == 255 {
			break
		}
		out = append(out, b...)
		n++
		flags += len(m.quests)
	}
	out[0] = byte(n)
	for _, m := range marks[:n] {
		for _, mq := range m.quests {
			out = append(out, questFlags(mq.ability))
		}
	}
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
			if xp := questXPShare(qu, rw.XP); xp > 0 {
				got = append(got, fmt.Sprintf("%d XP", xp))
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
				xp += questXPShare(qu, rw.XP)
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
		// an ability for a class (-1: the steps' own): a spell a step gives
		// it, or one a step's choice offers
		abilityOf := func(class int) bool {
			for i := range qu.Steps {
				if len(qu.Steps[i].RewardFor(class).Spells) > 0 {
					return true
				}
				for _, o := range qu.Steps[i].Choices.Options {
					if len(o.Reward.Spells) > 0 && !o.Stay {
						return true
					}
				}
			}
			return false
		}
		t.rewards = rewardsOf(-1)
		t.ability = abilityOf(-1)
		for _, st := range qu.Steps {
			for class := range st.ByClass {
				if _, done := t.byClass[class]; !done {
					if t.byClass == nil {
						t.byClass = map[int]string{}
						t.abilityBy = map[int]bool{}
					}
					t.byClass[class] = rewardsOf(class)
					t.abilityBy[class] = abilityOf(class)
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
