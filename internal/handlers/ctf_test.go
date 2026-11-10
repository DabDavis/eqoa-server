package handlers

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/worlds"
)

// ctfHerald stands the herald in the rig's world (a row of a database of its own, loaded as the server loads
// npcs), and returns it.
func ctfHerald(t *testing.T, r *portalRig) *entity.NPC {
	t.Helper()
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "ctf.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990001, ?, 0, 5002, 50, 5000, 0, 1893243078, 1.0, 60, 128, '0', 0)`, entity.CTFHeraldName); err != nil {
		t.Fatal(err)
	}
	r.gw.LoadNPCs(database.NewNPCStore(db))
	for _, n := range r.gw.NPCs {
		if n.Name == entity.CTFHeraldName {
			return n
		}
	}
	t.Fatal("no herald")
	return nil
}

// The arena's recipe (tools/worlds/arena.json) as cmd/worldbuild carries it into the built recipe: its arena block
// reads, both teams' stands and camps and every power-up stand inside its field, the stands apart, and the composer
// takes the recipe (one zone of the disc, in a slot past the disc's).
func TestCTFArenaRecipe(t *testing.T) {
	r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "arena.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.World != 11 || len(r.Zones) != 1 || r.Zones[0].From != "TUNARIA.ESF" || r.Zones[0].Zone != 121 {
		t.Fatalf("the recipe: world %d, zones %+v", r.World, r.Zones)
	}
	lay, err := ReadArena(worlds.Built{Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	inside := func(x, z float32) bool {
		in := false
		f := lay.Field
		for i, j := 0, len(f)-1; i < len(f); j, i = i, i+1 {
			if (f[i][1] > z) != (f[j][1] > z) && x < (f[j][0]-f[i][0])*(z-f[i][1])/(f[j][1]-f[i][1])+f[i][0] {
				in = !in
			}
		}
		return in
	}
	for i, tm := range lay.Teams {
		if tm.Body == 0 || tm.Name == "" || !inside(tm.Stand.X, tm.Stand.Z) || !inside(tm.Camp.X, tm.Camp.Z) {
			t.Fatalf("team %d: %+v", i, tm)
		}
	}
	a, b := lay.Teams[0].Stand, lay.Teams[1].Stand
	if dx, dz := a.X-b.X, a.Z-b.Z; dx*dx+dz*dz < 300*300 {
		t.Fatalf("the stands are %.0f apart", dx*dx+dz*dz)
	}
	if len(lay.PowerUps) != 3 {
		t.Fatalf("%d power-ups", len(lay.PowerUps))
	}
	for _, u := range lay.PowerUps {
		if !inside(u.At.X, u.At.Z) {
			t.Fatalf("%s outside the field", u.Name)
		}
	}
	// a camp faces its stand
	c := lay.Teams[0].Camp
	if h := heading(c.X, c.Z, a.X, a.Z); abs32(h-c.Facing) > 0.01 {
		t.Fatalf("the camp faces %.2f, its stand is at %.2f", c.Facing, h)
	}
}

func abs32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

// An arena block that is no capture the flag, has one team, or names no banner's body is refused.
func TestCTFArenaRefusals(t *testing.T) {
	for _, js := range []string{
		`{"game": "duel", "teams": []}`,
		`{"teams": [{"name": "a", "short": "A", "flag": "A Banner", "flag_body": "0x1", "stand": {"x": 1, "z": 1}, "camp": {"x": 2, "z": 2}}]}`,
		`{"teams": [{"name": "a", "short": "A", "flag": "A Banner", "stand": {"x": 1, "z": 1}, "camp": {"x": 2, "z": 2}},
		            {"name": "b", "short": "B", "flag": "B Banner", "stand": {"x": 3, "z": 3}, "camp": {"x": 4, "z": 4}}]}`,
		`{"teams": [{"name": "a", "short": "A", "flag": "A Banner", "flag_body": "0x1", "stand": {"x": 1, "z": 1}, "camp": {"x": 2, "z": 2}},
		            {"name": "b", "short": "B", "flag": "B Banner", "flag_body": "0x2", "stand": {"x": 3, "z": 3}, "camp": {"x": 4, "z": 4}}],
		 "powerups": [{"kind": "fire", "at": {"x": 1, "z": 1}}]}`,
	} {
		if _, err := ReadArena(worlds.Built{Recipe: worlds.Recipe{World: 11, Arena: []byte(js)}}); err == nil {
			t.Fatalf("taken: %s", js)
		}
	}
	if _, err := ReadArena(worlds.Built{Recipe: worlds.Recipe{World: 11}}); err == nil {
		t.Fatal("a world with no arena block was taken")
	}
}

// world.ctf's numbers: unset, the defaults; set, what is set; a wound_after below 0 none; no fewer to start than a
// team holds.
func TestCTFSettingsFromConfig(t *testing.T) {
	d := CTFSettings(config.CTFConfig{})
	if d != entity.DefaultCTFSettings() {
		t.Fatalf("defaults %+v", d)
	}
	if d.TeamSize != 10 || d.MinPerTeam != 2 || d.ScoreLimit != 3 || d.TimeLimit != 20*time.Minute || d.WaveEvery != 30*time.Second {
		t.Fatalf("the numbers the note gives: %+v", d)
	}
	s := CTFSettings(config.CTFConfig{World: 12, TeamSize: 3, MinPerTeam: 5, ScoreLimit: 5, TimeLimit: time.Minute, WoundAfter: -1})
	if s.World != 12 || s.TeamSize != 3 || s.MinPerTeam != 3 || s.ScoreLimit != 5 || s.TimeLimit != time.Minute || s.WoundAfter != 0 {
		t.Fatalf("set: %+v", s)
	}
}

// The herald end to end through the handlers: a client that cannot load the arena is told why and gets no box;
// one that can is asked, signs up by the box's first answer, is asked again with its name on the list and leaves
// by the first answer then; /ctf says how things stand.
func TestCTFHeraldEndToEnd(t *testing.T) {
	r := newPortalRig(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 11, Name: "the Test Canyon"}}})
	t.Cleanup(func() { SetServerWorlds(nil) })
	lay := entity.CTFLayout{Name: "the Test Canyon", Teams: [2]entity.CTFTeamLayout{
		{Name: "the Ember Band", Short: "Ember", Flag: "Ember Banner", Body: 1, Stand: entity.CTFSpot{X: 100, Z: 100}, Camp: entity.CTFSpot{X: 100, Z: 150}},
		{Name: "the Dusk Band", Short: "Dusk", Flag: "Dusk Banner", Body: 2, Stand: entity.CTFSpot{X: 900, Z: 100}, Camp: entity.CTFSpot{X: 900, Z: 150}}}}
	set := entity.DefaultCTFSettings()
	r.gw.SetCTF(true, set, lay)
	ctfRulesLines = ctfRules(set, lay)
	herald := ctfHerald(t, r)
	// Anna's game holds the arena; Cora's (a PS2's) does not
	r.anna.SetServerWorlds([]uint8{11})
	r.drain()
	r.talk(r.cora, herald)
	got := r.msgs(r.cora)
	if len(got) != 1 || !strings.Contains(got[0], "cannot see the arena") {
		t.Fatalf("a PS2 was sent %q", got)
	}
	r.talk(r.anna, herald)
	got = r.msgs(r.anna)
	box := boxID(got)
	if box&0xFF000000 != ctfBoxMark || !strings.Contains(got[0], ctfAnswerJoin) {
		t.Fatalf("the herald's box: %q", got)
	}
	r.pick(r.anna, box, 0)
	got = r.msgs(r.anna)
	if len(got) != 1 || !strings.Contains(got[0], "You are signed up") || !r.gw.CTFQueued(r.anna) {
		t.Fatalf("signing up: %q", got)
	}
	r.talk(r.anna, herald)
	got = r.msgs(r.anna)
	box = boxID(got)
	if !strings.Contains(got[0], ctfAnswerLeave) || !strings.Contains(got[0], "Your name is on my list") {
		t.Fatalf("asked again: %q", got)
	}
	r.pick(r.anna, box, 0)
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "You are no longer signed up.") || r.gw.CTFQueued(r.anna) {
		t.Fatalf("leaving: %q", got)
	}
	ctfCommand(r.gw, r.anna, "")
	if got := r.msgs(r.anna); len(got) != 1 || !strings.HasPrefix(strings.TrimPrefix(got[0], "colour:"), "No match is on.") {
		t.Fatalf("/ctf: %q", got)
	}
	ctfCommand(r.gw, r.anna, "rules")
	if got := r.msgs(r.anna); len(got) != len(ctfRulesLines) {
		t.Fatalf("/ctf rules: %d lines, want %d", len(got), len(ctfRulesLines))
	}
}

// A character saved in the arena's world starts beside the herald (no match is kept over a logout).
func TestCTFLoginInTheArena(t *testing.T) {
	r := newPortalRig(t)
	set := entity.DefaultCTFSettings()
	r.gw.SetCTF(true, set, entity.CTFLayout{Name: "the Test Canyon"})
	ctfHerald(t, r)
	got := &database.CharacterRecord{CharName: "Arenite", World: 11, X: 20500, Y: 6, Z: 21100}
	loginSpawn(r.gw, got, nil, nil)
	if got.World != 0 || abs32(got.Z-(5000+4)) > 0.01 || abs32(got.X-5002) > 0.01 {
		t.Fatalf("a login saved in the arena starts at world %d (%.1f, %.1f)", got.World, got.X, got.Z)
	}
}

// A banner's body that is art the decoration copies into the zone is used where the built world holds it among its
// extras, and its fallback (a CHAR.ESF orb, resident everywhere) where it does not (a composer without extras).
func TestCTFArenaBannerBody(t *testing.T) {
	r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "arena.json"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ReadArena(worlds.Built{Recipe: r}, []byte(`{"zones": [{"from": "TUNARIA.ESF", "zone": 121}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if plain.Teams[0].Body != 0xF97FD9FC || plain.Teams[1].Body != 0x4446A60B || plain.Teams[0].Size != 1.5 {
		t.Fatalf("undecorated: %08X %08X at %.2f", plain.Teams[0].Body, plain.Teams[1].Body, plain.Teams[0].Size)
	}
	deco, err := ReadArena(worlds.Built{Recipe: r}, []byte(`{"zones": [{"from": "TUNARIA.ESF", "zone": 121,
		"extras": [{"sprite": "0xB714B430", "at": [0, 0, 0]}, {"sprite": "0xF0CFEF05", "at": [0, 0, 0]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if deco.Teams[0].Body != 0xB714B430 || deco.Teams[1].Body != 0xF0CFEF05 || deco.Teams[0].Size != 0.17 {
		t.Fatalf("decorated: %08X %08X at %.2f", deco.Teams[0].Body, deco.Teams[1].Body, deco.Teams[0].Size)
	}
}
