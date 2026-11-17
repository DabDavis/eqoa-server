package entity

import (
	"encoding/binary"
	"math"
	"path/filepath"
	"testing"

	"github.com/eqoa/server/internal/database"
)

// A season's NPCs and lights (decorlights.go, spawnonly.go, database/seasons.go).

// A season's tree is out of every view while the season is off and in them
// while it is on, and its lights are laid only while it is on; a string of no
// season is always laid.
func TestASeasonsTreeAndLightsComeAndGoWithIt(t *testing.T) {
	db, err := database.NewSQLite(filepath.Join(t.TempDir(), "eqoa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO npcseason (name, season) VALUES ('Christmas Tree', 'Christmas')`,
		`INSERT INTO lighttemplates (name, colors, effect, spacing) VALUES ('Christmas lights', 'FF2020,20D040,3070FF', 'twinkle', 1)`,
		`INSERT INTO lightstrings (template, npc, shape, params, season) VALUES ('christmas lights', 'Christmas Tree', 'spiral', '{"r0":3,"y0":1,"r1":0.5,"y1":9,"turns":4,"top":"FFD040"}', 'Christmas')`,
		`INSERT INTO lightstrings (template, npc, world, shape, params) VALUES ('Christmas lights', '', 0, 'line', '{"from":[100,5,100],"to":[110,5,100]}')`,
		`INSERT INTO lightstrings (template, npc, shape, params, season) VALUES ('no such', 'Christmas Tree', 'spiral', '{}', 'Christmas')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	f := newAggroFix(t, 10)
	tree := f.npc(2001, "Christmas Tree", 1, creationBody, 1010, 1000)
	if _, _, err := f.w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if !tree.SpawnOnly || f.inView(tree) {
		t.Fatalf("Christmas off: the tree held %v, in view %v; want held, out of view", tree.SpawnOnly, f.inView(tree))
	}
	strs, bulbs, _, problems, err := f.w.LoadDecorLights(db)
	if err != nil || strs != 1 || bulbs != 11 || len(problems) != 0 {
		t.Fatalf("Christmas off: %d strings, %d bulbs, %v, %v; want the world's line alone (11 bulbs)", strs, bulbs, problems, err)
	}
	if _, err := db.SetSeason("Christmas", true, "dab"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.w.LoadSpawnOnly(db); err != nil {
		t.Fatal(err)
	}
	if tree.SpawnOnly || !f.inView(tree) {
		t.Fatalf("Christmas on: the tree held %v, in view %v; want it in the world", tree.SpawnOnly, f.inView(tree))
	}
	strs, bulbs, _, problems, _ = f.w.LoadDecorLights(db)
	if strs != 2 || bulbs <= 11+20 || len(problems) != 1 {
		t.Fatalf("Christmas on: %d strings, %d bulbs, problems %v; want the tree's too, and the string of no template said", strs, bulbs, problems)
	}
}

// OpcodeDecorLights's body, byte for byte at its head: the strings in order,
// an NPC's by its keyed name, a world's by its world.
func TestEncodeDecorLights(t *testing.T) {
	strs := []DecorString{
		{NPC: "christmas tree", Effect: 1, Period: 1.5, Size: 0.3, Bright: 1, Colors: []uint32{0xFF2020, 0x20D040},
			Bulbs: []database.Bulb{{X: 1, Y: 2, Z: 3, Color: 1, Phase: 7, Break: true}}, Cord: 0x1E3F1E, CordOn: true, CordWidth: 0.06, CordSag: 0.06},
		{Problem: "lays nothing"},
		{World: 3, Effect: 3, Period: 2, Size: 0.5, Bright: 2, Colors: []uint32{0xFFFFFF}, Bulbs: []database.Bulb{{}, {X: 1}}},
	}
	b, sent, bulbs := EncodeDecorLights(strs, []database.DecorArt{{Model: 0x0E318233, World: 0, Zone: 32, Sink: 9.5}})
	if sent != 2 || bulbs != 3 || binary.LittleEndian.Uint16(b) != 2 {
		t.Fatalf("sent %d strings, %d bulbs, count %d", sent, bulbs, binary.LittleEndian.Uint16(b))
	}
	r := b[2:]
	if r[0] != 1 || int(r[1]) != len("christmas tree") || string(r[2:16]) != "christmas tree" || r[16] != 1 {
		t.Fatalf("the tree's head: % X", r[:17])
	}
	r = r[17:]
	f32 := func(o int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(r[o:])) }
	if f32(0) != 1.5 || f32(4) != 0.3 || f32(8) != 1 || binary.LittleEndian.Uint32(r[12:]) != 0x1E3F1EFF || f32(16) != 0.06 || f32(20) != 0.06 ||
		r[24] != 2 || binary.LittleEndian.Uint32(r[25:]) != 0xFF2020FF {
		t.Fatalf("the tree's look: % X", r[:33])
	}
	r = r[33:]
	if binary.LittleEndian.Uint16(r) != 1 || f32(2) != 1 || f32(6) != 2 || f32(10) != 3 || r[14] != 1|DecorCordBreak || r[15] != 7 {
		t.Fatalf("the tree's bulb: % X", r[:16])
	}
	r = r[16:]
	if r[0] != 0 || r[1] != 3 || r[2] != 3 {
		t.Fatalf("the world's string's head: % X", r[:3])
	}
	// ... then the presents' count and the looks' three counts (none of each)
	if len(r) != 3+12+12+1+4+2+2*14+2+11+2+2+2+2 {
		t.Fatalf("the world's string and the art: %d bytes left", len(r))
	}
	r = r[3+12+12+1+4+2+2*14:]
	if binary.LittleEndian.Uint16(r) != 1 || binary.LittleEndian.Uint32(r[2:]) != 0x0E318233 || r[6] != 0 || binary.LittleEndian.Uint16(r[7:]) != 32 || math.Float32frombits(binary.LittleEndian.Uint32(r[9:])) != 9.5 {
		t.Errorf("the art: % X", r)
	}
	if r = r[13:]; binary.LittleEndian.Uint16(r) != 0 {
		t.Errorf("the presents: % X, want none", r)
	}
}

// A decoration (decorart) is selectable by no one, and its art is in its model
// only for a client that draws decorations; another NPC is as it was.
func TestADecorationsArtGoesOnlyToTheClientsThatDrawIt(t *testing.T) {
	f := newAggroFix(t, 10)
	tree := f.npc(2001, "Christmas Tree", 1, creationBody, 1010, 1000)
	other := f.npc(2002, "giant beetle", 1, creationBody, 1000, 1010)
	art := map[string]database.DecorArt{"christmas tree": {Name: "christmas tree", Model: 0x0E318233, Zone: 32, Sink: 9.6}}
	f.w.SetDecorLights(nil, art, nil)
	f.w.mu.Lock()
	ps2 := f.w.serializeEntityForChannel(f.sess, tree.ServerID, 5)
	beetle := f.w.serializeEntityForChannel(f.sess, other.ServerID, 6)
	f.sess.SetDecorLights(1)
	vanilla := f.w.serializeEntityForChannel(f.sess, tree.ServerID, 5)
	f.w.mu.Unlock()
	if m := binary.LittleEndian.Uint32(ps2[28:]); m != creationBody || ps2[26]&1 != 0 {
		t.Errorf("a PS2's tree: model %08X, byte 26 %d; want the row's body, not selectable", m, ps2[26])
	}
	if m := binary.LittleEndian.Uint32(vanilla[28:]); m != 0x0E318233 || vanilla[26]&1 != 0 {
		t.Errorf("vanilla's tree: model %08X, byte 26 %d; want the fir, not selectable", m, vanilla[26])
	}
	if m := binary.LittleEndian.Uint32(beetle[28:]); m != creationBody || beetle[26]&1 != 1 {
		t.Errorf("the control: model %08X, byte 26 %d", m, beetle[26])
	}
}

// A presents string carries its boxes after the art, needs no template, lays no
// bulbs; a box with no ribbon sends 0.
func TestDecorPresents(t *testing.T) {
	rows := []database.LightString{{ID: 7, NPC: "Christmas Tree", Shape: database.ShapePresents,
		Params: `{"boxes":[{"at":[1,0,2],"size":[1,0.8,1],"yaw":90,"color":"C02020","ribbon":"FFD040","bow":true},{"at":[0,0,0],"size":[0.5,0.5,0.5],"color":"2050C0"}]}`}}
	strs := BuildDecorStrings(nil, rows, nil)
	if len(strs) != 1 || strs[0].Problem != "" || len(strs[0].Gifts) != 2 || len(strs[0].Bulbs) != 0 {
		t.Fatalf("built: %+v", strs)
	}
	g := strs[0].Gifts[0]
	if g.Color != 0xC02020 || !g.RibbonOn || g.Ribbon != 0xFFD040 || !g.Bow || math.Abs(float64(g.Yaw)-math.Pi/2) > 1e-6 {
		t.Errorf("box 1: %+v", g)
	}
	b, sent, _ := EncodeDecorLights(strs, nil)
	if sent != 0 || binary.LittleEndian.Uint16(b) != 0 || binary.LittleEndian.Uint16(b[2:]) != 0 {
		t.Fatalf("no string, no art: % X", b[:4])
	}
	r := b[4:]
	if binary.LittleEndian.Uint16(r) != 1 || r[2] != 1 || string(r[4:4+int(r[3])]) != "christmas tree" {
		t.Fatalf("the set's head: % X", r[:20])
	}
	r = r[4+int(r[3]):]
	// the boxes, then the looks: no asset, no string's look, the set's two papers (none, gain 1)
	if binary.LittleEndian.Uint16(r) != 2 || len(r) != 2+2*(7*4+4+4+1)+2+2+2+2+2*8 {
		t.Fatalf("the boxes: %d bytes", len(r))
	}
	r = r[2:]
	if binary.LittleEndian.Uint32(r[28:]) != 0xC02020FF || binary.LittleEndian.Uint32(r[32:]) != 0xFFD040FF || r[36] != 1 {
		t.Errorf("box 1's colours: % X", r[28:37])
	}
	if binary.LittleEndian.Uint32(r[37+32:]) != 0 || r[37+36] != 0 {
		t.Errorf("box 2: no ribbon, no bow: % X", r[37+28:37+37])
	}
	for _, bad := range []string{`{"boxes":[]}`, `{"boxes":[{"at":[0,0,0],"size":[0,1,1],"color":"FF0000"}]}`, `{"boxes":[{"at":[0,0,0],"size":[1,1,1],"color":"red"}]}`} {
		if s := BuildDecorStrings(nil, []database.LightString{{Shape: "presents", Params: bad}}, nil); s[0].Problem == "" {
			t.Errorf("%s: no problem said", bad)
		}
	}
}

// A template's look and a box's paper go after the presents: the assets a look
// names (and none it does not), each string's look by its index among those
// sent, each box's paper and gain; a paper that is no DictID is a problem.
func TestDecorLooks(t *testing.T) {
	templates := map[string]database.LightTemplate{
		"globes": {Name: "Globes", Colors: []uint32{0xFF2020}, Pattern: database.LightCycle, Effect: "steady", Spacing: 1, Size: 0.2, Bright: 1,
			Cord: 0xC0A040, CordOn: true, CordWidth: 0.3, Bulb: 0xF8E6C045, BulbScale: 0.4, CordTexture: 0x7B3D8677, CordRepeat: 1, CordSag: 0.2},
		"plain": {Name: "Plain", Colors: []uint32{0xFFFFFF}, Pattern: database.LightCycle, Effect: "steady", Spacing: 1, Size: 0.3, Bright: 1},
	}
	rows := []database.LightString{
		{ID: 1, Template: "Plain", World: 0, Shape: "line", Params: `{"from":[0,0,0],"to":[3,0,0]}`},
		{ID: 2, Template: "Globes", World: 0, Shape: "line", Params: `{"from":[0,0,0],"to":[3,0,0]}`},
		{ID: 3, NPC: "Christmas Tree", Shape: database.ShapePresents, Params: `{"boxes":[{"at":[1,0,2],"size":[1,1,1],"color":"C02020","paper":"0x58B41D37","gain":2.5},{"at":[0,0,0],"size":[1,1,1],"color":"2050C0"}]}`},
	}
	strs := BuildDecorStrings(templates, rows, nil)
	if strs[1].BulbModel != 0xF8E6C045 || strs[1].CordTexture != 0x7B3D8677 || math.Abs(float64(strs[1].CordSag)-0.2) > 1e-6 {
		t.Fatalf("the globes' look: %+v", strs[1])
	}
	if g := strs[2].Gifts; g[0].Paper != 0x58B41D37 || g[0].Gain != 2.5 || g[1].Paper != 0 || g[1].Gain != 1 {
		t.Fatalf("papers: %+v", g)
	}
	assets := []database.DecorAsset{
		{Name: "globe", World: 0, Zone: 108, IDs: []uint32{0xF8E6C045}},
		{Name: "tent", World: 2, Zone: 15, IDs: []uint32{0x4BC0D7FA, 0x7B3D8677}},
		{Name: "plaid", World: 0, Zone: 28, IDs: []uint32{0x58B41D37}},
		{Name: "unused", World: 2, Zone: 34, IDs: []uint32{0x9196DAC4}},
	}
	b, sent, _ := EncodeDecorLights(strs, nil, assets...)
	if sent != 2 {
		t.Fatalf("sent %d strings", sent)
	}
	// the tail: assets (3 named: globe, tent, plaid), one look, one set of two papers
	tail := 2 + (3 + 1 + 4) + (3 + 1 + 8) + (3 + 1 + 4) + 2 + (2 + 16) + 2 + 2 + 2*8
	r := b[len(b)-tail:]
	if binary.LittleEndian.Uint16(r) != 3 || r[2] != 0 || binary.LittleEndian.Uint16(r[3:]) != 108 || r[5] != 1 || binary.LittleEndian.Uint32(r[6:]) != 0xF8E6C045 {
		t.Fatalf("the assets: % X", r[:12])
	}
	r = r[2+8+12+8:]
	if binary.LittleEndian.Uint16(r) != 1 || binary.LittleEndian.Uint16(r[2:]) != 1 || binary.LittleEndian.Uint32(r[4:]) != 0xF8E6C045 ||
		binary.LittleEndian.Uint32(r[12:]) != 0x7B3D8677 {
		t.Fatalf("the look: the second string sent's: % X", r[:20])
	}
	r = r[2+18:]
	if binary.LittleEndian.Uint16(r) != 1 || binary.LittleEndian.Uint16(r[2:]) != 2 || binary.LittleEndian.Uint32(r[4:]) != 0x58B41D37 ||
		math.Float32frombits(binary.LittleEndian.Uint32(r[8:])) != 2.5 || binary.LittleEndian.Uint32(r[12:]) != 0 {
		t.Fatalf("the papers: % X", r)
	}
	bad := []database.LightString{{Shape: "presents", Params: `{"boxes":[{"at":[0,0,0],"size":[1,1,1],"color":"FF0000","paper":"tent"}]}`}}
	if s := BuildDecorStrings(nil, bad, nil); s[0].Problem == "" {
		t.Error("a paper that is no DictID: no problem said")
	}
}
