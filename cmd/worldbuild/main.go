// Command worldbuild builds a world of the disc's own zones (internal/worlds'
// Compose) from the operator's copy of the game's disc: the recipe names each
// zone (a world file of the disc and a zone index), and each is copied whole,
// where it stands, into a world file of its own. Nothing of the disc is in a
// recipe, so recipes are kept in this repository (tools/worlds/); the world it
// builds is the operator's, beside the server (worlds/, which git never takes),
// as cmd/groundbake's ground is.
//
// A zone may be given extras, the disc's art placed in it as actors of its
// own (internal/worlds/extras.go): a sprite by DictID, where it stands, its
// turn and size, and where its art is on the disc when the zone does not hold
// it (copied in). A recipe's extraFiles keep a long list apart from it
// (tools/worlds/homelands-edges.json, the homelands' barriers).
//
// It writes <out>/worldN.ESF (N the recipe's slot) and <out>/worldN.json, the
// recipe as built (each zone's source record's size and CRC, the file's size
// and CRC, the grid), which the server reads at its start and tells a client
// that can build the same world from its own disc (handlers/serverworlds.go).
// Then bake the server's ground of it:
//
//	worldbuild -iso /path/to/frontiers.iso -recipe tools/worlds/homelands.json [-out worlds]
//	groundbake -iso /path/to/frontiers.iso -world 10=worlds/world10.ESF
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/eqoa/server/internal/worlds"
)

func main() {
	iso := flag.String("iso", "", "the game's disc image (EverQuest Online Adventures: Frontiers, SLUS-20744)")
	recipe := flag.String("recipe", "", "the world's recipe (tools/worlds/*.json)")
	out := flag.String("out", "worlds", "the folder the world and its built recipe go to")
	flag.Parse()
	if *iso == "" || *recipe == "" {
		flag.Usage()
		os.Exit(2)
	}
	r, err := worlds.ReadRecipe(*recipe)
	if err != nil {
		log.Fatal(err)
	}
	d, err := worlds.OpenDisc(*iso)
	if err != nil {
		log.Fatalf("%s: %v", *iso, err)
	}
	defer d.Close()
	b, built, err := worlds.Compose(d, r)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	esf := filepath.Join(*out, worlds.Name(r.World))
	if err := os.WriteFile(esf, b, 0o644); err != nil {
		log.Fatal(err)
	}
	j, err := json.MarshalIndent(built, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	meta := filepath.Join(*out, worlds.BuiltName(r.World))
	if err := os.WriteFile(meta, append(j, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: world %d %q, %d zones, %d bytes, CRC %08x, grid %d x %d\n", esf, r.World, r.Name, len(built.Parts), built.Size, built.CRC, built.Cols, built.Rows)
	for i, p := range built.Parts {
		extra := ""
		if p.Extras > 0 {
			extra = fmt.Sprintf("  + %d extras (%d records of the disc brought)", p.Extras, p.Brought)
		}
		fmt.Printf("  zone %2d  %-12s %3d  cell (%2d, %2d)  %8d bytes  %08x  %s%s\n", i, p.From, p.Zone, p.Col, p.Row, p.Size, p.CRC, r.Zones[i].Name, extra)
	}
	fmt.Printf("%s: the recipe as built\n", meta)
}
