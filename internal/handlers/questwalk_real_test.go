package handlers

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
	"github.com/eqoa/server/internal/session"
)

// A real server's quests walked through the engine (EQOA_REAL_DB names its
// database, a copy of which is walked), as jib's quest walk of 2026-10-30 did
// with the headless client: each quest by a character of a class and race it
// is for, the quests it requires walked first by the same character (of a
// group, the first its class and race may take), every step done as a player
// does it: the giver talked to and the quest taken, a step's NPC talked to and
// its answer (and a choice's first option that does it) picked, a kill step's
// kills credited, a loot step's drops taken off the corpse, a reach step's
// place stood on. Its hand-ins are only what the walk itself was given. Which
// quests: EQOA_WALK_QUESTS (ids, comma separated), else every quest
// tools/fix_quest_chains.py changed (quest_chain_fix_log) and the six of jib's
// give-then-give quests it left as they were; skipped on a database it has not
// been applied to and without EQOA_WALK_QUESTS. None is in this repository.
//
// The walk is the engine's, not the world's: an NPC talked to stands by the
// player, a kill is a credit, a teleport moves the server's player; levels,
// standings and room in the pack are given as a player would have them (the
// pack is emptied, between quests, of what no hand-in of a quest the character
// has not finished wants).
func TestRealQuestChainsWalk(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	w := newQuestWalker(t, src)
	defer w.db.Close()
	var ids []int
	if v := os.Getenv("EQOA_WALK_QUESTS"); v != "" {
		ids = database.ParseIntList(v)
	} else {
		var n int
		w.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'quest_chain_fix_log'`).Scan(&n)
		if n == 0 {
			t.Skip("tools/fix_quest_chains.py has not been applied to this database (or name EQOA_WALK_QUESTS)")
		}
		seen := map[int]bool{}
		rows, err := w.db.Query(`SELECT rid FROM quest_chain_fix_log WHERE tbl IN ('quests', 'queststeps')`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var rid string
			rows.Scan(&rid)
			if id, err := strconv.Atoi(strings.Split(rid, "/")[0]); err == nil && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		rows.Close()
		// jib's give-then-give quests the tool left: their items come back, or enough were given
		for _, id := range []int{100312, 100392, 100545, 100583, 100602, 101225} {
			if !seen[id] {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
	}
	// the engine's log lines are many thousands here: kept out of the test's
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	var failed []string
	start := time.Now()
	for i, id := range ids {
		if why := w.walkTo(id); why != "" {
			failed = append(failed, fmt.Sprintf("%d: %s", id, why))
		}
		if (i+1)%100 == 0 {
			t.Logf("%d of %d quests walked to (%d quests and %d steps walked, %d characters) in %v",
				i+1, len(ids), w.walked, w.steps, len(w.byCombo), time.Since(start).Round(time.Second))
		}
	}
	t.Logf("walked to %d quests (%d quests and %d steps walked, %d characters), %d failed, in %v",
		len(ids), w.walked, w.steps, len(w.byCombo), len(failed), time.Since(start).Round(time.Second))
	for _, f := range failed {
		t.Error(f)
	}
}

// questWalker is the engine on a real database and a character for each
// class and race, each walking its quests in order as jib's walk did.
type questWalker struct {
	t       *testing.T
	db      *database.SQLiteDB
	gw      *entity.World
	mgr     *session.Manager
	qe      *QuestEngine
	inv     *database.InventoryStore
	sess    *session.Session
	p       *entity.Player
	npcs    map[string]*entity.NPC
	drops   []int
	steps   int
	walked  int
	byCombo map[[2]int]*walkChar
	combo   [][2]int // the classes and races a character may be (defaultcharacter's, the froglok's)
	fit     map[[3]int]bool
}

// walkChar is one walking character.
type walkChar struct {
	sess *session.Session
	p    *entity.Player
}

func newQuestWalker(t *testing.T, src string) *questWalker {
	t.Helper()
	path := filepath.Join(t.TempDir(), "walk.db")
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
	gw := entity.NewWorld()
	mgr := session.NewManager()
	gw.SessionManager = mgr
	inv := database.NewInventoryStore(db)
	qe, err := NewQuestEngine(db, gw, mgr, inv, database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	// a copy the walk alone writes: no journal on disk, nothing synced
	db.Exec(`PRAGMA synchronous = OFF`)
	db.Exec(`PRAGMA journal_mode = MEMORY`)
	w := &questWalker{t: t, db: db, gw: gw, mgr: mgr, qe: qe, inv: inv, npcs: map[string]*entity.NPC{},
		byCombo: map[[2]int]*walkChar{}, fit: map[[3]int]bool{}}
	qe.roll = func() float64 { return 0 } // every loot step's drop lands
	qe.drop = func(_ entity.EntityID, _ time.Time, _ uint32, rec database.CharInventoryEntry) {
		w.drops = append(w.drops, rec.Pattern.PatternID)
	}
	rows, err := db.Query(`SELECT DISTINCT tclass, race, humType FROM defaultcharacter`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var c, r, h int
		rows.Scan(&c, &r, &h)
		w.combo = append(w.combo, [2]int{c, r})
		if r == 0 && h == 1 {
			w.combo = append(w.combo, [2]int{c, 10}) // the froglok: a human of the east
		}
	}
	rows.Close()
	return w
}

// quest is a quest as the engine read it.
func (w *questWalker) quest(id int) *database.Quest {
	w.qe.mu.Lock()
	defer w.qe.mu.Unlock()
	return w.qe.cat.byID[id]
}

// fits is whether a character of class c and race r may take the quest and
// walk to it: its classes and races, and every group it requires one of whose
// quests it may walk to (the chosen one returned by chain).
func (w *questWalker) fits(qu *database.Quest, c, r int, seen map[int]bool) bool {
	if qu == nil || seen[qu.ID] || (len(qu.Classes) > 0 && !containsInt(qu.Classes, c)) || (len(qu.Races) > 0 && !containsInt(qu.Races, r)) {
		return false
	}
	k := [3]int{qu.ID, c, r}
	if v, ok := w.fit[k]; ok {
		return v
	}
	seen[qu.ID] = true
	defer delete(seen, qu.ID)
	ok := true
	for _, g := range qu.Required() {
		any := false
		for _, id := range g {
			if w.fits(w.quest(id), c, r, seen) {
				any = true
				break
			}
		}
		if !any {
			ok = false
			break
		}
	}
	w.fit[k] = ok
	return ok
}

// chain is the quests to walk to take qu, prerequisites first, each once.
func (w *questWalker) chain(qu *database.Quest, c, r int, done map[int]bool, out *[]int) {
	if done[qu.ID] {
		return
	}
	done[qu.ID] = true
	for _, g := range qu.Required() {
		for _, id := range g {
			if pre := w.quest(id); w.fits(pre, c, r, map[int]bool{}) {
				w.chain(pre, c, r, done, out)
				break
			}
		}
	}
	*out = append(*out, qu.ID)
}

// walkTo walks a quest with the character of the first class and race (of
// those a character may be) that may take it and walk to it, the quests it
// requires first, each once for that character: "" when every one of them
// was finished.
func (w *questWalker) walkTo(id int) string {
	qu := w.quest(id)
	if qu == nil {
		return "no such quest"
	}
	c, r := -1, -1
	for _, cr := range w.combo {
		if w.fits(qu, cr[0], cr[1], map[int]bool{}) {
			c, r = cr[0], cr[1]
			break
		}
	}
	if c < 0 {
		return "no class and race a character may be can take it and the quests it requires"
	}
	var order []int
	w.chain(qu, c, r, map[int]bool{}, &order)
	w.become(c, r)
	for _, qid := range order {
		if _, done := w.progress(qid); done > 0 {
			continue // walked before by this character
		}
		if why := w.walkOne(w.quest(qid)); why != "" {
			return fmt.Sprintf("a class %d race %d character, walking %d %q (to %q): %s", c, r, qid, w.quest(qid).Name, qu.Name, why)
		}
		w.walked++
	}
	return ""
}

// become makes the character of the class and race the one walking, a new
// one the first time.
func (w *questWalker) become(c, r int) {
	w.t.Helper()
	if ch := w.byCombo[[2]int{c, r}]; ch != nil {
		w.sess, w.p = ch.sess, ch.p
		return
	}
	n := len(w.byCombo) + 1
	rec := &database.CharacterRecord{CharName: fmt.Sprintf("Walker%d", n), AccountID: 1, TClass: c, Race: r}
	if err := database.NewSQLiteCharacterStore(w.db).Create(rec); err != nil {
		w.t.Fatal(err)
	}
	s := w.mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 9, 9), Port: 7000 + n}, uint32(900+n), 1, 1, 2, true)
	p := &entity.Player{ServerID: entity.EntityID(50000 + n), CharServerID: rec.ServerID, Name: rec.CharName,
		Class: c, Race: r, Level: 1, HP: 50000, MaxHP: 50000, X: 25000, Y: 50, Z: 15000}
	w.gw.Players[p.ServerID] = p
	s.SetECSEntity(p)
	s.SetCharacterInWorld(true)
	s.SetDiscVersion(protocol.GameVersionEQOAVanilla)
	s.SetInventoryList(newInvList(nil))
	w.byCombo[[2]int{c, r}] = &walkChar{sess: s, p: p}
	w.sess, w.p = s, p
	w.qe.LoggedIn(rec.ServerID)
}

// npc is an NPC of the name standing by the player (a pinned step's own
// spawn), in the world and the session's view.
func (w *questWalker) npc(name string, pinned int) *entity.NPC {
	k := fmt.Sprintf("%s#%d", foldName(name), pinned)
	n := w.npcs[k]
	if n == nil {
		id := entity.EntityID(900000 + len(w.npcs))
		if pinned != 0 {
			id = entity.EntityID(pinned)
		}
		n = &entity.NPC{ServerID: id, Name: name, Level: 1, HP: 100, MaxHP: 100}
		w.npcs[k] = n
		w.gw.NPCs[n.ServerID] = n
	}
	n.X, n.Y, n.Z, n.World = w.p.X+1, w.p.Y, w.p.Z+1, w.p.World
	n.SpawnX, n.SpawnY, n.SpawnZ = n.X, n.Y, n.Z
	n.Dead = false
	return n
}

// progress is the character's step in a quest and times finished.
func (w *questWalker) progress(id int) (step, done int) {
	w.qe.mu.Lock()
	defer w.qe.mu.Unlock()
	if cq := w.qe.charLocked(w.p.CharServerID).quests[id]; cq != nil {
		return cq.Step, cq.Done
	}
	return -1, 0
}

// answer picks, in the box the character has open, the first choice that does
// a step or takes a quest (a choice's first option that does not stay), else
// closes a popup; false with no box.
func (w *questWalker) answer() bool {
	w.qe.mu.Lock()
	c := w.qe.charLocked(w.p.CharServerID)
	b := c.box
	w.qe.mu.Unlock()
	if b == nil {
		return false
	}
	pick := 0
	for i, ch := range b.choices {
		if ch.kind == choiceAccept || ch.kind == choiceStep {
			pick = i
			break
		}
		if ch.kind == choicePick {
			qu := w.quest(ch.quest)
			if st := qu.Step(ch.step); st != nil && ch.option < len(st.Choices.Options) && !st.Choices.Options[ch.option].Stay {
				pick = i
				break
			}
		}
	}
	w.qe.Answer(w.sess, w.p, b.id, byte(pick))
	return true
}

// settle answers every box the engine opens (popups, a quest's stages).
func (w *questWalker) settle() {
	for i := 0; i < 12 && w.answer(); i++ {
	}
}

// walkOne takes a quest and does its steps; "" when finished.
func (w *questWalker) walkOne(qu *database.Quest) string {
	w.tidy()
	w.p.Level = max(w.p.Level, qu.MinLevel, 1)
	if qu.MaxLevel > 0 && w.p.Level > qu.MaxLevel {
		w.p.Level = qu.MaxLevel
	}
	for _, fr := range qu.FactionReqs {
		if w.p.Factions == nil {
			w.p.Factions = map[string]int{}
		}
		w.p.Factions[fr.Faction] = fr.Min
	}
	_, before := w.progress(qu.ID)
	w.qe.mu.Lock()
	why := w.qe.mayTakeLocked(w.qe.lookAt(w.sess), w.p, w.qe.charLocked(w.p.CharServerID), qu)
	w.qe.mu.Unlock()
	if why != "" {
		return "not to be taken: " + why
	}
	if _, err := w.qe.SetCharacterQuest(w.p.CharServerID, qu.ID, QuestGive, 0); err != nil {
		return "not taken: " + err.Error()
	}
	w.settle()
	for guard := 0; guard < 4*len(qu.Steps)+8; guard++ {
		step, done := w.progress(qu.ID)
		if done > before && step == 0 {
			return ""
		}
		st := qu.Step(step)
		if st == nil {
			return fmt.Sprintf("at step %d, which it lacks", step)
		}
		w.steps++
		if why := w.doStep(qu, st); why != "" {
			return fmt.Sprintf("step %d (%s %q): %s", st.Step, st.Kind, st.Target, why)
		}
		w.settle()
		if now, d := w.progress(qu.ID); now == step && d == done {
			return fmt.Sprintf("step %d (%s %q) not done: %s", st.Step, st.Kind, st.Target, w.lastWords())
		}
	}
	return "too many steps"
}

// lastWords is what the engine said last, for a failure's line.
func (w *questWalker) lastWords() string {
	ms := questMsgs(w.sess)
	if len(ms) > 4 {
		ms = ms[len(ms)-4:]
	}
	return strings.Join(ms, " | ")
}

// doStep does one step as a player does it.
func (w *questWalker) doStep(qu *database.Quest, st *database.QuestStep) string {
	questMsgs(w.sess) // what was said before is not this step's
	switch st.Kind {
	case database.StepTalk, database.StepShow, database.StepGive:
		// tunar is had (a step's price: Sandstorm's coachmen, Osbit's dynamite)
		w.p.Tunar += st.Tunar
		if st.Kind != database.StepTalk {
			w.qe.mu.Lock()
			lack := w.qe.lackingLocked(w.p, st)
			w.qe.mu.Unlock()
			if lack != "" {
				return "the walk does not hold " + lack
			}
		}
		w.qe.Talk(w.sess, w.p, w.npc(st.Target, st.NPCID))
		w.settle()
	case database.StepKill:
		for i := 0; i < max(st.Count, 1); i++ {
			n := w.npc(st.Target, st.NPCID)
			w.qe.killed(entity.KillCredit{NPCName: n.Name, NPCSpawn: n.ServerID, NPCID: n.ServerID, Died: time.Now(),
				Players: []entity.KillPlayer{{Session: w.sess, ServerID: w.p.CharServerID, Name: w.p.Name}}})
		}
		w.settle()
	case database.StepLoot:
		pid, need, _ := st.LootItem()
		for i := 0; i < need+2; i++ {
			if s, _ := w.progress(qu.ID); s != st.Step {
				break
			}
			n := w.npc(st.Target, st.NPCID)
			w.drops = nil
			w.qe.killed(entity.KillCredit{NPCName: n.Name, NPCSpawn: n.ServerID, NPCID: n.ServerID, Died: time.Now(),
				Players: []entity.KillPlayer{{Session: w.sess, ServerID: w.p.CharServerID, Name: w.p.Name}}})
			for _, d := range w.drops {
				// taken off the corpse, as HandleLootItem does
				pat, err := w.qe.items.GetItemPattern(d)
				if err != nil || pat == nil {
					return fmt.Sprintf("its item %d: %v", d, err)
				}
				if got, err := GiveItemNow(w.sess, w.gw, w.inv, *pat, 1); err != nil || got == 0 {
					return fmt.Sprintf("its %q not taken (%v)", pat.ItemName, err)
				}
				w.qe.Looted(w.sess, w.p, pid, 1)
			}
			w.settle()
		}
	case database.StepReach:
		w.gw.UpdatePosition(w.p.ServerID, float32(st.X), float32(st.Y), float32(st.Z), 0)
		w.p.World = uint8(st.World)
		w.qe.checkReach()
		w.settle()
	default:
		return "a kind the walk does not know"
	}
	return ""
}

// tidy empties the pack of what no hand-in of a quest the character has not
// finished wants (rewards pile up over a chain of thirty quests), as a player
// sells and banks, and reads the list again.
func (w *questWalker) tidy() {
	w.qe.mu.Lock()
	c := w.qe.charLocked(w.p.CharServerID)
	keep := map[int]bool{}
	for _, q := range w.qe.cat.all {
		if c.hasFinished(q.ID) && !q.Repeatable {
			continue
		}
		for _, st := range q.Steps {
			for _, it := range st.Items {
				keep[it.PatternID] = true
			}
		}
	}
	w.qe.mu.Unlock()
	rows, err := w.inv.GetCharacterItems(w.p.CharServerID)
	if err != nil {
		w.t.Fatal(err)
	}
	for _, r := range rows {
		if !keep[r.Pattern.PatternID] {
			w.db.Exec(`DELETE FROM charinventory WHERE itemid = ?`, r.ItemID)
		}
	}
	rows, _ = w.inv.GetCharacterItems(w.p.CharServerID)
	w.sess.SetInventoryList(newInvList(rows))
}
