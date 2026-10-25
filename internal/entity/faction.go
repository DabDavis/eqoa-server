package entity

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Factions, as this server keeps them (F3 20261025-044451: killing Freeport's
// guards changed nothing, since every character had the same fixed standings).
//
//   - Every faction has a starting standing (table factions, filled from
//     database.DefaultFactions: the 331 an eqoa.live capture sent one gnome),
//     and a character has its own where it moved off one (Player.Factions,
//     table charfactions: loaded at the login and saved at a logout, a
//     takeover and a shutdown, as its spell effects are).
//   - Killing an NPC changes the killer's standings by the factionhits rows
//     for the NPC's name (a GM's #spawn copy has its original's name, so it
//     counts as the original): the KILLER's alone, as the kill's XP is.
//     The numbers are this server's, not measured: no capture holds a
//     faction hit, and the town rows are tools/seed_faction_hits.py's.
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

// factionTable is the world's factions: the starting standings in the
// window's order and by name, and the hits by NPC name. Until one is set it
// is database.DefaultFactions with no hits.
type factionTable struct {
	set    bool
	list   []FactionStanding // standing high to low, then name
	byName map[string]int
	hits   map[string][]FactionHit
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

// LoadFactions reads the factions and the hits off the database: at the start
// and after a GM's edit of either. Call it without the database held (the
// database has one connection).
func (w *World) LoadFactions(db *database.SQLiteDB) error {
	defaults, err := db.FactionDefaults()
	if err != nil {
		return fmt.Errorf("factions: %w", err)
	}
	hits, err := db.FactionHits()
	if err != nil {
		return fmt.Errorf("factionhits: %w", err)
	}
	if len(defaults) > 0 {
		w.SetFactionDefaults(defaults)
	}
	kept, unknown := w.SetFactionHits(hits)
	log.Printf("Factions: %d factions, %d kill hits on %d NPC names (%d naming no faction)",
		len(defaults), kept, w.factionHitNames(), unknown)
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

// standingLocked is p's standing with a faction: its own, else the faction's
// starting one. Must be called with the lock held.
func (w *World) standingLocked(p *Player, faction string) int {
	if v, ok := p.Factions[faction]; ok {
		return v
	}
	w.factions.ensure()
	return w.factions.byName[faction]
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
// order: every faction, with the player's own standing where it has one (no
// player: the starting standings). A standing kept for a faction the table no
// longer has is not listed.
func (w *World) PlayerFactions(sess *session.Session) []FactionStanding {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.factions.ensure()
	out := append([]FactionStanding(nil), w.factions.list...)
	p := w.sessionPlayerLocked(sess)
	if p == nil || len(p.Factions) == 0 {
		return out
	}
	for i := range out {
		if v, ok := p.Factions[out[i].Name]; ok {
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
