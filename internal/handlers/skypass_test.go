package handlers

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eqoa/server/internal/database"
	"github.com/eqoa/server/internal/entity"
	"github.com/eqoa/server/internal/session"
)

// skyRig is the portal rig's Anna and Cora grouped on the Plane of Sky (Bert
// far off, ungrouped), a quest engine on a database holding the chains' steps
// the tests walk, and the essences of rain at their spots.
type skyRig struct {
	*portalRig
	db    *database.SQLiteDB
	inv   *database.InventoryStore
	keys  map[string]int
	npcs  map[string]*entity.NPC
	idSeq entity.EntityID
}

func newSkyRig(t *testing.T) *skyRig {
	t.Helper()
	r := &skyRig{portalRig: newPortalRig(t), keys: map[string]int{}, npcs: map[string]*entity.NPC{}, idSeq: 9000}
	db, err := database.NewSQLite(t.TempDir() + "/sky.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r.db, r.inv = db, database.NewInventoryStore(db)
	for _, p := range []*entity.Player{r.pa, r.pc, r.pb} {
		rec := &database.CharacterRecord{CharName: p.Name, AccountID: 1}
		if err := database.NewSQLiteCharacterStore(db).Create(rec); err != nil {
			t.Fatal(err)
		}
		p.CharServerID, p.Level = rec.ServerID, 60
	}
	for _, s := range r.sessions() {
		s.SetInventoryList(newInvList(nil))
	}
	for _, k := range []string{"rain key", "Rain Key 2", "Rain Key 3"} {
		res, err := db.Exec(`INSERT INTO itempattern
			(itemcost, unk1, itemicon, unk2, equipslot, unk3, trade, rent, unk4, attacktype, weapondamage, unk5, levelreq, maxstack,
			 maxhp, duration, classuse, raceuse, procanim, lore, unk6, craft, itemname, itemdesc, model, color)
			VALUES (0, 0, 1342617881, 0, -1, 0, 1, 1, 0, 0, 0, 0, 0, 99, 0, 50, 0, 0, 0, 0, 0, 1, ?, '', 0, 0)`, k)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		r.keys[k] = int(id)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// the chain's steps at the essences, as imported (their teleports taken
	// off by tools/sky_keys_20261111.py), and a quest of another's at one
	exec(`INSERT INTO quests (questid, name, enabled, giver, minlevel) VALUES (101301, 'The Key of Rain', 1, 'Liztin Misseliania', 50),
		(101302, 'Sterie Glitterlight', 1, 'Trixie Longwings', 50), (101303, 'The Necrologist of Rain', 1, 'Singe Burnfeathers', 50),
		(900001, 'A word with the rain', 1, 'Nobody', 1)`)
	exec(fmt.Sprintf(`INSERT INTO queststeps (questid, step, kind, target, items, journal, choices) VALUES
		(101301, 1, 'give', 'Liztin Misseliania', '', 'Buy a rain key.', ''),
		(101301, 2, 'give', 'essence of rain', '%d', 'Use the rain key on the essence of rain.',
		 '{"ask":"Place your key in it?","options":[{"answer":"Place the key in the hole.","says":"The key turns, and the light takes you."},{"answer":"Step back from the light.","says":"The essence dims and waits.","stay":true}]}'),
		(101301, 3, 'kill', 'Artiokanis', '', 'Kill Artiokanis.', ''),
		(101302, 4, 'talk', 'essence of rain', '', 'Use the wand''s keys on the essence of rain.', ''),
		(101302, 5, 'talk', 'Nobody', '', 'Then nothing.', ''),
		(101303, 5, 'talk', 'essence of rain', '', 'Use the wand''s keys on the essence of rain down the road.', ''),
		(101303, 6, 'talk', 'Nobody', '', 'Then nothing.', ''),
		(900001, 1, 'talk', 'essence of rain', '', 'Speak to the essence of rain.', ''),
		(900001, 2, 'talk', 'Nobody', '', 'Then nothing.', '')`, r.keys["rain key"]))
	qe, err := NewQuestEngine(db, r.gw, r.sm, r.inv, database.NewSpellbookStore(db))
	if err != nil {
		t.Fatal(err)
	}
	qe.Start()
	t.Cleanup(qe.Stop)
	// the essences of rain: the main isle's, the second isle's, a third
	// nowhere a passage names; Granavik's portal
	r.essence("main", "essence of rain", 4482.5, 244.7, 5293.2)
	r.essence("rain2", "essence of rain", 4158.6, 180.9, 5122.5)
	r.essence("stray", "essence of day", 4000, 100, 4000)
	r.essence("portal", "Rain 3 Portal", 4310, -69, 4705)
	return r
}

func (r *skyRig) sessions() []*session.Session { return []*session.Session{r.anna, r.cora, r.bert} }

// essence puts an NPC of the name at its spawn in world 4.
func (r *skyRig) essence(key, name string, x, y, z float32) {
	r.idSeq++
	n := &entity.NPC{ServerID: r.idSeq, Name: name, World: 4, Level: 60, NPCType: 128, SpawnX: x, SpawnY: y, SpawnZ: z, X: x, Y: y, Z: z}
	r.gw.NPCs[n.ServerID] = n
	r.npcs[key] = n
}

// by stands the players at the NPC (Anna on it, Cora a step off), Bert far.
func (r *skyRig) by(key string) {
	n := r.npcs[key]
	for i, p := range []*entity.Player{r.pa, r.pc} {
		p.World = 4
		r.gw.UpdatePosition(p.ServerID, n.X+float32(i)*2, n.Y, n.Z+1, 0)
	}
	r.drain()
}

// own gives the player n of the key.
func (r *skyRig) own(p *entity.Player, key string, n int) {
	r.t.Helper()
	if _, err := r.inv.AddCharItem(p.CharServerID, r.keys[key], n, 0, 0, 0); err != nil {
		r.t.Fatal(err)
	}
	rows, err := r.inv.GetCharacterItems(p.CharServerID)
	if err != nil {
		r.t.Fatal(err)
	}
	r.gw.FindSessionByEntityID(p.ServerID).SetInventoryList(newInvList(rows))
}

func (r *skyRig) held(p *entity.Player, key string) int {
	var n int
	r.db.QueryRow(`SELECT COALESCE(SUM(stackleft), 0) FROM charinventory WHERE serverid = ? AND patternid = ?`, p.CharServerID, r.keys[key]).Scan(&n)
	return n
}

// on puts the character on a quest's step.
func (r *skyRig) on(p *entity.Player, quest, step int) {
	r.t.Helper()
	if _, err := r.db.Exec(`INSERT OR REPLACE INTO charquests (serverid, questid, step, slot) VALUES (?, ?, ?, ?)`, p.CharServerID, quest, step, quest%100); err != nil {
		r.t.Fatal(err)
	}
	if err := Quests().Reload(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *skyRig) step(p *entity.Player, quest int) (int, int) {
	var step, done int
	if err := r.db.QueryRow(`SELECT step, done FROM charquests WHERE serverid = ? AND questid = ?`, p.CharServerID, quest).Scan(&step, &done); err != nil {
		return -1, -1
	}
	return step, done
}

func (r *skyRig) group() {
	r.t.Helper()
	r.gw.GroupInvite(r.anna, r.gw.IDSeenBy(r.anna, r.pc))
	r.gw.GroupAccept(r.cora, r.gw.IDSeenBy(r.cora, r.pa))
	if n := len(r.gw.GroupMembers(r.pa)); n != 2 {
		r.t.Fatalf("the group: %d members", n)
	}
}

// Petition 1608 (Jibrish, at the second isle of rain's essence): without its
// boss's key the essence says what it wants and sends no one, its quest step
// waiting; with it, it takes one key and sends the giver and the group by it to
// the third isle's landing, each one's step of the chain done there.
func TestTheSecondIsleOfRainsEssenceTakesItsKey(t *testing.T) {
	r := newSkyRig(t)
	r.group()
	r.on(r.pa, 101302, 4)
	r.on(r.pc, 101302, 4)
	r.by("rain2")
	r.talk(r.anna, r.npcs["rain2"])
	got := r.msgs(r.anna)
	if !hasMsg(got, "box:The essence of rain wants a Rain Key 2 before it will take you on.") || hasMsg(got, "teleport") {
		t.Fatalf("no key: %q", got)
	}
	if s, _ := r.step(r.pa, 101302); s != 4 {
		t.Errorf("the step without the key: %d", s)
	}

	r.own(r.pa, "Rain Key 2", 3)
	r.drain()
	r.talk(r.anna, r.npcs["rain2"])
	got = r.msgs(r.anna)
	if !hasMsg(got, "chat:essence of rain: your Rain Key 2 turns in the light, and it takes you to the third isle of rain.") || !hasMsg(got, "teleport") {
		t.Fatalf("with the key: %q", got)
	}
	if r.pa.World != 4 || r.pa.X != 4397.54 || r.pa.Y != 110.80 || r.pa.Z != 4559.93 {
		t.Errorf("Anna landed at world %d (%.2f, %.2f, %.2f)", r.pa.World, r.pa.X, r.pa.Y, r.pa.Z)
	}
	if n := r.held(r.pa, "Rain Key 2"); n != 2 {
		t.Errorf("Anna holds %d keys, want 2 (one a trip)", n)
	}
	if s, d := r.step(r.pa, 101302); s != 5 || d != 0 {
		t.Errorf("Anna's step: %d done %d, want 5", s, d)
	}
	cora := r.msgs(r.cora)
	if !hasMsg(cora, "chat:essence of rain takes the group to the third isle of rain.") || !hasMsg(cora, "teleport") ||
		r.pc.X != r.pa.X || r.pc.Z != r.pa.Z {
		t.Errorf("Cora was not taken along: %q at (%.1f, %.1f)", cora, r.pc.X, r.pc.Z)
	}
	if s, _ := r.step(r.pc, 101302); s != 5 {
		t.Errorf("Cora's step: %d, want 5", s)
	}
	if hasMsg(r.msgs(r.bert), "teleport") || r.pb.X != 20000 {
		t.Errorf("Bert, far off and ungrouped, was moved")
	}
}

// The main isle's essence takes the key Liztin sells: the chain's give step
// takes it (one key, not two), its question answered by the passage.
func TestTheMainIslesEssenceTakesTheRainKeyOnce(t *testing.T) {
	r := newSkyRig(t)
	r.on(r.pa, 101301, 2)
	r.own(r.pa, "rain key", 2)
	r.by("main")
	r.talk(r.anna, r.npcs["main"])
	got := r.msgs(r.anna)
	if !hasMsg(got, "teleport") || r.pa.X != 4236.2 || r.pa.Z != 5727.7 {
		t.Fatalf("the first isle: %q at (%.1f, %.1f)", got, r.pa.X, r.pa.Z)
	}
	if n := r.held(r.pa, "rain key"); n != 1 {
		t.Errorf("Anna holds %d rain keys, want 1", n)
	}
	if s, _ := r.step(r.pa, 101301); s != 3 {
		t.Errorf("the give step: at %d, want 3", s)
	}
	// with no quest at all, the key alone
	r.by("main")
	r.talk(r.anna, r.npcs["main"])
	if got := r.msgs(r.anna); !hasMsg(got, "teleport") || r.held(r.pa, "rain key") != 0 {
		t.Errorf("no quest: %q, %d keys left", got, r.held(r.pa, "rain key"))
	}
}

// Granavik's portal (petition 1612) takes a Rain Key 3 to the fourth isle and
// does the chain's step at the third isle's essence; another isle's key does
// nothing there.
func TestGranaviksPortalTakesARainKey3(t *testing.T) {
	r := newSkyRig(t)
	r.on(r.pa, 101303, 5)
	r.own(r.pa, "Rain Key 2", 1)
	r.by("portal")
	r.talk(r.anna, r.npcs["portal"])
	if got := r.msgs(r.anna); !hasMsg(got, "box:The Rain 3 Portal wants a Rain Key 3 before it will take you on.") || hasMsg(got, "teleport") {
		t.Fatalf("the wrong key: %q", got)
	}
	r.own(r.pa, "Rain Key 3", 20)
	r.talk(r.anna, r.npcs["portal"])
	got := r.msgs(r.anna)
	if !hasMsg(got, "teleport") || r.pa.X != 4242.0 || r.pa.Z != 4305.3 {
		t.Fatalf("the fourth isle: %q at (%.1f, %.1f)", got, r.pa.X, r.pa.Z)
	}
	if r.held(r.pa, "Rain Key 3") != 19 || r.held(r.pa, "Rain Key 2") != 1 {
		t.Errorf("keys: %d Rain Key 3, %d Rain Key 2", r.held(r.pa, "Rain Key 3"), r.held(r.pa, "Rain Key 2"))
	}
	if s, _ := r.step(r.pa, 101303); s != 6 {
		t.Errorf("the essence's step: %d, want 6", s)
	}
}

// A quest step at an essence that no passage does goes as its quest has it,
// key or none; an essence of a passage's name where none stands, with no trip
// of its own, is dim.
func TestAnEssencesOtherBusiness(t *testing.T) {
	r := newSkyRig(t)
	r.on(r.pa, 900001, 1)
	r.own(r.pa, "Rain Key 2", 1)
	r.by("rain2")
	r.talk(r.anna, r.npcs["rain2"])
	got := r.msgs(r.anna)
	if hasMsg(got, "teleport") || r.held(r.pa, "Rain Key 2") != 1 {
		t.Errorf("another quest's step took the passage: %q", got)
	}
	if s, _ := r.step(r.pa, 900001); s != 2 {
		t.Errorf("its step: %d, want 2", s)
	}
	r.by("stray")
	r.talk(r.anna, r.npcs["stray"])
	if got := r.msgs(r.anna); !hasMsg(got, "box:The essence of day is dim: it takes no one from here.") || hasMsg(got, "teleport") {
		t.Errorf("a stray essence: %q", got)
	}
}

// Every passage lands in world 4 on a spot a passage of the same chain does
// not stand on, takes a key with a name, and each essence spot is one of a
// kind; the steps they do are each one passage's or the portal's twin.
func TestThePassagesTable(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range skyPasses {
		if p.At.World != 4 || p.Key == "" || p.To == "" {
			t.Errorf("%s at (%.0f, %.0f): %+v", p.NPC, p.X, p.Z, p)
		}
		k := fmt.Sprintf("%s@%.0f,%.0f", strings.ToLower(p.NPC), p.X, p.Z)
		if seen[k] {
			t.Errorf("two passages at %s", k)
		}
		seen[k] = true
		if _, err := ValidateTeleport(p.At.X, p.At.Y, p.At.Z, p.At.World); err != nil {
			t.Errorf("%s's landing: %v", k, err)
		}
	}
	if len(skyPassSteps()) != 23 {
		t.Errorf("%d steps, want the 23 the tool takes the teleports off", len(skyPassSteps()))
	}
}
