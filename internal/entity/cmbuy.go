package entity

import (
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/session"
)

// Buying class masteries (F3 pass 17, the user: "we can implement buying
// class masteries ... in our own way", and the general ones "buyable too").
//
// The window is CLIENT's (decomp viwndclassmastery*.cpp, viclientpart4ec58.cpp):
// the list (0x1402 sub-op 8, a VICMSkill a rank: its key, owned and known
// flags, id, parent key, category, cost, name), one rank opened (sub-op 9, a
// VICMNode: the same, the level, the points spent and the base stats it
// needs, its text, the stats it modifies, the abilities it adds and those it
// removes or needs, the masteries it needs), a purchase (sub-op 5: the points
// left and spent, the run speed, the 29 stats the masteries add, the seven
// stat caps they raise, the known flags again) and a removal (sub-op 6). It
// shows a rank "known" when its requirements are met (else "???", "Locked"),
// in five categories: CLIENT's words 1653-1656 and 511, General, Racial,
// Archetype, Class and Master Class.
//
// What each rank costs, needs and gives is table cmcatalog (GM-editable;
// tools/seed_masteries.py writes EQOA-Data's lists into it). A character's
// ranks are charcms, its points charmastery. What is ours, as no record of
// the live game's rules exists: the base stats a requirement is checked
// against (the trainer's base, before items and spells, plus the masteries'
// own), and one Master Class at a time held by refusing a second until the
// first is removed. A removal gives nothing back, as jib has the live game's
// (F3 20261026-043542: "permanent unless you delete them, which lowers your
// total cm's as you do that have to be re-earned"): the rank's points are
// gone, so under the 1500 the character may hold they are earned again (the
// next point costing what that point did); a GM's #cmreset gives them back.

// CMSlots is how many stats the window's 29 rows name (STR..CHA, their
// caps, HP and power maxima, HoT, PoT, AC, OFF and DEF MOD, HP FACTOR, the
// six resists, MOVEMENT RATE): the order of CLIENT's inspect window and of
// the dump's and the purchase's 29 stats.
const CMSlots = 29

// The window's slots this server acts on.
const (
	cmSlotCap      = 7 // STR MAX: the seven caps are 7-13
	cmSlotHP       = 14
	cmSlotPower    = 15
	cmSlotHoT      = 16
	cmSlotPoT      = 17
	cmSlotAC       = 18
	cmSlotOff      = 19
	cmSlotDef      = 20
	cmSlotHPFactor = 21
	cmSlotFR       = 22
	cmSlotLR       = 23
	cmSlotCR       = 24
	cmSlotAR       = 25
	cmSlotPR       = 26
	cmSlotDR       = 27
	cmSlotMoveRate = 28
)

// CMCategoryMaster is the Master Class category, of which a character holds
// one (CLIENT's word 1807: "You can possess only one "Master Class" at a time.").
const CMCategoryMaster = 4

// cmStatSlots maps a stat name, as EQOA-Data's lists write them and
// cmcatalog keeps them, to its slot.
var cmStatSlots = map[string]int{
	"str": 0, "sta": 1, "agi": 2, "dex": 3, "wis": 4, "int": 5, "cha": 6,
	"strmax": 7, "stamax": 8, "agimax": 9, "dexmax": 10, "wismax": 11, "intmax": 12, "chamax": 13,
	"hp": cmSlotHP, "maxhp": cmSlotHP, "hpmax": cmSlotHP,
	"pow": cmSlotPower, "pwrmax": cmSlotPower, "power": cmSlotPower, "maxpower": cmSlotPower,
	"hot": cmSlotHoT, "pot": cmSlotPoT, "ac": cmSlotAC,
	"offmod": cmSlotOff, "defmod": cmSlotDef, "hpfactor": cmSlotHPFactor,
	"fr": cmSlotFR, "lr": cmSlotLR, "cr": cmSlotCR, "ar": cmSlotAR, "pr": cmSlotPR, "dr": cmSlotDR,
	"movrate": cmSlotMoveRate, "moverate": cmSlotMoveRate,
}

// cmSlotNames is each slot's name in a GM's line.
var cmSlotNames = [CMSlots]string{"STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA",
	"STR cap", "STA cap", "AGI cap", "DEX cap", "WIS cap", "INT cap", "CHA cap",
	"max HP", "max power", "HoT", "PoT", "AC", "OFF MOD", "DEF MOD", "HP factor",
	"fire", "lightning", "cold", "arcane", "poison", "disease", "movement"}

// statNames are the seven stats' names, STR..CHA.
var statNames = [7]string{"STR", "STA", "AGI", "DEX", "WIS", "INT", "CHA"}

// CMAbility is an ability a rank adds, removes or needs: its name and level
// as written, and the spell it was found as (0 when none).
type CMAbility struct {
	Name    string
	Level   int
	SpellID int
}

// CMRank is one rank of cmcatalog, read.
type CMRank struct {
	ID, Category, Classes, Races int
	Mastery                      string
	Rank                         int
	Name                         string
	Parent, Cost, Level, Spent   int
	MinStats                     [7]int
	Requires                     []CMAbility
	// RequireRanks are other masteries' ranks owned first (require_ranks);
	// one the catalog lacks is not checked
	RequireRanks []int
	// RequireQuests are the quests finished first (require_quests, their
	// ids): the stat-boost quests open the stat-boost lines, a trial a rank
	// (the user, 2026-10-29: "gate the stat boost masteries behind the
	// quests"; EQOA-Data's guide: "Bulging Muscles 1 ... is now available
	// for purchase")
	RequireQuests []int
	Effects       [CMSlots]int
	Adds, Removes []CMAbility
	Description   string
	Enabled       bool
}

// BookChange is what buying the rank does to the spell book: the abilities
// it gives and those it takes out (a removal does the reverse). An ability
// the rank both adds and removes as one spell is in neither: EQOA-Data's
// lists write a rank that improves a spell in place so (Quick Blade 1 adds
// and removes Dancing Blades, Hydromancy 1 Water Elementalkin), the live
// game swapping in an improved record of the same name, and this server has
// the one record, which the book keeps (jib's review, F3 pass 19). Until
// then a purchase gave such a spell and then took it out, so Hydromancy 1
// cost a magician the Water Elementalkin that Hydromancy 2 needs.
func (r *CMRank) BookChange() (gives, takes []CMAbility) {
	added := map[int]bool{}
	for _, a := range r.Adds {
		if a.SpellID > 0 {
			added[a.SpellID] = true
		}
	}
	both := map[int]bool{}
	for _, a := range r.Removes {
		if a.SpellID > 0 && added[a.SpellID] {
			both[a.SpellID] = true
		}
	}
	for _, a := range r.Adds {
		if !both[a.SpellID] {
			gives = append(gives, a)
		}
	}
	for _, a := range r.Removes {
		if !both[a.SpellID] {
			takes = append(takes, a)
		}
	}
	return gives, takes
}

// cmCatalog is the ranks by id.
type cmCatalog struct {
	byID  map[int]*CMRank
	order []*CMRank
}

// ParseCMRank reads a cmcatalog row; what it cannot read it leaves out.
func ParseCMRank(r database.CMRankRow) *CMRank {
	c := &CMRank{ID: r.ID, Category: r.Category, Classes: r.Classes, Races: r.Races, Mastery: r.Mastery,
		Rank: r.Rank, Name: r.Name, Parent: r.Parent, Cost: r.Cost, Level: r.Level, Spent: r.Spent,
		Description: r.Description, Enabled: r.Enabled}
	for k, v := range parseStatList(r.MinStats) {
		if s, ok := cmStatSlots[k]; ok && s < 7 {
			c.MinStats[s] = v
		}
	}
	for k, v := range parseStatList(r.Effects) {
		if s, ok := cmStatSlots[k]; ok {
			c.Effects[s] += v
		}
	}
	c.Requires = parseAbilities(r.Requires, r.RequireSpells)
	c.RequireRanks = database.SpellIDs(r.RequireRanks) // a list of ids, as the spells' are
	c.RequireQuests = database.SpellIDs(r.RequireQuests)
	c.Adds = parseAbilities(r.Adds, r.AddSpells)
	c.Removes = parseAbilities(r.Removes, r.RemoveSpells)
	return c
}

// parseStatList reads "STR:100, HP Factor:1" into lower-case keys without
// spaces.
func parseStatList(s string) map[string]int {
	out := map[string]int{}
	for _, f := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(f, ":")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			continue
		}
		key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(k), " ", ""))
		out[key] += n
	}
	return out
}

// parseAbilities reads "Name@level; Name@level" and, beside it, the spell ids
// they were found as ("12,34", in the same order; missing ones 0).
func parseAbilities(names, ids string) []CMAbility {
	var out []CMAbility
	for _, f := range strings.Split(names, ";") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		a := CMAbility{Name: f}
		if n, l, ok := strings.Cut(f, "@"); ok {
			a.Name = strings.TrimSpace(n)
			a.Level, _ = strconv.Atoi(strings.TrimSpace(l))
		}
		out = append(out, a)
	}
	idList := strings.Split(ids, ",")
	for i := range out {
		if i < len(idList) {
			out[i].SpellID, _ = strconv.Atoi(strings.TrimSpace(idList[i]))
		}
	}
	return out
}

// LoadCMCatalog reads cmcatalog into the World (at the start, and when a GM
// edits it): how many ranks there are.
func (w *World) LoadCMCatalog(db *database.SQLiteDB) (int, error) {
	rows, err := db.CMCatalog()
	if err != nil {
		return 0, fmt.Errorf("cmcatalog: %w", err)
	}
	cat := &cmCatalog{byID: make(map[int]*CMRank, len(rows))}
	for _, r := range rows {
		c := ParseCMRank(r)
		cat.byID[c.ID] = c
		cat.order = append(cat.order, c)
	}
	w.mu.Lock()
	w.cms = cat
	for _, p := range w.Players {
		p.refreshCMLocked(cat)
		p.RecalcStats(false)
		p.StatsDirty = true
	}
	w.mu.Unlock()
	return len(rows), nil
}

// SetCMCatalog is LoadCMCatalog with the rows given (tests).
func (w *World) SetCMCatalog(rows []database.CMRankRow) {
	cat := &cmCatalog{byID: make(map[int]*CMRank, len(rows))}
	for _, r := range rows {
		c := ParseCMRank(r)
		cat.byID[c.ID] = c
		cat.order = append(cat.order, c)
	}
	w.mu.Lock()
	w.cms = cat
	w.mu.Unlock()
}

// CMSummary says what the catalog holds, for the log at the start.
func (w *World) CMSummary() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.cms == nil || len(w.cms.order) == 0 {
		return "masteries: none in cmcatalog (none can be bought; tools/seed_masteries.py fills it)"
	}
	var n [5]int
	quests := 0
	for _, r := range w.cms.order {
		if r.Enabled && r.Category >= 0 && r.Category < 5 {
			n[r.Category]++
			if len(r.RequireQuests) > 0 {
				quests++
			}
		}
	}
	return fmt.Sprintf("masteries: %d ranks to buy (general %d, racial %d, archetype %d, class %d, master class %d; %d need a quest finished)",
		n[0]+n[1]+n[2]+n[3]+n[4], n[0], n[1], n[2], n[3], n[4], quests)
}

// offered is whether the rank is listed for the player: enabled, and for its
// class and race.
func (r *CMRank) offered(p *Player) bool {
	return r.Enabled && r.Classes&(1<<uint(p.Class)) != 0 && r.Races&(1<<uint(p.Race)) != 0
}

// refreshCMLocked sums the player's ranks' effects again.
func (p *Player) refreshCMLocked(cat *cmCatalog) {
	var s [CMSlots]int
	if cat != nil {
		for id := range p.CMs {
			if r := cat.byID[id]; r != nil {
				for i, v := range r.Effects {
					s[i] += v
				}
			}
		}
	}
	p.CMEffects = s
	p.MoveRateBonus = s[cmSlotMoveRate]
}

// RestoreCMs puts the ranks a character bought on its session's player at
// the login (before its gear, whose RecalcStats counts them).
func (w *World) RestoreCMs(sess *session.Session, owned []database.OwnedCM) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return
	}
	p.CMs = make(map[int]int, len(owned))
	for _, o := range owned {
		p.CMs[o.ID] = o.Cost
	}
	p.refreshCMLocked(w.cms)
	p.RecalcStats(false)
}

// masterClassLocked is the Master Class the player holds, by its mastery's
// name ("Spellbinder"; a row with none by the rank's own), "" for none. Of
// two (a GM's doing: a second cannot be bought) the lowest rank id's.
func (w *World) masterClassLocked(p *Player) string {
	if w.cms == nil {
		return ""
	}
	best, name := 0, ""
	for id := range p.CMs {
		r := w.cms.byID[id]
		if r == nil || r.Category != CMCategoryMaster || (best != 0 && id > best) {
			continue
		}
		best, name = id, r.Mastery
		if name == "" {
			name = r.Name
		}
	}
	return name
}

// TargetInfoLines are the two lines 0x0010 carries about a player: its
// title, which CLIENT's info window shows in place of its class
// (VIWndGame::DrawTargetInfo), its Master Class (F3 20261026-135950: a
// Spellbinder is shown as one, not as an Enchanter), and its guild, shown
// under its name.
func (w *World) TargetInfoLines(p *Player) (title, guild string) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.masterClassLocked(p), p.GuildName
}

// CMEntry is one rank as the window lists it.
type CMEntry struct {
	ID, Parent, Category, Cost int
	Owned, Known               bool
	Name                       string
}

// CMNode is one rank opened.
type CMNode struct {
	CMEntry
	Level, Spent int
	MinStats     [7]int
	Rank         int
	Title, Text  string
	Mods         []CMMod
	Abilities    []CMEntryLine // adds (Add true) and removes or needs
	Masteries    []string      // what it still needs, said
}

// CMMod is a stat a rank modifies: its slot and by how much.
type CMMod struct {
	Slot, Value int
}

// CMEntryLine is an ability line of an opened rank.
type CMEntryLine struct {
	Text string
	Add  bool
}

// CMState is what the dump's mastery block and a purchase's reply carry
// besides the points: the run speed, the 29 stats the masteries add and the
// seven caps they raise.
type CMState struct {
	Speed   float32
	Stats   [CMSlots]int
	Bonuses [7]int
}

func (p *Player) cmStateLocked() CMState {
	st := CMState{Speed: p.SpeedNow(), Stats: p.CMEffects}
	copy(st.Bonuses[:], p.CMEffects[cmSlotCap:cmSlotCap+7])
	return st
}

// CMStateOf is the session's player's CMState (the MemoryDump's).
func (w *World) CMStateOf(sess *session.Session) (CMState, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CMState{}, false
	}
	return p.cmStateLocked(), true
}

// CMStateOfPlayer is p's CMState (the MemoryDump writer holds the player).
func CMStateOfPlayer(p *Player) CMState { return p.cmStateLocked() }

// CMKnows is what a rank's prerequisites look at that the World does not
// keep, each by id: the spells in the character's book, the quests it has
// finished, and the names of the quests in the game (the mastery window's
// handler reads them, the book off the database and the quests off the quest
// engine, before it asks the World).
type CMKnows struct {
	Book, Quests map[int]bool
	QuestNames   map[int]string
}

// cmBlockLocked is why the player cannot buy the rank now, "" when it can
// (the points aside): what makes it "known" in the window.
func (w *World) cmBlockLocked(p *Player, r *CMRank, k CMKnows) string {
	if _, ok := p.CMs[r.ID]; ok {
		return "You already have that mastery."
	}
	if r.Parent != 0 {
		if _, ok := p.CMs[r.Parent]; !ok {
			name := "the rank before it"
			if pr := w.cms.byID[r.Parent]; pr != nil {
				name = pr.Name
			}
			return fmt.Sprintf("You must have %s first.", name)
		}
	}
	for _, id := range r.RequireRanks {
		if o := w.cms.byID[id]; o != nil {
			if _, ok := p.CMs[id]; !ok {
				return fmt.Sprintf("You must have %s first.", o.Name)
			}
		}
	}
	if p.Level < r.Level {
		return fmt.Sprintf("You must be level %d to buy %s.", r.Level, r.Name)
	}
	for _, id := range r.RequireQuests {
		// a quest not in the game (a draft, or none) bars no one, as a trip
		// a draft unlocks does not (questtrips.go): no one can finish it, and
		// a GM taking a quest out locks no rank for good
		if name, ok := k.QuestNames[id]; ok && !k.Quests[id] {
			return fmt.Sprintf("You must finish %s to buy %s.", name, r.Name)
		}
	}
	if p.Mastery.Spent < r.Spent {
		return fmt.Sprintf("You must have spent %d mastery points to buy %s.", r.Spent, r.Name)
	}
	base := [7]int{p.BaseStr, p.BaseSta, p.BaseAgi, p.BaseDex, p.BaseWis, p.BaseInt, p.BaseCha}
	for i, need := range r.MinStats {
		if need > 0 && base[i]+p.CMEffects[i] < need {
			return fmt.Sprintf("Your %s must be at least %d to buy %s.", statNames[i], need, r.Name)
		}
	}
	for _, a := range r.Requires {
		if a.SpellID > 0 && !k.Book[a.SpellID] { // a name never found is not checked
			return fmt.Sprintf("You must know %s to buy %s.", a.Name, r.Name)
		}
	}
	if r.Category == CMCategoryMaster {
		for id := range p.CMs {
			if o := w.cms.byID[id]; o != nil && o.Category == CMCategoryMaster && o.Mastery != r.Mastery {
				return fmt.Sprintf("You can possess only one Master Class at a time: remove %s first.", o.Mastery)
			}
		}
	}
	return ""
}

// CMList is the window's list for the session's player: every rank its
// class and race are offered, owned and known flags its own.
func (w *World) CMList(sess *session.Session, k CMKnows) []CMEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.cms == nil {
		return nil
	}
	var out []CMEntry
	for _, r := range w.cms.order {
		if !r.offered(p) {
			continue
		}
		_, owned := p.CMs[r.ID]
		out = append(out, CMEntry{ID: r.ID, Parent: r.Parent, Category: r.Category, Cost: r.Cost,
			Owned: owned, Known: !owned && w.cmBlockLocked(p, r, k) == "", Name: r.Name})
	}
	w.cmListParentsLocked(out)
	return out
}

// CMOpenedBy is the names of the ranks a quest just finished opens for the
// session's player, in the catalog's order: those offered to it and not owned
// that need the quest (RequireQuests) and now have every quest they need in
// the game finished (k's), whatever else they still need.
func (w *World) CMOpenedBy(sess *session.Session, questID int, k CMKnows) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.cms == nil {
		return nil
	}
	var out []string
	for _, r := range w.cms.order {
		if !r.offered(p) || !slices.Contains(r.RequireQuests, questID) {
			continue
		}
		if _, owned := p.CMs[r.ID]; owned {
			continue
		}
		open := true
		for _, id := range r.RequireQuests {
			if _, ok := k.QuestNames[id]; ok && !k.Quests[id] {
				open = false
			}
		}
		if open {
			out = append(out, r.Name)
		}
	}
	return out
}

// cmListParentsLocked sets the parent key each master class's ability is
// listed with. A purchase checks the catalog's parent (cmBlockLocked); the
// window reads the listed one only to sort a category into chains (decomp
// viwndclassmastery.cpp, SelectCategory and Chain: a skill whose parent is
// no listed skill's key, then one at a time the first whose parent is the
// last one's key; Owned shows each chain's LAST skill, Available each
// chain's FIRST, "???" and "Locked" unless known). The keys come with the
// list, at the window's opening (Recv08), and stay through a purchase
// (Recv05 brings the flags alone). A master class's abilities need only the
// class, its first rank (tools/seed_masteries.py; F3 20261026-222613), and
// are listed:
//   - the class owned: with no parent (0), so Owned shows the class and every
//     ability bought, and Available every one to buy; one improving another
//     (its parent another ability, Immortal Guard Guard's) keeps it, shown in
//     its place once owned;
//   - the class buyable now: as the catalog has them, so once it is bought in
//     this window every ability is a chain of its own (each ability past the
//     first a "???" of its own until then);
//   - else: one after another in rank order, the first under the class, so a
//     locked master class is one "???", as when its ranks were a chain.
func (w *World) cmListParentsLocked(out []CMEntry) {
	at := make(map[int]int, len(out))
	for i, e := range out {
		at[e.ID] = i
	}
	abilities := map[int][]int{} // a master class's first rank: its abilities listed
	for i, e := range out {
		if e.Category != CMCategoryMaster || e.Parent == 0 {
			continue
		}
		if tag := w.cmMasterClassOf(e.ID); tag != 0 {
			if _, ok := at[tag]; ok {
				abilities[tag] = append(abilities[tag], i)
			}
		}
	}
	for tag, idx := range abilities {
		switch class := out[at[tag]]; {
		case class.Owned:
			for _, i := range idx {
				if out[i].Parent == tag {
					out[i].Parent = 0
				}
			}
		case class.Known:
		default:
			sort.SliceStable(idx, func(a, b int) bool {
				ra, rb := w.cms.byID[out[idx[a]].ID], w.cms.byID[out[idx[b]].ID]
				return ra.Rank < rb.Rank || ra.Rank == rb.Rank && ra.ID < rb.ID
			})
			prev := tag
			for _, i := range idx {
				out[i].Parent, prev = prev, out[i].ID
			}
		}
	}
}

// cmMasterClassOf is the first rank of the master class a rank is of: its
// parents followed to one with none, every one a Master Class rank; 0 when
// not, or past a loop a GM's rows could make.
func (w *World) cmMasterClassOf(id int) int {
	for range 16 {
		r := w.cms.byID[id]
		if r == nil || r.Category != CMCategoryMaster {
			return 0
		}
		if r.Parent == 0 {
			return id
		}
		id = r.Parent
	}
	return 0
}

// CMNodeOf is one rank opened for the session's player; false when it is
// not offered to it.
func (w *World) CMNodeOf(sess *session.Session, id int, k CMKnows) (CMNode, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil || w.cms == nil {
		return CMNode{}, false
	}
	r := w.cms.byID[id]
	if r == nil || !r.offered(p) {
		return CMNode{}, false
	}
	_, owned := p.CMs[r.ID]
	block := w.cmBlockLocked(p, r, k)
	n := CMNode{CMEntry: CMEntry{ID: r.ID, Parent: r.Parent, Category: r.Category, Cost: r.Cost,
		Owned: owned, Known: !owned && block == "", Name: r.Name},
		Level: r.Level, Spent: r.Spent, MinStats: r.MinStats, Rank: r.Rank, Title: r.Name}
	n.Text = r.Description
	if n.Text == "" {
		n.Text = r.Mastery
	}
	n.Text += fmt.Sprintf(" Cost: %d.", r.Cost)
	if !owned && block != "" {
		n.Text += " " + block
	}
	for i, v := range r.Effects {
		if v != 0 {
			n.Mods = append(n.Mods, CMMod{Slot: i, Value: v})
		}
	}
	for _, a := range r.Adds {
		n.Abilities = append(n.Abilities, CMEntryLine{Text: a.Name, Add: true})
	}
	seen := map[string]bool{}
	for _, a := range r.Removes {
		seen[strings.ToLower(a.Name)] = true
		n.Abilities = append(n.Abilities, CMEntryLine{Text: a.Name})
	}
	for _, a := range r.Requires {
		if !seen[strings.ToLower(a.Name)] {
			n.Abilities = append(n.Abilities, CMEntryLine{Text: a.Name})
		}
	}
	if r.Parent != 0 {
		if _, ok := p.CMs[r.Parent]; !ok {
			if pr := w.cms.byID[r.Parent]; pr != nil {
				n.Masteries = append(n.Masteries, pr.Name)
			}
		}
	}
	for _, id := range r.RequireRanks {
		if o := w.cms.byID[id]; o != nil {
			if _, ok := p.CMs[id]; !ok {
				n.Masteries = append(n.Masteries, o.Name)
			}
		}
	}
	return n, true
}

// CMResult is what a purchase or a removal did: the refusal when it did
// nothing, else the rank, the points now, the stats for the reply, and the
// ranks it took away (a removal).
type CMResult struct {
	Refusal string
	Rank    *CMRank
	Paid    int
	Mastery Mastery
	State   CMState
	Removed []int
}

// BuyCM buys a rank for the session's player: its points and ranks changed
// and its stats again; the caller keeps it and grants the abilities.
func (w *World) BuyCM(sess *session.Session, id int, k CMKnows) CMResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CMResult{Refusal: "You are not in the world."}
	}
	if w.cms == nil {
		return CMResult{Refusal: "No mastery can be bought here."}
	}
	r := w.cms.byID[id]
	if r == nil || !r.offered(p) {
		return CMResult{Refusal: "That mastery is not offered to you."}
	}
	if why := w.cmBlockLocked(p, r, k); why != "" {
		return CMResult{Refusal: why, Rank: r}
	}
	if p.Mastery.Points < r.Cost {
		return CMResult{Refusal: fmt.Sprintf("You need %d mastery points to buy %s: you have %d.", r.Cost, r.Name, p.Mastery.Points), Rank: r}
	}
	if p.CMs == nil {
		p.CMs = map[int]int{}
	}
	p.Mastery.Points -= r.Cost
	p.Mastery.Spent += r.Cost
	p.CMs[r.ID] = r.Cost
	w.cmChangedLocked(p)
	log.Printf("Mastery: %s bought %s (%d) for %d: %s", p.Name, r.Name, r.ID, r.Cost, masteryLine(p.Mastery))
	return CMResult{Rank: r, Paid: r.Cost, Mastery: p.Mastery, State: p.cmStateLocked()}
}

// RemoveCM removes a rank the session's player has, giving nothing back
// (the file's doc): its points leave the spent ones, so the character holds
// that many fewer and earns them again. Not one another of its ranks
// follows, nor one a rank it keeps needs the points spent of. Paid is what
// the rank cost.
func (w *World) RemoveCM(sess *session.Session, id int) CMResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CMResult{Refusal: "You are not in the world."}
	}
	paid, ok := p.CMs[id]
	if !ok {
		return CMResult{Refusal: "You do not have that mastery."}
	}
	var r *CMRank
	if w.cms != nil {
		r = w.cms.byID[id]
	}
	name := fmt.Sprintf("mastery %d", id)
	if r != nil {
		name = r.Name
	}
	// a later rank of it first, and a rank of another mastery that needs it
	for oid := range p.CMs {
		if o := w.cmRank(oid); o != nil && (o.Parent == id || slices.Contains(o.RequireRanks, id)) {
			return CMResult{Refusal: fmt.Sprintf("Remove %s before %s.", o.Name, name), Rank: r}
		}
	}
	spent := p.Mastery.Spent - paid
	for oid, opaid := range p.CMs {
		if oid == id {
			continue
		}
		if o := w.cmRank(oid); o != nil && o.Spent > 0 && spent-opaid < o.Spent {
			return CMResult{Refusal: fmt.Sprintf("%s needs %d mastery points spent: remove it before %s.", o.Name, o.Spent, name), Rank: r}
		}
	}
	delete(p.CMs, id)
	p.Mastery.Spent = max(0, spent)
	w.cmChangedLocked(p)
	log.Printf("Mastery: %s removed %s (%d), its %d points gone: %s", p.Name, name, id, paid, masteryLine(p.Mastery))
	return CMResult{Rank: r, Paid: paid, Mastery: p.Mastery, State: p.cmStateLocked(), Removed: []int{id}}
}

// CMRankByID is a rank of the catalog (nil when none). The catalog is never
// changed in place (a GM's edit loads a new one), so the rank may be read
// without the lock.
func (w *World) CMRankByID(id int) *CMRank {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cmRank(id)
}

// CMSubject is whose masteries a GM's #cm and #cmreset are about: the player
// the GM targets, else the GM.
func (w *World) CMSubject(sess *session.Session, objectID uint32) *session.Session {
	w.mu.Lock()
	defer w.mu.Unlock()
	if objectID != 0 && objectID != 0xFFFFFFFF && w.SessionManager != nil {
		if ent, ok := w.findEntityByObjectIDLocked(sess, objectID); ok {
			if t, ok := ent.(*Player); ok {
				if s := w.findSessionByEntityIDLocked(t.ServerID); s != nil {
					return s
				}
			}
		}
	}
	return sess
}

// ReloadCMs puts a character's ranks, as charcms holds them now, on it where
// it stands (a GM's edit of its rows in the panel): its name, and false when
// it is not in the world.
func (w *World) ReloadCMs(serverID uint32, owned []database.OwnedCM) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.playerByCharacterLocked(serverID)
	if p == nil {
		return "", false
	}
	p.CMs = make(map[int]int, len(owned))
	for _, o := range owned {
		p.CMs[o.ID] = o.Cost
	}
	w.cmChangedLocked(p)
	return p.Name, true
}

func (w *World) cmRank(id int) *CMRank {
	if w.cms == nil {
		return nil
	}
	return w.cms.byID[id]
}

// cmChangedLocked puts the player's ranks on its stats: the effects summed,
// the stats again within the new caps, a new run speed told.
func (w *World) cmChangedLocked(p *Player) {
	speed := p.SpeedNow()
	p.refreshCMLocked(w.cms)
	p.RecalcStats(false)
	p.StatsDirty = true
	if s := p.SpeedNow(); s != speed && w.SessionManager != nil {
		if sess := w.findSessionByEntityIDLocked(p.ServerID); sess != nil {
			sendPlayerSpeed(sess, s)
		}
	}
}

// ResetCMs takes every rank from the session's player and gives back what
// they cost (#cmreset): the ranks taken.
func (w *World) ResetCMs(sess *session.Session) CMResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return CMResult{Refusal: "You are not in the world."}
	}
	var ids []int
	back := 0
	for id, paid := range p.CMs {
		ids = append(ids, id)
		back += paid
	}
	sort.Ints(ids)
	p.CMs = map[int]int{}
	p.Mastery.Points += back
	p.Mastery.Spent = max(0, p.Mastery.Spent-back)
	w.cmChangedLocked(p)
	return CMResult{Paid: back, Mastery: p.Mastery, State: p.cmStateLocked(), Removed: ids}
}

// CMLines is a character's ranks for a GM, by category and name.
func (w *World) CMLines(sess *session.Session) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	p := w.sessionPlayerLocked(sess)
	if p == nil {
		return nil
	}
	cats := [5]string{"general", "racial", "archetype", "class", "master class"}
	var names []string
	for id, paid := range p.CMs {
		r := w.cmRank(id)
		if r == nil {
			names = append(names, fmt.Sprintf("#%d (not in the catalog, %d paid)", id, paid))
			continue
		}
		cat := "?"
		if r.Category >= 0 && r.Category < 5 {
			cat = cats[r.Category]
		}
		names = append(names, fmt.Sprintf("%s (%s, %d)", r.Name, cat, paid))
	}
	sort.Strings(names)
	out := []string{fmt.Sprintf("%s: %d masteries; %s", p.Name, len(p.CMs), masteryLine(p.Mastery))}
	if len(names) > 0 {
		out = append(out, strings.Join(names, ", "))
	}
	var gives []string
	for i, v := range p.CMEffects {
		if v != 0 {
			gives = append(gives, fmt.Sprintf("%s %+d", cmSlotNames[i], v))
		}
	}
	if len(gives) > 0 {
		out = append(out, "They give: "+strings.Join(gives, ", "))
	}
	return out
}
