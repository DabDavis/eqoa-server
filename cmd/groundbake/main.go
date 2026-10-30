// Command groundbake writes the ground the server's creatures stand on
// (internal/ground) from the operator's own copy of the game's disc: each of
// the disc's six worlds' terrain (the zone rooms' collision), a file a world
// (<out>/<world>.ground, world 0 Tunaria to 5 Secrets). The server reads
// them a zone at a time; it never opens the disc's world files itself, as
// the esf library reads one whole to parse it (Tunaria's 997 MB, about 2 GB
// at its peak). So run this where there is the memory, and copy the folder
// beside the server (worlds/ground, which git never takes).
//
// It writes each world's environment too (<out>/<world>.env, ground/env.go):
// the world's and its zones' BSPs and their leaves' flags, which say where
// the water is (F3 20261026-020316); -only-env writes those alone, leaving
// the ground files as they are.
//
// And what stands on the ground (<out>/<world>.walls, ground/walls.go): the
// collision of everything the zones place on their terrain (a house, a fence,
// a tree's trunk), each buffer once and each placement's transform, which a
// creature's wander does not walk through (F3 pass 24); -only-walls writes
// those alone.
//
//	groundbake -iso /path/to/frontiers.iso [-out worlds/ground] [-only-env | -only-walls]
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/eqoa/pkg/pkg/esf"
	"github.com/eqoa/server/internal/ground"
)

// worldFiles are the disc's worlds by the server's world numbers
// (handlers' worldPaths 0-5).
var worldFiles = []string{"TUNARIA.ESF", "RATHE.ESF", "ODUS.ESF", "LAVASTM.ESF", "PLANESKY.ESF", "SECRETS.ESF"}

func main() {
	iso := flag.String("iso", "", "the game's disc image (EverQuest Online Adventures: Frontiers, SLUS-20744)")
	out := flag.String("out", "worlds/ground", "the folder the ground files go to")
	onlyEnv := flag.Bool("only-env", false, "write the environment files (<world>.env) alone")
	onlyWalls := flag.Bool("only-walls", false, "write the walls files (<world>.walls) alone")
	flag.Parse()
	if *iso == "" || *onlyEnv && *onlyWalls {
		flag.Usage()
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	want := what{terrain: !*onlyEnv && !*onlyWalls, walls: !*onlyEnv, env: !*onlyWalls}
	for n, name := range worldFiles {
		t := time.Now()
		b, err := bake(*iso, name, want)
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		if want.terrain {
			path := filepath.Join(*out, fmt.Sprintf("%d.ground", n))
			if err := write(path, func(f *os.File) error { return ground.Write(f, b.zones) }); err != nil {
				log.Fatalf("%s: %v", path, err)
			}
			st, _ := os.Stat(path)
			log.Printf("world %d (%s): %d zones, %d terrain vertices, %d KB, in %v", n, name, len(b.zones), b.verts, st.Size()>>10, time.Since(t).Round(time.Millisecond))
		}
		if want.walls {
			path := filepath.Join(*out, fmt.Sprintf("%d.walls", n))
			if err := write(path, func(f *os.File) error { return ground.WriteWalls(f, b.bufs, b.wallZones) }); err != nil {
				log.Fatalf("%s: %v", path, err)
			}
			st, _ := os.Stat(path)
			placed := 0
			for _, z := range b.wallZones {
				placed += len(z.Placements)
			}
			log.Printf("world %d (%s): what stands on it: %d placements of %d collision buffers, %d KB", n, name, placed, len(b.bufs), st.Size()>>10)
		}
		if want.env {
			path := filepath.Join(*out, fmt.Sprintf("%d.env", n))
			if err := write(path, func(f *os.File) error { return ground.WriteEnv(f, b.env) }); err != nil {
				log.Fatalf("%s: %v", path, err)
			}
			st, _ := os.Stat(path)
			water := 0
			for _, z := range b.env.Zones {
				for _, fl := range z.Leaves {
					if fl&ground.EnvWater != 0 {
						water++
					}
				}
			}
			log.Printf("world %d (%s): environment of %d zones (%d world nodes, %d water leaves), %d KB", n, name, len(b.env.Zones), len(b.env.Tree.Nodes), water, st.Size()>>10)
		}
		runtime.GC() // the world file's bytes go before the next is read
	}
}

// what is what a bake writes.
type what struct{ terrain, walls, env bool }

// baked is a world baked: its zones' terrain, what stands on it (the
// collision buffers once each, and each zone's placements of them) and its
// environment.
type baked struct {
	zones     []ground.Zone
	verts     int
	bufs      []ground.Buffer
	wallZones []ground.WallZone
	env       *ground.WorldEnv
}

// bake is a world baked: each zone's room placements' collision strips,
// placed in the world (the terrain); each zone's actor placements'
// collision buffers, kept once each in their own space, and where each is
// placed (what stands on the terrain: a plant, which collides as nothing
// (vanilla's SpriteKind::Flora), is left out); and its environment; each as
// asked.
func bake(iso, name string, want what) (*baked, error) {
	f, err := esf.OpenISOFile(iso, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	root, err := f.Root()
	if err != nil {
		return nil, err
	}
	w := root.Child(esf.TypeWorld)
	if w == nil {
		return nil, fmt.Errorf("no world in it")
	}
	b := &baked{}
	if want.env {
		if b.env, err = bakeEnv(f, root, w.ChildrenOfType(esf.TypeZone)); err != nil {
			return nil, err
		}
	}
	if !want.terrain && !want.walls {
		return b, nil
	}
	index := make(map[int]int) // a collision buffer's place in the file: its index in b.bufs, -1 for none
	for _, zi := range w.ChildrenOfType(esf.TypeZone) {
		obj, err := f.GetObject(zi)
		if err != nil {
			return nil, err
		}
		z, ok := obj.(*esf.Zone)
		if !ok {
			continue
		}
		pls, err := z.GetSpritePlacements(f)
		if err != nil {
			return nil, err
		}
		var zone ground.Zone
		var wz ground.WallZone
		for _, sp := range pls {
			if !sp.IsRoom && !want.walls || sp.IsRoom && !want.terrain {
				continue
			}
			s, err := sp.GetSprite(f)
			if err != nil || s == nil || sp.IsFlora {
				continue
			}
			cb, err := s.GetCollBuffer(f)
			if err != nil || cb == nil {
				continue
			}
			if !sp.IsRoom {
				bi, seen := index[cb.ObjInfo().Offset]
				if !seen {
					bi = -1
					if buf := ownSpace(cb); len(buf.Strips) > 0 {
						bi = len(b.bufs)
						b.bufs = append(b.bufs, buf)
					}
					index[cb.ObjInfo().Offset] = bi
				}
				if bi >= 0 {
					wz.Placements = append(wz.Placements, ground.Placement{Buffer: uint32(bi), M: matrix(sp)})
				}
				continue
			}
			for _, l := range cb.Lists {
				if len(l.Vertices) < 3 {
					continue
				}
				strip := make([]ground.Vertex, len(l.Vertices))
				for i, v := range l.Vertices {
					p := esf.Point{X: v.X, Y: v.Y, Z: v.Z}
					sp.Transform(&p)
					strip[i] = ground.Vertex{X: p.X, Y: p.Y, Z: p.Z}
				}
				zone.Strips = append(zone.Strips, strip)
				b.verts += len(strip)
			}
		}
		b.zones = append(b.zones, zone)
		b.wallZones = append(b.wallZones, wz)
	}
	return b, nil
}

// ownSpace is a collision buffer's strips in its own space.
func ownSpace(cb *esf.CollBuffer) ground.Buffer {
	var buf ground.Buffer
	for _, l := range cb.Lists {
		if len(l.Vertices) < 3 {
			continue
		}
		strip := make([]ground.Vertex, len(l.Vertices))
		for i, v := range l.Vertices {
			strip[i] = ground.Vertex{X: v.X, Y: v.Y, Z: v.Z}
		}
		buf.Strips = append(buf.Strips, strip)
	}
	return buf
}

// matrix is a placement's transform (sp.Transform: scale, turn, move) as a
// 3x4 by rows: its turn and scale are where it takes each axis with the
// move left out (so the columns keep their precision far from the origin),
// its move the placement's position.
func matrix(sp *esf.SpritePlacement) [12]float32 {
	turn := *sp
	turn.Pos = esf.Point{}
	axis := func(x, y, z float32) esf.Point {
		p := esf.Point{X: x, Y: y, Z: z}
		turn.Transform(&p)
		return p
	}
	ex, ey, ez := axis(1, 0, 0), axis(0, 1, 0), axis(0, 0, 1)
	return [12]float32{
		ex.X, ey.X, ez.X, sp.Pos.X,
		ex.Y, ey.Y, ez.Y, sp.Pos.Y,
		ex.Z, ey.Z, ez.Z, sp.Pos.Z,
	}
}

// bakeEnv is a world's environment: the world base's tree (0x8200 -> 0x8230,
// a word a leaf: its zone) and each zone's (0x3200 -> 0x3220, two words a
// leaf: the second its flags; read with RawBytes, which a streamed file
// needs), in the zones' file order, which the world
// tree's leaves count in (vanilla's World::load and zone_base).
func bakeEnv(f *esf.ObjFile, root *esf.ObjInfo, zones []*esf.ObjInfo) (*ground.WorldEnv, error) {
	base := root.Child(esf.TypeWorldBase)
	if base == nil {
		return nil, fmt.Errorf("no world base")
	}
	wt := base.Child(esf.TypeWorldTree)
	if wt == nil {
		return nil, fmt.Errorf("no world tree")
	}
	env := &ground.WorldEnv{}
	var err error
	if env.Tree, err = readTree(f.RawBytes(wt.Offset, int(wt.Size)), 1); err != nil {
		return nil, fmt.Errorf("world tree: %w", err)
	}
	env.Zones = make([]ground.EnvTree, len(zones))
	for i, zi := range zones {
		zb := zi.Child(esf.TypeZoneBase)
		if zb == nil {
			continue
		}
		zt := zb.Child(esf.TypeZoneTree)
		if zt == nil {
			continue
		}
		if env.Zones[i], err = readTree(f.RawBytes(zt.Offset, int(zt.Size)), 2); err != nil {
			return nil, fmt.Errorf("zone %d tree: %w", i, err)
		}
	}
	return env, nil
}

// readTree reads a tree record's body as CLIENT does (vanilla's read_tree):
// the node and leaf counts, each node a plane (normal, d) and its front and
// back, each leaf leafWords words, of which the last is kept (the world's
// zone, a zone's flags).
func readTree(d []byte, leafWords int) (ground.EnvTree, error) {
	var t ground.EnvTree
	if len(d) < 8 {
		return t, fmt.Errorf("%d bytes", len(d))
	}
	le := binary.LittleEndian
	nn, nl := int(int32(le.Uint32(d))), int(int32(le.Uint32(d[4:])))
	if nn < 0 || nl < 0 || 8+nn*24+nl*4*leafWords > len(d) {
		return t, fmt.Errorf("%d nodes and %d leaves in %d bytes", nn, nl, len(d))
	}
	p := 8
	fl := func(o int) float32 { return math.Float32frombits(le.Uint32(d[o:])) }
	t.Nodes = make([]ground.EnvNode, nn)
	for i := range t.Nodes {
		t.Nodes[i] = ground.EnvNode{NX: fl(p), NY: fl(p + 4), NZ: fl(p + 8), D: fl(p + 12),
			Front: le.Uint32(d[p+16:]), Back: le.Uint32(d[p+20:])}
		p += 24
	}
	t.Leaves = make([]uint32, nl)
	for i := range t.Leaves {
		t.Leaves[i] = le.Uint32(d[p+4*(leafWords-1):])
		p += 4 * leafWords
	}
	return t, nil
}

// write writes the file whole beside its place, then puts it there.
func write(path string, fill func(*os.File) error) error {
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := fill(f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
