// Command groundbake writes the ground the server's creatures stand on
// (internal/ground) from the operator's own copy of the game's disc: each of
// the disc's six worlds' terrain (the zone rooms' collision), a file a world
// (<out>/<world>.ground, world 0 Tunaria to 5 Secrets). The server reads
// them a zone at a time; it never opens the disc's world files itself, as
// the esf library reads one whole to parse it (Tunaria's 997 MB, about 2 GB
// at its peak). So run this where there is the memory, and copy the folder
// beside the server (worlds/ground, which git never takes).
//
//	groundbake -iso /path/to/frontiers.iso [-out worlds/ground]
package main

import (
	"flag"
	"fmt"
	"log"
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
	flag.Parse()
	if *iso == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for n, name := range worldFiles {
		t := time.Now()
		zones, verts, err := bake(*iso, name)
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		path := filepath.Join(*out, fmt.Sprintf("%d.ground", n))
		if err := write(path, zones); err != nil {
			log.Fatalf("%s: %v", path, err)
		}
		st, _ := os.Stat(path)
		log.Printf("world %d (%s): %d zones, %d terrain vertices, %d KB, in %v", n, name, len(zones), verts, st.Size()>>10, time.Since(t).Round(time.Millisecond))
		runtime.GC() // the world file's bytes go before the next is read
	}
}

// bake is a world's zones' terrain: each zone's room placements' collision
// strips, placed in the world.
func bake(iso, name string) ([]ground.Zone, int, error) {
	f, err := esf.OpenISOFile(iso, name)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	root, err := f.Root()
	if err != nil {
		return nil, 0, err
	}
	w := root.Child(esf.TypeWorld)
	if w == nil {
		return nil, 0, fmt.Errorf("no world in it")
	}
	var zones []ground.Zone
	verts := 0
	for _, zi := range w.ChildrenOfType(esf.TypeZone) {
		obj, err := f.GetObject(zi)
		if err != nil {
			return nil, 0, err
		}
		z, ok := obj.(*esf.Zone)
		if !ok {
			continue
		}
		pls, err := z.GetSpritePlacements(f)
		if err != nil {
			return nil, 0, err
		}
		var zone ground.Zone
		for _, sp := range pls {
			if !sp.IsRoom {
				continue // an actor's (a tree, a rock, a house): not the ground
			}
			s, err := sp.GetSprite(f)
			if err != nil || s == nil {
				continue
			}
			cb, err := s.GetCollBuffer(f)
			if err != nil || cb == nil {
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
				verts += len(strip)
			}
		}
		zones = append(zones, zone)
	}
	return zones, verts, nil
}

// write writes the file whole beside its place, then puts it there.
func write(path string, zones []ground.Zone) error {
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := ground.Write(f, zones); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
