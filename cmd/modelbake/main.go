// Command modelbake writes the sizes of the game's bodies (entity/reach.go)
// from the operator's own copy of the game's disc: each CSprite of CHAR.ESF,
// its DictID (npcs.modelid as the server sends it) and its edge, half the
// wider of its box's two level sides (the box is already in the body's own
// scale). The server reads <worlds>/models.txt at its start and gives a
// creature whose body is much bigger than a man's CLIENT's large or huge
// reach (TargetInRange's +3 or +5); without the file every creature is
// reached at 4.5. Copy the file beside the server (worlds/, which git never
// takes).
//
//	modelbake -iso /path/to/frontiers.iso [-out worlds/models.txt]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/eqoa/pkg/pkg/esf"
)

func main() {
	iso := flag.String("iso", "", "the game's disc image (EverQuest Online Adventures: Frontiers, SLUS-20744)")
	out := flag.String("out", "worlds/models.txt", "the file the sizes go to")
	flag.Parse()
	if *iso == "" {
		flag.Usage()
		os.Exit(2)
	}
	bodies, err := read(*iso)
	if err != nil {
		log.Fatal(err)
	}
	if err := write(*out, bodies); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d bodies to %s", len(bodies), *out)
}

// body is a CSprite's DictID and its edge.
type body struct {
	id   uint32
	edge float32
}

// read is every CSprite of the disc's CHAR.ESF.
func read(iso string) ([]body, error) {
	f, err := esf.OpenISOFile(iso, "CHAR.ESF")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := f.BuildDictionary(); err != nil {
		return nil, err
	}
	var out []body
	for _, k := range f.DictKeys() {
		o, err := f.FindObject(k)
		if err != nil {
			continue
		}
		c, ok := o.(*esf.CSprite)
		if !ok {
			continue
		}
		out = append(out, body{uint32(k), Edge(c.BBox)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out, nil
}

// Edge is a body's edge: half the wider of its box's two level sides.
func Edge(b esf.Box) float32 {
	return max(b.MaxX-b.MinX, b.MaxZ-b.MinZ) / 2
}

func write(path string, bodies []body) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	fmt.Fprintf(w, "# the bodies' edges (cmd/modelbake): CHAR.ESF CSprite DictID, half the wider of its box's level sides\n")
	for _, b := range bodies {
		fmt.Fprintf(w, "%08X %.3f\n", b.id, b.edge)
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
