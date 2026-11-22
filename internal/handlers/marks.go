package handlers

import (
	"fmt"
	"log"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// marks.go — Heirloom Marks (work board 1927, a GM's spec, every number his; the Heirloom Test Server
// alone, world.heirloom_marks: SetHeirloomMarks). The goal: players below 60 go out and test low-level
// quests and camps, and earn things that matter at 60. database/marks.go has the tables and the quest
// formula, entity/marks.go who earns and Relive's World side.
//
//   - Who earns: a character below 60, or a 60 while it relives (relive.go), never a GM's
//     (entity.MarksEarner). Hardcore characters earn as everyone. Marks are the character's own.
//   - A quest (QuestDone, from the quest engine at a quest's last step): its first finish, not
//     repeatable, at a level (a Relive's) no more than its minlevel + 5, pays quests.marks or the
//     computed figure (database.QuestMarksFor), at most twice the real minutes since it was taken
//     (charquests.taken) in marks.
//   - A kill (entity's OnKillMarks): each sharer given experience rolls once for a mark at 20% x
//     the kill's weight (at most 1.5: 30%); KillMarksAnHour (38) kill marks an hour, past which a
//     kill does not roll. Only a creature's kill rolls: no player's death gives experience, so no
//     PvP kill, duel or arena death ever gives a mark.
//   - Bonuses: x(1 + 0.10 x the other earners of the group within 200 in its world), at most
//     x1.3, times x(1 + bonus/100) of the best enabled Hot zone that fits (a camp zone the creature's
//     spawn, a quest zone the quest's import key or id); the fraction carried in charmarks.frac.
//   - /marks (marksCommand), the Hot zones (hotzones.go), the Heirloom Keeper (keeper.go) and Relive
//     (relive.go).
//
// The marks are written off the world's lock, by one worker in order (events), each award one
// transaction with its log line.

// The figures (work 1927 sections 4 and 5).
const (
	killMarkChance    = 0.20 // a kill's chance at weight 1
	marksGroupStep    = 0.10 // a member's share of the group bonus
	marksGroupMost    = 3    // members counted: x1.3 at most
	marksLevelOver    = 5    // a quest pays to its minlevel + 5
	marksZonesTTL     = 30 * time.Second
	marksKeysTTL      = 10 * time.Minute
	marksEventsQueued = 4096
)

// The lines (ours).
const (
	marksEarnLine    = "You earn %d Heirloom Marks."
	marksEarnOne     = "You earn 1 Heirloom Mark."
	marksAt60Line    = "You earn marks below level 60, or while you relive at a Rehabilitator."
	marksOffLine     = "Heirloom Marks are not kept on this server."
	marksNoZonesPart = "Hot zones: none."
)

// heirloomMarks is the running marks, for the quest engine (QuestDone); nil none.
var heirloomMarks atomic.Pointer[HeirloomMarks]

// SetHeirloomMarksService makes m the running marks.
func SetHeirloomMarksService(m *HeirloomMarks) { heirloomMarks.Store(m) }

// HeirloomMarks is the marks: their awards, the Hot zones, the Keeper and Relive.
type HeirloomMarks struct {
	db     *database.SQLiteDB
	gw     *entity.World
	heir   *Heirlooms
	inv    *database.InventoryStore
	items  *database.ItemPatternStore
	spells *database.SpellbookStore
	now    func() time.Time
	roll   func() float64
	// dump sends a MemoryDump where the player stands (a Relive's start and end: relive.go);
	// refresh puts the worn gear's figures together again (a rank bought); travel sends a player on
	// a trip (the Hot Zone Herald's)
	dump    func(sess *session.Session)
	refresh func(p *entity.Player)
	travel  func(sess *session.Session, p *entity.Player, dest entity.CoachDest)

	mu      sync.Mutex
	zones   []database.HotZone
	zonesAt time.Time
	zonesOn map[int]bool // the zones on at the last read: one turned on since, of announce 1, is announced
	keys    map[int]string
	keysAt  time.Time
	names   map[int]string // spell names by id (the guises')
	byName  map[string]int // records by name (the ring, the tome)
	boxes   map[uint32]marksBox
	seq     uint32
	ringAt  map[uint32]time.Time // a ring's last use, by character

	events chan func()
	inline bool // tests: every event at once, in the caller
}

// marksBox is a question asked of a character (keeper.go, hotzones.go, relive.go).
type marksBox struct {
	id      uint32
	kind    int
	npc     entity.EntityID
	speaker string
	opts    []int
	zones   []database.HotZone
}

// NewHeirloomMarks is the marks over db; its worker starts with Start.
func NewHeirloomMarks(db *database.SQLiteDB, gw *entity.World, heir *Heirlooms, inv *database.InventoryStore,
	spells *database.SpellbookStore) *HeirloomMarks {
	m := &HeirloomMarks{db: db, gw: gw, heir: heir, inv: inv, spells: spells, now: time.Now, roll: rand.Float64,
		boxes: map[uint32]marksBox{}, ringAt: map[uint32]time.Time{}, events: make(chan func(), marksEventsQueued)}
	if db != nil {
		m.items = database.NewItemPatternStore(db)
	}
	return m
}

// Start runs the worker and hears the World's kills and Relives.
func (m *HeirloomMarks) Start() {
	if m == nil || m.gw == nil {
		return
	}
	go func() {
		for fn := range m.events {
			fn()
		}
	}()
	// the Hot zones read every marksZonesTTL whoever earns, so a panel edit turning one on is
	// announced (hotZones)
	go func() {
		for range time.Tick(marksZonesTTL) {
			if HeirloomMarksOn() {
				m.queue(func() { m.hotZones() })
			}
		}
	}()
	m.gw.OnKillMarks(m.killHeard)
	m.gw.OnReliveSave(m.saveRelive)
}

// queue runs fn on the worker (at once in a test).
func (m *HeirloomMarks) queue(fn func()) {
	if m.inline {
		fn()
		return
	}
	select {
	case m.events <- fn:
	default:
		log.Printf("Marks: the queue is full: an award is dropped")
	}
}

// --- kills ---

// killHeard is the World's kill (its lock held): rolled and written on the worker.
func (m *HeirloomMarks) killHeard(k entity.KillMarks) {
	if !HeirloomMarksOn() {
		return
	}
	m.queue(func() { m.killRoll(k) })
}

// killRoll is a kill's roll: under the hour's cap, at 20% x its weight, a mark x the bonuses.
func (m *HeirloomMarks) killRoll(k entity.KillMarks) {
	now := m.now()
	row, err := m.db.CharMarksOf(k.ServerID)
	if err != nil {
		log.Printf("Marks: %s's row: %v", k.Name, err)
		return
	}
	if row.KillHourNow(now) >= database.KillMarksAnHour {
		return // past the cap a kill does not roll
	}
	if m.roll() >= killMarkChance*k.Weight {
		return
	}
	group := groupFactor(k.Group)
	hot, zone := m.bestZone(func(z database.HotZone) bool { return z.FitsSpawn(int(k.World), k.SpawnX, k.SpawnZ) })
	ref := fmt.Sprintf("npc %d %s", k.NPCRow, k.NPCName)
	n, after, ok, err := m.db.AwardMarks(k.ServerID, database.MarksAward{Source: "kill", Base: 1, Bonus: group * hot, Ref: ref}, now)
	if err != nil || !ok {
		if err != nil {
			log.Printf("Marks: %s's kill mark: %v", k.Name, err)
		}
		return
	}
	log.Printf("Marks: %s +%d source=kill ref=%d group=%.2f hot=%.2f hour=%d/%d%s", k.Name, n, k.NPCRow, group, hot,
		after.KillHourNow(now), database.KillMarksAnHour, zoneNote(zone))
	tellEarned(k.Sess, n, group, hot)
}

// groupFactor is the group bonus for n other earners near.
func groupFactor(n int) float64 {
	return 1 + marksGroupStep*float64(min(max(n, 0), marksGroupMost))
}

func zoneNote(z *database.HotZone) string {
	if z == nil {
		return ""
	}
	return fmt.Sprintf(" zone=%d", z.ID)
}

// tellEarned is an award's line: its marks and the bonuses that counted.
func tellEarned(sess *session.Session, n int, group, hot float64) {
	if sess == nil || n <= 0 {
		return
	}
	line := fmt.Sprintf(marksEarnLine, n)
	if n == 1 {
		line = marksEarnOne
	}
	var parts []string
	if g := pctOver(group); g > 0 {
		parts = append(parts, fmt.Sprintf("group +%d%%", g))
	}
	if h := pctOver(hot); h > 0 {
		parts = append(parts, fmt.Sprintf("Hot zone +%d%%", h))
	}
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}
	SendClientMessage(sess, line)
}

func pctOver(f float64) int { return int((f-1)*100 + 0.5) }

// --- quests ---

// QuestDone is a quest finished by p (its last step, the quest engine's), first is whether it is
// its first finish. The award is computed here and written on the worker.
func (m *HeirloomMarks) QuestDone(sess *session.Session, p *entity.Player, qu *database.Quest, first bool) {
	if m == nil || !HeirloomMarksOn() || p == nil || qu == nil || !first || qu.Repeatable || !entity.MarksEarner(p) {
		return
	}
	if p.Level > qu.MinLevel+marksLevelOver {
		return
	}
	key := m.importKey(qu.ID)
	base := database.QuestMarksFor(qu, key)
	if base <= 0 {
		return
	}
	now := m.now()
	if taken, ok := m.db.QuestTaken(p.CharServerID, qu.ID); ok {
		base = min(base, database.QuestMarksLimit(now.Sub(taken).Minutes()))
		if base <= 0 {
			return
		}
	}
	group := groupFactor(m.gw.MarksGroup(p))
	hot, zone := m.bestZone(func(z database.HotZone) bool { return z.FitsQuest(qu.ID, key) })
	id, name := p.CharServerID, p.Name
	m.queue(func() {
		n, _, _, err := m.db.AwardMarks(id, database.MarksAward{Source: "quest", Base: float64(base), Bonus: group * hot,
			Ref: strconv.Itoa(qu.ID)}, now)
		if err != nil {
			log.Printf("Marks: %s's quest %d: %v", name, qu.ID, err)
			return
		}
		row, _ := m.db.CharMarksOf(id)
		log.Printf("Marks: %s +%d source=quest ref=%d group=%.2f hot=%.2f hour=%d/%d%s", name, n, qu.ID, group, hot,
			row.KillHourNow(now), database.KillMarksAnHour, zoneNote(zone))
		tellEarned(sess, n, group, hot)
	})
}

// importKey is a quest's import key ("" none), the keys read again once marksKeysTTL old.
func (m *HeirloomMarks) importKey(id int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys == nil || m.now().Sub(m.keysAt) > marksKeysTTL {
		k, err := m.db.QuestImportKeys()
		if err != nil {
			log.Printf("Marks: the quests' import keys: %v", err)
		}
		m.keys, m.keysAt = k, m.now()
	}
	return m.keys[id]
}

// --- the Hot zones' bonus ---

// hotZones is the table, at most marksZonesTTL old (a GM's change through #hotzone or the panel's
// row edit forgets it at once: forgetZones).
func (m *HeirloomMarks) hotZones() []database.HotZone {
	m.mu.Lock()
	if m.zones != nil && m.now().Sub(m.zonesAt) <= marksZonesTTL {
		defer m.mu.Unlock()
		return m.zones
	}
	z, err := m.db.HotZones()
	if err != nil {
		log.Printf("Marks: the Hot zones: %v", err)
	}
	if z == nil {
		z = []database.HotZone{}
	}
	// a zone of announce 1 turned on since the last read (a GM's #hotzone or the panel's edit) is
	// announced once; the first read only takes note
	var up []database.HotZone
	on := map[int]bool{}
	for _, zz := range z {
		if zz.Enabled {
			on[zz.ID] = true
			if m.zonesOn != nil && !m.zonesOn[zz.ID] && zz.Announce {
				up = append(up, zz)
			}
		}
	}
	m.zones, m.zonesAt, m.zonesOn = z, m.now(), on
	m.mu.Unlock()
	for _, zz := range up {
		m.announce(zz)
	}
	return z
}

// forgetZones has the next look read the table.
func (m *HeirloomMarks) forgetZones() {
	m.mu.Lock()
	m.zones = nil
	m.mu.Unlock()
}

// bestZone is the factor of the best enabled zone that fits (1 and nil: none).
func (m *HeirloomMarks) bestZone(fits func(database.HotZone) bool) (float64, *database.HotZone) {
	best, bonus := (*database.HotZone)(nil), 0
	for _, z := range m.hotZones() {
		if !z.Enabled || z.BonusPct <= 0 || !fits(z) {
			continue
		}
		if best == nil || z.BonusPct > bonus {
			zz := z
			best, bonus = &zz, z.BonusPct
		}
	}
	return 1 + float64(bonus)/100, best
}

// enabledZones is the zones on, by id.
func (m *HeirloomMarks) enabledZones() []database.HotZone {
	var out []database.HotZone
	for _, z := range m.hotZones() {
		if z.Enabled {
			out = append(out, z)
		}
	}
	return out
}

// zonesPart is /marks's end: "Hot zones: Hexbones (+10%), Erudite homeland quests (+10%)."
func (m *HeirloomMarks) zonesPart() string {
	on := m.enabledZones()
	if len(on) == 0 {
		return marksNoZonesPart
	}
	parts := make([]string, len(on))
	for i, z := range on {
		parts[i] = fmt.Sprintf("%s (+%d%%)", z.Name, z.BonusPct)
	}
	return "Hot zones: " + strings.Join(parts, ", ") + "."
}

// --- /marks ---

// marksCommand is /marks (#marks, a PS2's /t marks): the character's marks, its hour's kill
// marks, its ring and rank, and the Hot zones on.
func (m *HeirloomMarks) marksCommand(sess *session.Session, p *entity.Player) {
	if sess == nil || p == nil {
		return
	}
	if m == nil || !HeirloomMarksOn() {
		SendClientMessage(sess, marksOffLine)
		return
	}
	for _, l := range m.marksLines(p) {
		SendClientMessage(sess, l)
	}
}

// marksLines are /marks's lines.
func (m *HeirloomMarks) marksLines(p *entity.Player) []string {
	row, err := m.db.CharMarksOf(p.CharServerID)
	if err != nil {
		log.Printf("Marks: %s's row: %v", p.Name, err)
	}
	ring := "none yet"
	if row.RingRank > 0 {
		ring = fmt.Sprintf("rank %d, %s", row.RingRank, m.guiseName(row.RingGuise))
	}
	lines := []string{
		fmt.Sprintf("Heirloom Marks: %s (%s earned in all).", commas(row.Marks), commas(row.Lifetime)),
		fmt.Sprintf("Kill marks this hour: %d of %d.", row.KillHourNow(m.now()), database.KillMarksAnHour),
		fmt.Sprintf("Ring of Guises: %s. Heirloom rank: %d.", ring, heirloomTopLevel+row.HeirloomRank),
		m.zonesPart(),
	}
	if p.Level >= entity.MaxLevel && !p.Reliving() {
		lines = append(lines, marksAt60Line)
	}
	return lines
}

// commas is n with thousands' commas: 1,960.
func commas(n int) string {
	s := strconv.Itoa(max(n, 0))
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// guiseName is a guise's spell's name without its "Illusion: ".
func (m *HeirloomMarks) guiseName(spellID int) string {
	name := m.spellName(spellID)
	if i := strings.Index(name, ":"); i >= 0 {
		name = strings.TrimSpace(name[i+1:])
	}
	if name == "" {
		name = "no guise"
	}
	return name
}

// spellName is a spell's name, kept once read.
func (m *HeirloomMarks) spellName(id int) string {
	m.mu.Lock()
	n, ok := m.names[id]
	m.mu.Unlock()
	if ok {
		return n
	}
	if m.spells != nil {
		if sp, err := m.spells.SpellPattern(id); err == nil && sp != nil {
			n = sp.SpellName
		}
	}
	m.mu.Lock()
	if m.names == nil {
		m.names = map[int]string{}
	}
	m.names[id] = n
	m.mu.Unlock()
	return n
}

// recordNamed is the first record of an exact name (0 none), kept once found.
func (m *HeirloomMarks) recordNamed(name string) int {
	m.mu.Lock()
	id, ok := m.byName[name]
	m.mu.Unlock()
	if ok && id != 0 {
		return id
	}
	if err := m.db.QueryRow(`SELECT patternid FROM itempattern WHERE itemname = ? ORDER BY patternid LIMIT 1`, name).Scan(&id); err != nil {
		return 0
	}
	m.mu.Lock()
	if m.byName == nil {
		m.byName = map[string]int{}
	}
	m.byName[name] = id
	m.mu.Unlock()
	return id
}

// askLocked keeps a question asked of a character under mark and returns its id. Called with
// m.mu held.
func (m *HeirloomMarks) askLocked(serverID uint32, mark uint32, b marksBox) uint32 {
	m.seq++
	b.id = mark | (m.seq & 0xFFFFFF)
	m.boxes[serverID] = b
	return b.id
}

// ask asks a question (a 0x0034 box) under mark.
func (m *HeirloomMarks) ask(sess *session.Session, p *entity.Player, mark uint32, b marksBox, question string, answers []string) {
	m.mu.Lock()
	id := m.askLocked(p.CharServerID, mark, b)
	m.mu.Unlock()
	SendQuestPopupMulti(sess, id, question, answers)
}

// answered is the question box answered, taken off its list; ok false for an old one.
func (m *HeirloomMarks) answered(p *entity.Player, box uint32) (marksBox, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.boxes[p.CharServerID]
	if !ok || b.id != box {
		return marksBox{}, false
	}
	delete(m.boxes, p.CharServerID)
	return b, true
}

// say is a speaker's line in dialogue boxes.
func marksSay(sess *session.Session, speaker, line string) {
	text := line
	if speaker != "" {
		text = speaker + ": " + line
	}
	for _, part := range splitSay(text, questBoxChars) {
		SendDialogueBox(sess, 0, part)
	}
}

// sortZones is the zones by id.
func sortZones(z []database.HotZone) {
	sort.Slice(z, func(i, j int) bool { return z[i].ID < z[j].ID })
}
