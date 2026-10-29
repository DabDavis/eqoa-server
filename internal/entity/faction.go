package entity

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Factions, as this server keeps them (F3 20261025-044451: killing Freeport's
// guards changed nothing, since every character had the same fixed standings).
//
//   - Every faction has a starting standing (table factions, filled from
//     database.DefaultFactions: the 331 an eqoa.live capture sent one gnome).
//     Where the characters of a race, class or home city start otherwise
//     (table factionstart, by race, class and humtype, -1 for any: the most
//     specific row a character matches), that is its start: a dark elf does
//     not start with a gnome's Neriak (2026-10-29, the user: "i made a dark
//     elf sk and tried to run to the coach and the ghoul servant killed me
//     because it hated me, but should be revered"). tools/seed_faction_starts.py
//     writes Sandstorm's measured starts. A character has its own standing
//     where it moved off its start (Player.Factions, table charfactions:
//     loaded at the login and saved at a logout, a takeover and a shutdown,
//     as its spell effects are), so a character made before the starts were
//     written takes them for every faction it has not moved.
//   - An NPC is of a faction by its name (table npcfaction, by NPCClassKey;
//     a row may hold only the NPCs that spawned within a radius of a place, to
//     tell a city's namesakes from the wild's): its target face and its aggro
//     follow a player's standing with that faction (aggro.go).
//     tools/seed_npc_factions.py writes the towns' people and creatures.
//   - Killing an NPC changes the killer's standings by the factionhits rows
//     for the NPC's name (a GM's #spawn copy has its original's name, so it
//     counts as the original): the KILLER's alone, as the kill's XP is.
//     The numbers are this server's, not measured: no capture holds a
//     faction hit, and the town rows are tools/seed_faction_hits.py's. An NPC
//     of a faction (npcfaction) with no factionhits row changes nothing when
//     killed (ours): the town creatures the seed gives a city include quest
//     targets (Klick'Anon's clockworks, which EQOA-Data's guide says cost no
//     faction), so a default would invent hits no record holds.
//   - Each change is a line as EverQuest words it ("Your faction standing
//     with X got worse."), unless the client's Faction Messages option is off
//     (0x00C9: session.FactionMessagesOff); the client filters none itself.
//   - A standing stays within FactionMin..FactionMax.

// FactionMin and FactionMax bound a standing: -2000 and 2000, the capture's
// lowest and the faction report's (20261024-125237) "already at -2000 or 2000".
const (
	FactionMin = -2000
	FactionMax = 2000
)

// The dispositions 0x0010 carries (CLIENT's words, WORDS.TXT 440-446), the
// faces VIWndGame::DrawTarget draws for them: 0-1 the red face, 2-3 the white,
// 4-6 the blue (decomp viwndgame.cpp, 0x00C51B50).
const (
	DispAggressive byte = iota
	DispDubious
	DispCautious
	DispNeutral
	DispFriendly
	DispWarm
	DispAllied
)

// FactionKOSBelow is the standing below which an NPC of a faction attacks a
// player on sight: the red faces' bands, Aggressive and Dubious
// (StandingDisposition), which VIWndGame::DrawTarget draws alike, so a
// creature attacks on sight exactly the players it shows the red face to (the
// user's "aggro from mobs that are red faced", 2026-10-25).
const FactionKOSBelow = -350

// StandingDisposition is the disposition a standing shows: CLIENT's own
// edges, the faction window's colour bands (VIWndFaction::DrawStanding,
// 0x00C4ACD0: -700, -350, -150, 0, 150, 350, 700); which band is which
// disposition is ours, the window's two nearest 0 being one Neutral.
func StandingDisposition(v int) byte {
	switch {
	case v < -700:
		return DispAggressive
	case v < FactionKOSBelow:
		return DispDubious
	case v < -150:
		return DispCautious
	case v < 150:
		return DispNeutral
	case v < 350:
		return DispFriendly
	case v < 700:
		return DispWarm
	}
	return DispAllied
}

// DispositionWords are the target box's words for the dispositions, CLIENT's
// own (WORDS.TXT lines 440-446), DispAggressive first.
var DispositionWords = [...]string{"Aggressive", "Dubious", "Cautious", "Neutral", "Friendly", "Warm", "Allied"}

// FactionStanding is a faction and a standing with it.
type FactionStanding struct {
	Name     string
	Standing int
}

// FactionHit is what a kill does to one faction.
type FactionHit struct {
	Faction string
	Delta   int
}

// startRow is a factionstart row, by its faction: the race, class and home
// city it holds (-1 any) and the standing.
type startRow struct {
	race, class, hum int
	standing         int
}

// specificity orders start rows: the one naming more of the three first, then
// the one naming the race, then the class.
func (r startRow) specificity() int {
	n, s := 0, 0
	for i, v := range [3]int{r.race, r.class, r.hum} {
		if v >= 0 {
			n++
			s |= 4 >> i
		}
	}
	return n<<3 | s
}

func (r startRow) holds(c factionCombo) bool {
	return (r.race < 0 || r.race == c.race) && (r.class < 0 || r.class == c.class) && (r.hum < 0 || r.hum == c.hum)
}

// factionCombo is the race, class and home city a character's starts are
// looked up by.
type factionCombo struct{ race, class, hum int }

// raceFroglok is the race slot the froglok takes (handlers.RaceFroglok).
const raceFroglok = 10

// comboOf is the combo a character's starts are read for: a froglok as a
// human of the east (the city it starts in, database.startPlace), a human
// of no west city (humType 2) as the east's, and every other race's home city
// 0, the only one defaultcharacter gives them (a client may send another).
func comboOf(race, class, hum int) factionCombo {
	switch {
	case race == raceFroglok:
		return factionCombo{0, class, 1}
	case race == 0 && hum != 2:
		return factionCombo{0, class, 1}
	case race != 0:
		hum = 0
	}
	return factionCombo{race, class, hum}
}

// npcFactionRow is an npcfaction row: the faction of the NPCs of its name
// anywhere (world -1), in a world (radius 0), or spawned within radius of
// (x, z) in a world.
type npcFactionRow struct {
	world   int
	x, z, r float32
	faction string
}

// holds reports an NPC of the row's name the row is for, by where it spawned.
func (r npcFactionRow) holds(npc *NPC) bool {
	if r.world >= 0 && int(npc.World) != r.world {
		return false
	}
	return r.r <= 0 || distXZ(npc.SpawnX, npc.SpawnZ, r.x, r.z) <= r.r
}

// rank orders npcfaction rows of one name: a place's first (the smallest
// first), then a world's, then anywhere's.
func (r npcFactionRow) rank() float32 {
	switch {
	case r.r > 0:
		return r.r
	case r.world >= 0:
		return math.MaxFloat32 / 2
	}
	return math.MaxFloat32
}

// factionTable is the world's factions: the starting standings in the
// window's order and by name, the starts by race, class and home city, the
// hits by NPC name and which faction the NPCs of a name are of. Until one is
// set it is database.DefaultFactions with no hits, starts or members.
type factionTable struct {
	set    bool
	list   []FactionStanding // standing high to low, then name
	byName map[string]int
	hits   map[string][]FactionHit
	starts map[string][]startRow      // by faction, the most specific first
	npcs   map[string][]npcFactionRow // by NPCClassKey, the most specific first
	nStart int                        // starts kept
	nNPC   int                        // npcfaction rows kept

	// what was worked out from starts and npcs (cacheMu: the world's read
	// lock may be all a caller holds): a combo's starts, a raw NPC name's rows
	cacheMu  sync.Mutex
	combos   map[factionCombo]map[string]int
	npcNames map[string][]npcFactionRow
}

func (t *factionTable) ensure() {
	if !t.set {
		t.setDefaults(database.DefaultFactions)
	}
}

func (t *factionTable) setDefaults(d []database.FactionDefault) {
	t.set = true
	t.list = make([]FactionStanding, 0, len(d))
	t.byName = make(map[string]int, len(d))
	for _, f := range d {
		if _, dup := t.byName[f.Name]; dup {
			continue
		}
		v := clampFaction(f.Standing)
		t.byName[f.Name] = v
		t.list = append(t.list, FactionStanding{f.Name, v})
	}
	sortFactions(t.list)
}

// forget drops what was worked out from the starts and the members.
func (t *factionTable) forget() {
	t.cacheMu.Lock()
	t.combos, t.npcNames = nil, nil
	t.cacheMu.Unlock()
}

// startsFor is every faction a combo starts with otherwise than the
// faction's own: its most specific row of each.
func (t *factionTable) startsFor(c factionCombo) map[string]int {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	if m, ok := t.combos[c]; ok {
		return m
	}
	m := make(map[string]int)
	for f, rows := range t.starts {
		for _, r := range rows {
			if r.holds(c) {
				m[f] = r.standing
				break
			}
		}
	}
	if t.combos == nil {
		t.combos = make(map[factionCombo]map[string]int)
	}
	t.combos[c] = m
	return m
}

// npcRows is the npcfaction rows for an NPC name, as written (looked up by
// NPCClassKey once a name).
func (t *factionTable) npcRows(name string) []npcFactionRow {
	if len(t.npcs) == 0 {
		return nil
	}
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	rows, ok := t.npcNames[name]
	if !ok {
		rows = t.npcs[database.NPCClassKey(name)]
		if t.npcNames == nil {
			t.npcNames = make(map[string][]npcFactionRow)
		}
		t.npcNames[name] = rows
	}
	return rows
}

// sortFactions puts standings in the faction window's order, the capture's
// own: standing high to low, then name.
func sortFactions(l []FactionStanding) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Standing != l[j].Standing {
			return l[i].Standing > l[j].Standing
		}
		return l[i].Name < l[j].Name
	})
}

func clampFaction(v int) int {
	return max(FactionMin, min(FactionMax, v))
}

// SetFactionDefaults sets every faction and its starting standing (the
// factions table): at the start, and when a GM edits it.
func (w *World) SetFactionDefaults(d []database.FactionDefault) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.setDefaults(d)
}

// SetFactionHits sets what killing an NPC of each name does (the factionhits
// table): at the start, and when a GM edits it. It reports the rows kept and
// those naming no faction, which change nothing.
func (w *World) SetFactionHits(rows []database.FactionHitRow) (kept, unknown int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	hits := make(map[string][]FactionHit)
	for _, r := range rows {
		if _, ok := w.factions.byName[r.Faction]; !ok {
			unknown++
			log.Printf("Factions: a hit for killing %q names %q, which is no faction: it changes nothing", r.NPCName, r.Faction)
			continue
		}
		hits[r.NPCName] = append(hits[r.NPCName], FactionHit{r.Faction, r.Delta})
		kept++
	}
	w.factions.hits = hits
	return kept, unknown
}

// SetFactionStarts sets the standings the characters of a race, class and
// home city start with (the factionstart table): at the start, and when a GM
// edits it. It reports the rows kept and those naming no faction.
func (w *World) SetFactionStarts(rows []database.FactionStartRow) (kept, unknown int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	starts := make(map[string][]startRow)
	for _, r := range rows {
		if _, ok := w.factions.byName[r.Faction]; !ok {
			unknown++
			log.Printf("Factions: a start (race %d, class %d, home %d) names %q, which is no faction: no one starts with it",
				r.Race, r.Class, r.HumType, r.Faction)
			continue
		}
		starts[r.Faction] = append(starts[r.Faction], startRow{max(r.Race, -1), max(r.Class, -1), max(r.HumType, -1), clampFaction(r.Standing)})
		kept++
	}
	for _, l := range starts {
		sort.SliceStable(l, func(i, j int) bool { return l[i].specificity() > l[j].specificity() })
	}
	w.factions.starts, w.factions.nStart = starts, kept
	w.factions.forget()
	return kept, unknown
}

// SetNPCFactions sets which faction the NPCs of a name are of (the npcfaction
// table): at the start, and when a GM edits it. It reports the rows kept and
// those naming no faction (an empty faction is kept: none).
func (w *World) SetNPCFactions(rows []database.NPCFactionRow) (kept, unknown int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	npcs := make(map[string][]npcFactionRow)
	for _, r := range rows {
		if _, ok := w.factions.byName[r.Faction]; !ok && r.Faction != "" {
			unknown++
			log.Printf("Factions: the NPCs named %q are of %q, which is no faction: of none", r.Name, r.Faction)
			continue
		}
		k := database.NPCClassKey(r.Name)
		npcs[k] = append(npcs[k], npcFactionRow{max(r.World, -1), float32(r.X), float32(r.Z), float32(r.Radius), r.Faction})
		kept++
	}
	for _, l := range npcs {
		sort.SliceStable(l, func(i, j int) bool { return l[i].rank() < l[j].rank() })
	}
	w.factions.npcs, w.factions.nNPC = npcs, kept
	w.factions.forget()
	return kept, unknown
}

// LoadFactions reads the factions, the starts, the hits and the NPCs' factions
// off the database: at the start and after a GM's edit of any of them. Call it
// without the database held (the database has one connection).
func (w *World) LoadFactions(db *database.SQLiteDB) error {
	defaults, err := db.FactionDefaults()
	if err != nil {
		return fmt.Errorf("factions: %w", err)
	}
	hits, err := db.FactionHits()
	if err != nil {
		return fmt.Errorf("factionhits: %w", err)
	}
	starts, err := db.FactionStarts()
	if err != nil {
		return fmt.Errorf("factionstart: %w", err)
	}
	members, err := db.NPCFactions()
	if err != nil {
		return fmt.Errorf("npcfaction: %w", err)
	}
	if len(defaults) > 0 {
		w.SetFactionDefaults(defaults)
	}
	kept, unknown := w.SetFactionHits(hits)
	sKept, sUnknown := w.SetFactionStarts(starts)
	nKept, nUnknown := w.SetNPCFactions(members)
	log.Printf("Factions: %d factions, %d kill hits on %d NPC names (%d naming no faction); %d starts by race, class and home city (%d naming no faction); %d npcfaction rows (%d naming no faction): %s",
		len(defaults), kept, w.factionHitNames(), unknown, sKept, sUnknown, nKept, nUnknown, w.FactionMembersSummary())
	return nil
}

func (w *World) factionHitNames() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.factions.hits)
}

// FactionHitsFor is what killing an NPC of this name does.
func (w *World) FactionHitsFor(npcName string) []FactionHit {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return append([]FactionHit(nil), w.factions.hits[npcName]...)
}

// startLocked is the standing p starts with in a faction: its combo's most
// specific factionstart row, else the faction's own. Must be called with the
// lock held.
func (w *World) startLocked(p *Player, faction string) int {
	w.factions.ensure()
	if len(w.factions.starts[faction]) > 0 {
		if v, ok := w.factions.startsFor(comboOf(p.Race, p.Class, p.HumType))[faction]; ok {
			return v
		}
	}
	return w.factions.byName[faction]
}

// standingLocked is p's standing with a faction: its own, else its start.
// Must be called with the lock held.
func (w *World) standingLocked(p *Player, faction string) int {
	if v, ok := p.Factions[faction]; ok {
		return v
	}
	return w.startLocked(p, faction)
}

// Standing is the session's player's standing with a faction (its own, else
// its starting one), false when the session has no player in the world: for
// the quests' faction requirements and anything else outside the World.
func (w *World) Standing(sess *session.Session, faction string) (int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0, false
	}
	return w.standingLocked(p, faction), true
}

// npcFactionLocked is the faction an NPC is of (npcfaction: its name's row
// for where it spawned, the most specific), "" none. Must be called with the
// lock held (a read lock will do).
func (w *World) npcFactionLocked(npc *NPC) string {
	for _, r := range w.factions.npcRows(npc.Name) {
		if r.holds(npc) {
			return r.faction
		}
	}
	return ""
}

// NPCFaction is the faction an NPC is of, "" none (npcfaction).
func (w *World) NPCFaction(npc *NPC) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.npcFactionLocked(npc)
}

// FactionMembersSummary says how many NPCs of the world are of a faction, and
// of how many factions: for the start's line.
func (w *World) FactionMembersSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.factions.nNPC == 0 {
		return "no NPC is of a faction (npcfaction is empty)"
	}
	n, of := 0, map[string]int{}
	for _, npc := range w.NPCs {
		if f := w.npcFactionLocked(npc); f != "" {
			n++
			of[f]++
		}
	}
	type fc struct {
		f string
		n int
	}
	var top []fc
	for f, c := range of {
		top = append(top, fc{f, c})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n || top[i].n == top[j].n && top[i].f < top[j].f })
	var parts []string
	for i := 0; i < len(top) && i < 5; i++ {
		parts = append(parts, fmt.Sprintf("%s %d", top[i].f, top[i].n))
	}
	return fmt.Sprintf("%d NPCs are of %d factions (%s ...)", n, len(of), strings.Join(parts, ", "))
}

// factionKillLocked is a kill's faction hits on its killer, and the lines that
// tell it (sess nil: none). Must be called with the lock held.
func (w *World) factionKillLocked(sess *session.Session, p *Player, npc *NPC) {
	if p == nil || npc == nil {
		return
	}
	// every kill a player is credited with passes here (melee, a spell, a
	// pet, a GM's #kill): the quests hear it (questhooks.go)
	w.creditKillLocked(sess, p, npc)
	w.factions.ensure()
	hits := w.factions.hits[npc.Name]
	if len(hits) == 0 {
		return
	}
	quiet := sess == nil || sess.FactionMessagesOff()
	for _, h := range hits {
		if h.Delta == 0 {
			continue
		}
		old := w.standingLocked(p, h.Faction)
		now := clampFaction(old + h.Delta)
		if now != old {
			if p.Factions == nil {
				p.Factions = make(map[string]int)
			}
			p.Factions[h.Faction] = now
		}
		log.Printf("Factions: %s killed %q: %s %d -> %d", p.Name, npc.Name, h.Faction, old, now)
		if !quiet {
			w.sendChatToSession(sess, factionLine(h.Faction, h.Delta, now != old))
		}
	}
}

// factionLine is EverQuest's words for a standing moved, or held at its end.
func factionLine(faction string, delta int, moved bool) string {
	name := strings.TrimSpace(faction)
	switch {
	case delta < 0 && moved:
		return fmt.Sprintf("Your faction standing with %s got worse.", name)
	case delta < 0:
		return fmt.Sprintf("Your faction standing with %s could not possibly get any worse.", name)
	case moved:
		return fmt.Sprintf("Your faction standing with %s got better.", name)
	default:
		return fmt.Sprintf("Your faction standing with %s could not possibly get any better.", name)
	}
}

// PlayerFactions is the standings the session's faction window lists, in its
// order: every faction, with the player's own standing where it has one, else
// its start (no player: the factions' own). A standing kept for a faction the
// table no longer has is not listed.
func (w *World) PlayerFactions(sess *session.Session) []FactionStanding {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	out := append([]FactionStanding(nil), w.factions.list...)
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return out
	}
	starts := w.factions.startsFor(comboOf(p.Race, p.Class, p.HumType))
	if len(p.Factions) == 0 && len(starts) == 0 {
		return out
	}
	for i := range out {
		if v, ok := p.Factions[out[i].Name]; ok {
			out[i].Standing = v
		} else if v, ok := starts[out[i].Name]; ok {
			out[i].Standing = v
		}
	}
	sortFactions(out)
	return out
}

// SavedFactions is a copy of a player's own standings, for the save at a
// logout; false when the player is not in the world.
func (w *World) SavedFactions(id EntityID) (map[string]int, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p, ok := w.Players[id]
	if !ok {
		return nil, false
	}
	out := make(map[string]int, len(p.Factions))
	for k, v := range p.Factions {
		out[k] = v
	}
	return out, true
}

// RestoreFactions puts a character's kept standings on its session's player
// at the login. It reports how many it kept.
func (w *World) RestoreFactions(sess *session.Session, m map[string]int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return 0
	}
	p.Factions = make(map[string]int, len(m))
	for k, v := range m {
		p.Factions[k] = clampFaction(v)
	}
	return len(p.Factions)
}

// factionNamedLocked is the faction a GM's words name: the one of that name
// (any case), else the one whose name holds them, else none (ambiguous: the
// names that hold them, a few).
func (w *World) factionNamedLocked(query string) (string, []string) {
	w.factions.ensure()
	q := strings.ToLower(strings.TrimSpace(query))
	var holds []string
	for _, f := range w.factions.list {
		n := strings.ToLower(strings.TrimSpace(f.Name))
		if n == q {
			return f.Name, nil
		}
		if strings.Contains(n, q) {
			holds = append(holds, f.Name)
		}
	}
	if len(holds) == 1 {
		return holds[0], nil
	}
	return "", holds
}

// FactionsMatching is the session's player's standings with the factions
// whose names hold query (any case), in the window's order.
func (w *World) FactionsMatching(sess *session.Session, query string) []FactionStanding {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []FactionStanding
	for _, f := range w.PlayerFactions(sess) {
		if strings.Contains(strings.ToLower(f.Name), q) {
			out = append(out, f)
		}
	}
	return out
}

// SetFaction sets the session's player's standing with the faction its words
// name (#faction), within FactionMin..FactionMax, kept at the logout as a
// kill's change is. It reports the faction and the standing kept, or why it
// set none.
func (w *World) SetFaction(sess *session.Session, query string, standing int) (string, int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return "", 0, fmt.Errorf("not in the world")
	}
	name, holds := w.factionNamedLocked(query)
	if name == "" {
		if len(holds) == 0 {
			return "", 0, fmt.Errorf("no faction is named %q", query)
		}
		n := len(holds)
		if n > 4 {
			holds = append(holds[:4], "...")
		}
		return "", 0, fmt.Errorf("%q names %d factions: %s", query, n, strings.Join(holds, ", "))
	}
	if p.Factions == nil {
		p.Factions = make(map[string]int)
	}
	v := clampFaction(standing)
	p.Factions[name] = v
	return name, v, nil
}

// SetOnlineFaction applies a GM's edit of a charfactions row to the character
// while it is in the world (remove: the row deleted, the starting standing
// again), so its logout does not save the old one over it. It reports the
// character's name and whether it is in the world.
func (w *World) SetOnlineFaction(charServerID uint32, faction string, standing int, remove bool) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range w.Players {
		if p.CharServerID != charServerID {
			continue
		}
		if remove {
			delete(p.Factions, faction)
		} else {
			if p.Factions == nil {
				p.Factions = make(map[string]int)
			}
			p.Factions[faction] = clampFaction(standing)
		}
		return p.Name, true
	}
	return "", false
}

// StartThreat is an NPC that would attack a new character on sight where it
// first stands (StartThreats), and why.
type StartThreat struct {
	Spot     database.StartSpot
	NPC      string
	Level    int
	Distance float32
	Faction  string // the faction it attacks for; "" its nature (aggro.go)
	Standing int
}

// startThreatReach is how near a new character's start spot an NPC that
// attacks it on sight is counted: past the 30 a creature notices from, as a
// wanderer or a chase brings one on.
const startThreatReach float32 = 60

// StartThreats is every NPC within startThreatReach of a start spot that would
// attack a new level-1 character of its race, class and city on sight, there
// and with its starting standings: what killed the user's new dark elf by
// Neriak's coach (2026-10-29). At the start the server says how many.
func (w *World) StartThreats(spots []database.StartSpot) []StartThreat {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []StartThreat
	for _, s := range spots {
		p := &Player{Name: "a new character", Level: 1, Race: s.Race, Class: s.Class, HumType: s.HumType,
			X: s.X, Y: s.Y, Z: s.Z, World: uint8(s.World)}
		for _, npc := range w.NPCs {
			if int(npc.World) != s.World || npc.Temporary {
				continue
			}
			d := dist3(npc.SpawnX, npc.SpawnY, npc.SpawnZ, s.X, s.Y, s.Z)
			if d > startThreatReach || !w.threatensLocked(npc, p) {
				continue
			}
			t := StartThreat{Spot: s, NPC: npc.Name, Level: npc.Level, Distance: d}
			if f, ok := w.factionAggroLocked(npc); ok {
				t.Faction, t.Standing = f, w.standingLocked(p, f)
			}
			out = append(out, t)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Distance < out[j].Distance })
	return out
}

// StartThreatLines is StartThreats as the start's log lines: one a start spot
// in danger, or one saying none is.
func (w *World) StartThreatLines(spots []database.StartSpot) []string {
	threats := w.StartThreats(spots)
	if len(threats) == 0 {
		return []string{fmt.Sprintf("no NPC attacks a new character on sight at any of the %d start spots", len(spots))}
	}
	type spotKey struct{ race, class, hum int }
	by := map[spotKey][]StartThreat{}
	var order []spotKey
	for _, t := range threats {
		k := spotKey{t.Spot.Race, t.Spot.Class, t.Spot.HumType}
		if _, ok := by[k]; !ok {
			order = append(order, k)
		}
		by[k] = append(by[k], t)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		return a.race < b.race || a.race == b.race && (a.hum < b.hum || a.hum == b.hum && a.class < b.class)
	})
	var out []string
	for _, k := range order {
		l := by[k]
		var what []string
		for i, t := range l {
			if i == 3 {
				what = append(what, "...")
				break
			}
			if t.Faction != "" {
				what = append(what, fmt.Sprintf("%s %.0f off (%s %d)", strings.TrimSpace(t.NPC), t.Distance, t.Faction, t.Standing))
			} else {
				what = append(what, fmt.Sprintf("%s (level %d) %.0f off", strings.TrimSpace(t.NPC), t.Level, t.Distance))
			}
		}
		out = append(out, fmt.Sprintf("WARNING: a new %s %s (home %d) is attacked on sight at its start by %d NPCs: %s",
			nameIn(raceNames, k.race), nameIn(classNames, k.class), k.hum, len(l), strings.Join(what, ", ")))
	}
	return out
}
