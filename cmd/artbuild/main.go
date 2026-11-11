// Command artbuild builds the patch's art file (internal/worlds' AddArt):
// a base art file (Sandstorm's ss-art.csf, patches/ss-art.csf) with the
// disc's sprites a recipe names (tools/art/*.json) added, read off the
// operator's copy of the game's disc, so nothing of the disc is in this
// repository. The file it writes is what patches/serve/ serves to every
// client (a PS2's S2 and vanilla read it at world entry):
//
//	artbuild -iso /path/to/frontiers.iso -base patches/ss-art.csf \
//	    -recipe tools/art/ss-art.json -out patches/serve/ss-art.csf
//
// It refuses a file a PS2 would read cut short (MaxPatchArt, our S2's
// buffer, or -max), and a sprite whose records a file the game has already
// registered (the disc's CHAR.ESF, ITEM.ESF, ... read before the patch
// art) holds as other bytes.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/eqoa/server/internal/worlds"
)

// resident are the disc files EnterGameLoad parses before the patch art
// (vanilla's crates/eqoa/src/main.rs, in its order).
var resident = []string{"CHAR.ESF", "ITEM.ESF", "ARIAL.CSF", "UI.CSF", "ARIALUI.CSF", "ITEMICON.ESF", "SPELLFX.CSF", "FX.CSF", "ADDART.CSF"}

func main() {
	iso := flag.String("iso", "", "the game's disc image (EverQuest Online Adventures: Frontiers, SLUS-20744)")
	base := flag.String("base", "patches/ss-art.csf", "the art file the pieces are added to")
	recipe := flag.String("recipe", "tools/art/ss-art.json", "the pieces (tools/art/*.json)")
	out := flag.String("out", "", "the file written (patches/serve/ss-art.csf)")
	most := flag.Int("max", worlds.MaxPatchArt, "refuse a file this big or bigger (a PS2's read buffer)")
	flag.Parse()
	if *iso == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	var r worlds.ArtRecipe
	b, err := os.ReadFile(*recipe)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(b, &r); err != nil {
		log.Fatalf("%s: %v", *recipe, err)
	}
	bb, err := os.ReadFile(*base)
	if err != nil {
		log.Fatal(err)
	}
	d, err := worlds.OpenDisc(*iso)
	if err != nil {
		log.Fatalf("%s: %v", *iso, err)
	}
	defer d.Close()
	var res []worlds.Resident
	for _, name := range resident {
		rr, err := worlds.OpenResident(d, name)
		if err != nil {
			log.Fatal(err)
		}
		res = append(res, rr)
	}
	csf, built, err := worlds.AddArt(d, bb, r, res)
	if err != nil {
		log.Fatal(err)
	}
	if len(csf) >= *most {
		log.Fatalf("%d bytes: a PS2 reads at most %d of it", len(csf), *most-1)
	}
	if err := os.WriteFile(*out, csf, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d bytes (%d inflated), CRC %08x, %d records (the base's %d)\n", *out, built.Size, built.Inflated, built.CRC, built.Objects, built.BaseObjects)
	for i, p := range r.Pieces {
		ids := make([]string, len(built.Brought[i]))
		for j, id := range built.Brought[i] {
			ids[j] = fmt.Sprintf("%08X", uint32(id))
		}
		fmt.Printf("  %08X from %s zone %d: %d records (%s)  %s\n", uint32(p.Sprite), p.From, p.Zone, len(ids), strings.Join(ids, " "), p.Note)
	}
}
