package entity

import (
	"os"
	"path/filepath"
	"testing"
)

// The bodies of reach_test: a man's, a bear's a little bigger, a giant's,
// a treant's and a dragon's (the man's, the treant's and the dragon's edges
// as cmd/modelbake reads them off the disc: 70D898C6 1.395, 46392747 11.1,
// 28E4DDC6 16.186).
const (
	testMan    = 0x70D898C6
	testBear   = 0x11111111
	testGiant  = 0x33333333
	testTreant = 0x46392747
	testDragon = 0x28E4DDC6
)

func withBodies(t *testing.T) {
	t.Helper()
	SetModelRadii(map[uint32]float32{testMan: 1.395, testBear: 2.4, testGiant: 4.0, testTreant: 11.1, testDragon: 16.186})
	t.Cleanup(func() { SetModelRadii(nil) })
}

// The allowance whose gap to the body's edge is nearest a man's: none for a
// man or a body under 1.5 past his edge, 3 from there, 5 from 4 past it; a
// creature's size scales its body.
func TestReachFlagIsTheNearestGap(t *testing.T) {
	for _, c := range []struct {
		r, size float32
		want    uint16
	}{
		{1.395, 1, 0},
		{2.4, 1, 0},            // 1.0 past a man's edge: 4.5 leaves 2.1, nearer his 3.1 than 7.5's 5.1
		{2.4, 1.5, ReachLarge}, // 3.6, 2.2 past
		{2.894, 1, 0},
		{2.896, 1, ReachLarge},
		{5.39, 1, ReachLarge},
		{5.4, 1, ReachHuge},
		{16.186, 1, ReachHuge},
		{1.0, 6, ReachHuge}, // a man-sized body at size 6
		{2.4, 0, 0},         // a size of 0 counts as 1
	} {
		if got := ReachFlagFor(c.r, c.size); got != c.want {
			t.Errorf("edge %.3f size %.2f: %#x, want %#x", c.r, c.size, got, c.want)
		}
	}
}

// A dragon or a treant is reached from 9.5 and a giant from 7.5, the same as CLIENT's
// TargetInRange with the bits its channel now carries; a man-sized creature
// from 4.5. The control: the database's npc_type 0x400 or 0x800 (byte 145
// and the class byte on the wire, never CLIENT's reach bits) adds nothing, as
// it once did on the server alone.
func TestALargeCreatureIsReachedFromFurther(t *testing.T) {
	withBodies(t)
	p := &Player{Swing: Swing{Reach: 4.5}}
	for _, c := range []struct {
		model uint32
		size  float32
		in    float32 // in reach at this distance, out of it 0.05 further
	}{
		{testMan, 1, 4.5},
		{testBear, 1, 4.5},
		{testGiant, 1, 7.5},
		{testTreant, 1, 9.5},
		{testDragon, 1, 9.5},
		{testBear, 1.5, 7.5},
	} {
		npc := &NPC{ModelID: int64(c.model), Size: c.size, X: c.in}
		if !inReach(p, npc) {
			t.Errorf("%08X at %.2f: out of reach at %.2f", c.model, c.size, c.in)
		}
		npc.X = c.in + 0.05
		if inReach(p, npc) {
			t.Errorf("%08X at %.2f: in reach at %.2f", c.model, c.size, npc.X)
		}
		if got, want := npcReach(npc), c.in; got != want {
			t.Errorf("%08X at %.2f: hits from %.2f, want %.2f", c.model, c.size, got, want)
		}
	}
	for _, f := range []int{0x400, 0x800, 0x0004, 0x0008} {
		npc := &NPC{ModelID: testMan, Size: 1, X: 5, NPCType: f}
		if inReach(p, npc) {
			t.Errorf("npc_type %#x gave reach", f)
		}
	}
	// a body the server does not know is a man's
	if npc := (&NPC{ModelID: 0x22222222, Size: 6, X: 5}); inReach(p, npc) {
		t.Error("an unknown body gave reach")
	}
}

// CLIENT's npc flags are the channel's bytes 146-147 (after race 144 and
// class 145): a large creature's 0x400 is byte 147's 0x04, a huge one's
// 0x800 its 0x08, and nothing else there moves; byte 146 is the npc_type's
// low byte as before. The control: without the bodies' file byte 147 is 0,
// as before F3 pass 16.
func TestTheChannelCarriesTheReachBits(t *testing.T) {
	npc := &NPC{ModelID: testGiant, Size: 1, NPCType: 0x0081, Level: 30}
	before := SerializeNPCUpdate(1, npc)
	if before[147] != 0 {
		t.Fatalf("byte 147 %#x with no bodies known", before[147])
	}
	withBodies(t)
	buf := SerializeNPCUpdate(1, npc)
	if buf[146] != 0x81 || buf[147] != 0x04 {
		t.Errorf("a giant's flags %02x %02x, want 81 04 (0x0481)", buf[146], buf[147])
	}
	for i := range buf {
		if i != 147 && buf[i] != before[i] {
			t.Errorf("byte %d moved: %#x, was %#x", i, buf[i], before[i])
		}
	}
	npc.ModelID = testDragon
	if buf = SerializeNPCUpdate(1, npc); buf[147] != 0x08 {
		t.Errorf("a dragon's byte 147 %#x, want 0x08", buf[147])
	}
	npc.ModelID = testMan
	if buf = SerializeNPCUpdate(1, npc); buf[147] != 0 {
		t.Errorf("a man's byte 147 %#x", buf[147])
	}
}

// The file cmd/modelbake writes: a line a body, # a comment.
func TestLoadModelRadii(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.txt")
	if err := os.WriteFile(path, []byte("# edges\n70D898C6 1.395\n\n28E4DDC6 16.186  # Nagafen's\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModelRadii(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m[0x70D898C6] != 1.395 || m[0x28E4DDC6] != 16.186 {
		t.Errorf("%v", m)
	}
	if err := os.WriteFile(path, []byte("70D898C6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadModelRadii(path); err == nil {
		t.Error("a line with no edge read")
	}
}
