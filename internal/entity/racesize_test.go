package entity

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eqoa/server/internal/database"
)

func float32frombytes(b []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b)) }

// jib's F3 20261029-212125: gnome players a tenth smaller, by table
// racesize. A gnome logging in is sent at 0.9 on the state channel's scale
// (bytes 36-39, every client's view of it, its own included); the control, a
// human, stays at 1.0. An online gnome takes a GM's new size at once, and
// back at 1.0 when the row goes; a row out of bounds (0: an invisible race)
// is dropped and the race drawn at 1.0. NPCs are not players: a gnome NPC
// keeps its row's size.
func TestAGnomePlayerIsATenthSmaller(t *testing.T) {
	w := NewWorld()
	w.SetRaceSizes(map[int]float64{3: 0.9})
	gnome := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 44, CharName: "Jibrish", Race: 3, TClass: 12, Level: 60,
		ModelID: int64(gnomeMale), X: 25300, Y: 54, Z: 15700})
	human := w.CreatePlayer(nil, &database.CharacterRecord{ServerID: 45, CharName: "Plainman", Race: 0, TClass: 0, Level: 60,
		ModelID: 0x70D898C6, X: 25302, Y: 54, Z: 15700})
	if _, s := channelBody(gnome); s != float32(0.9) || gnome.Size != float32(0.9) {
		t.Fatalf("a gnome player's scale %v, want 0.9", s)
	}
	if _, s := channelBody(human); s != 1 {
		t.Fatalf("the control: a human player's scale %v, want 1.0", s)
	}
	if w.RaceSize(3) != float32(0.9) || w.RaceSize(0) != 1 || w.RaceSize(10) != 1 {
		t.Fatalf("race sizes: gnome %v, human %v, froglok %v", w.RaceSize(3), w.RaceSize(0), w.RaceSize(10))
	}
	// and #stats says so to a GM
	w.mu.Lock()
	lines := w.playerStatsLocked(gnome, time.Now())
	w.mu.Unlock()
	if len(lines) == 0 || !strings.HasSuffix(lines[0], ", size 0.90") {
		t.Fatalf("#stats: %q", lines)
	}

	// a GM's change reaches the gnome where it stands, and only the gnome
	if n := w.SetRaceSizes(map[int]float64{3: 0.8}); n != 1 {
		t.Fatalf("%d players changed, want the gnome", n)
	}
	if _, s := channelBody(gnome); s != float32(0.8) {
		t.Fatalf("after the change the gnome is %v", s)
	}
	if _, s := channelBody(human); s != 1 {
		t.Fatalf("the change touched the human: %v", s)
	}
	// the row deleted, or a size the server does not send: the race's own 1.0
	for _, sizes := range []map[int]float64{{}, {3: 0}, {3: 40}} {
		w.SetRaceSizes(map[int]float64{3: 0.9})
		w.SetRaceSizes(sizes)
		if _, s := channelBody(gnome); s != 1 {
			t.Fatalf("racesize %v: the gnome is %v, want 1.0", sizes, s)
		}
	}
	// NPCs keep their own rows' sizes
	npc := &NPC{ServerID: 9000, Name: "Gnome Tinkerer", ModelID: int64(gnomeMale), Size: 1.2}
	if b := SerializeNPCUpdate(9000, npc); len(b) < 40 {
		t.Fatal("no NPC channel")
	} else if s := float32frombytes(b[36:40]); s != float32(1.2) {
		t.Fatalf("a gnome NPC's scale %v, want its row's 1.2", s)
	}
}

// A race's illusion shows that race's players' size: a human under Illusion:
// Gnome is a gnome player's 0.9, a gnome under Illusion: Troll the troll's
// 1.0, not its own 0.9. The control: a creature's illusion keeps the
// creature's size.
func TestARaceIllusionTakesTheRacesSize(t *testing.T) {
	c := illusionRig(t)
	c.w.SetRaceSizes(map[int]float64{3: 0.9})
	c.p.Size = c.w.RaceSize(c.p.Race) // the gnome enchanter at its race's size, as a login sets it
	if _, s := channelBody(c.p); s != float32(0.9) {
		t.Fatalf("setup: the gnome is %v", s)
	}
	c.land(t, troll(), 0xFFFFFFFF, 0)
	if m, s := channelBody(c.p); m != trollMale || s != 1 {
		t.Fatalf("a gnome as a troll: model %d at %v, want the troll's at 1.0", m, s)
	}
	golem := illusionSpell(575, "Illusion: Clay Golem", "Self, Golem Form, 10min", "Illusion only")
	c.land(t, golem, 0xFFFFFFFF, 20*time.Second)
	if m, s := channelBody(c.p); m != clayGolem || s != 1.25 {
		t.Fatalf("the control: the golem at %v, want its 1.25", s)
	}

	d := illusionRig(t)
	d.w.SetRaceSizes(map[int]float64{3: 0.9})
	d.p.Race, d.p.ModelID, d.p.Size = 0, 0x70D898C6, 1 // a human
	gnomeForm := illusionSpell(566, "Illusion: Gnome", "Self, Gnome form, Faction Change, 10min", "Illusion only")
	d.land(t, gnomeForm, 0xFFFFFFFF, 0)
	if m, s := channelBody(d.p); m != gnomeMale || s != float32(0.9) {
		t.Fatalf("a human as a gnome: model %d at %v, want the gnome's at 0.9", m, s)
	}
}
