package handlers

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/protocol"
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

// The arenas' recipes as cmd/worldbuild carries them into the built recipe: the Spirit Ring (tools/worlds/ring.json,
// slot 12, the default, a round field) and the Canyon of Banners (tools/worlds/arena.json, slot 11, kept): each arena
// block reads, both teams' stands and camps and every power-up inside its field, the stands as far apart as the
// arena is meant to be, the camps facing their stands, and the composer takes the recipe (one zone of the disc).
func TestCTFArenaRecipe(t *testing.T) {
	for _, c := range []struct {
		file       string
		world      int
		zone       int
		ring       bool
		near, far  float32 // the stands' distance apart
		default_ok bool
	}{
		{"ring.json", 12, 51, true, 250, 350, true},
		{"arena.json", 11, 121, false, 450, 1000, false},
	} {
		r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", c.file))
		if err != nil {
			t.Fatal(err)
		}
		if r.World != c.world || len(r.Zones) != 1 || r.Zones[0].From != "TUNARIA.ESF" || r.Zones[0].Zone != c.zone {
			t.Fatalf("%s: world %d, zones %+v", c.file, r.World, r.Zones)
		}
		if got := int(entity.DefaultCTFSettings().World) == c.world; got != c.default_ok {
			t.Fatalf("%s: the default world is %d", c.file, entity.DefaultCTFSettings().World)
		}
		lay, err := ReadArena(worlds.Built{Recipe: r})
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if (lay.Ring.Radius > 0) != c.ring || (!c.ring && len(lay.Field) < 3) {
			t.Fatalf("%s: field %v, ring %+v", c.file, lay.Field, lay.Ring)
		}
		for i, tm := range lay.Teams {
			if tm.Body == 0 || tm.Name == "" || !lay.InField(tm.Stand.X, tm.Stand.Z) || !lay.InField(tm.Camp.X, tm.Camp.Z) {
				t.Fatalf("%s: team %d: %+v", c.file, i, tm)
			}
			// a camp faces its stand
			cp, st := tm.Camp, tm.Stand
			if h := heading(cp.X, cp.Z, st.X, st.Z); abs32(h-cp.Facing) > 0.01 {
				t.Fatalf("%s: team %d's camp faces %.2f, its stand is at %.2f", c.file, i, cp.Facing, h)
			}
		}
		a, b := lay.Teams[0].Stand, lay.Teams[1].Stand
		if d := float32(math.Hypot(float64(a.X-b.X), float64(a.Z-b.Z))); d < c.near || d > c.far {
			t.Fatalf("%s: the stands are %.0f apart", c.file, d)
		}
		if len(lay.PowerUps) != 3 {
			t.Fatalf("%s: %d power-ups", c.file, len(lay.PowerUps))
		}
		for _, u := range lay.PowerUps {
			if !lay.InField(u.At.X, u.At.Z) {
				t.Fatalf("%s: %s outside the field", c.file, u.Name)
			}
		}
		if c.ring && lay.InField(lay.Ring.X+lay.Ring.Radius+1, lay.Ring.Z) {
			t.Fatalf("%s: a step past the ring is in the field", c.file)
		}
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
		`{"teams": [{"name": "a", "short": "A", "flag": "A Banner", "flag_body": "0x1", "stand": {"x": 1, "z": 1}, "camp": {"x": 2, "z": 2}},
		            {"name": "b", "short": "B", "flag": "B Banner", "flag_body": "0x2", "stand": {"x": 3, "z": 3}, "camp": {"x": 4, "z": 4}}],
		 "field_circle": {"x": 0, "z": 0, "r": 0}}`,
		`{"teams": [{"name": "a", "short": "A", "flag": "A Banner", "flag_body": "0x1", "stand": {"x": 1, "z": 1}, "camp": {"x": 2, "z": 2}},
		            {"name": "b", "short": "B", "flag": "B Banner", "flag_body": "0x2", "stand": {"x": 3, "z": 3}, "camp": {"x": 400, "z": 4}}],
		 "field_circle": {"x": 0, "z": 0, "r": 100}}`,
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
	if d.World != 12 || d.TeamSize != 10 || d.MinPerTeam != 2 || d.ScoreLimit != 3 || d.TimeLimit != 20*time.Minute || d.WaveEvery != 30*time.Second {
		t.Fatalf("the numbers the note gives: %+v", d)
	}
	s := CTFSettings(config.CTFConfig{World: 11, TeamSize: 3, MinPerTeam: 5, ScoreLimit: 5, TimeLimit: time.Minute, WoundAfter: -1})
	if s.World != 11 || s.TeamSize != 3 || s.MinPerTeam != 3 || s.ScoreLimit != 5 || s.TimeLimit != time.Minute || s.WoundAfter != 0 {
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
	set.World = 11 // the Canyon of Banners, by config (world.ctf.world)
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

// The herald and /ctf join let in a client that said it holds the arena (0x7E31's answer: vanilla, natively or in a
// page, which the server cannot tell apart and need not) and tell every other why: a PS2 of the disc, a PS2 on the
// July S2 (its table takes every slot below 30, the server's own worlds among them), one on Sandstorm's live S2, a
// vanilla from before the server's worlds or one that read no version 2 list (either holds none), and one holding
// the Homelands but not the arena. The control: the July S2's table alone says it loads world 11, so CanLoadWorld
// does; the herald goes by what the client holds.
func TestCTFHeraldReachesOnlyWhoHoldsTheArena(t *testing.T) {
	r := newPortalRig(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 10, Name: "the Homelands"}}, {Recipe: worlds.Recipe{World: 11, Name: "the Test Canyon"}}})
	t.Cleanup(func() { SetServerWorlds(nil) })
	lay := entity.CTFLayout{Name: "the Test Canyon", Teams: [2]entity.CTFTeamLayout{
		{Name: "the Ember Band", Short: "Ember", Flag: "Ember Banner", Body: 1, Stand: entity.CTFSpot{X: 100, Z: 100}, Camp: entity.CTFSpot{X: 100, Z: 150}},
		{Name: "the Dusk Band", Short: "Dusk", Flag: "Dusk Banner", Body: 2, Stand: entity.CTFSpot{X: 900, Z: 100}, Camp: entity.CTFSpot{X: 900, Z: 150}}}}
	set := entity.DefaultCTFSettings()
	set.World = 11 // the Canyon of Banners, by config (world.ctf.world)
	r.gw.SetCTF(true, set, lay)
	herald := ctfHerald(t, r)
	for _, c := range []struct {
		what string
		disc byte
		held []uint8
		in   bool
	}{
		{"a PS2 of the disc", protocol.GameVersionEQOAVanilla, nil, false},
		{"a PS2 on the July S2", protocol.GameVersionSandstormS2, nil, false},
		{"a PS2 on Sandstorm's live S2", protocol.GameVersionSandstormS2Live, nil, false},
		{"a vanilla holding none", protocol.GameVersionEQOAVanilla, nil, false},
		{"a vanilla holding the Homelands alone", protocol.GameVersionEQOAVanilla, []uint8{10}, false},
		{"a vanilla holding both (natively or in a page)", protocol.GameVersionEQOAVanilla, []uint8{10, 11}, true},
		{"a vanilla on the July S2 holding both", protocol.GameVersionSandstormS2, []uint8{10, 11}, true},
	} {
		r.anna.SetDiscVersion(c.disc)
		r.anna.SetServerWorlds(c.held)
		r.drain()
		r.talk(r.anna, herald)
		got := r.msgs(r.anna)
		asked := len(got) == 1 && strings.HasPrefix(got[0], "ask#") && boxID(got)&0xFF000000 == ctfBoxMark
		told := len(got) == 1 && strings.Contains(got[0], "cannot see the arena")
		if asked != c.in || told == c.in {
			t.Errorf("%s at the herald: %q", c.what, got)
		}
		ctfCommand(r.gw, r.anna, "join")
		got = r.msgs(r.anna)
		joined := r.gw.CTFQueued(r.anna)
		if joined != c.in || (!c.in && (len(got) != 1 || !strings.Contains(got[0], "cannot see the arena"))) {
			t.Errorf("%s at /ctf join: %q, queued %v", c.what, got, joined)
		}
		if joined {
			ctfCommand(r.gw, r.anna, "leave")
		}
	}
	// the control: the July S2's table takes world 11, which the herald does not go by
	r.anna.SetDiscVersion(protocol.GameVersionSandstormS2)
	r.anna.SetServerWorlds(nil)
	if !CanLoadWorld(r.anna, set.World) || ctfReaches(r.anna, set.World) {
		t.Errorf("a PS2 on the July S2: loads world %d %v, reaches the arena %v", set.World, CanLoadWorld(r.anna, set.World), ctfReaches(r.anna, set.World))
	}
}

// A character saved in the arena's world starts beside the herald (no match is kept over a logout).
func TestCTFLoginInTheArena(t *testing.T) {
	r := newPortalRig(t)
	set := entity.DefaultCTFSettings()
	set.World = 11 // the Canyon of Banners, by config (world.ctf.world)
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
