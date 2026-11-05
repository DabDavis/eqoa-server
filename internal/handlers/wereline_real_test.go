package handlers

import (
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// TestRealWereLines plays the five were lines (wolf, bear, rat, lion, gator)
// on a real server's database (EQOA_REAL_DB names it; a copy is played) as
// players play them, where TestRealQuestChainsWalk walks the engine alone
// (its NPCs made beside the player, its standings given):
//
//   - the World is the server's own at its start: every npcs row loaded
//     (LoadNPCs), the factions and their starts, the aggro rules, the NPCs
//     kept out of the world as originals to copy (npcspawnonly);
//   - a new character of each archetype (a troll warrior, a dark elf rogue, an
//     erudite wizard, a dwarf cleric) at the line's first level, its standings
//     its race's and class's start: none is given;
//   - each quest is taken by talking to its giver, the NPC of that name the
//     World holds, and only when the giver offers it (the engine's own pick:
//     its levels, classes, quests first and standings);
//   - a talk, show or give step is done at the World's NPC of its name, the
//     player stood beside it; a kill or loot step's kills are the World's own
//     creatures of the step's names (a step's spawns first), each attackable
//     (no NPC bit 0x80, as a player's attack is refused one), killed by the
//     GM's kill (World.KillNPCByObjectID), so the kill's faction hits and its
//     credit are any kill's; a creature killed comes back at once (the
//     corpse's time aside); a loot step's drops are rolled at its chance
//     (EQOA_WERE_SEED, else 1) and taken off the corpse; a reach step's place
//     is stood on;
//   - the tail quest is handed in again until its giver offers the
//     infection, which must come by the tenth; then the infection, the form
//     it teaches (a spell in the book), its standings; then at level 40 the
//     archetype's level-40 quest and its spell.
//
// Each line's report says how many of the tail quest's creatures the World
// holds and at what levels, the kills each hand-in took, the standings, the
// spells taught, and what blocked it, if anything (TestRealWereLinesPlaces
// says where every NPC the lines name stands). EQOA_WERE_LINES (wolfwere,
// bearwere, ratwere, lionwere, gatorwere, comma separated) plays fewer.
func TestRealWereLines(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	lines := []string{"wolfwere", "bearwere", "ratwere", "lionwere", "gatorwere"}
	if v := os.Getenv("EQOA_WERE_LINES"); v != "" {
		lines = strings.Split(v, ",")
	}
	seed := int64(1)
	if v := os.Getenv("EQOA_WERE_SEED"); v != "" {
		fmt.Sscan(v, &seed)
	}
	log.SetOutput(io.Discard) // the World's and the engine's log lines are many thousands
	defer log.SetOutput(os.Stderr)
	r := newWereRig(t, src, seed)
	defer r.db.Close()
	for _, line := range lines {
		line := strings.TrimSpace(line)
		t.Run(line, func(t *testing.T) {
			for _, a := range wereArchetypes {
				r.playLine(t, line, a)
			}
		})
	}
}

// wereArchetype is a character of one archetype the lines are played with.
type wereArchetype struct {
	name        string
	class, race int
}

var wereArchetypes = []wereArchetype{
	{"tank", 0, 5},    // troll warrior
	{"melee", 6, 2},   // dark elf rogue
	{"caster", 13, 8}, // erudite wizard
	{"healer", 9, 4},  // dwarf cleric
}

// wereRig is the World and the quest engine on a copy of a real database.
type wereRig struct {
	t       *testing.T
	db      *database.SQLiteDB
	gw      *entity.World
	mgr     *session.Manager
	qe      *QuestEngine
	inv     *database.InventoryStore
	spells  *database.SpellbookStore
	rng     *rand.Rand
	credits []entity.KillCredit
	drops   []int
	chars   int
	ids     map[string]int // quest ids by import key
}

func newWereRig(t *testing.T, src string, seed int64) *wereRig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "were.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	db, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`PRAGMA synchronous = OFF`)
	db.Exec(`PRAGMA journal_mode = MEMORY`)
	// the World as cmd/gameserver makes it, what the quests touch
	gw := entity.NewWorld()
	npcStore := database.NewNPCStore(db)
	if m, err := npcStore.PlayerModels(); err == nil {
		gw.SetPlayerModels(m)
	}
	if m, err := npcStore.RaceModels(); err == nil {
		gw.SetRaceModels(m)
	}
	if m, err := npcStore.ModelSexes(); err == nil {
		gw.SetModelSexes(m)
	}
	if m, err := npcStore.ModelRaces(); err == nil {
		gw.SetModelRaces(m)
	}
	gw.LoadNPCs(npcStore)
	if err := gw.LoadFactions(db); err != nil {
		t.Fatal(err)
	}
	if _, err := gw.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gw.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	mgr := session.NewManager()
	gw.SessionManager = mgr
	inv := database.NewInventoryStore(db)
	spells := database.NewSpellbookStore(db)
	qe, err := NewQuestEngine(db, gw, mgr, inv, spells)
	if err != nil {
		t.Fatal(err)
	}
	r := &wereRig{t: t, db: db, gw: gw, mgr: mgr, qe: qe, inv: inv, spells: spells,
		rng: rand.New(rand.NewSource(seed)), ids: map[string]int{}}
	qe.roll = r.rng.Float64
	qe.drop = func(_ entity.EntityID, _ time.Time, _ uint32, rec database.CharInventoryEntry) {
		r.drops = append(r.drops, rec.Pattern.PatternID)
	}
	// the World's kill credits, as the engine's queue hears them (the
	// World's lock is held: only kept here)
	gw.OnKill(func(k entity.KillCredit) { r.credits = append(r.credits, k) })
	rows, err := db.Query(`SELECT key, questid FROM quest_import_quests WHERE key LIKE 'Weres/%'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k string
		var id int
		rows.Scan(&k, &id)
		r.ids[k] = id
	}
	rows.Close()
	return r
}

// wereChar is one character playing a line, and what it saw.
type wereChar struct {
	w      *questWalker // the walk's box answering and progress, on this World
	sess   *session.Session
	p      *entity.Player
	report []string
}

func (c *wereChar) say(format string, args ...any) {
	c.report = append(c.report, fmt.Sprintf(format, args...))
}

// newChar makes a new character of the archetype at a level, in the World.
func (r *wereRig) newChar(a wereArchetype, level int) *wereChar {
	r.t.Helper()
	r.chars++
	n := r.chars
	rec := &database.CharacterRecord{CharName: fmt.Sprintf("Were%s%d", a.name, n), AccountID: 1, TClass: a.class, Race: a.race}
	if err := database.NewSQLiteCharacterStore(r.db).Create(rec); err != nil {
		r.t.Fatal(err)
	}
	s := r.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, 8), Port: 7100 + n}, uint32(1900+n), 1, 1, 2, true)
	p := &entity.Player{ServerID: entity.EntityID(60000 + n), CharServerID: rec.ServerID, Name: rec.CharName,
		Class: a.class, Race: a.race, Level: level, HP: 500000, MaxHP: 500000, X: 25000, Y: 50, Z: 15000}
	r.gw.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
	s.SetInventoryList(newInvList(nil))
	r.qe.LoggedIn(rec.ServerID)
	w := &questWalker{t: r.t, db: r.db, gw: r.gw, mgr: r.mgr, qe: r.qe, inv: r.inv, sess: s, p: p,
		npcs: map[string]*entity.NPC{}, byCombo: map[[2]int]*walkChar{}, fit: map[[3]int]bool{}}
	return &wereChar{w: w, sess: s, p: p}
}

// named is the World's NPCs of a name, alive or dead: in the world (not an
// original kept out of it), the copies a quest spawned for this character
// first, then by distance from the player.
func (r *wereRig) named(c *wereChar, names ...string) []*entity.NPC {
	var mine map[entity.EntityID]bool
	r.qe.mu.Lock()
	for _, s := range r.qe.spawns[c.p.CharServerID] {
		if mine == nil {
			mine = map[entity.EntityID]bool{}
		}
		mine[s.id] = true
	}
	r.qe.mu.Unlock()
	var out []*entity.NPC
	for _, n := range r.gw.NPCs {
		if n.SpawnOnly || (n.Temporary && !mine[n.ServerID]) {
			continue
		}
		for _, name := range names {
			if strings.EqualFold(strings.TrimSpace(n.Name), strings.TrimSpace(name)) {
				out = append(out, n)
				break
			}
		}
	}
	d := func(n *entity.NPC) float64 {
		if mine[n.ServerID] {
			return -1
		}
		dx, dz := float64(n.SpawnX-c.p.X), float64(n.SpawnZ-c.p.Z)
		if n.World != c.p.World {
			return 1e12 + dx*dx + dz*dz
		}
		return dx*dx + dz*dz
	}
	sort.Slice(out, func(i, j int) bool {
		if di, dj := d(out[i]), d(out[j]); di != dj {
			return di < dj
		}
		return out[i].ServerID < out[j].ServerID
	})
	return out
}

// standBeside puts the player by an NPC, in its world.
func (r *wereRig) standBeside(c *wereChar, n *entity.NPC) {
	c.p.World = n.World
	r.gw.UpdatePosition(c.p.ServerID, n.X+1, n.Y, n.Z+1, 0)
}

// kill kills one NPC as the GM's kill does (its faction hits, its credit) and
// hands the engine the credits, as its queue would; the creature comes back.
func (r *wereRig) kill(c *wereChar, n *entity.NPC) string {
	if n.NPCType&0x80 != 0 {
		return fmt.Sprintf("%q (%d) has the NPC bit (npc_type %d): a player's attack is refused", n.Name, n.ServerID, n.NPCType)
	}
	r.standBeside(c, n)
	vs := entity.GetOrCreateVisibility(c.sess)
	vs.ChannelEntity[0], vs.ChannelObjectID[0] = n.ServerID, 100001
	r.credits, r.drops = nil, nil
	if _, ok := r.gw.KillNPCByObjectID(c.sess, 100001); !ok {
		return fmt.Sprintf("%q (%d) could not be killed", n.Name, n.ServerID)
	}
	credits := r.credits
	r.credits = nil
	for _, k := range credits {
		r.qe.killed(k)
	}
	for _, d := range r.drops {
		pat, err := r.qe.items.GetItemPattern(d)
		if err != nil || pat == nil {
			return fmt.Sprintf("its drop %d: %v", d, err)
		}
		if got, err := GiveItemNow(c.sess, r.gw, r.inv, *pat, 1); err != nil || got == 0 {
			return fmt.Sprintf("its drop %q not taken (%v)", pat.ItemName, err)
		}
		r.qe.Looted(c.sess, c.p, d, 1)
	}
	if !n.Temporary {
		n.Dead, n.HP = false, n.MaxHP // back on its spawn (its corpse's time aside)
	}
	return ""
}

// talkTo talks to the living NPC of a name nearest the player, stood beside
// it, and answers what it asks (a quest it offers taken, a step's answer
// given): the NPC, or why there is none.
func (r *wereRig) talkTo(c *wereChar, name string) (*entity.NPC, string) {
	ns := r.named(c, name)
	var live []*entity.NPC
	for _, n := range ns {
		if !n.Dead {
			live = append(live, n)
		}
	}
	if len(live) == 0 {
		return nil, fmt.Sprintf("no NPC %q in the world (%d rows of it out of it or dead)", name, len(ns))
	}
	n := live[0]
	r.standBeside(c, n)
	questMsgs(c.sess)
	r.qe.Talk(c.sess, c.p, n)
	c.w.settle()
	return n, ""
}

// lineQuest is one of a line's quests by its key's last part.
func (r *wereRig) lineQuest(line, part string) *database.Quest {
	id := r.ids["Weres/"+line+"/"+part]
	if id == 0 {
		return nil
	}
	r.qe.mu.Lock()
	defer r.qe.mu.Unlock()
	return r.qe.cat.byID[id]
}

// why is why the character may not take the quest now ("" it may).
func (r *wereRig) why(c *wereChar, qu *database.Quest) string {
	r.qe.mu.Lock()
	defer r.qe.mu.Unlock()
	return r.qe.mayTakeLocked(r.qe.lookAt(c.sess), c.p, r.qe.charLocked(c.p.CharServerID), qu)
}

func (r *wereRig) standing(c *wereChar, faction string) int {
	v, _ := r.gw.Standing(c.sess, faction)
	return v
}

// take talks to the quest's giver and checks the quest it offered is qu.
func (r *wereRig) take(c *wereChar, qu *database.Quest) string {
	if why := r.why(c, qu); why != "" {
		return fmt.Sprintf("%q is not to be taken: %s", qu.Name, why)
	}
	_, before := c.w.progress(qu.ID)
	n, miss := r.talkTo(c, qu.Giver)
	if miss != "" {
		return miss
	}
	if st, done := c.w.progress(qu.ID); st <= 0 && done == before {
		return fmt.Sprintf("%s at (%.0f, %.0f) did not offer %q (it said: %s)", qu.Giver, n.X, n.Z, qu.Name, c.w.lastWords())
	}
	return ""
}

// play does the quest's steps once taken, as a player does them; it counts
// the kills.
func (r *wereRig) play(c *wereChar, qu *database.Quest) (kills int, why string) {
	_, before := c.w.progress(qu.ID)
	for guard := 0; guard < 400; guard++ {
		step, done := c.w.progress(qu.ID)
		if done > before && step == 0 {
			return kills, ""
		}
		st := qu.Step(step)
		if st == nil {
			return kills, fmt.Sprintf("at step %d, which it lacks", step)
		}
		switch st.Kind {
		case database.StepTalk, database.StepShow, database.StepGive:
			if st.Kind != database.StepTalk {
				r.qe.mu.Lock()
				lack := r.qe.lackingLocked(c.p, st)
				r.qe.mu.Unlock()
				if lack != "" {
					return kills, fmt.Sprintf("step %d (%s %q): the character does not hold %s", st.Step, st.Kind, st.Target, lack)
				}
			}
			n, miss := r.talkTo(c, st.Target)
			if miss != "" {
				return kills, fmt.Sprintf("step %d (%s): %s", st.Step, st.Kind, miss)
			}
			if s2, d2 := c.w.progress(qu.ID); s2 == step && d2 == done {
				return kills, fmt.Sprintf("step %d (%s %q at (%.0f, %.0f)) not done: %s", st.Step, st.Kind, st.Target, n.X, n.Z, c.w.lastWords())
			}
		case database.StepKill, database.StepLoot:
			names := append([]string{st.Target}, st.Also...)
			ns := r.named(c, names...)
			if len(ns) == 0 {
				return kills, fmt.Sprintf("step %d (%s %q): no creature of its names in the world", st.Step, st.Kind, strings.Join(names, ", "))
			}
			var n *entity.NPC
			for _, x := range ns {
				if !x.Dead {
					n = x
					break
				}
			}
			if n == nil {
				return kills, fmt.Sprintf("step %d (%s %q): every one dead", st.Step, st.Kind, st.Target)
			}
			if miss := r.kill(c, n); miss != "" {
				return kills, fmt.Sprintf("step %d (%s): %s", st.Step, st.Kind, miss)
			}
			kills++
			if kills > 300 {
				return kills, fmt.Sprintf("step %d (%s %q): 300 kills and not done", st.Step, st.Kind, st.Target)
			}
		case database.StepReach:
			c.p.World = uint8(st.World)
			r.gw.UpdatePosition(c.p.ServerID, float32(st.X), float32(st.Y), float32(st.Z), 0)
			r.qe.checkReach()
			c.w.settle()
			if s2, d2 := c.w.progress(qu.ID); s2 == step && d2 == done {
				return kills, fmt.Sprintf("step %d (reach %d (%.0f, %.0f)) not done", st.Step, st.World, st.X, st.Z)
			}
		default:
			return kills, fmt.Sprintf("step %d: kind %q", st.Step, st.Kind)
		}
		c.w.settle()
	}
	return kills, "too many steps"
}

// book is the character's spells by name.
func (r *wereRig) book(c *wereChar) map[string]bool {
	out := map[string]bool{}
	es, _ := r.spells.GetCharacterSpells(c.p.CharServerID)
	for _, e := range es {
		out[e.Pattern.SpellName] = true
	}
	return out
}

// spellsOf is a step's reward spells (the class's too) by name.
func (r *wereRig) spellsOf(c *wereChar, qu *database.Quest) []string {
	var out []string
	for _, st := range qu.Steps {
		rw := st.RewardFor(c.p.Class)
		for _, id := range rw.Spells {
			var name string
			r.db.QueryRow(`SELECT spellname FROM spellpattern WHERE spellid = ?`, id).Scan(&name)
			out = append(out, fmt.Sprintf("%s (%d)", name, id))
		}
	}
	return out
}

// where is a place as the report gives it.
func where(n *entity.NPC) string {
	return fmt.Sprintf("w%d (%.0f, %.0f) L%d type %d", n.World, n.X, n.Z, n.Level, n.NPCType)
}

// playLine plays one line with one archetype's new character and reports it.
func (r *wereRig) playLine(t *testing.T, line string, a wereArchetype) {
	p1, p2, p3 := r.lineQuest(line, "p1"), r.lineQuest(line, "p2"), r.lineQuest(line, "p3")
	l40 := r.lineQuest(line, "L40-"+a.name)
	if p1 == nil || p2 == nil || p3 == nil || l40 == nil {
		t.Errorf("%s: a quest of the line is missing (p1 %v, p2 %v, p3 %v, L40-%s %v)", line, p1 != nil, p2 != nil, p3 != nil, a.name, l40 != nil)
		return
	}
	for _, qu := range []*database.Quest{p1, p2, p3, l40} {
		if !qu.Enabled {
			t.Errorf("%s: %q (%d) is a draft", line, qu.Name, qu.ID)
		}
	}
	c := r.newChar(a, max(p1.MinLevel, 1))
	defer func() {
		for _, l := range c.report {
			t.Logf("%s/%s: %s", line, a.name, l)
		}
	}()
	fail := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		c.say("BLOCKED: %s", msg)
		t.Errorf("%s (%s, class %d race %d): %s", line, a.name, a.class, a.race, msg)
	}
	soc := ""
	for _, fr := range p3.FactionReqs {
		if fr.Min >= 1000 {
			soc = fr.Faction
		}
	}
	c.say("new character class %d race %d level %d: %s %d, Werehunters of Tunaria %d", a.class, a.race, c.p.Level,
		soc, r.standing(c, soc), r.standing(c, "Werehunters of Tunaria"))
	// p1
	if why := r.take(c, p1); why != "" {
		fail("p1: %s", why)
		return
	}
	k, why := r.play(c, p1)
	if why != "" {
		fail("p1 %q: %s", p1.Name, why)
		return
	}
	c.say("p1 %q done (%d kills)", p1.Name, k)
	// p2 again and again until the giver offers p3
	tail := p2.Steps[0]
	ns := r.named(c, append([]string{tail.Target}, tail.Also...)...)
	attack := 0
	lv := map[int]int{}
	for _, n := range ns {
		if n.NPCType&0x80 == 0 {
			attack++
		}
		lv[n.Level]++
	}
	c.say("p2's loot step: %q%s at chance %.2f for %d of item %v; the World holds %d of them (%d attackable), levels %v",
		tail.Target, alsoText(tail.Also), tail.Chance, tail.Count, tail.Items, len(ns), attack, lv)
	hand := 0
	for hand < 10 {
		if r.why(c, p3) == "" {
			break
		}
		if why := r.take(c, p2); why != "" {
			fail("p2 hand-in %d: %s", hand+1, why)
			return
		}
		k, why := r.play(c, p2)
		if why != "" {
			fail("p2 %q hand-in %d: %s", p2.Name, hand+1, why)
			return
		}
		hand++
		c.say("p2 hand-in %d: %d kills; %s now %d", hand, k, soc, r.standing(c, soc))
	}
	if why := r.why(c, p3); why != "" {
		fail("p3 not offered after %d tail hand-ins: %s", hand, why)
		return
	}
	// the giver's next offer is the infection
	if why := r.take(c, p3); why != "" {
		fail("p3: %s", why)
		return
	}
	k, why = r.play(c, p3)
	if why != "" {
		fail("p3 %q: %s", p3.Name, why)
		return
	}
	book := r.book(c)
	var got, lack []string
	for _, s := range r.spellsOf(c, p3) {
		name := s[:strings.LastIndex(s, " (")]
		if book[name] {
			got = append(got, s)
		} else {
			lack = append(lack, s)
		}
	}
	c.say("p3 %q done (%d kills) after %d hand-ins: taught %v; %s %d, Werehunters of Tunaria %d", p3.Name, k, hand,
		got, soc, r.standing(c, soc), r.standing(c, "Werehunters of Tunaria"))
	if len(got) == 0 || len(lack) > 0 {
		fail("p3's form not in the book: %v", lack)
	}
	// the level-40 quest of the archetype
	c.p.Level = max(l40.MinLevel, 40)
	if why := r.take(c, l40); why != "" {
		fail("L40-%s: %s", a.name, why)
		return
	}
	k, why = r.play(c, l40)
	if why != "" {
		fail("L40-%s %q: %s", a.name, l40.Name, why)
		return
	}
	book = r.book(c)
	got, lack = nil, nil
	for _, s := range r.spellsOf(c, l40) {
		name := s[:strings.LastIndex(s, " (")]
		if book[name] {
			got = append(got, s)
		} else {
			lack = append(lack, s)
		}
	}
	c.say("L40-%s %q done (%d kills): taught %v", a.name, l40.Name, k, got)
	if len(got) == 0 || len(lack) > 0 {
		fail("L40-%s's spell not in the book: %v", a.name, lack)
	}
}

func alsoText(also []string) string {
	if len(also) == 0 {
		return ""
	}
	return " (and " + strings.Join(also, ", ") + ")"
}

// TestRealWereLinesPlaces says where every NPC the were lines name stands in
// the World, and which are missing, kept out of the world, or have the NPC
// bit where a kill needs none (EQOA_REAL_DB).
func TestRealWereLinesPlaces(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	r := newWereRig(t, src, 1)
	defer r.db.Close()
	c := r.newChar(wereArchetypes[0], 30)
	keys := make([]string, 0, len(r.ids))
	for k := range r.ids {
		if strings.Contains(k, "were/") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		qu := r.lineQuest(strings.Split(k, "/")[1], strings.Split(k, "/")[2])
		if qu == nil {
			continue
		}
		type use struct{ name, role string }
		uses := []use{{qu.Giver, "giver"}}
		for _, st := range qu.Steps {
			uses = append(uses, use{st.Target, fmt.Sprintf("s%d %s", st.Step, st.Kind)})
			for _, a := range st.Also {
				uses = append(uses, use{a, fmt.Sprintf("s%d %s also", st.Step, st.Kind)})
			}
		}
		for _, u := range uses {
			if u.name == "" {
				continue
			}
			ns := r.named(c, u.name)
			out := 0
			for _, n := range r.gw.NPCs {
				if n.SpawnOnly && strings.EqualFold(n.Name, u.name) {
					out++
				}
			}
			switch {
			case len(ns) == 0 && out > 0:
				t.Logf("%s %s %q: kept out of the world (%d originals), only a spawn brings it", k, u.role, u.name, out)
			case len(ns) == 0:
				t.Errorf("%s %s %q: no NPC of the name", k, u.role, u.name)
			default:
				bits := 0
				for _, n := range ns {
					if n.NPCType&0x80 != 0 {
						bits++
					}
				}
				if strings.Contains(u.role, "kill") || strings.Contains(u.role, "loot") {
					if bits == len(ns) {
						t.Errorf("%s %s %q: every one of %d has the NPC bit", k, u.role, u.name, len(ns))
					}
				}
				near := ns[0]
				// a talker that shows a player of the quest's level the red
				// face attacks it on sight (aggro.go), unless it is green to it
				c.p.Level = max(qu.MinLevel, 1)
				face := ""
				if !strings.Contains(u.role, "kill") && !strings.Contains(u.role, "loot") && r.gw.TargetFace(c.p, near) == entity.FaceHostile {
					face = fmt.Sprintf("; RED FACE to a level %d: it attacks on sight", c.p.Level)
				}
				t.Logf("%s %s %q: %d in the world, nearest %s, %.0f from the giver%s", k, u.role, u.name, len(ns), where(near), distTo(r, c, qu.Giver, near), face)
			}
		}
	}
}

// distTo is how far an NPC stands from the nearest NPC of a name.
func distTo(r *wereRig, c *wereChar, name string, n *entity.NPC) float64 {
	best := math.Inf(1)
	for _, g := range r.named(c, name) {
		if g.World != n.World {
			continue
		}
		if d := math.Hypot(float64(g.X-n.X), float64(g.Z-n.Z)); d < best {
			best = d
		}
	}
	return best
}
