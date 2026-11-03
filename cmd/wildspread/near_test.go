package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// -near's circles, inline and from a file with comments, and what it refuses.
func TestReadNear(t *testing.T) {
	got, err := readNear("100,200,50; 7.5,-8,1")
	if err != nil {
		t.Fatal(err)
	}
	if want := []circle{{100, 200, 50, 0, nil}, {7.5, -8, 1, 0, nil}}; !reflect.DeepEqual(got, want) {
		t.Errorf("inline: %v, want %v", got, want)
	}
	path := filepath.Join(t.TempDir(), "near.txt")
	if err := os.WriteFile(path, []byte("# towns\n1,2,3  # one\n\n4,5,6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err = readNear("@" + path); err != nil || !reflect.DeepEqual(got, []circle{{1, 2, 3, 0, nil}, {4, 5, 6, 0, nil}}) {
		t.Errorf("file: %v %v", got, err)
	}
	for _, bad := range []string{"1,2", "1,2,x", "1,2,0", "1,2,-5"} {
		if _, err := readNear(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
	if got, err = readNear(""); err != nil || got != nil {
		t.Errorf("none: %v %v", got, err)
	}
}

// Without -near every place is the run's; with it only its circles. A quest's
// kind counts near its giver, in the giver's world.
func TestNearAndQuestNear(t *testing.T) {
	p := &plan{}
	if !p.inNear(5e4, -5e4) {
		t.Error("no -near: everywhere")
	}
	p.near = []circle{{0, 0, 100, 0, nil}, {1000, 0, 10, 0, nil}}
	for _, c := range []struct {
		x, z float64
		in   bool
	}{{0, 0, true}, {99, 0, true}, {101, 0, false}, {1005, 5, true}, {1020, 0, false}} {
		if p.inNear(c.x, c.z) != c.in {
			t.Errorf("(%v, %v): in %v", c.x, c.z, !c.in)
		}
	}
	p.questGivers = map[string][]place{"marsh newt": {{0, 500, 500}}}
	if !p.questNear("marsh newt", 0, 500+questReach-1, 500) {
		t.Error("within questReach of the giver: near")
	}
	if p.questNear("marsh newt", 0, 500+questReach+1, 500) || p.questNear("marsh newt", 1, 500, 500) ||
		p.questNear("reed toad", 0, 500, 500) {
		t.Error("too far, another world or another kind: not near")
	}
}

// A tagged run's log is its own: applied beside the world's run (whose log
// would refuse a second untagged one), and each undone alone.
func TestATaggedRunAppliesAndUndoesBesideTheWorlds(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer func() { logs = tablesFor("") }()
	res, err := db.Exec(`INSERT INTO npcs (npc_name, world, x_coord, y_coord, z_coord, npc_level, modelid) VALUES ('marsh newt', 0, 100, 50, 100, 3, 77)`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	k := &kind{key: "marsh newt", spelling: "marsh newt"}
	run := func(x float64) *plan {
		return &plan{gapFill: gapFill, fillSpacing: fillSpacing, questWeight: 1,
			worlds: []*worldPlan{{w: 0, fills: []fill{{kind: k, template: &npcRow{id: id}, level: 3, x: x, y: 50, z: 100}}}}}
	}
	count := func() (n int) {
		if err := db.QueryRow(`SELECT COUNT(*) FROM npcs`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	logs = tablesFor("")
	if err := applyPlan(db, run(200), 31); err != nil {
		t.Fatal(err)
	}
	if ok, _ := hasLog(db); !ok {
		t.Fatal("the world's run left no log (the control: a second untagged run is refused by it)")
	}
	logs = tablesFor("town")
	if logs.adds != "wildspread_adds_town" || logs.meta != "wildspread_meta_town" {
		t.Fatalf("tables %+v", logs)
	}
	if ok, _ := hasLog(db); ok {
		t.Fatal("the tagged run sees the world's log as its own")
	}
	if err := applyPlan(db, run(300), 31); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 3 {
		t.Fatalf("%d rows after both runs, want 3", n)
	}
	if err := undoPlan(db); err != nil {
		t.Fatal(err)
	}
	var x float64
	if err := db.QueryRow(`SELECT MAX(x_coord) FROM npcs`).Scan(&x); err != nil || count() != 2 || x != 200 {
		t.Fatalf("the tagged run's undo: %d rows, the farthest at %v (%v), want 2 and the world's at 200", count(), x, err)
	}
	logs = tablesFor("")
	if err := undoPlan(db); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 1 {
		t.Fatalf("%d rows after both undone, want 1", n)
	}
}

// -layers: a place's level by its distance from the town's edge, between the
// layers in a line, the last one's beyond them, none outside every circle.
func TestLayerLevel(t *testing.T) {
	p := &plan{near: []circle{{0, 0, 1500, 300, nil}}, layers: []layer{{0, 2}, {500, 4}, {1000, 8}}}
	for _, c := range []struct {
		x     float64
		level int
	}{{0, 2}, {300, 2}, {550, 3}, {1050, 6}, {1400, 8}, {2000, 0}} {
		if got := p.layerLevel(c.x, 0); got != c.level {
			t.Errorf("%v from the middle: level %d, want %d", c.x, got, c.level)
		}
	}
	if ls, err := readLayers("0:2, 500:4,1000:8"); err != nil || len(ls) != 3 || ls[2] != (layer{1000, 8}) {
		t.Errorf("readLayers: %v %v", ls, err)
	}
	for _, bad := range []string{"500:4,0:2", "a:1", "0:0", "5"} {
		if _, err := readLayers(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
	if _, err := readNear("0,0,100,100"); err == nil {
		t.Error("a town as wide as its circle taken")
	}
	if cs, err := readNear("0,0,100,10,Swamp+Tunaria/Forest"); err != nil || !reflect.DeepEqual(cs[0].Sections, []string{"Tunaria/Swamp", "Tunaria/Forest"}) {
		t.Errorf("sections: %v %v", cs, err)
	}
	p.near[0].Sections = []string{"Tunaria/Swamp"}
	if s := p.nearSections(10, 0); !reflect.DeepEqual(s, []string{"Tunaria/Swamp"}) || p.nearSections(5000, 0) != nil {
		t.Errorf("nearSections: %v", s)
	}
}

// -group's sizes by weight, and a group of one draws nothing: a run without
// -group spends its random numbers as it did before the flag.
func TestGroups(t *testing.T) {
	sizes, weights, err := readGroup("1:40, 2:30,3:15")
	if err != nil || !reflect.DeepEqual(sizes, []int{1, 2, 3}) || !reflect.DeepEqual(weights, []float64{40, 30, 15}) {
		t.Fatalf("readGroup: %v %v %v", sizes, weights, err)
	}
	for _, bad := range []string{"0:5", "2", "2:0", "x:1", "21:1"} {
		if _, _, err := readGroup(bad); err == nil {
			t.Errorf("%q taken", bad)
		}
	}
	p := &plan{}
	a, b := rand.New(rand.NewSource(7)), rand.New(rand.NewSource(7))
	if p.groupSize(a) != 1 || a.Float64() != b.Float64() {
		t.Error("without -group a group is one and draws nothing")
	}
	p.groupSizes, p.groupWeights = []int{3}, []float64{1}
	if p.groupSize(a) != 3 {
		t.Error("one size: that size")
	}
}

// A step's names (its target and queststeptargets' others) are one step, its
// givers the NPCs of the giver's name with the NPC bit when any has it.
func TestReadQuestGiversGroupsAStepsNames(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS quests (questid INTEGER PRIMARY KEY, name TEXT, enabled INTEGER, giver TEXT, minlevel INTEGER)`,
		`CREATE TABLE IF NOT EXISTS queststeps (questid INTEGER, step INTEGER, kind TEXT, target TEXT)`,
		`CREATE TABLE IF NOT EXISTS queststeptargets (questid INTEGER, step INTEGER, target TEXT)`,
		`DELETE FROM quests`, `DELETE FROM queststeps`, `DELETE FROM queststeptargets`,
		`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (1, 'Scales', 1, 'Ostrel', 3), (2, 'Off', 0, 'Ostrel', 3)`,
		`INSERT INTO queststeps (questid, step, kind, target) VALUES (1, 1, 'loot', 'reed python'), (1, 2, 'give', 'Ostrel'), (2, 1, 'loot', 'marsh newt')`,
		`INSERT INTO queststeptargets (questid, step, target) VALUES (1, 1, 'Reed Serpent')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	in := &input{rows: []*npcRow{{key: "ostrel", world: 0, x: 10, z: 20}, {key: "ostrel", world: 0, x: 50, z: 60, typ: 0x80}}}
	givers, steps, err := readQuestGivers(db, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || !reflect.DeepEqual(steps[0].names, []string{"reed python", "reed serpent"}) || steps[0].minLevel != 3 {
		t.Fatalf("steps %+v, want the one enabled loot step with both names", steps)
	}
	if !reflect.DeepEqual(steps[0].givers, []place{{0, 50, 60}}) || len(givers["reed serpent"]) != 1 {
		t.Errorf("givers %+v %v: the NPC-bit Ostrel's place", steps[0].givers, givers)
	}
}
