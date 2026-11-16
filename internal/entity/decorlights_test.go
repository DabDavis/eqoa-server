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
			Bulbs: []database.Bulb{{X: 1, Y: 2, Z: 3, Color: 1, Phase: 7}}},
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
	if f32(0) != 1.5 || f32(4) != 0.3 || f32(8) != 1 || r[12] != 2 || binary.LittleEndian.Uint32(r[13:]) != 0xFF2020FF {
		t.Fatalf("the tree's look: % X", r[:21])
	}
	r = r[21:]
	if binary.LittleEndian.Uint16(r) != 1 || f32(2) != 1 || f32(6) != 2 || f32(10) != 3 || r[14] != 1 || r[15] != 7 {
		t.Fatalf("the tree's bulb: % X", r[:16])
	}
	r = r[16:]
	if r[0] != 0 || r[1] != 3 || r[2] != 3 {
		t.Fatalf("the world's string's head: % X", r[:3])
	}
	if len(r) != 3+12+1+4+2+2*14+2+11 {
		t.Fatalf("the world's string and the art: %d bytes left", len(r))
	}
	r = r[3+12+1+4+2+2*14:]
	if binary.LittleEndian.Uint16(r) != 1 || binary.LittleEndian.Uint32(r[2:]) != 0x0E318233 || r[6] != 0 || binary.LittleEndian.Uint16(r[7:]) != 32 || math.Float32frombits(binary.LittleEndian.Uint32(r[9:])) != 9.5 {
		t.Errorf("the art: % X", r)
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
