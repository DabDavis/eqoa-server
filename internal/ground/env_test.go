package ground

import (
	"bytes"
	"reflect"
	"testing"
)

// twoZones is a world split at x = 0: in front (x >= 0) zone 0, whose own
// tree splits at y = 10 (above: air, below: water); behind, zone 1, one
// leaf of lava.
func twoZones() *WorldEnv {
	return &WorldEnv{
		Tree: EnvTree{
			Nodes:  []EnvNode{{NX: 1, Front: LeafBit | 0, Back: LeafBit | 1}},
			Leaves: []uint32{0, 1},
		},
		Zones: []EnvTree{
			{Nodes: []EnvNode{{NY: 1, D: -10, Front: LeafBit | 0, Back: LeafBit | 1}}, Leaves: []uint32{0, EnvWater}},
			{Leaves: []uint32{EnvLava}},
		},
	}
}

// A point's flags are its zone leaf's, found as CLIENT's FindLeaf finds
// them (front when the plane's distance is zero or more); the file reads
// back as it was written.
func TestEnvFindsTheLeaf(t *testing.T) {
	w := twoZones()
	for _, c := range []struct {
		x, y, z float32
		want    uint32
	}{
		{5, 20, 0, 0},         // zone 0, above 10: air
		{5, 5, 0, EnvWater},   // zone 0, below 10: water
		{5, 10, 0, 0},         // on the plane: in front
		{-5, 5, 0, EnvLava},   // zone 1
		{-5, 500, 3, EnvLava}, // zone 1 has no tree: its one leaf everywhere
	} {
		if got, ok := w.Flags(c.x, c.y, c.z); !ok || got != c.want {
			t.Errorf("(%g, %g, %g): %d %v, want %d", c.x, c.y, c.z, got, ok, c.want)
		}
	}
	var buf bytes.Buffer
	if err := WriteEnv(&buf, w); err != nil {
		t.Fatal(err)
	}
	back, err := ReadEnv(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, w) {
		t.Errorf("read back %+v, want %+v", back, w)
	}
}

// A tree that loops, a leaf past the leaves or a zone past the zones finds
// nothing rather than hanging or reading past the end.
func TestEnvBadTreesFindNothing(t *testing.T) {
	loop := &WorldEnv{Tree: EnvTree{Nodes: []EnvNode{{NX: 1, Front: 0, Back: 0}}, Leaves: []uint32{0}}, Zones: []EnvTree{{Leaves: []uint32{EnvWater}}}}
	if _, ok := loop.Flags(1, 0, 0); ok {
		t.Error("a looping tree found a leaf")
	}
	pastLeaves := twoZones()
	pastLeaves.Tree.Nodes[0].Front = LeafBit | 7
	if _, ok := pastLeaves.Flags(1, 0, 0); ok {
		t.Error("a leaf past the leaves was found")
	}
	pastZones := twoZones()
	pastZones.Tree.Leaves[0] = 9
	if _, ok := pastZones.Flags(1, 0, 0); ok {
		t.Error("a zone past the zones was found")
	}
	if _, ok := twoZones().Flags(1, 0, 0); !ok {
		t.Error("the good world found nothing: the test proves nothing")
	}
}
