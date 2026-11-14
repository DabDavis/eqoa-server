package handlers

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/config"
	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
	"github.com/eqoa/server/internal/worlds"
)

// TestReadDungeonsRecipe reads the repository's recipe's dungeons block (tools/worlds/dungeons.json) and refuses a
// bad one.
func TestReadDungeonsRecipe(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "tools", "worlds", "dungeons.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r worlds.Recipe
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.World != DefaultDungeonsWorld {
		t.Fatalf("the recipe's slot %d, the default %d", r.World, DefaultDungeonsWorld)
	}
	defs, err := ReadDungeons(worlds.Built{Recipe: r})
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]entity.DungeonDef{}
	for _, d := range defs {
		keys[d.Key] = d
	}
	for _, k := range []string{"guk", "blackburrow", "runnyeye", "saltmines"} {
		d, ok := keys[k]
		if !ok || len(d.Boxes) == 0 || d.Entrance.X == 0 {
			t.Fatalf("%s: %+v", k, d)
		}
	}
	// every one a heroic dungeon of 4 players (petition 1879: "For all heroic dungeons, such as heroic guk, set this
	// cap to 4")
	for _, d := range defs {
		if d.Cap != 4 {
			t.Errorf("%s's cap %d, want 4", d.Key, d.Cap)
		}
	}
	// the Salt Mines (2026-11-14): its shaft's box inside Tunaria's cell (7, 9), the mine's zone 105, and its
	// landing outside the box, east of the mouth
	sm := keys["saltmines"]
	for _, b := range sm.Boxes {
		if b.X0 < 14000 || b.X1 > 16000 || b.Z0 < 18000 || b.Z1 > 20000 {
			t.Fatalf("the Salt Mines' box %+v is not in cell (7, 9)", b)
		}
		if sm.Entrance.X >= b.X0 && sm.Entrance.X <= b.X1 && sm.Entrance.Z >= b.Z0 && sm.Entrance.Z <= b.Z1 {
			t.Fatalf("the Salt Mines' landing %+v is inside its box", sm.Entrance)
		}
	}
	// Guk's entrance faces north (toward smaller z: its gate), a facing of pi
	if f := keys["guk"].Entrance.Facing; f < 3.1 || f > 3.2 {
		t.Fatalf("Guk's entrance faces %.2f", f)
	}
	for _, bad := range []string{`[{"key":"a","name":"A","boxes":[]}]`, `[{"key":"a","name":"A","boxes":[[0,0,1,1]]}]`,
		`[{"key":"a","name":"A","boxes":[[0,0,1,1]],"entrance":{"x":5,"z":5}},{"key":"A","name":"B","boxes":[[0,0,1,1]],"entrance":{"x":5,"z":5}}]`,
		`[{"key":"a","name":"A","boxes":[[0,0,1,1]],"entrance":{"x":5,"z":5},"cap":-1}]`} {
		if _, err := ReadDungeons(worlds.Built{Recipe: worlds.Recipe{World: 14, Dungeons: json.RawMessage(bad)}}); err == nil {
			t.Fatalf("read %s", bad)
		}
	}
}

// TestDungeonCreatureRollsItsRowsLoot: a creature of the dungeons' world rolls its row's tables ("#n") and every
// creature's, never its namesakes' in the open world.
func TestDungeonCreatureRollsItsRowsLoot(t *testing.T) {
	gw := entity.NewWorld()
	gw.SetDungeons(true, 14, nil, entity.DungeonSettings{})
	t.Cleanup(func() { gw.SetDungeons(false, 14, nil, entity.DungeonSettings{}) })
	tmpl := &entity.NPC{ServerID: 777001, Name: "gnoll scout", World: 14}
	copyOf := &entity.NPC{ServerID: 5, Name: "gnoll scout", World: 14, Temporary: true, CopyOf: 777001}
	out := &entity.NPC{ServerID: 6, Name: "gnoll scout", World: 0}
	for _, c := range []struct {
		n    *entity.NPC
		want []string
	}{{tmpl, []string{"#777001", "*"}}, {copyOf, []string{"#777001", "*"}}, {out, []string{"gnoll scout", "*"}}} {
		if got := lootNames(c.n); len(got) != len(c.want) || got[0] != c.want[0] || got[1] != c.want[1] {
			t.Fatalf("%+v rolls %v, want %v", c.n, got, c.want)
		}
	}
}

// TestDungeonLoginAtTheEntrance: a login saved inside its party's open instance resumes at the dungeon's entrance,
// never where it logged out (petition 1808: a player stuck under a rock at Blackburrow's old landing would have been
// put back under it), and in the instance.
func TestDungeonLoginAtTheEntrance(t *testing.T) {
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	r.anna.SetServerWorlds([]uint8{14})
	dungeonCommand(r.gw, r.anna, "enter bb")
	r.travel()
	if r.pa.World != 14 || r.pa.InstanceKey() == 0 {
		t.Fatalf("Anna not in the dungeon: world %d instance %d", r.pa.World, r.pa.InstanceKey())
	}
	held := func(w uint8) bool { return w == 14 }
	rec := &database.CharacterRecord{ServerID: r.pa.CharServerID, CharName: "Anna", World: 14, X: 1050, Y: 57, Z: 960}
	loginSpawn(r.gw, rec, held, nil)
	if rec.World != 14 || rec.X != 1000 || rec.Y != 50 || rec.Z != 1150 {
		t.Fatalf("a login inside the instance: world %d (%.1f, %.1f, %.1f), want its entrance", rec.World, rec.X, rec.Y, rec.Z)
	}
}

// TestDungeonsPagesTheirBox: ten dungeons (eleven since the Salt Mines) and the way out are more than a box holds (maxDialogOptions): five a box and
// "More dungeons", the last page back to the first.
func TestDungeonsPagesTheirBox(t *testing.T) {
	var all []dungeonsAnswer
	for i := 0; i < 11; i++ {
		all = append(all, dungeonsAnswer{do: dungeonsDoEnter, key: string(rune('a' + i))})
	}
	seen := map[string]bool{}
	page := 0
	for k := 0; k < 3; k++ {
		box := dungeonsPage(all, page)
		if len(box) > maxDialogOptions || box[len(box)-1].do != dungeonsDoMore {
			t.Fatalf("page %d: %d answers, last %+v", page, len(box), box[len(box)-1])
		}
		for _, a := range box[:len(box)-1] {
			seen[a.key] = true
		}
		page = box[len(box)-1].page
	}
	if len(seen) != 11 || page != 0 {
		t.Fatalf("three pages showed %d of 11, then page %d", len(seen), page)
	}
	if got := dungeonsPage(all[:6], 0); len(got) != 6 || got[5].do == dungeonsDoMore {
		t.Fatalf("six answers fit one box: %+v", got)
	}
}

// TestDungeonKeepRestartSendsOut: a character inside an instance at a keep restart whose instance did not come through
// (a file from before entity/keepdungeon.go, or its dungeon gone from the recipe) comes back in front of the Dungeon Warden outside, in its world, and its session is marked moved, so the
// client's reports of the instance's place until its dump do not carry the character there in that world (the owner,
// 2026-11-12: "I was in instanced Castle Felstar and the server restarted and I reloaded into the normal level 20
// version").
func TestDungeonKeepRestartSendsOut(t *testing.T) {
	db := keepDB(t)
	if _, err := db.Exec(`UPDATE characters SET world = 14, x = 1050, y = 57, z = 960 WHERE serverid = 7`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO npcs (id, npc_name, world, x_coord, y_coord, z_coord, facing, modelid, size, npc_level, npc_type, race, zone)
		VALUES (990201, ?, 0, 5000, 50, 5000, 0, 1843770060, 1, 60, 128, '0', 0)`, entity.DungeonWardenName); err != nil {
		t.Fatal(err)
	}
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	_, gw, sm := keepProcess(t, db)
	t.Cleanup(func() {
		SetServerWorlds(nil)
		gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	gw.LoadNPCs(database.NewNPCStore(db))
	if !SetupDungeons(gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	sess := sm.CreateSession(&net.UDPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 5009}, 9, 9, 9, 1, true)
	sess.SetAccountID(3)
	sess.SetPlayerObjectID(500000009)
	sess.SetState(session.StateInGame)
	sess.SetCharacterInWorld(true)
	sess.SetServerWorlds([]uint8{14})
	sess.SetCharacterServerID(7)
	p, err := ResumeCharacter(sess, KeptExtras{}, entity.KeptPlayer{Name: "Hero"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if p.World != 0 || abs32(p.X-5000) > 10 || abs32(p.Z-5000) > 10 {
		t.Fatalf("resumed at (%.1f, %.1f) world %d, want before the warden in world 0", p.X, p.Z, p.World)
	}
	sess.SetResumeDump(time.Now().Add(time.Second), false)
	if !sess.ResumeMoved() {
		t.Fatal("the session is not marked moved: the client's reports would carry it to the instance's place")
	}
	if due, _ := sess.TakeResumeDue(time.Now().Add(2 * time.Second)); !due || sess.ResumeMoved() {
		t.Fatalf("the dump due %v, still moved %v", due, sess.ResumeMoved())
	}
}

// Petition 1834 (Waldorf, 2026-11-12): "When in a group, selecting a raid instance from the board should transport
// the group all at once if within range". Anna leads; Cora beside her goes in with her, into her instance; Bert, of
// the group but 15,000 off, stays where he is.
func TestDungeonEntryTakesTheGroupNear(t *testing.T) {
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	for _, s := range []*session.Session{r.anna, r.cora, r.bert} {
		s.SetServerWorlds([]uint8{14})
	}
	r.group(t, r.anna, r.pa, r.cora, r.pc)
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pb))
	r.gw.GroupAccept(r.bert, r.gw.IDSeenBy(r.bert, r.pa))
	if len(r.gw.GroupMembers(r.pa)) != 3 {
		t.Fatalf("group: %v", r.gw.GroupMembers(r.pa))
	}
	r.drain()
	dungeonCommand(r.gw, r.anna, "enter bb")
	r.travel()
	if r.pa.World != 14 || r.pa.InstanceKey() == 0 {
		t.Fatalf("Anna not in: world %d instance %d", r.pa.World, r.pa.InstanceKey())
	}
	if r.pc.World != 14 || r.pc.InstanceKey() != r.pa.InstanceKey() {
		t.Fatalf("Cora not taken along: world %d instance %d (Anna's %d)", r.pc.World, r.pc.InstanceKey(), r.pa.InstanceKey())
	}
	if r.pb.World != 0 || r.pb.InstanceKey() != 0 {
		t.Fatalf("Bert, far off, was taken: world %d instance %d", r.pb.World, r.pb.InstanceKey())
	}
	found := false
	for _, m := range r.msgs(r.cora) {
		found = found || strings.Contains(m, "Anna takes the group into the instance.")
	}
	if !found {
		t.Fatalf("Cora not told: %q", r.msgs(r.cora))
	}
}

// Petition 1840 (Waldorf, 2026-11-12): an "instance ground rules" choice on the board. The warden's box opens with
// "How instances work", and picking it tells the rules (no respawn among them); the dungeons follow it.
func TestDungeonBoardTellsTheRules(t *testing.T) {
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	r.anna.SetServerWorlds([]uint8{14})
	warden := &entity.NPC{ServerID: 7791, Name: entity.DungeonWardenName, X: 5001, Y: 50, Z: 5000, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	r.gw.NPCs[warden.ServerID] = warden
	ds := dungeonsH.Load()
	ds.askRoot(r.anna, r.pa, warden, 0)
	ds.mu.Lock()
	asked := ds.asked[r.pa.CharServerID]
	ds.mu.Unlock()
	if len(asked.answers) < 2 || asked.answers[0].label != dungeonsAnsRules || asked.answers[1].key != "bb" {
		t.Fatalf("the box: %+v", asked.answers)
	}
	r.drain()
	r.ih.answerDungeons(r.anna, r.pa, asked.box, 0)
	got := strings.Join(r.msgs(r.anna), "\n")
	if !strings.Contains(got, "How instances work:") || !strings.Contains(got, "nothing respawns") {
		t.Fatalf("the rules not told: %q", got)
	}
}

// /addinstance and /removeinstance (petition 1879) are /dungeon add and remove, either spelling; another word is none.
func TestDungeonMemberWords(t *testing.T) {
	for w, want := range map[string]string{"/addinstance": "add", "#ADDINSTANCE": "add", "/removeinstance": "remove",
		"#removeinstance": "remove", "/dungeon": "", "/add": ""} {
		if got := dungeonMemberWord(w); got != want {
			t.Errorf("%s: %q, want %q", w, got, want)
		}
	}
}

// The Instanced Raids board resets an instance its player opened (the owner, 2026-11-13): its box offers "Reset
// <dungeon>" for each, and picking it closes the instance when no one is inside. The control: a Dungeon Warden offers
// no reset.
func TestDungeonBoardResetsAnInstance(t *testing.T) {
	r := newPortalRig(t)
	block := `[{"key":"bb","name":"Test Burrow","boxes":[[900,900,1100,1100]],"entrance":{"x":1000,"y":50,"z":1150,"toward":[1000,1000]}}]`
	SetServerWorlds([]worlds.Built{{Recipe: worlds.Recipe{World: 14, Name: "the Test Dungeons", Dungeons: json.RawMessage(block)}}})
	t.Cleanup(func() {
		SetServerWorlds(nil)
		r.gw.SetDungeons(false, 14, nil, entity.DungeonSettings{})
	})
	if !SetupDungeons(r.gw, config.DungeonsConfig{Enabled: true}, nil) {
		t.Fatal("the dungeons are not on")
	}
	r.anna.SetServerWorlds([]uint8{14})
	if line, ok := r.gw.DungeonEnter(r.anna, "bb", time.Now()); !ok {
		t.Fatalf("not opened: %s", line)
	}
	r.pa.World, r.pa.Instance = 0, 0 // back out in Freeport: no one inside
	board := &entity.NPC{ServerID: 7792, Name: entity.DungeonRaidBoardName, X: 5001, Y: 50, Z: 5000, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	warden := &entity.NPC{ServerID: 7793, Name: entity.DungeonWardenName, X: 5001, Y: 50, Z: 5001, Level: 60, HP: 1, MaxHP: 1, NPCType: 0x80}
	r.gw.NPCs[board.ServerID], r.gw.NPCs[warden.ServerID] = board, warden
	ds := dungeonsH.Load()
	reset := func(npc *entity.NPC) (dungeonsAsk, int) {
		ds.askRoot(r.anna, r.pa, npc, 0)
		ds.mu.Lock()
		defer ds.mu.Unlock()
		a := ds.asked[r.pa.CharServerID]
		for i, x := range a.answers {
			if x.do == dungeonsDoReset {
				return a, i
			}
		}
		return a, -1
	}
	if _, i := reset(warden); i >= 0 {
		t.Fatal("a Dungeon Warden offers a reset")
	}
	asked, i := reset(board)
	if i < 0 || asked.answers[i].label != "Reset Test Burrow" {
		t.Fatalf("the board offers no reset: %+v", asked.answers)
	}
	r.drain()
	r.ih.answerDungeons(r.anna, r.pa, asked.box, byte(i))
	if got := r.gw.DungeonChoices(r.anna); len(got) != 1 || got[0].Open {
		t.Fatalf("the instance is still open: %+v (told %q)", got, r.msgs(r.anna))
	}
}
