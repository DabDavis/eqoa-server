package handlers

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// The Cazic-Thule trials on a real server's database after
// tools/ct_trials_20261111.py (EQOA_REAL_DB names it; a copy is read), through
// the quest engine and the trigger engine as the server runs them: the first
// Kithgrin finishes The Faceless Generals; The Trials of Cazic Thule's long
// hallway brings six thoughtbleeders at the player, kept up; the thirty
// minutes up and every one dead, the trial is done and the catacombs' wall
// lets the player through (and not before); the second Kithgrin and the Trial
// of Celerity bring the red copies at 63 (three Bloodletters among them) and an
// attackable Qual'Tin; his kill and the third Kithgrin bring the Watchman 4 s
// later, walking his round, by his template. Skipped without a database, or on
// one the tool has not been applied to.
func TestRealCazicThuleTrials(t *testing.T) {
	src := os.Getenv("EQOA_REAL_DB")
	if src == "" {
		t.Skip("EQOA_REAL_DB names a server database")
	}
	path := filepath.Join(t.TempDir(), "ct.db")
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
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'ct_trials_1111_log'`).Scan(&n); err != nil || n == 0 {
		t.Skip("tools/ct_trials_20261111.py has not been applied to this database")
	}
	w := entity.NewWorld()
	mgr := session.NewManager()
	w.SessionManager = mgr
	log.SetOutput(io.Discard)
	w.LoadNPCs(database.NewNPCStore(db))
	if _, _, err := w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadQuestWalls(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadAggroRules(db); err != nil {
		t.Fatal(err)
	}
	if _, err := w.LoadClassTemplates(db); err != nil {
		t.Fatal(err)
	}
	recs, _, err := database.NewNPCPathStore(db).All()
	if err != nil {
		t.Fatal(err)
	}
	w.LoadNPCPaths(recs)
	log.SetOutput(os.Stderr)
	inv := database.NewInventoryStore(db)
	te := NewTriggerEngine(db, w, mgr, inv)
	te.Start()
	defer te.Stop()
	qe, err := NewQuestEngine(db, w, mgr, inv, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := &database.CharacterRecord{CharName: "Trialtest", AccountID: 1, Level: 60}
	if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
		t.Fatal(err)
	}
	sess := mgr.CreateSession(&net.UDPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 7031}, 31, 1, 1, 2, true)
	p := &entity.Player{ServerID: 900031, CharServerID: rec.ServerID, Name: "Trialtest", Level: 60, HP: 5000, MaxHP: 5000,
		X: 25715, Y: 49, Z: 3233, World: 0}
	w.Players[p.ServerID] = p
	w.SpatialIndex.Insert(p.ServerID, p.X, p.Z)
	sess.SetECSEntity(p)
	sess.SetCharacterInWorld(true)
	sess.SetInventoryList(newInvList(nil))
	moveTo := func(x, y, z float32) {
		w.UpdatePosition(p.ServerID, x, y, z, 0)
	}
	npcAt := func(name string, x, z, within float32) *entity.NPC {
		for _, c := range w.NPCs {
			if c.Name == name && !c.Dead && !c.SpawnOnly && !c.Temporary {
				if dx, dz := c.X-x, c.Z-z; dx*dx+dz*dz <= within*within {
					return c
				}
			}
		}
		t.Fatalf("no %q within %.0f of (%.0f, %.0f)", name, within, x, z)
		return nil
	}
	alive := func(name string) (out []*entity.NPC) {
		for _, c := range w.NPCs {
			if c.Temporary && !c.Dead && strings.EqualFold(c.Name, name) {
				out = append(out, c)
			}
		}
		return out
	}
	stepOf := func(q int) int {
		for _, cq := range qe.CharacterQuests(rec.ServerID) {
			if cq.QuestID == q {
				if cq.Done > 0 && cq.Step == 0 {
					return -1
				}
				return cq.Step
			}
		}
		return 0
	}
	for slot := 150; slot <= 155; slot++ {
		tr, ok, err := db.GMTriggerBySlot(slot)
		if err != nil || !ok {
			t.Fatalf("trigger %d: %v %v", slot, ok, err)
		}
		if pr := database.TriggerProblems(tr); len(pr) > 0 {
			t.Errorf("trigger %d (%s): %v", slot, tr.Name, pr)
		}
	}
	walls := w.QuestWalls()
	if len(walls) != 3 {
		t.Fatalf("%d quest walls, want 3", len(walls))
	}

	// the first Kithgrin finishes The Faceless Generals
	if _, err := qe.SetCharacterQuest(rec.ServerID, 101311, QuestSetStep, 6); err != nil {
		t.Fatal(err)
	}
	k1 := w.NPCs[118858]
	qe.Talk(sess, p, k1)
	if s := stepOf(101311); s != -1 {
		t.Fatalf("101311 after the first Kithgrin: step %d, want finished", s)
	}
	if _, err := qe.SetCharacterQuest(rec.ServerID, 101312, QuestGive, 0); err != nil {
		t.Fatal(err)
	}
	qe.syncQuestState()
	if sent := qe.chars[rec.ServerID].qstate.sent; sent != "+" {
		t.Fatalf("at the start of the trials the walls passed: %q, want none", sent)
	}

	// the long hallway: six thoughtbleeders at the player, kept up
	moveTo(25709, 37, 3207)
	qe.checkReach()
	if s := stepOf(101312); s != 2 {
		t.Fatalf("in the hallway: step %d, want 2", s)
	}
	bees := alive("thoughtbleeder")
	if len(bees) != 6 {
		t.Fatalf("%d thoughtbleeders, want 6", len(bees))
	}
	for _, b := range bees {
		if b.CurrentEnemy != p.ServerID || b.NPCType&0x80 != 0 {
			t.Fatalf("a thoughtbleeder not at the player: enemy %d type %#x", b.CurrentEnemy, b.NPCType)
		}
	}
	bees[0].Dead, bees[0].DeathTime, bees[0].CorpseGone = true, time.Now(), time.Now()
	qe.checkSurvive()
	if got := len(alive("thoughtbleeder")); got != 6 {
		t.Fatalf("one gone: %d alive a second later, want 6", got)
	}
	qe.mu.Lock()
	qe.chars[rec.ServerID].quests[101312].Count = 1799
	qe.mu.Unlock()
	qe.checkSurvive()
	if s := stepOf(101312); s != 2 {
		t.Fatalf("the time up with thoughtbleeders alive: step %d, want 2", s)
	}
	for _, b := range alive("thoughtbleeder") {
		b.Dead, b.DeathTime, b.CorpseGone = true, time.Now(), time.Now()
	}
	qe.checkSurvive()
	if s := stepOf(101312); s != 3 {
		t.Fatalf("endured: step %d, want 3", s)
	}
	qe.syncQuestState()
	if sent := qe.chars[rec.ServerID].qstate.sent; !strings.Contains(sent, "w") {
		t.Fatalf("endured, the catacombs' wall is not passed: %q", sent)
	}

	// the second Kithgrin, the Trial of Celerity
	qe.Talk(sess, p, npcAt("Kithgrin", 25636, 3200, 3))
	if s := stepOf(101312); s != 4 {
		t.Fatalf("after the second Kithgrin: step %d, want 4", s)
	}
	moveTo(25686, 17.9, 3306)
	qe.Talk(sess, p, npcAt("Trial of Celerity", 25686, 3310, 3))
	if s := stepOf(101312); s != 5 {
		t.Fatalf("after the Trial of Celerity: step %d, want 5", s)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(alive("Qual'Tin")) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	qt := alive("Qual'Tin")
	if len(qt) != 1 || qt[0].NPCType&0x80 != 0 || qt[0].EventAggro {
		t.Fatalf("Qual'Tin: %d, want one attackable and not KoS", len(qt))
	}
	if b := alive("Bloodletter"); len(b) != 3 || b[0].Level != 63 {
		t.Fatalf("%d red Bloodletters (level %d), want 3 at 63", len(b), func() int {
			if len(b) > 0 {
				return b[0].Level
			}
			return 0
		}())
	}
	for _, name := range []string{"Contagion", "Festering Jack", "Zin the Desiccator", "Impale"} {
		if c := alive(name); len(c) != 1 || c[0].Level != 63 || !c[0].EventAggro {
			t.Fatalf("the red %s: %d", name, len(c))
		}
	}
	qe.killed(entity.KillCredit{NPCName: "Qual'Tin", NPCID: qt[0].ServerID, Died: time.Now(),
		Players: []entity.KillPlayer{{Session: sess, ServerID: rec.ServerID, Name: p.Name}}})
	if s := stepOf(101312); s != 6 {
		t.Fatalf("Qual'Tin dead: step %d, want 6", s)
	}

	// the third Kithgrin: he goes, and 4 s later the Watchman walks his round
	k3 := npcAt("Kithgrin", 25694, 3310, 3)
	qe.Talk(sess, p, k3)
	if s := stepOf(101312); s != 7 {
		t.Fatalf("after the third Kithgrin: step %d, want 7", s)
	}
	deadline = time.Now().Add(8 * time.Second)
	for len(alive("The Watchman")) == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	wm := alive("The Watchman")
	if len(wm) != 1 {
		t.Fatalf("%d Watchmen after the third Kithgrin, want one", len(wm))
	}
	if _, ok := w.NPCPathOf(wm[0].ServerID); !ok {
		t.Fatal("the Watchman walks no path")
	}
	if !k3.HiddenUntil.After(time.Now()) {
		t.Fatal("the third Kithgrin is not gone")
	}
	if st, ok := w.NPCStateOf(wm[0].ServerID); !ok || st.Template != "iod watchman" {
		t.Fatalf("the Watchman fights by %q", st.Template)
	}
}
