package handlers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/worlds"
)

// homesTestHome is a home block as a recipe carries it.
const homesTestHome = `{"name": "the Test Homesteads", "door": {"x": 1000, "y": 50, "z": 1000, "facing": 90},
	"board": {"x": 1010, "y": 50, "z": 1000, "toward": [1000, 1000]}, "yard": {"x": 1000, "z": 1000, "r": 60}}`

// homesRig is the portal rig with the homes on in world 13 over a database: a board beside Anna (world 0) and its
// twin in the yard, the characters' rows of Anna, Cora and Bert.
func homesRig(t *testing.T) (*portalRig, *database.SQLiteDB, *entity.NPC) {
	t.Helper()
	r := newPortalRig(t)
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 13, Name: "the Test Homesteads", Home: json.RawMessage(homesTestHome)}}})
	t.Cleanup(func() { SetServerWorlds(nil); entity.SetInstancedWorld(13, false) })
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "homes.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, c := range []struct {
		id   uint32
		name string
	}{{r.pa.CharServerID, "Anna"}, {r.pc.CharServerID, "Cora"}, {r.pb.CharServerID, "Bert"}} {
		if _, err := db.Exec(`INSERT INTO characters (serverid, charName, accountid, modelid, tclass, race, humType, level, haircolor, hairlength, hairstyle, faceoption, sex) VALUES (?, ?, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0)`, c.id, c.name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990101, ?, 0, 5002, 50, 5000, 0, 1843770060, 0.3, 1, 384, '0', 0), (990102, ?, 13, 1010, 50, 1000, 192, 1843770060, 0.3, 1, 384, '0', 0)`,
		entity.HomesBoardName, entity.HomesBoardName); err != nil {
		t.Fatal(err)
	}
	r.gw.LoadNPCs(database.NewNPCStore(db))
	if !SetupHomes(r.gw, config.HomesConfig{Enabled: true}, db) {
		t.Fatal("the homes are not on")
	}
	var board *entity.NPC
	for _, n := range r.gw.NPCs {
		if n.Name == entity.HomesBoardName && n.World == 0 {
			board = n
		}
	}
	return r, db, board
}

// travel runs the World's tick, which lands the travels it queued.
func (r *portalRig) travel() {
	r.gw.Tick(time.Now())
}

func TestHomesRecipeBlock(t *testing.T) {
	b := worlds.Built{Recipe: worlds.Recipe{World: 13, Name: "x", Home: json.RawMessage(homesTestHome)}}
	lay, err := ReadHome(b)
	if err != nil {
		t.Fatal(err)
	}
	if lay.Name != "the Test Homesteads" || lay.Door.X != 1000 || lay.YardRadius != 60 || !lay.InYard(1050, 1000) || lay.InYard(1070, 1000) {
		t.Fatalf("layout %+v", lay)
	}
	for _, bad := range []string{`{"door": {"x": 0, "z": 0}, "yard": {"x": 1, "z": 1, "r": 5}}`,
		`{"door": {"x": 1000, "z": 1000}, "yard": {"x": 1000, "z": 1000, "r": 0}}`,
		`{"door": {"x": 2000, "z": 1000}, "yard": {"x": 1000, "z": 1000, "r": 60}}`} {
		if _, err := ReadHome(worlds.Built{Recipe: worlds.Recipe{World: 13, Home: json.RawMessage(bad)}}); err == nil {
			t.Fatalf("%s read", bad)
		}
	}
	if _, err := ReadHome(worlds.Built{Recipe: worlds.Recipe{World: 13}}); err == nil {
		t.Fatal("no home block read")
	}
	// the recipe in the tree reads (built: worldbuild carries it whole)
	raw, err := os.ReadFile(filepath.Join("..", "..", "tools", "worlds", "homesteads.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec worlds.Recipe
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.World != DefaultHomesWorld || len(rec.Zones) != 1 {
		t.Fatalf("homesteads.json: world %d, %d zones", rec.World, len(rec.Zones))
	}
	if _, err := ReadHome(worlds.Built{Recipe: rec}); err != nil {
		t.Fatalf("homesteads.json: %v", err)
	}
}

// The board end to end: a PS2 (or an old vanilla) is told it cannot go and not moved; Anna goes home, Cora to
// hers, they do not share a place; Anna invites Cora, Cora visits; Anna takes it back and Cora goes out to where
// she came from; the way out from the yard's board; /home list, kick.
func TestHomesBoardEndToEnd(t *testing.T) {
	r, db, board := homesRig(t)
	r.drain()
	r.talk(r.cora, board)
	if got := r.msgs(r.cora); len(got) != 1 || !strings.Contains(got[0], "cannot see the Homesteads") {
		t.Fatalf("a client without the world was sent %q", got)
	}
	homeCommand(r.gw, r.cora, "")
	r.travel()
	if r.pc.World != 0 {
		t.Fatal("a client without the world was moved")
	}
	r.anna.SetServerWorlds([]uint8{13})
	r.cora.SetServerWorlds([]uint8{13})
	r.talk(r.anna, board)
	got := r.msgs(r.anna)
	box := boxID(got)
	if box&0xFF000000 != homesBoxMark || !strings.Contains(got[0], homesAnswerHome) {
		t.Fatalf("the board's box: %q", got)
	}
	r.pick(r.anna, box, 0)
	r.travel()
	if r.pa.World != 13 || r.pa.InstanceKey() != r.pa.CharServerID {
		t.Fatalf("Anna home: world %d instance %d", r.pa.World, r.pa.InstanceKey())
	}
	homeCommand(r.gw, r.cora, "")
	r.travel()
	if r.pc.World != 13 || entity.SamePlace(r.pa, r.pc) {
		t.Fatalf("Cora at home: world %d, one place with Anna: %v", r.pc.World, entity.SamePlace(r.pa, r.pc))
	}
	var host uint32
	if err := db.QueryRow(`SELECT host FROM charhomestay WHERE serverid = ?`, r.pc.CharServerID).Scan(&host); err != nil || host != r.pc.CharServerID {
		t.Fatalf("Cora's stay: %d %v", host, err)
	}
	r.drain()
	homeCommand(r.gw, r.anna, "invite cora")
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "You invite Cora") {
		t.Fatalf("invite: %q", got)
	}
	if got := r.msgs(r.cora); len(got) != 1 || !strings.Contains(got[0], "Anna invites you") {
		t.Fatalf("Cora told: %q", got)
	}
	homeCommand(r.gw, r.anna, "list")
	if got := r.msgs(r.anna); len(got) != 1 || !strings.Contains(got[0], "Cora") {
		t.Fatalf("list: %q", got)
	}
	// Cora at the yard's board in her own home: visit a friend's
	var yard *entity.NPC
	for _, n := range r.gw.NPCs {
		if n.Name == entity.HomesBoardName && n.World == 13 {
			yard = n
		}
	}
	r.pc.X, r.pc.Z = yard.X-2, yard.Z
	r.talk(r.cora, yard)
	got = r.msgs(r.cora)
	box = boxID(got)
	if !strings.Contains(got[0], "Welcome home") || !strings.Contains(got[0], "Return to") {
		t.Fatalf("the yard's box: %q", got)
	}
	r.pick(r.cora, box, 1) // Visit a friend's home
	got = r.msgs(r.cora)
	box = boxID(got)
	if !strings.Contains(got[0], "Anna's home") {
		t.Fatalf("the friends' box: %q", got)
	}
	r.pick(r.cora, box, 0)
	r.travel()
	if !entity.SamePlace(r.pa, r.pc) {
		t.Fatal("Cora visiting Anna is not in Anna's home")
	}
	homeCommand(r.gw, r.anna, "uninvite Cora")
	r.travel()
	if r.pc.World != 0 || abs32(r.pc.X-5001) > 0.01 {
		t.Fatalf("Cora out after the uninvite: world %d (%.1f, %.1f)", r.pc.World, r.pc.X, r.pc.Z)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM charhomeguests`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d invites left", n)
	}
	// Anna out by the yard's board's first answer, back by Freeport's board
	r.pa.X, r.pa.Z = yard.X-2, yard.Z
	r.drain()
	r.talk(r.anna, yard)
	r.pick(r.anna, boxID(r.msgs(r.anna)), 0)
	r.travel()
	if r.pa.World != 0 {
		t.Fatalf("Anna out: world %d", r.pa.World)
	}
}

// A login saved in a home: back in it while it may stand there, else where it came from; a client that does not
// hold the homes' world starts where it came from too.
func TestHomesLoginRule(t *testing.T) {
	r, db, _ := homesRig(t)
	r.anna.SetServerWorlds([]uint8{13})
	r.cora.SetServerWorlds([]uint8{13})
	homeCommand(r.gw, r.anna, "invite Cora")
	homeCommand(r.gw, r.cora, "") // Cora's own home first (where she came from: by the board)
	r.travel()
	(&Homes{gw: r.gw, db: db, now: time.Now}).goHome(r.cora, r.pc, r.pa.CharServerID)
	r.travel()
	held := func(w uint8) bool { return w == 13 }
	rec := &database.CharacterRecord{ServerID: r.pc.CharServerID, CharName: "Cora", World: 13, X: 1000, Y: 50, Z: 1000}
	loginSpawn(r.gw, rec, held, nil)
	if rec.World != 13 {
		t.Fatalf("an invited guest's login: world %d", rec.World)
	}
	rec = &database.CharacterRecord{ServerID: r.pc.CharServerID, CharName: "Cora", World: 13, X: 1000, Y: 50, Z: 1000}
	loginSpawn(r.gw, rec, nil, nil)
	if rec.World != 0 || abs32(rec.X-5001) > 0.01 {
		t.Fatalf("a login without the world: world %d (%.1f, %.1f)", rec.World, rec.X, rec.Z)
	}
	homeCommand(r.gw, r.anna, "uninvite Cora")
	r.travel()
	rec = &database.CharacterRecord{ServerID: r.pc.CharServerID, CharName: "Cora", World: 13, X: 1000, Y: 50, Z: 1000}
	loginSpawn(r.gw, rec, held, nil)
	if rec.World != 0 {
		t.Fatalf("an uninvited guest's login: world %d", rec.World)
	}
	// the stays as the database keeps them come back at a restart (SetupHomes reads them)
	if !SetupHomes(r.gw, config.HomesConfig{Enabled: true}, db) {
		t.Fatal("homes off")
	}
	if s, ok := r.gw.HomeStayOf(r.pc.CharServerID); !ok || s.Host != r.pa.CharServerID {
		t.Fatalf("Cora's stay after a restart: %+v %v", s, ok)
	}
}
