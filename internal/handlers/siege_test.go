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

// The siege's recipe (tools/worlds/siege.json) as cmd/worldbuild carries it into the built recipe: world 15, the one
// zone of Zentars Keep where it stands, no extras (the gates' portcullis is the zone's own art); its arena block
// reads: the defenders' camp and the King inside the castle's walls (west of the gatehouse's inner door), the
// attackers' camp outside its outer door, both gates in the gatehouse's doors (on the door's line, z 6477.25, facing
// across it as the zone's own portcullis does, the outer one east of the inner), each a copy of its original with
// the portcullis for a body, all of it in the field; the default world is 15. ReadArena (capture the flag's) refuses
// it, and ReadSiege refuses capture the flag's recipes.
func TestSiegeRecipe(t *testing.T) {
	r, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "siege.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.World != 15 || len(r.Zones) != 1 || r.Zones[0].From != "TUNARIA.ESF" || r.Zones[0].Zone != 22 || len(r.Zones[0].Extras) != 0 {
		t.Fatalf("world %d, zones %+v", r.World, r.Zones)
	}
	if entity.DefaultSiegeSettings().World != 15 {
		t.Fatalf("the default world is %d", entity.DefaultSiegeSettings().World)
	}
	lay, err := ReadSiege(worlds.Built{Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	def, att := lay.Teams[entity.SiegeDefenders], lay.Teams[entity.SiegeAttackers]
	outer, inner := lay.Siege.Gates[0], lay.Siege.Gates[1]
	if len(lay.Siege.Gates) != 2 || outer.Name != "Outer Gate" || inner.Name != "Inner Gate" || lay.Siege.King.Name != "King Zentar" {
		t.Fatalf("the siege's NPCs: %+v", lay.Siege)
	}
	if def.Camp.X >= inner.At.X || lay.Siege.King.At.X >= inner.At.X || att.Camp.X <= outer.At.X || outer.At.X <= inner.At.X {
		t.Fatalf("camps %+v / %+v, King %+v, gates %+v %+v", def.Camp, att.Camp, lay.Siege.King.At, outer.At, inner.At)
	}
	for _, g := range lay.Siege.Gates {
		if g.Body != 0xAA243482 || g.Original != g.Name || abs32(g.At.Z-6477.25) > 0.01 || abs32(g.At.Facing-1.5708) > 0.001 || g.At.Y != 59.5 {
			t.Fatalf("a gate: %+v", g)
		}
	}
	for _, s := range []entity.CTFSpot{def.Camp, att.Camp, lay.Siege.King.At, outer.At, inner.At} {
		if !lay.InField(s.X, s.Z) {
			t.Fatalf("%+v outside the field", s)
		}
	}
	// the attackers' camp faces the outer door
	if h := heading(att.Camp.X, att.Camp.Z, 5521, 6477); abs32(h-att.Camp.Facing) > 0.01 {
		t.Fatalf("the attackers' camp faces %.2f, the door is at %.2f", att.Camp.Facing, h)
	}
	if _, err := ReadArena(worlds.Built{Recipe: r}); err == nil {
		t.Fatal("capture the flag took the siege's recipe")
	}
	ring, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "ring.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadSiege(worlds.Built{Recipe: ring}); err == nil {
		t.Fatal("the siege took capture the flag's recipe")
	}
}

// A siege block with no King, a gate of neither original nor body, a team with no camp, or the King outside the
// field is refused.
func TestSiegeRecipeRefusals(t *testing.T) {
	teams := `"teams": [{"name": "a", "short": "A", "camp": {"x": 10, "z": 10}}, {"name": "b", "short": "B", "camp": {"x": 20, "z": 20}}]`
	for _, js := range []string{
		`{"game": "siege", ` + teams + `}`,
		`{"game": "siege", ` + teams + `, "king": {"name": "K", "body": "0x1", "at": {"x": 5, "z": 5}}, "gates": [{"name": "G", "at": {"x": 6, "z": 6}}]}`,
		`{"game": "siege", "teams": [{"name": "a", "short": "A"}, {"name": "b", "short": "B", "camp": {"x": 20, "z": 20}}], "king": {"name": "K", "body": "0x1", "at": {"x": 5, "z": 5}}}`,
		`{"game": "siege", ` + teams + `, "king": {"name": "K", "body": "0x1", "at": {"x": 500, "z": 5}}, "field_circle": {"x": 10, "z": 10, "r": 50}}`,
	} {
		if _, err := ReadSiege(worlds.Built{Recipe: worlds.Recipe{World: 15, Arena: []byte(js)}}); err == nil {
			t.Fatalf("taken: %s", js)
		}
	}
}

// world.siege's numbers: unset, the siege's defaults (capture the flag's engine numbers in world 15, the King's
// 100000 and the gates' 30000 hit points); set, what is set.
func TestSiegeSettingsFromConfig(t *testing.T) {
	d := SiegeSettings(config.SiegeConfig{})
	if d != entity.DefaultSiegeSettings() || d.World != 15 || d.KingHP != 100000 || d.GateHP != 30000 || d.TimeLimit != 20*time.Minute {
		t.Fatalf("defaults %+v", d)
	}
	s := SiegeSettings(config.SiegeConfig{CTFConfig: config.CTFConfig{World: 16, TeamSize: 4, TimeLimit: 5 * time.Minute}, KingHP: 7, GateHP: 3})
	if s.World != 16 || s.TeamSize != 4 || s.TimeLimit != 5*time.Minute || s.KingHP != 7 || s.GateHP != 3 {
		t.Fatalf("set: %+v", s)
	}
}

// siegeRig is newPortalRig's world with both games on: capture the flag in world 11, the siege in world 15, the
// herald standing beside the players.
func siegeRig(t *testing.T) (*portalRig, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 11, Name: "the Test Canyon"}}, {Recipe: worlds.Recipe{World: 15, Name: "Zentar's Keep"}}})
	t.Cleanup(func() { SetServerWorlds(nil) })
	lay := entity.CTFLayout{Name: "the Test Canyon", Teams: [2]entity.CTFTeamLayout{
		{Name: "the Ember Band", Short: "Ember", Flag: "Ember Banner", Body: 1, Stand: entity.CTFSpot{X: 100, Z: 100}, Camp: entity.CTFSpot{X: 100, Z: 150}},
		{Name: "the Dusk Band", Short: "Dusk", Flag: "Dusk Banner", Body: 2, Stand: entity.CTFSpot{X: 900, Z: 100}, Camp: entity.CTFSpot{X: 900, Z: 150}}}}
	set := entity.DefaultCTFSettings()
	set.World = 11
	r.gw.SetCTF(true, set, lay)
	ctfRulesLines = ctfRules(set, lay)
	sr, err := worlds.ReadRecipe(filepath.Join("..", "..", "tools", "worlds", "siege.json"))
	if err != nil {
		t.Fatal(err)
	}
	slay, err := ReadSiege(worlds.Built{Recipe: sr})
	if err != nil {
		t.Fatal(err)
	}
	sset := entity.DefaultSiegeSettings()
	r.gw.SetArena(entity.GameSiege, true, sset, slay)
	siegeRulesLines = siegeRules(sset, slay)
	return r, ctfHerald(t, r)
}

// The herald offers both games to a client holding both worlds: a sign-up for each, the rules and the state of
// both; the siege's answer signs up for the siege, asked again its answer leaves it; a client holding only the
// siege's world is asked of the siege alone, one holding only the arena's of capture the flag alone (as before).
// /siege join, /siege, /siege rules, and a GM's #siege help.
func TestSiegeHeraldOffersBoth(t *testing.T) {
	r, herald := siegeRig(t)
	r.anna.SetServerWorlds([]uint8{11, 15})
	r.drain()
	r.talk(r.anna, herald)
	got := r.msgs(r.anna)
	box := boxID(got)
	if box&0xFF000000 != ctfBoxMark || !strings.Contains(got[0], ctfAnswerJoinTwo) || !strings.Contains(got[0], siegeAnswerJoin) ||
		!strings.Contains(got[0], ctfAnswerRulesTwo) || !strings.Contains(got[0], "is under siege") {
		t.Fatalf("the herald's box: %q", got)
	}
	r.pick(r.anna, box, 1) // the siege
	got = r.msgs(r.anna)
	if len(got) != 1 || !strings.Contains(got[0], "You are signed up for the siege.") || !r.gw.ArenaQueued(entity.GameSiege, r.anna) ||
		r.gw.CTFQueued(r.anna) {
		t.Fatalf("signing up for the siege: %q", got)
	}
	r.talk(r.anna, herald)
	got = r.msgs(r.anna)
	box = boxID(got)
	if !strings.Contains(got[0], siegeAnswerLeave) || !strings.Contains(got[0], ctfAnswerJoinTwo) {
		t.Fatalf("asked again: %q", got)
	}
	r.pick(r.anna, box, 2) // the rules: both games'
	if got := r.msgs(r.anna); len(got) != len(ctfRulesLines)+len(siegeRulesLines) {
		t.Fatalf("the rules: %d lines, want %d", len(got), len(ctfRulesLines)+len(siegeRulesLines))
	}
	r.talk(r.anna, herald)
	box = boxID(r.msgs(r.anna))
	r.pick(r.anna, box, 1) // leave the siege
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "You are no longer signed up.") || r.gw.ArenaQueued(entity.GameSiege, r.anna) {
		t.Fatalf("leaving: %q", got)
	}
	// the siege's world alone held: the siege alone asked
	r.anna.SetServerWorlds([]uint8{15})
	r.talk(r.anna, herald)
	got = r.msgs(r.anna)
	if !strings.Contains(got[0], siegeAnswerJoin) || strings.Contains(got[0], "banner") {
		t.Fatalf("a client of the siege's world alone: %q", got)
	}
	// the arena's world alone held: capture the flag's box as it always was
	r.anna.SetServerWorlds([]uint8{11})
	r.talk(r.anna, herald)
	got = r.msgs(r.anna)
	if !strings.Contains(got[0], ctfAnswerJoin) || strings.Contains(got[0], "siege") {
		t.Fatalf("a client of the arena's world alone: %q", got)
	}
	// /siege
	r.anna.SetServerWorlds([]uint8{11, 15})
	siegeCommand(r.gw, r.anna, "join")
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "You are signed up for the siege.") {
		t.Fatalf("/siege join: %q", got)
	}
	siegeCommand(r.gw, r.anna, "")
	if got := r.msgs(r.anna); len(got) != 2 || !strings.Contains(got[0], "No match is on.") || !strings.Contains(got[1], "signed up for the siege") {
		t.Fatalf("/siege: %q", got)
	}
	siegeCommand(r.gw, r.anna, "rules")
	if got := r.msgs(r.anna); len(got) != len(siegeRulesLines) || !strings.Contains(strings.Join(got, " "), "King Zentar stands in the courtyard with 100000 hit points") {
		t.Fatalf("/siege rules: %q", got)
	}
	r.cora.SetServerWorlds(nil)
	siegeCommand(r.gw, r.cora, "join")
	if got := r.msgs(r.cora); len(got) != 1 || !strings.Contains(got[0], "cannot see the arena") {
		t.Fatalf("a PS2's /siege join: %q", got)
	}
	if !arenaGMCommand(r.gw, entity.GameSiege, "#siege", r.anna, "help") || arenaGMCommand(r.gw, entity.GameSiege, "#siege", r.anna, "join") {
		t.Fatal("#siege help is not a GM's, or join is")
	}
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "#siege start") {
		t.Fatalf("#siege help: %q", got)
	}
}

// A character saved in the siege's world starts beside the herald, as one saved in capture the flag's arena.
func TestSiegeLoginInItsWorld(t *testing.T) {
	r, _ := siegeRig(t)
	got := &database.CharacterRecord{CharName: "Besieger", World: 15, X: 5600, Y: 54, Z: 6478}
	loginSpawn(r.gw, got, nil, nil)
	if got.World != 0 || abs32(got.Z-(5000+4)) > 0.01 || abs32(got.X-5002) > 0.01 {
		t.Fatalf("a login saved in the siege's world starts at world %d (%.1f, %.1f)", got.World, got.X, got.Z)
	}
}
