package handlers

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// /ph (placeholder.go): its list read into kinds and odds, or one clear line
// of what is wrong; a GM's and a sub GM's (the tester drawn [Test]), as /ph,
// /placeholder, #ph and a PS2's "/t ph", a player's an unknown command (or a
// tell); the target's campspawns row written with its gm_audit line, read
// into the world, shown and cleared.

// phLookup is a lookup and suggestions over a few names.
func phLookup() (func(string) (string, bool), func(string) []string) {
	return phNames([]string{"Deathfist reaver", "Deathfist prophet", "deathfist oracle", "Deathfist slayer", "Deathfist cleric",
		"Deathfist champion", "RE PC Fire 12", "RE PC Fire 1", "wolf"})
}

func TestPlaceholderSurvivesDeletingItsOnlyNamedSpawn(t *testing.T) {
	c, db, path, gs, gp, _ := campWorld(t)
	stand(c, gp, 25205, 50, 15200, 0)
	targetAs(gp, campPariah, 100301, gs)
	c.say(gs, "/move")
	if got := gmReplies(t, gs); !hasLine(got, "Moved pariah of Xrag") {
		t.Fatal(got)
	}
	targetAs(gp, campWolfNear, 100302, gs)
	c.say(gs, "/ph wolf75, pariah of Xrag25")
	if got := gmReplies(t, gs); !hasLine(got, "is a placeholder now") {
		t.Fatal(got)
	}
	rule, has, err := db.CampSpawnOf(campWolfNear)
	if err != nil || !has || rule.Templates["pariah of xrag"].ModelID != campPariahBody {
		t.Fatalf("saved binding: %+v, %v", rule, err)
	}

	targetAs(gp, campPariah, 100303, gs)
	c.say(gs, "/delete")
	if got := gmReplies(t, gs); !hasLine(got, "Deleted pariah of Xrag") {
		t.Fatal(got)
	}
	if n := count(t, db, "SELECT COUNT(*) FROM npcs WHERE npc_name = 'pariah of Xrag'"); n != 0 {
		t.Fatalf("%d fixed rare spawns remain", n)
	}
	if _, err := c.gw.LoadCampSpawns(db); err != nil {
		t.Fatal(err)
	}
	targetAs(gp, campWolfNear, 100304, gs)
	c.say(gs, "/ph")
	if got := gmReplies(t, gs); !hasLine(got, "wolf 75%, pariah of Xrag 25%") {
		t.Fatal(got)
	}
	// Edit the rule while the only NPC on this point is the ordinary wolf.
	c.say(gs, "/ph pariah of Xrag")
	if got := gmReplies(t, gs); !hasLine(got, "is a placeholder now") {
		t.Fatal(got)
	}
	reopened, err := database.NewSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	w := entity.NewWorld()
	w.LoadNPCs(database.NewNPCStore(reopened))
	if _, err := w.LoadCampSpawns(reopened); err != nil {
		t.Fatal(err)
	}
	spot := w.NPCs[campWolfNear]
	x, z := spot.X, spot.Z
	if !w.KillNPCNow(campWolfNear) || !w.RespawnNPCNow(campWolfNear) {
		t.Fatal("the original wolf point failed to respawn")
	}
	if spot.Name != "pariah of Xrag" || spot.Level != 63 || spot.ModelID != campPariahBody ||
		spot.ServerID != campWolfNear || spot.X != x || spot.Z != z || w.NPCs[campPariah] != nil {
		t.Fatalf("cold respawn: %q, level %d, model %x, row %d at %v,%v", spot.Name, spot.Level, spot.ModelID, spot.ServerID, spot.X, spot.Z)
	}
}

func TestPhParse(t *testing.T) {
	lookup, suggest := phLookup()
	for _, tc := range []struct {
		in   string
		want string // "name:weight;..." or the error's part
		bad  bool
	}{
		{"deathfist reaver, deathfist prophet", "Deathfist reaver:0;Deathfist prophet:0", false},
		{"deathfist reaver90,deathfist prophet10", "Deathfist reaver:90;Deathfist prophet:10", false},
		{"  DEATHFIST REAVER 90 ,  deathfist   prophet  10%  ", "Deathfist reaver:90;Deathfist prophet:10", false},
		{"deathfist reaver100", "Deathfist reaver:100", false},
		{"deathfist reaver", "Deathfist reaver:0", false},
		{"wolf 50, RE PC Fire 1250", "wolf:50;RE PC Fire 12:50", false},
		{"RE PC Fire 12, RE PC Fire 1", "RE PC Fire 12:0;RE PC Fire 1:0", false},
		{"deathfist reaver20, deathfist prophet20, deathfist oracle20, deathfist slayer20, deathfist cleric20",
			"Deathfist reaver:20;Deathfist prophet:20;deathfist oracle:20;Deathfist slayer:20;Deathfist cleric:20", false},

		{"", "No names given.", true},
		{" , ,", "No names given.", true},
		{"deathfist reaver, deathfist prophet, deathfist oracle, deathfist slayer, deathfist cleric, deathfist champion",
			"6 names given: a spawn point pops at most 5.", true},
		{"deathfirst reaver90, deathfist prophet10", `Unknown name "deathfirst reaver" (did you mean Deathfist reaver?)`, true},
		{"nobody, nor this", `Unknown names "nobody", "nor this": no NPC is called that.`, true},
		{"deathfist reaver90, deathfist prophet5", "The odds add to 95, not 100.", true},
		{"deathfist reaver90, deathfist prophet", "Odds on 1 of 2 names (none on Deathfist prophet)", true},
		{"deathfist reaver 0, deathfist prophet 100", "The odds of Deathfist reaver are 0: each must be 1 to 100.", true},
		{"deathfist reaver 150", "The odds of Deathfist reaver are 150", true},
		{"deathfist reaver50, Deathfist Reaver 50", "Deathfist reaver is named twice", true},
	} {
		kinds, bad := phParse(tc.in, lookup, suggest)
		if tc.bad {
			if bad == "" || !strings.Contains(bad, tc.want) {
				t.Errorf("%q: %q, %+v; want an error with %q", tc.in, bad, kinds, tc.want)
			}
			continue
		}
		var got []string
		for _, k := range kinds {
			got = append(got, k.Name+":"+strconv.Itoa(k.Weight))
		}
		if bad != "" || strings.Join(got, ";") != tc.want {
			t.Errorf("%q: %q %q; want %q", tc.in, got, bad, tc.want)
		}
	}
	if s := phOdds([]string{"a", "b", "c"}, nil); s != "a 33.3%, b 33.3%, c 33.3%" {
		t.Errorf("even odds: %q", s)
	}
	if s := phOdds([]string{"a", "b"}, []int{90, 10}); s != "a 90%, b 10%" {
		t.Errorf("odds: %q", s)
	}
}

// The GM's, the sub GM's and the player's /ph on the near wolf (row 101).
func TestPhIsAGMsAndASubGMsAndBindsTheTarget(t *testing.T) {
	c, db, _, gs, gp, ps, pp := campWorldWithPlayer(t)
	c.h.SetAccessLevels(subs{levels{1: 1, 2: 0, 3: 0}, map[int]bool{3: true}})
	ts, tp := c.join(3, "Tester", 25020, 15020, 0)
	ts.SetAccountID(3)
	n, _ := c.gw.NPCStateOf(campWolfNear)
	for _, s := range []*session.Session{gs, ps, ts} {
		vs := entity.GetOrCreateVisibility(s)
		vs.ChannelEntity[1], vs.ChannelObjectID[1] = n.ID, 100107
	}
	for _, p := range []*entity.Player{gp, pp, tp} {
		p.TargetObjectID = 100107
	}
	gmReplies(t, gs)
	gmReplies(t, ts)
	gmReplies(t, ps)

	// the player: an unknown command in each spelling, and "/t ph" a tell; nothing bound
	for _, line := range []string{"/ph wolf", "/placeholder wolf", "#ph wolf"} {
		c.say(ps, line)
		if l := gmReplies(t, ps); !hasLine(l, "Unrecognized command") {
			t.Fatalf("a player's %s: %q", line, l)
		}
	}
	c.say(ps, "/t ph wolf")
	gmReplies(t, ps)
	if _, has, _ := db.CampSpawnOf(campWolfNear); has {
		t.Fatal("a player's /ph bound the wolf")
	}

	// the GM's mistakes: one line each, and the usage; nothing bound
	for line, want := range map[string]string{
		"/ph wolf90, pariah of Xrag5":           "The odds add to 95, not 100.",
		"/ph wolv, pariah of Xrag":              `Unknown name "wolv" (did you mean wolf?)`,
		"/ph wolf90, pariah of Xrag":            "Odds on 1 of 2 names",
		"/ph wolf, Training Dummy Lv50":         "is a training dummy",
		"/ph a,b,c,d,e,f":                       "6 names given",
		"/ph wolf 0, pariah of Xrag 100":        "each must be 1 to 100",
		"/ph wolf50, pariah of xrag50, wolf 0x": "Unknown name",
	} {
		c.say(gs, line)
		l := gmReplies(t, gs)
		if !hasLine(l, want) || !hasLine(l, phUsage) && !strings.Contains(want, "dummy") {
			t.Fatalf("%s: %q; want %q and the usage", line, l, want)
		}
	}
	if _, has, _ := db.CampSpawnOf(campWolfNear); has {
		t.Fatal("a refused /ph bound the wolf")
	}
	gp.TargetObjectID = 0xFFFFFFFF
	c.say(gs, "/ph wolf")
	if l := gmReplies(t, gs); !hasLine(l, "/ph: target the creature whose spawn point it is first.") {
		t.Fatalf("no target: %q", l)
	}
	gp.TargetObjectID = 100107

	// the GM binds it
	c.say(gs, "/ph")
	if l := gmReplies(t, gs); !hasLine(l, "wolf (row 101) pops only itself: it is no placeholder.") {
		t.Fatalf("/ph alone before: %q", l)
	}
	c.say(gs, "/ph wolf90, pariah of xrag10")
	if l := gmReplies(t, gs); !hasLine(l, "wolf (row 101) is a placeholder now: it pops wolf 90%, pariah of Xrag 10%, from its next respawn.") {
		t.Fatalf("/ph: %q", l)
	}
	r, has, err := db.CampSpawnOf(campWolfNear)
	if err != nil || !has || r.Camp != 0 || strings.Join(r.Kinds, "|") != "wolf|pariah of Xrag" || len(r.Weights) != 2 || r.Weights[0] != 90 || r.Weights[1] != 10 {
		t.Fatalf("its row: %+v %v %v", r, has, err)
	}
	var weights string
	db.QueryRow(`SELECT weights FROM campspawns WHERE npcid = ?`, campWolfNear).Scan(&weights)
	if weights != "90, 10" {
		t.Fatalf("weights column %q", weights)
	}
	if n := auditLines(t, db, "campspawns", "insert"); n != 1 {
		t.Fatalf("%d campspawns inserts audited", n)
	}
	var detail, via string
	db.QueryRow(`SELECT detail, via FROM gm_audit WHERE tbl = 'campspawns' ORDER BY id DESC LIMIT 1`).Scan(&detail, &via)
	if detail != "/ph in the game: wolf (row 101) pops wolf 90%, pariah of Xrag 10%" || via != "game" {
		t.Fatalf("the audit line: %q via %q", detail, via)
	}
	// it is no camp's static spawn: it moves as a wolf
	if st, _ := c.gw.NPCStateOf(campWolfNear); strings.Contains(st.Movement, "camp's static spawn") {
		t.Fatalf("a placeholder stands as a camp's: %q", st.Movement)
	}

	// the sub GM's /t ph (a PS2) and /placeholder change it
	c.say(ts, "/t ph wolf, pariah of Xrag")
	if l := gmReplies(t, ts); !hasLine(l, "it pops wolf 50%, pariah of Xrag 50%") {
		t.Fatalf("the sub GM's /t ph: %q", l)
	}
	if r, _, _ := db.CampSpawnOf(campWolfNear); r.Weights != nil || len(r.Kinds) != 2 {
		t.Fatalf("after the sub GM's: %+v", r)
	}
	c.say(ts, "/placeholder pariah of Xrag 75, wolf 25")
	if l := gmReplies(t, ts); !hasLine(l, "it pops pariah of Xrag 75%, wolf 25%") {
		t.Fatalf("the sub GM's /placeholder: %q", l)
	}
	if n := auditLines(t, db, "campspawns", "update"); n != 2 {
		t.Fatalf("%d campspawns updates audited", n)
	}
	c.say(gs, "#ph")
	if l := gmReplies(t, gs); !hasLine(l, "wolf (row 101) pops pariah of Xrag 75%, wolf 25% each time it comes back.") {
		t.Fatalf("#ph alone: %q", l)
	}
	// /help: the sub GM's has it, the GM's #help
	c.say(ts, "/help")
	if l := gmReplies(t, ts); !hasLine(l, "/ph <name>[odds]") {
		t.Fatalf("the sub GM's /help: %q", l)
	}
	c.say(gs, "#help")
	if l := gmReplies(t, gs); !hasLine(l, "#ph <name>[odds]") {
		t.Fatalf("the GM's #help: %q", l)
	}

	// it pops by its odds: killed and back, a pariah at last
	c.gw.SetCorpseTimes(time.Millisecond, time.Millisecond)
	pariah := false
	for i := 0; i < 40 && !pariah; i++ {
		if !c.gw.KillNPCNow(campWolfNear) {
			t.Fatal("not killed")
		}
		c.gw.Tick(time.Now().Add(time.Second))
		st, _ := c.gw.NPCStateOf(campWolfNear)
		pariah = st.Name == "pariah of Xrag"
	}
	if !pariah {
		t.Fatal("never a pariah in 40 returns at 75%")
	}

	// /ph clear: its row gone, audited; it is itself again at its next return
	c.say(gs, "/ph clear")
	if l := gmReplies(t, gs); !hasLine(l, "wolf (row 101) is no placeholder now: it comes back as itself from its next respawn.") {
		t.Fatalf("/ph clear: %q", l)
	}
	if _, has, _ := db.CampSpawnOf(campWolfNear); has {
		t.Fatal("still bound")
	}
	if n := auditLines(t, db, "campspawns", "delete"); n != 1 {
		t.Fatalf("%d campspawns deletes audited", n)
	}
	if st, _ := c.gw.NPCStateOf(campWolfNear); st.Name != "pariah of Xrag" {
		t.Fatalf("cleared, it changed before its return: %q", st.Name)
	}
	c.gw.KillNPCNow(campWolfNear)
	c.gw.Tick(time.Now().Add(2 * time.Second))
	if st, _ := c.gw.NPCStateOf(campWolfNear); st.Name != "wolf" || st.Dead {
		t.Fatalf("after its return: %q dead %v", st.Name, st.Dead)
	}
	c.say(gs, "/ph clear")
	if l := gmReplies(t, gs); !hasLine(l, "is no placeholder: nothing to clear.") {
		t.Fatalf("/ph clear again: %q", l)
	}
	_ = database.MaxPlaceholderKinds
}
