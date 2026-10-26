package ground

// The worlds' environment: where the water is (and the lava, the slime and
// the mud), as the game keeps it. CLIENT finds what a point is in by its
// world's BSP (record 0x8230 in the world base 0x8200: planes down to a
// world leaf, which names a zone) and then the zone's own BSP (0x3220 in the
// zone base 0x3200: planes down to a zone leaf), whose second word is the
// leaf's flags: 1 water, 2 slime, 4 lava, 8 mud (VIWorld::FindLeaf,
// GetEnvFlags; the walk sets its swimming from them: vanilla's
// crates/vi/src/world.rs find_leaf and env_flags, walk.rs calc_floor). The
// bake (cmd/groundbake) copies the two trees off the operator's disc into
// <world>.env beside <world>.ground: planes and flags only, no geometry, a
// few hundred KB a world, read whole at the start.
//
// A file: "EQEV", a version (1); the world tree: its node and leaf counts,
// each node a plane (nx, ny, nz, d as float32) and its front and back (as
// the record has them: a child node, or a leaf with LeafBit set), each leaf
// a zone index (int32); then the zone count and each zone's tree the same,
// its leaves the flags (uint32). Little-endian.

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	envMagic   = "EQEV"
	envVersion = 1

	// LeafBit marks a front or back that is a leaf, not a node.
	LeafBit = 0x4000_0000
	// indexMask is a node's or leaf's index in a front or back.
	indexMask = 0x0FFF_FFFF
)

// The flags of a zone leaf (GetEnvFlags).
const (
	EnvWater uint32 = 1
	EnvSlime uint32 = 2
	EnvLava  uint32 = 4
	EnvMud   uint32 = 8
)

// EnvNode is a node of a tree: a point is in front of its plane when
// x*NX + y*NY + z*NZ + D >= 0 (VIPlane's distance, the point first).
type EnvNode struct {
	NX, NY, NZ, D float32
	Front, Back   uint32
}

// EnvTree is a BSP and its leaves' words: a world's (zone indices) or a
// zone's (flags).
type EnvTree struct {
	Nodes  []EnvNode
	Leaves []uint32
}

// leaf is the leaf p falls in, false past the leaves (or a tree that loops).
func (t *EnvTree) leaf(x, y, z float32) (uint32, bool) {
	at := uint32(0)
	if len(t.Nodes) > 0 {
		for steps := 0; ; steps++ {
			i := at & indexMask
			if int(i) >= len(t.Nodes) || steps > len(t.Nodes) {
				return 0, false
			}
			n := &t.Nodes[i]
			if x*n.NX+y*n.NY+z*n.NZ+n.D >= 0 {
				at = n.Front
			} else {
				at = n.Back
			}
			if at&LeafBit != 0 {
				break
			}
		}
		at &= indexMask
	}
	if int(at) >= len(t.Leaves) {
		return 0, false
	}
	return t.Leaves[at], true
}

// WorldEnv is a world's environment: its tree, and each zone's.
type WorldEnv struct {
	Tree  EnvTree
	Zones []EnvTree
}

// Flags is what (x, y, z) is in: the flags of its zone leaf, false outside
// every zone.
func (w *WorldEnv) Flags(x, y, z float32) (uint32, bool) {
	zi, ok := w.Tree.leaf(x, y, z)
	if !ok || int(int32(zi)) < 0 || int(zi) >= len(w.Zones) {
		return 0, false
	}
	return w.Zones[zi].leaf(x, y, z)
}

// WriteEnv writes a world's environment file.
func WriteEnv(out io.Writer, w *WorldEnv) error {
	bw := bufio.NewWriter(out)
	put := func(v any) error { return binary.Write(bw, binary.LittleEndian, v) }
	tree := func(t *EnvTree) error {
		if err := put([]uint32{uint32(len(t.Nodes)), uint32(len(t.Leaves))}); err != nil {
			return err
		}
		if err := put(t.Nodes); err != nil {
			return err
		}
		return put(t.Leaves)
	}
	if _, err := bw.WriteString(envMagic); err != nil {
		return err
	}
	if err := put(uint32(envVersion)); err != nil {
		return err
	}
	if err := tree(&w.Tree); err != nil {
		return err
	}
	if err := put(uint32(len(w.Zones))); err != nil {
		return err
	}
	for i := range w.Zones {
		if err := tree(&w.Zones[i]); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadEnv reads a world's environment file.
func ReadEnv(in io.Reader) (*WorldEnv, error) {
	br := bufio.NewReader(in)
	var m [4]byte
	if _, err := io.ReadFull(br, m[:]); err != nil {
		return nil, err
	}
	if string(m[:]) != envMagic {
		return nil, fmt.Errorf("not an environment file")
	}
	get := func(v any) error { return binary.Read(br, binary.LittleEndian, v) }
	var ver uint32
	if err := get(&ver); err != nil {
		return nil, err
	}
	if ver != envVersion {
		return nil, fmt.Errorf("environment file version %d, this server reads %d", ver, envVersion)
	}
	tree := func(t *EnvTree) error {
		var counts [2]uint32
		if err := get(&counts); err != nil {
			return err
		}
		if counts[0] > 1<<24 || counts[1] > 1<<24 {
			return fmt.Errorf("a tree of %d nodes and %d leaves", counts[0], counts[1])
		}
		if counts[0] > 0 {
			t.Nodes = make([]EnvNode, counts[0])
			if err := get(t.Nodes); err != nil {
				return err
			}
		}
		if counts[1] > 0 {
			t.Leaves = make([]uint32, counts[1])
			return get(t.Leaves)
		}
		return nil
	}
	w := &WorldEnv{}
	if err := tree(&w.Tree); err != nil {
		return nil, err
	}
	var zones uint32
	if err := get(&zones); err != nil {
		return nil, err
	}
	if zones > 1<<16 {
		return nil, fmt.Errorf("%d zones", zones)
	}
	w.Zones = make([]EnvTree, zones)
	for i := range w.Zones {
		if err := tree(&w.Zones[i]); err != nil {
			return nil, fmt.Errorf("zone %d: %w", i, err)
		}
	}
	return w, nil
}

// Env is every world's environment the server has a file for.
type Env struct {
	worlds [256]*WorldEnv
}

// OpenEnv reads the environment files in dir (<world>.env, world 0 to 255):
// none is no error (the server then knows no water anywhere).
func OpenEnv(dir string) (*Env, error) {
	e := &Env{}
	for n := 0; n < 256; n++ {
		f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%d.env", n)))
		if err != nil {
			continue
		}
		w, err := ReadEnv(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%d.env: %w", n, err)
		}
		e.worlds[n] = w
	}
	return e, nil
}

// Worlds is how many worlds have their environment.
func (e *Env) Worlds() int {
	n := 0
	for _, w := range e.worlds {
		if w != nil {
			n++
		}
	}
	return n
}

// Flags is what (x, y, z) in world is in (EnvWater and the rest), false
// where the server has no file for the world or the point is in no zone.
func (e *Env) Flags(world uint8, x, y, z float32) (uint32, bool) {
	w := e.worlds[world]
	if w == nil {
		return 0, false
	}
	return w.Flags(x, y, z)
}
